package github

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/utils"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errorReader) Close() error             { return nil }

func response(status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}
}

func writeMalformedAuth(t *testing.T, store *config.Store) {
	t.Helper()
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials: ["), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClientConfigurationAndHelpers(t *testing.T) {
	t.Setenv("GH_GHA_CLIENT_ID", " preferred ")
	t.Setenv("GITHUB_OAUTH_CLIENT_ID", "fallback")
	if got := ClientIDFromEnv(); got != "preferred" {
		t.Fatalf("ClientIDFromEnv = %q", got)
	}
	t.Setenv("GH_GHA_CLIENT_ID", "")
	if got := ClientIDFromEnv(); got != "fallback" {
		t.Fatalf("fallback = %q", got)
	}

	c := NewOAuthClient(nil, nil, "")
	if c.Store == nil || c.Output == nil || c.ClientID != "fallback" {
		t.Fatalf("unexpected client: %+v", c)
	}
	c.SetClientID(" changed ")
	if got := c.ConfiguredClientID(); got != "changed" {
		t.Fatalf("ConfiguredClientID = %q", got)
	}
	c.HTTP = nil
	if c.httpClient() != http.DefaultClient {
		t.Fatal("nil HTTP did not use default client")
	}

	for input, want := range map[string]string{"": "github.com", "  ": "github.com", "https://ghe.example/": "ghe.example", "http://ghe.example": "ghe.example", "HTTPS://GHE.Example/": "ghe.example"} {
		if got := normalizeHostname(input); got != want {
			t.Errorf("normalizeHostname(%q) = %q", input, got)
		}
	}
	if got := apiURL("github.com", "/user"); got != "https://api.github.com/user" {
		t.Fatal(got)
	}
	if got := apiURL("GITHUB.COM", "/user"); got != "https://api.github.com/user" {
		t.Fatal(got)
	}
	if got := apiURL("ghe.example", "/user"); got != "https://ghe.example/api/v3/user" {
		t.Fatal(got)
	}
	if got := oauthURL("", "/login"); got != "https://github.com/login" {
		t.Fatal(got)
	}
	if got := credentialKey("github.com", " Alice "); got != "github.com|alice" {
		t.Fatal(got)
	}
	if got := credentialKey("HTTPS://GHE.Example/", " Alice "); got != "ghe.example|alice" {
		t.Fatal(got)
	}

	if got := responseMessage([]byte(`{"message":"bad"}`)); got != "bad" {
		t.Fatal(got)
	}
	if got := responseMessage([]byte(`{"error":"denied"}`)); got != "denied" {
		t.Fatal(got)
	}
	if got := responseMessage([]byte(" plain ")); got != "plain" {
		t.Fatal(got)
	}
	if got := responseMessage(bytes.Repeat([]byte("x"), 400)); len(got) != 300 {
		t.Fatalf("len = %d", len(got))
	}

	if err := wait(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(wait(ctx, time.Hour), context.Canceled) {
		t.Fatal("wait did not return cancellation")
	}
}

func TestIdentityCredentialStatusAndStoreErrors(t *testing.T) {
	ctx := context.Background()
	store := config.NewStore(t.TempDir())
	c := NewOAuthClient(store, io.Discard, "id")
	writeMalformedAuth(t, store)
	if _, _, err := c.CurrentIdentity(ctx); err == nil || !strings.Contains(err.Error(), "load auth") {
		t.Fatal(err)
	}
	if _, err := c.HasCredential(ctx, "alice", "github.com"); err == nil {
		t.Fatal("HasCredential accepted malformed auth")
	}

	store = config.NewStore(t.TempDir())
	c = NewOAuthClient(store, io.Discard, "id")
	auth := config.DefaultAuth()
	auth.Active = "missing"
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.CurrentIdentity(ctx); err == nil || !strings.Contains(err.Error(), "credential is missing") {
		t.Fatal(err)
	}

	delete(auth.Credentials, "missing")
	auth.Active = credentialKey("ghe.example", "alice")
	auth.Credentials[auth.Active] = config.Credential{Hostname: "ghe.example", Login: "alice", AccessToken: "token"}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, _, err := c.CurrentIdentity(ctx); err == nil || !strings.Contains(err.Error(), "get GitHub user") {
		t.Fatal(err)
	}

	c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{`{"login":"alice"}`, `{"login":"alice"}`}}}
	login, host, err := c.CurrentIdentity(ctx)
	if err != nil || login != "alice" || host != "ghe.example" {
		t.Fatalf("identity = %q %q %v", login, host, err)
	}
	ok, err := c.HasCredential(ctx, "alice", "ghe.example")
	if err != nil || !ok {
		t.Fatalf("HasCredential = %v, %v", ok, err)
	}
	ok, err = c.HasCredential(ctx, "nobody", "ghe.example")
	if err != nil || ok {
		t.Fatalf("missing credential = %v, %v", ok, err)
	}
	ok, err = c.HasCredential(ctx, "ALICE", "HTTPS://GHE.EXAMPLE/")
	if err != nil || !ok {
		t.Fatalf("normalized credential = %v, %v", ok, err)
	}
	status, err := c.Status(ctx)
	if err != nil || status != "Logged in to ghe.example account alice (gha OAuth)" {
		t.Fatalf("Status = %q, %v", status, err)
	}

	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := c.Status(ctx); err == nil || !strings.Contains(err.Error(), "get current login") {
		t.Fatal(err)
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials: ["), 0o600); err != nil {
			t.Fatal(err)
		}
		return response(200, io.NopCloser(strings.NewReader(`{"login":"alice"}`))), nil
	})}
	if _, err := c.Status(ctx); err == nil || !strings.Contains(err.Error(), "load auth") {
		t.Fatal(err)
	}
}

