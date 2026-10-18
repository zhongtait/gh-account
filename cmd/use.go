package cmd

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "use [alias]",
		Aliases:           []string{"switch"},
		Short:             "Switch GitHub account and sync git identity",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			ctx := commandContext(cmd)
			alias := ""
			if len(args) == 1 {
				alias = args[0]
			} else {
				var err error
				alias, err = promptSelectAccount(ctx)
				if err != nil {
					return err
				}
				if alias == "" {
					return nil
				}
			}
			account, err := deps.Store.GetAccount(alias)
			if err != nil {
				return err
			}

			cfg, err := deps.Store.LoadConfig()
			if err != nil {
				return err
			}

			scope, err := resolveScope(cfg)
			if err != nil {
				return err
			}

			var switchErr error
			if hostClient, ok := deps.GitHub.(github.HostClient); ok {
				switchErr = hostClient.SwitchUserAtHost(ctx, account.Login, account.Hostname)
			} else {
				switchErr = deps.GitHub.SwitchUser(ctx, account.Login)
			}
			if switchErr != nil {
				terminal.Warn(deps.Stdout, "OAuth account switch failed: %v", switchErr)
				terminal.Info(deps.Stdout, "Continuing with git identity sync for %s", alias)
			} else {
				terminal.Success(deps.Stdout, "Switched GitHub account to %s", account.Login)
			}

			if scope == git.ScopeLocal {
				isRepo, err := deps.Git.IsRepo(ctx)
				if err != nil {
					return err
				}
				if !isRepo {
					return fmt.Errorf("not inside a git repository; use --global or run inside a repo")
				}
			}

			credentialConfigured := false
			if credentialClient, ok := deps.Git.(git.CredentialClient); ok {
				if err := credentialClient.SetCredentialHelper(ctx, scope, credentialHelperCommand(deps.Store, account), credentialAccountKey(account)); err != nil {
					return fmt.Errorf("configure Git credential helper: %w", err)
				}
				credentialConfigured = true
			} else {
				terminal.Warn(deps.Stdout, "Git credential helper integration is unavailable")
			}
			if credentialConfigured {
				terminal.Success(deps.Stdout, "Configured Git credentials for %s", account.Login)
			}

			if err := deps.Git.SetIdentity(ctx, scope, git.Identity{Name: account.GitName, Email: account.Email}); err != nil {
				return err
			}
			terminal.Success(deps.Stdout, "Updated git identity (%s): %s <%s>", scope, account.GitName, account.Email)

			updateRemote := flagUpdateRemote || cfg.UpdateRemote
			if updateRemote {
				if err := updateOriginRemote(ctx, account); err != nil {
					terminal.Warn(deps.Stdout, "remote update skipped: %v", err)
				}
			}

			return printCurrentSummary(cmd, alias, account, scope)
		},
	}

	cmd.Flags().BoolVar(&flagGlobal, "global", false, "write git identity to global config")
	cmd.Flags().BoolVar(&flagUpdateRemote, "update-remote", false, "rewrite origin remote owner to the account login")
	return cmd
}

func resolveScope(cfg config.ConfigFile) (git.Scope, error) {
	if flagGlobal {
		return git.ScopeGlobal, nil
	}
	return git.ParseScope(cfg.DefaultScope, git.ScopeLocal)
}

func updateOriginRemote(ctx context.Context, account config.Account) error {
	isRepo, err := deps.Git.IsRepo(ctx)
	if err != nil {
		return err
	}
	if !isRepo {
		return fmt.Errorf("not inside a git repository")
	}

	currentURL, err := deps.Git.GetRemoteURL(ctx, "origin")
	if err != nil {
		return err
	}

	info, err := remote.Parse(currentURL)
	if err != nil {
		return err
	}

	// Prefer account protocol when rewriting.
	if account.Protocol != "" {
		info.Protocol = account.Protocol
	}
	info.Owner = account.Login

	nextURL, err := remote.Build(info)
	if err != nil {
		return err
	}
	if nextURL == currentURL {
		terminal.Info(deps.Stdout, "Remote already points to %s", nextURL)
		return nil
	}
	if err := deps.Git.SetRemoteURL(ctx, "origin", nextURL); err != nil {
		return err
	}
	terminal.Success(deps.Stdout, "Updated origin remote: %s", nextURL)
	return nil
}

func promptSelectAccount(ctx context.Context) (string, error) {
	file, err := deps.Store.LoadAccounts()
	if err != nil {
		return "", err
	}
	if len(file.Accounts) == 0 {
		terminal.Info(deps.Stdout, "No accounts configured. Run %s", terminal.Bold("gha add"))
		return "", nil
	}

	aliases := make([]string, 0, len(file.Accounts))
	for a := range file.Accounts {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	activeLogin, activeHostname, _ := currentGitHubIdentity(ctx)

	fmt.Fprintln(deps.Stdout, terminal.Bold("Available accounts:"))
	for i, a := range aliases {
		acc := file.Accounts[a]
		activeMark := ""
		if activeLogin != "" && sameGitHubAccount(activeLogin, activeHostname, acc.Login, acc.Hostname) {
			activeMark = " *"
		}
		fmt.Fprintf(deps.Stdout, "  %d) %s (%s <%s>)%s\n", i+1, a, acc.GitName, acc.Email, activeMark)
	}

	fmt.Fprintf(deps.Stdout, "Choose [1-%d]: ", len(aliases))
	reader := bufio.NewReader(deps.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", nil
	}
	input := strings.TrimSpace(line)
	if input == "" {
		return "", nil
	}

	if idx, convErr := strconv.Atoi(input); convErr == nil {
		if idx >= 1 && idx <= len(aliases) {
			return aliases[idx-1], nil
		}
		return "", fmt.Errorf("invalid selection %q; number out of range", input)
	}

	if _, ok := file.Accounts[input]; ok {
		return input, nil
	}

	return "", fmt.Errorf("invalid selection %q; account not found", input)
}
