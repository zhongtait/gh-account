package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zhongtait/gh-account/internal/config"
)

const (
	defaultHostname = "github.com"
	defaultScope    = "read:user,repo"
)

// Client abstracts GitHub authentication operations.
type Client interface {
	CurrentLogin(ctx context.Context) (string, error)
	SwitchUser(ctx context.Context, login string) error
	Status(ctx context.Context) (string, error)
	Login(ctx context.Context, hostname string, gitProtocol string) error
	Logout(ctx context.Context, login string, hostname string) error
}

// HostClient is implemented by clients that can switch an account on a
// specific GitHub host, including GitHub Enterprise Server.
type HostClient interface {
	SwitchUserAtHost(ctx context.Context, login string, hostname string) error
}

// IdentityClient exposes the active login together with its GitHub host.
type IdentityClient interface {
	CurrentIdentity(ctx context.Context) (login string, hostname string, err error)
}

// CredentialChecker reports whether a login has a locally stored OAuth
// credential for a specific GitHub host.
type CredentialChecker interface {
	HasCredential(ctx context.Context, login string, hostname string) (bool, error)
}

// ClientIDClient allows the command layer to configure the public OAuth App
// client ID interactively without depending on the concrete client type.
type ClientIDClient interface {
	ConfiguredClientID() string
	SetClientID(clientID string)
}

// BrowserOpener is the optional system integration used to open OAuth URLs.
type BrowserOpener func(url string) error

// OAuthClient implements GitHub OAuth Device Flow using only Go's standard
// library. It does not invoke gh, a browser command, or any other binary.
type OAuthClient struct {
	Store       *config.Store
	HTTP        *http.Client
	Output      io.Writer
	ClientID    string
	OpenBrowser BrowserOpener
}

// NewOAuthClient creates a self-contained GitHub OAuth client.
func NewOAuthClient(store *config.Store, output io.Writer, clientID string) *OAuthClient {
	if store == nil {
		store = config.NewStore("")
	}
	if output == nil {
		output = io.Discard
	}
	if strings.TrimSpace(clientID) == "" {
		clientID = ClientIDFromEnv()
	}
	return &OAuthClient{Store: store, HTTP: &http.Client{Timeout: 30 * time.Second}, Output: output, ClientID: strings.TrimSpace(clientID), OpenBrowser: OpenBrowser}
}

// ClientIDFromEnv returns the configured public OAuth App client ID.
func ClientIDFromEnv() string {
	if value := strings.TrimSpace(os.Getenv("GH_GHA_CLIENT_ID")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CLIENT_ID"))
}

// ConfiguredClientID returns the currently configured public OAuth client ID.
func (c *OAuthClient) ConfiguredClientID() string {
	return strings.TrimSpace(c.ClientID)
}

// SetClientID updates the public OAuth client ID used for future login calls.
func (c *OAuthClient) SetClientID(clientID string) {
	c.ClientID = strings.TrimSpace(clientID)
}

// CurrentLogin returns the login for the active locally stored credential.
func (c *OAuthClient) CurrentLogin(ctx context.Context) (string, error) {
	login, _, err := c.CurrentIdentity(ctx)
	return login, err
}

// CurrentIdentity returns the active login and host after validating the
// locally stored OAuth credential against GitHub.
func (c *OAuthClient) CurrentIdentity(ctx context.Context) (string, string, error) {
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return "", "", fmt.Errorf("load auth: %w", err)
	}
	if auth.Active == "" {
		return "", "", errors.New("no active GitHub OAuth account; run gha login")
	}
	credential, ok := auth.Credentials[auth.Active]
	if !ok || strings.TrimSpace(credential.AccessToken) == "" {
		return "", "", errors.New("active GitHub OAuth credential is missing; run gha login")
	}
	user, err := c.getUser(ctx, credential)
	if err != nil {
		return "", "", fmt.Errorf("get GitHub user: %w", err)
	}
	return user.Login, normalizeHostname(credential.Hostname), nil
}

// SwitchUser makes a previously authenticated account active.
func (c *OAuthClient) SwitchUser(ctx context.Context, login string) error {
	return c.SwitchUserAtHost(ctx, login, "")
}

// SwitchUserAtHost makes a credential active, disambiguating accounts that
// use the same login on different GitHub hosts.
func (c *OAuthClient) SwitchUserAtHost(ctx context.Context, login string, hostname string) error {
	login = strings.TrimSpace(login)
	if login == "" {
		return errors.New("login is required")
	}
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	hostname = strings.TrimSpace(hostname)
	if hostname != "" {
		hostname = normalizeHostname(hostname)
	}
	var match string
	for key, credential := range auth.Credentials {
		if !strings.EqualFold(credential.Login, login) {
			continue
		}
		if hostname != "" && !strings.EqualFold(credential.Hostname, hostname) {
			continue
		}
		if match != "" {
			return fmt.Errorf("multiple OAuth credentials found for %q; specify its GitHub host", login)
		}
		match = key
	}
	if match != "" {
		auth.Active = match
		if err := c.Store.SaveAuth(auth); err != nil {
			return fmt.Errorf("save auth: %w", err)
		}
		return nil
	}
	return fmt.Errorf("no OAuth credential found for GitHub account %q; run gha login", login)
}

// HasCredential reports whether the requested account has been authenticated
// locally, without making a network request.
func (c *OAuthClient) HasCredential(ctx context.Context, login string, hostname string) (bool, error) {
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return false, fmt.Errorf("load auth: %w", err)
	}
	credential, ok := auth.Credentials[credentialKey(hostname, login)]
	return ok && strings.TrimSpace(credential.AccessToken) != "", nil
}