func TestSwitchAndLogoutBranches(t *testing.T) {
	ctx := context.Background()
	store := config.NewStore(t.TempDir())
	c := NewOAuthClient(store, io.Discard, "id")
	if err := c.SwitchUserAtHost(ctx, " ", ""); err == nil {
		t.Fatal("empty login accepted")
	}
	if err := c.SwitchUserAtHost(ctx, "missing", "HTTPS://github.com/"); err == nil {
		t.Fatal("missing login accepted")
	}

	writeMalformedAuth(t, store)
	if err := c.SwitchUserAtHost(ctx, "alice", "github.com"); err == nil || !strings.Contains(err.Error(), "save auth") {
		t.Fatal(err)
	}

	store = config.NewStore(t.TempDir())
	c = NewOAuthClient(store, io.Discard, "id")
	auth := config.DefaultAuth()
	for _, item := range []struct{ host, login string }{{"github.com", "one"}, {"github.com", "two"}, {"ghe.example", "three"}} {
		key := credentialKey(item.host, item.login)
		auth.Credentials[key] = config.Credential{Hostname: item.host, Login: item.login, AccessToken: "token"}
	}
	auth.Active = credentialKey("github.com", "two")
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	if err := c.SwitchUserAtHost(ctx, "three", "HTTPS://GHE.EXAMPLE/"); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(ctx, "", "github.com"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.LoadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Active != credentialKey("ghe.example", "three") || len(updated.Credentials) != 1 {
		t.Fatalf("unexpected auth: %+v", updated)
	}
	if err := c.Logout(ctx, "missing", "github.com"); err == nil {
		t.Fatal("missing logout accepted")
	}
	auth = config.DefaultAuth()
	for _, login := range []string{"one", "two"} {
		key := credentialKey("github.com", login)
		auth.Credentials[key] = config.Credential{Hostname: "github.com", Login: login, AccessToken: "token"}
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(ctx, "two", "github.com"); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPErrorBranches(t *testing.T) {
	ctx := context.Background()
	c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := c.getUser(ctx, config.Credential{Hostname: "github.com"}); err == nil || !strings.Contains(err.Error(), "execute") {
		t.Fatal(err)
	}
	if _, _, err := c.doForm(ctx, "https://github.com", nil); err == nil || !strings.Contains(err.Error(), "execute") {
		t.Fatal(err)
	}
	if _, _, err := c.doForm(ctx, "://bad", nil); err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatal(err)
	}
	originalRequest := newRequestWithContext
	newRequestWithContext = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("forced create failure")
	}
	if _, err := c.getUser(ctx, config.Credential{}); err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatal(err)
	}
	newRequestWithContext = originalRequest

	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return response(200, errorReader{}), nil })}
	if _, err := c.getUser(ctx, config.Credential{Hostname: "github.com"}); err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatal(err)
	}
	if _, _, err := c.doForm(ctx, "https://github.com", nil); err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		body   string
		status int
		want   string
	}{
		{`{"message":"bad"}`, 401, "HTTP 401"}, {`{`, 200, "parse response"}, {`{"login":""}`, 200, "empty login"},
	} {
		c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{tc.body}, statuses: []int{tc.status}}}
		if _, err := c.getUser(ctx, config.Credential{Hostname: "github.com"}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %q, got %v", tc.want, err)
		}
	}
}

