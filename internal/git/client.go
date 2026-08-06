package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Scope controls whether git config is written locally or globally.
type Scope string

const (
	ScopeLocal  Scope = "local"
	ScopeGlobal Scope = "global"
)

// Identity is the commit author identity.
type Identity struct {
	Name  string
	Email string
}

// Client abstracts git operations used by gha.
type Client interface {
	IsRepo(ctx context.Context) (bool, error)
	TopLevel(ctx context.Context) (string, error)
	GetIdentity(ctx context.Context, scope Scope) (Identity, error)
	SetIdentity(ctx context.Context, scope Scope, identity Identity) error
	GetRemoteURL(ctx context.Context, name string) (string, error)
	SetRemoteURL(ctx context.Context, name, url string) error
	CurrentBranch(ctx context.Context) (string, error)
}

// CredentialClient configures Git's credential helper for a scope.
type CredentialClient interface {
	SetCredentialHelper(ctx context.Context, scope Scope, command, accountKey string) error
}

// NativeClient implements the subset of Git repository/config operations
// required by gha without invoking the git executable.
type NativeClient struct {
	Dir string
}

// NewNativeClient creates a pure Go Git client rooted at dir.
func NewNativeClient(dir string) *NativeClient {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return &NativeClient{Dir: dir}
}

// IsRepo reports whether Dir is inside a Git work tree.
func (c *NativeClient) IsRepo(ctx context.Context) (bool, error) {
	_, _, found, err := c.repo()
	return found, err
}

// TopLevel returns the repository root.
func (c *NativeClient) TopLevel(ctx context.Context) (string, error) {
	root, _, found, err := c.repo()
	if err != nil {
		return "", fmt.Errorf("check git repository: %w", err)
	}
	if !found {
		return "", errors.New("not inside a git repository")
	}
	return root, nil
}

// GetIdentity reads user.name and user.email for the given scope.
func (c *NativeClient) GetIdentity(ctx context.Context, scope Scope) (Identity, error) {
	file, err := c.loadConfig(scope)
	if err != nil {
		return Identity{}, fmt.Errorf("load git config: %w", err)
	}
	return Identity{Name: file.get("user.name"), Email: file.get("user.email")}, nil
}

