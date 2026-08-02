package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhongtait/gh-account/internal/utils"
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

// CLIClient implements Client via the git binary.
type CLIClient struct {
	Runner utils.Runner
	Dir    string
}

// NewCLIClient creates a git client for the given working directory.
func NewCLIClient(runner utils.Runner, dir string) *CLIClient {
	if runner == nil {
		runner = utils.RealRunner{}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return &CLIClient{Runner: runner, Dir: dir}
}

func (c *CLIClient) run(ctx context.Context, args ...string) (string, error) {
	cmdArgs := make([]string, 0, len(args)+2)
	if c.Dir != "" {
		cmdArgs = append(cmdArgs, "-C", c.Dir)
	}
	cmdArgs = append(cmdArgs, args...)
	out, _, err := c.Runner.Run(ctx, "git", cmdArgs...)
	return out, err
}

// IsRepo reports whether Dir is inside a git work tree.
func (c *CLIClient) IsRepo(ctx context.Context) (bool, error) {
	out, err := c.run(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) == "true", nil
}

// TopLevel returns the repository root.
func (c *CLIClient) TopLevel(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(out), nil
}

// GetIdentity reads user.name and user.email for the given scope.
func (c *CLIClient) GetIdentity(ctx context.Context, scope Scope) (Identity, error) {
	name, err := c.getConfig(ctx, scope, "user.name")
	if err != nil {
		return Identity{}, err
	}
	email, err := c.getConfig(ctx, scope, "user.email")
	if err != nil {
		return Identity{}, err
	}
	return Identity{Name: name, Email: email}, nil
}

// SetIdentity writes user.name and user.email for the given scope.
func (c *CLIClient) SetIdentity(ctx context.Context, scope Scope, identity Identity) error {
	if err := c.setConfig(ctx, scope, "user.name", identity.Name); err != nil {
		return err
	}
	return c.setConfig(ctx, scope, "user.email", identity.Email)
}

// GetRemoteURL returns the URL for a remote.
func (c *CLIClient) GetRemoteURL(ctx context.Context, name string) (string, error) {
	if name == "" {
		name = "origin"
	}
	return c.run(ctx, "remote", "get-url", name)
}

// SetRemoteURL updates a remote URL.
func (c *CLIClient) SetRemoteURL(ctx context.Context, name, url string) error {
	if name == "" {
		name = "origin"
	}
	_, err := c.run(ctx, "remote", "set-url", name, url)
	return err
}

// CurrentBranch returns the current branch name when available.
func (c *CLIClient) CurrentBranch(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return out, nil
}

func (c *CLIClient) getConfig(ctx context.Context, scope Scope, key string) (string, error) {
	args := []string{"config"}
	switch scope {
	case ScopeGlobal:
		args = append(args, "--global")
	case ScopeLocal:
		args = append(args, "--local")
	default:
		return "", fmt.Errorf("unsupported git scope %q", scope)
	}
	args = append(args, "--get", key)
	out, err := c.run(ctx, args...)
	if err != nil {
		// Missing keys are treated as empty rather than hard failures.
		return "", nil
	}
	return out, nil
}

func (c *CLIClient) setConfig(ctx context.Context, scope Scope, key, value string) error {
	args := []string{"config"}
	switch scope {
	case ScopeGlobal:
		args = append(args, "--global")
	case ScopeLocal:
		args = append(args, "--local")
	default:
		return fmt.Errorf("unsupported git scope %q", scope)
	}
	args = append(args, key, value)
	_, err := c.run(ctx, args...)
	return err
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