func TestDeviceCodeResponses(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		body     string
		status   int
		wantErr  string
		interval int
	}{
		{`{"message":"bad"}`, 500, "HTTP 500", 0},
		{`%`, 200, "decode device response", 0},
		{`device_code=d&user_code=u&verification_uri=https%3A%2F%2Fgithub.com%2Flogin%2Fdevice&expires_in=60`, 200, "", 5},
		{`{"device_code":"d"}`, 200, "incomplete", 0},
	} {
		c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
		c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{tc.body}, statuses: []int{tc.status}}}
		got, err := c.requestDeviceCode(ctx, "github.com")
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want %q, got %v", tc.wantErr, err)
			}
			continue
		}
		if err != nil || got.Interval != tc.interval {
			t.Fatalf("response = %+v, %v", got, err)
		}
	}
	c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := c.requestDeviceCode(ctx, "github.com"); err == nil {
		t.Fatal("transport error ignored")
	}
}

func TestPollTokenResponses(t *testing.T) {
	original := waitForPoll
	waitForPoll = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { waitForPoll = original })
	device := deviceCodeResponse{DeviceCode: "d", ExpiresIn: 60}

	tests := []struct {
		responses []string
		statuses  []int
		want      string
	}{
		{[]string{`%`}, nil, "decode token response"},
		{[]string{`{"error":"expired_token"}`}, nil, "expired"},
		{[]string{`{"error":"access_denied"}`}, nil, "denied"},
		{[]string{`{"error":"other","error_description":"details"}`}, nil, "details"},
		{[]string{`{"error":"other"}`}, nil, "other"},
		{[]string{`{"message":"bad"}`}, []int{500}, "HTTP 500"},
		{[]string{`access_token=token&token_type=bearer&scope=repo`, `{"login":"alice"}`}, nil, ""},
		{[]string{`{"error":"slow_down"}`, `{"access_token":"token"}`, `{"login":"alice"}`}, nil, ""},
		{[]string{`{"error":"authorization_pending"}`, `{"access_token":"token"}`, `{"login":"alice"}`}, nil, ""},
		{[]string{`{"access_token":"token"}`, `{"login":""}`}, nil, "authenticated GitHub user"},
	}
	for _, tc := range tests {
		c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
		c.HTTP = &http.Client{Transport: &fakeTransport{responses: tc.responses, statuses: tc.statuses}}
		got, err := c.pollToken(context.Background(), "github.com", device)
		if tc.want != "" {
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want %q, got %+v, %v", tc.want, got, err)
			}
		} else if err != nil || got.Login != "alice" {
			t.Errorf("got %+v, %v", got, err)
		}
	}

	c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := c.pollToken(context.Background(), "github.com", device); err == nil {
		t.Fatal("transport error ignored")
	}
	waitForPoll = func(context.Context, time.Duration) error { return context.Canceled }
	if _, err := c.pollToken(context.Background(), "github.com", device); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := c.pollToken(context.Background(), "github.com", deviceCodeResponse{DeviceCode: "d"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestLoginOutputAndErrors(t *testing.T) {
	original := waitForPoll
	originalCopy := copyToClipboard
	waitForPoll = func(context.Context, time.Duration) error { return nil }
	copyToClipboard = func(string) error { return nil }
	t.Cleanup(func() { waitForPoll, copyToClipboard = original, originalCopy })

	for _, openerErr := range []error{nil, errors.New("no browser")} {
		var output bytes.Buffer
		transport := &fakeTransport{responses: []string{
			`{"device_code":"d","user_code":"CODE","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`,
			`{"access_token":"token"}`, `{"login":"alice"}`,
		}}
		c := NewOAuthClient(config.NewStore(t.TempDir()), &output, "id")
		c.HTTP = &http.Client{Transport: transport}
		c.OpenBrowser = func(string) error { return openerErr }
		if err := c.Login(context.Background(), "", "https"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Enter code: CODE") {
			t.Fatal(output.String())
		}
		if openerErr == nil && !strings.Contains(output.String(), "Browser opened") {
			t.Fatal(output.String())
		}
		if openerErr != nil && !strings.Contains(output.String(), "Could not open") {
			t.Fatal(output.String())
		}
	}

	c := NewOAuthClient(config.NewStore(t.TempDir()), io.Discard, "id")
	c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{`{"message":"bad"}`}, statuses: []int{500}}}
	if err := c.Login(context.Background(), "github.com", "https"); err == nil || !strings.Contains(err.Error(), "request GitHub device") {
		t.Fatal(err)
	}
	c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{
		`{"device_code":"d","user_code":"CODE","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`,
		`{"error":"expired_token"}`,
	}}}
	if err := c.Login(context.Background(), "github.com", "https"); err == nil || !strings.Contains(err.Error(), "complete GitHub OAuth") {
		t.Fatal(err)
	}

	badDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c = NewOAuthClient(config.NewStore(badDir), io.Discard, "id")
	c.HTTP = &http.Client{Transport: &fakeTransport{responses: []string{
		`{"device_code":"d","user_code":"CODE","verification_uri":"https://github.com/login/device","expires_in":60,"interval":1}`,
		`{"access_token":"token"}`, `{"login":"alice"}`,
	}}}
	c.OpenBrowser = nil
	if err := c.Login(context.Background(), "github.com", "https"); err == nil || !strings.Contains(err.Error(), "save GitHub OAuth") {
		t.Fatal(err)
	}
}

func TestOpenBrowserPlatformsAndFailures(t *testing.T) {
	originalOS, originalCommand := browserGOOS, browserCommand
	t.Cleanup(func() { browserGOOS, browserCommand = originalOS, originalCommand })
	if err := OpenBrowser("http://example.com"); err == nil {
		t.Fatal("http URL accepted")
	}
	if err := OpenBrowser("://bad"); err == nil {
		t.Fatal("bad URL accepted")
	}

	for _, platform := range []string{"darwin", "windows", "linux", "freebsd", "openbsd", "netbsd"} {
		browserGOOS = platform
		browserCommand = func(string, ...string) *exec.Cmd { return exec.Command(os.Args[0], "-test.run=^$") }
		if err := OpenBrowser("https://github.com/login/device"); err != nil {
			t.Errorf("%s: %v", platform, err)
		}
	}
	browserGOOS = "plan9"
	if err := OpenBrowser("https://github.com"); err == nil {
		t.Fatal("unsupported OS accepted")
	}
	browserGOOS = "linux"
	browserCommand = func(string, ...string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing")) }
	if err := OpenBrowser("https://github.com"); err == nil || !strings.Contains(err.Error(), "start browser") {
		t.Fatal(err)
	}
}