// SetIdentity writes user.name and user.email for the given scope.
func (c *NativeClient) SetIdentity(ctx context.Context, scope Scope, identity Identity) error {
	file, err := c.loadConfig(scope)
	if err != nil {
		return fmt.Errorf("load git config: %w", err)
	}
	file.set("user.name", identity.Name)
	file.set("user.email", identity.Email)
	if err := c.saveConfig(scope, file); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// SetCredentialHelper configures gha as the only credential helper for the
// selected Git config scope. An empty helper value resets helpers inherited
// from broader scopes, so a stale system credential cannot win first.
func (c *NativeClient) SetCredentialHelper(ctx context.Context, scope Scope, command, accountKey string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("credential helper command is required")
	}
	if strings.TrimSpace(accountKey) == "" {
		return errors.New("credential account key is required")
	}
	file, err := c.loadConfig(scope)
	if err != nil {
		return fmt.Errorf("load git config: %w", err)
	}
	file.setMulti("credential.helper", []string{"", strings.TrimSpace(command)})
	file.set("gha.account-key", strings.TrimSpace(accountKey))
	if err := c.saveConfig(scope, file); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// GetRemoteURL returns a remote URL.
func (c *NativeClient) GetRemoteURL(ctx context.Context, name string) (string, error) {
	if name == "" {
		name = "origin"
	}
	file, err := c.loadConfig(ScopeLocal)
	if err != nil {
		return "", fmt.Errorf("load git config: %w", err)
	}
	value := file.get("remote." + strings.ToLower(strings.TrimSpace(name)) + ".url")
	if value == "" {
		return "", fmt.Errorf("remote %q not found", name)
	}
	return value, nil
}

// SetRemoteURL updates a remote URL.
func (c *NativeClient) SetRemoteURL(ctx context.Context, name, remoteURL string) error {
	if name == "" {
		name = "origin"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("remote name is required")
	}
	if strings.TrimSpace(remoteURL) == "" {
		return errors.New("remote URL is required")
	}
	file, err := c.loadConfig(ScopeLocal)
	if err != nil {
		return fmt.Errorf("load git config: %w", err)
	}
	file.set("remote."+strings.ToLower(name)+".url", strings.TrimSpace(remoteURL))
	if err := c.saveConfig(ScopeLocal, file); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// CurrentBranch returns the symbolic branch from HEAD. Detached HEAD returns
// an empty branch, matching the previous git CLI behavior.
func (c *NativeClient) CurrentBranch(ctx context.Context) (string, error) {
	_, gitDir, found, err := c.repo()
	if err != nil {
		return "", fmt.Errorf("check git repository: %w", err)
	}
	if !found {
		return "", errors.New("not inside a git repository")
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", fmt.Errorf("read HEAD: %w", err)
	}
	head := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(head, prefix) {
		return strings.TrimPrefix(head, prefix), nil
	}
	return "", nil
}

func (c *NativeClient) repo() (root, gitDir string, found bool, err error) {
	start, err := filepath.Abs(c.Dir)
	if err != nil {
		return "", "", false, fmt.Errorf("get absolute path: %w", err)
	}
	if info, statErr := os.Stat(start); statErr == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	for current := start; ; current = filepath.Dir(current) {
		marker := filepath.Join(current, ".git")
		info, statErr := os.Stat(marker)
		if statErr == nil {
			if info.IsDir() {
				return current, marker, true, nil
			}
			data, readErr := os.ReadFile(marker)
			if readErr != nil {
				return "", "", false, fmt.Errorf("read .git file: %w", readErr)
			}
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(strings.ToLower(line), "gitdir:") {
				value := strings.TrimSpace(line[len("gitdir:"):])
				if !filepath.IsAbs(value) {
					value = filepath.Join(current, value)
				}
				return current, filepath.Clean(value), true, nil
			}
			return "", "", false, errors.New("invalid .git file")
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", false, fmt.Errorf("stat .git: %w", statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", false, nil
		}
	}
}

func (c *NativeClient) configPath(scope Scope) (string, error) {
	switch scope {
	case ScopeLocal:
		_, gitDir, found, err := c.repo()
		if err != nil {
			return "", fmt.Errorf("check git repository: %w", err)
		}
		if !found {
			return "", errors.New("not inside a git repository")
		}
		return filepath.Join(gitDir, "config"), nil
	case ScopeGlobal:
		if path := strings.TrimSpace(os.Getenv("GIT_CONFIG_GLOBAL")); path != "" {
			return path, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return filepath.Join(xdg, "git", "config"), nil
		}
		return filepath.Join(home, ".gitconfig"), nil
	default:
		return "", fmt.Errorf("unsupported git scope %q", scope)
	}
}

func (c *NativeClient) loadConfig(scope Scope) (gitConfig, error) {
	path, err := c.configPath(scope)
	if err != nil {
		return gitConfig{}, fmt.Errorf("get config path: %w", err)
	}
	config, err := readGitConfig(path)
	if err != nil {
		return gitConfig{}, fmt.Errorf("read git config from %s: %w", path, err)
	}
	return config, nil
}

func (c *NativeClient) saveConfig(scope Scope, file gitConfig) error {
	path, err := c.configPath(scope)
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data := strings.Join(file.lines, "\n")
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return fmt.Errorf("write git config to %s: %w", path, err)
	}
	return nil
}

type gitConfig struct {
	lines []string
}

var (
	sectionPattern = regexp.MustCompile(`^\s*\[([^]]+)\]\s*$`)
	keyPattern     = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*(?:=|\s+)\s*(.*?)\s*$`)
)

func readGitConfig(path string) (gitConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return gitConfig{lines: []string{}}, nil
	}
	if err != nil {
		return gitConfig{}, fmt.Errorf("read file: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, strings.TrimSuffix(scanner.Text(), "\r"))
	}
	if err := scanner.Err(); err != nil {
		return gitConfig{}, fmt.Errorf("scan config file: %w", err)
	}
	return gitConfig{lines: lines}, nil
}

func (f gitConfig) get(wanted string) string {
	section := ""
	value := ""
	for _, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(strings.TrimSpace(line), ";") {
			continue
		}
		if section+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			value = unquoteGitValue(match[2])
		}
	}
	return value
}

func (f *gitConfig) set(wanted, value string) {
	parts := strings.Split(wanted, ".")
	section := strings.Join(parts[:len(parts)-1], ".")
	key := parts[len(parts)-1]
	currentSection := ""
	last := -1
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			currentSection = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && currentSection+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			last = index
		}
	}
	formatted := key + " = " + quoteGitValue(value)
	if last >= 0 {
		f.lines[last] = formatted
		return
	}
	if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
		f.lines = append(f.lines, "")
	}
	f.lines = append(f.lines, "["+formatSection(section)+"]", formatted)
}

func (f *gitConfig) setMulti(wanted string, values []string) {
	parts := strings.Split(wanted, ".")
	section := strings.Join(parts[:len(parts)-1], ".")
	key := parts[len(parts)-1]

	filtered := make([]string, 0, len(f.lines)+len(values))
	currentSection := ""
	for _, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			currentSection = canonicalSection(match[1])
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && currentSection+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			continue
		}
		filtered = append(filtered, line)
	}
	f.lines = filtered

	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, key+" = "+quoteGitValue(value))
	}

	targetSection := canonicalSection(section)
	sectionStart := -1
	sectionEnd := len(f.lines)
	currentSection = ""
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			if sectionStart >= 0 && sectionEnd == len(f.lines) {
				sectionEnd = index
			}
			currentSection = canonicalSection(match[1])
			if currentSection == targetSection {
				sectionStart = index
				sectionEnd = len(f.lines)
			}
		}
	}
	if sectionStart < 0 {
		if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
			f.lines = append(f.lines, "")
		}
		f.lines = append(f.lines, "["+formatSection(section)+"]")
		f.lines = append(f.lines, formatted...)
		return
	}

	// Insert into the last matching section, after its existing contents.
	insertAt := sectionEnd
	f.lines = append(f.lines, make([]string, len(formatted))...)
	copy(f.lines[insertAt+len(formatted):], f.lines[insertAt:len(f.lines)-len(formatted)])
	copy(f.lines[insertAt:insertAt+len(formatted)], formatted)
}

func canonicalSection(section string) string {
	section = strings.TrimSpace(section)
	if quote := strings.Index(section, `"`); quote >= 0 {
		section = strings.TrimSpace(section[:quote]) + "." + strings.Trim(section[quote:], `"`)
	}
	return strings.ToLower(strings.TrimSpace(section))
}

func formatSection(section string) string {
	parts := strings.SplitN(section, ".", 2)
	if len(parts) == 2 {
		return parts[0] + ` "` + parts[1] + `"`
	}
	return section
}

func quoteGitValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "#;\t\n\r") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	}
	return value
}

func unquoteGitValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		value = strings.TrimSuffix(strings.TrimPrefix(value, `"`), `"`)
		value = strings.ReplaceAll(strings.ReplaceAll(value, `\"`, `"`), `\\`, `\`)
	}
	return value
}

// ParseScope converts a CLI scope string into a Scope value.
func ParseScope(raw string, fallback Scope) (Scope, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		if fallback == "" {
			return ScopeLocal, nil
		}
		return fallback, nil
	}
	switch Scope(value) {
	case ScopeLocal, ScopeGlobal:
		return Scope(value), nil
	default:
		return "", fmt.Errorf("invalid scope %q (expected local or global)", raw)
	}
}
