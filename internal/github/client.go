package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/zhongtait/gh-account/internal/utils"
)

// Client abstracts GitHub CLI authentication operations.
type Client interface {
	CurrentLogin(ctx context.Context) (string, error)
	SwitchUser(ctx context.Context, login string) error
	Status(ctx context.Context) (string, error)
	Login(ctx context.Context, hostname string, gitProtocol string) error
	Logout(ctx context.Context, login string, hostname string) error
}

// CLIClient implements Client via the gh binary.
type CLIClient struct {
	Runner            utils.Runner
	InteractiveRunner utils.InteractiveRunner
}

// NewCLIClient creates a GitHub CLI client.
func NewCLIClient(runner utils.Runner) *CLIClient {
	if runner == nil {
		runner = utils.RealRunner{}
	}
	return &CLIClient{
		Runner:            runner,
		InteractiveRunner: utils.RealInteractiveRunner{},
	}
}

// CurrentLogin returns the active GitHub login.
func (c *CLIClient) CurrentLogin(ctx context.Context) (string, error) {
	out, _, err := c.Runner.Run(ctx, "gh", "api", "user", "--jq", ".login")
	if err != nil {
		// Fallback for environments where api is unavailable/unauthorized.
		status, statusErr := c.Status(ctx)
		if statusErr != nil {
			return "", err
		}
		if login := extractActiveLogin(status); login != "" {
			return login, nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SwitchUser switches the active gh auth user.
func (c *CLIClient) SwitchUser(ctx context.Context, login string) error {
	login = strings.TrimSpace(login)
	if login == "" {
		return fmt.Errorf("login is required")
	}
	_, _, err := c.Runner.Run(ctx, "gh", "auth", "switch", "--user", login)
	return err
}

// Status returns raw gh auth status output.
func (c *CLIClient) Status(ctx context.Context) (string, error) {
	out, _, err := c.Runner.Run(ctx, "gh", "auth", "status")
	return out, err
}

// Login starts an interactive gh auth login flow.
func (c *CLIClient) Login(ctx context.Context, hostname string, gitProtocol string) error {
	args := []string{"auth", "login"}
	if strings.TrimSpace(hostname) != "" {
		args = append(args, "--hostname", hostname)
	}
	if protocol := strings.ToLower(strings.TrimSpace(gitProtocol)); protocol == "https" || protocol == "ssh" {
		args = append(args, "--git-protocol", protocol)
	}

	if c.InteractiveRunner != nil {
		return c.InteractiveRunner.RunInteractive(ctx, "gh", args...)
	}
	_, _, err := c.Runner.Run(ctx, "gh", args...)
	return err
}

// Logout logs out a specific user when provided.
func (c *CLIClient) Logout(ctx context.Context, login string, hostname string) error {
	args := []string{"auth", "logout"}
	if strings.TrimSpace(hostname) != "" {
		args = append(args, "--hostname", hostname)
	}
	if strings.TrimSpace(login) != "" {
		args = append(args, "--user", login)
	}
	_, _, err := c.Runner.Run(ctx, "gh", args...)
	return err
}

func extractActiveLogin(status string) string {
	lines := strings.Split(status, "\n")
	var currentHostLogin string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "✓ Logged in to") || strings.HasPrefix(trimmed, "Logged in to") {
			// Example: ✓ Logged in to github.com account tu-xiao (keyring)
			parts := strings.Fields(trimmed)
			for i := 0; i < len(parts); i++ {
				if parts[i] == "account" && i+1 < len(parts) {
					currentHostLogin = strings.Trim(parts[i+1], "()")
				}
			}
		}
		if strings.Contains(trimmed, "Active account: true") && currentHostLogin != "" {
			return currentHostLogin
		}
	}
	return currentHostLogin
}