// Status returns a safe, human-readable authentication status without
// including access tokens.
func (c *OAuthClient) Status(ctx context.Context) (string, error) {
	login, err := c.CurrentLogin(ctx)
	if err != nil {
		return "", fmt.Errorf("get current login: %w", err)
	}
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return "", fmt.Errorf("load auth: %w", err)
	}
	credential := auth.Credentials[auth.Active]
	return fmt.Sprintf("Logged in to %s account %s (gha OAuth)", credential.Hostname, login), nil
}

// Login authenticates with GitHub's OAuth Device Flow and stores the token.
// The verification URL and one-time code are printed for the user to open in
// any browser; no browser executable is required.
func (c *OAuthClient) Login(ctx context.Context, hostname string, gitProtocol string) error {
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("GitHub OAuth client ID is required; set GH_GHA_CLIENT_ID or use --client-id")
	}
	hostname = normalizeHostname(hostname)
	device, err := c.requestDeviceCode(ctx, hostname)
	if err != nil {
		return fmt.Errorf("request GitHub device code: %w", err)
	}

	verificationURI := device.VerificationURIComplete
	if verificationURI == "" {
		verificationURI = device.VerificationURI
	}
	fmt.Fprintf(c.Output, "Open %s\n", verificationURI)
	fmt.Fprintf(c.Output, "Enter code: %s\n", device.UserCode)

	// Try to copy the user code to clipboard
	if err := copyToClipboard(device.UserCode); err == nil {
		fmt.Fprintln(c.Output, "Code copied to clipboard!")
	}

	if c.OpenBrowser != nil {
		if err := c.OpenBrowser(verificationURI); err == nil {
			fmt.Fprintln(c.Output, "Browser opened; complete authorization there.")
		} else {
			fmt.Fprintf(c.Output, "Could not open browser automatically: %v\n", err)
		}
	}

	token, err := c.pollToken(ctx, hostname, device)
	if err != nil {
		return fmt.Errorf("complete GitHub OAuth login: %w", err)
	}
	credential := config.Credential{
		Hostname:    hostname,
		Login:       token.Login,
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Scope:       token.Scope,
	}
	key := credentialKey(hostname, token.Login)
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	auth.Credentials[key] = credential
	auth.Active = key
	if err := c.Store.SaveAuth(auth); err != nil {
		return fmt.Errorf("save GitHub OAuth credential: %w", err)
	}
	return nil
}

// Logout removes a stored credential. With no login, only the active
// credential for the requested host is removed; other accounts remain usable.
func (c *OAuthClient) Logout(ctx context.Context, login string, hostname string) error {
	hostname = normalizeHostname(hostname)
	auth, err := c.Store.LoadAuth()
	if err != nil {
		return fmt.Errorf("load auth: %w", err)
	}
	login = strings.TrimSpace(login)
	removed := false
	for key, credential := range auth.Credentials {
		if !strings.EqualFold(credential.Hostname, hostname) {
			continue
		}
		if login != "" && !strings.EqualFold(credential.Login, login) {
			continue
		}
		delete(auth.Credentials, key)
		if auth.Active == key {
			auth.Active = ""
		}
		removed = true
		if login != "" {
			break
		}
	}
	if !removed {
		return fmt.Errorf("no stored OAuth credential found for %s", hostname)
	}
	if err := c.Store.SaveAuth(auth); err != nil {
		return fmt.Errorf("save auth: %w", err)
	}
	return nil
}

func (c *OAuthClient) getUser(ctx context.Context, credential config.Credential) (userResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL(credential.Hostname, "/user"), nil)
	if err != nil {
		return userResponse{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	response, err := c.httpClient().Do(request)
	if err != nil {
		return userResponse{}, fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return userResponse{}, fmt.Errorf("read response body: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return userResponse{}, fmt.Errorf("GitHub returned HTTP %d: %s", response.StatusCode, responseMessage(body))
	}
	var user userResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return userResponse{}, fmt.Errorf("parse response: %w", err)
	}
	if strings.TrimSpace(user.Login) == "" {
		return userResponse{}, errors.New("GitHub returned an empty login")
	}
	return user, nil
}

func (c *OAuthClient) doForm(ctx context.Context, endpoint string, form url.Values) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("read response body: %w", err)
	}
	return body, response.StatusCode, nil
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func normalizeHostname(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimSuffix(hostname, "/")
	if hostname == "" {
		return defaultHostname
	}
	return hostname
}

func oauthURL(hostname, path string) string {
	return "https://" + normalizeHostname(hostname) + path
}

func apiURL(hostname, path string) string {
	hostname = normalizeHostname(hostname)
	if hostname == defaultHostname {
		return "https://api.github.com" + path
	}
	return "https://" + hostname + "/api/v3" + path
}

func credentialKey(hostname, login string) string {
	return normalizeHostname(hostname) + "|" + strings.ToLower(strings.TrimSpace(login))
}

func responseMessage(body []byte) string {
	var response struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &response) == nil {
		if response.Message != "" {
			return response.Message
		}
		if response.Error != "" {
			return response.Error
		}
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel, then wl-copy (Wayland)
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			return fmt.Errorf("no clipboard utility found")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("clipboard not supported")
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := stdin.Write([]byte(text)); err != nil {
		stdin.Close()
		return err
	}

	if err := stdin.Close(); err != nil {
		return err
	}

	return cmd.Wait()
}
