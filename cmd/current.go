package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show current GitHub and git identity state",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)

			var matchedAlias string
			var account config.Account
			if err := requireInitialized(); err == nil {
				if login, err := deps.GitHub.CurrentLogin(ctx); err == nil {
					if file, err := deps.Store.LoadAccounts(); err == nil {
						for alias, acc := range file.Accounts {
							if acc.Login == login {
								matchedAlias = alias
								account = acc
								break
							}
						}
					}
				}
			}

			cfgScope := git.ScopeLocal
			if cfg, err := deps.Store.LoadConfig(); err == nil {
				if parsed, err := git.ParseScope(cfg.DefaultScope, git.ScopeLocal); err == nil {
					cfgScope = parsed
				}
			}

			return printCurrentSummary(cmd, matchedAlias, account, cfgScope)
		},
	}
}

func printCurrentSummary(cmd *cobra.Command, alias string, account config.Account, scope git.Scope) error {
	ctx := commandContext(cmd)

	fmt.Fprintln(deps.Stdout, terminal.Bold("Current"))

	if login, err := deps.GitHub.CurrentLogin(ctx); err != nil {
		terminal.Info(deps.Stdout, "GitHub : (unavailable)")
	} else {
		if alias != "" {
			terminal.Info(deps.Stdout, "GitHub : %s (%s)", login, alias)
		} else {
			terminal.Info(deps.Stdout, "GitHub : %s", login)
		}
	}

	identity, err := deps.Git.GetIdentity(ctx, scope)
	if err != nil {
		terminal.Info(deps.Stdout, "Git     : (unavailable)")
	} else {
		name := identity.Name
		email := identity.Email
		if name == "" && account.GitName != "" {
			name = account.GitName
		}
		if email == "" && account.Email != "" {
			email = account.Email
		}
		if name == "" {
			name = "(unset)"
		}
		if email == "" {
			email = "(unset)"
		}
		terminal.Info(deps.Stdout, "Git     : %s <%s> [%s]", name, email, scope)
	}

	isRepo, err := deps.Git.IsRepo(ctx)
	if err != nil || !isRepo {
		terminal.Info(deps.Stdout, "Repo    : (not a git repository)")
		return nil
	}

	top, err := deps.Git.TopLevel(ctx)
	if err != nil {
		terminal.Info(deps.Stdout, "Repo    : (unknown)")
	} else {
		terminal.Info(deps.Stdout, "Repo    : %s", top)
	}

	if branch, err := deps.Git.CurrentBranch(ctx); err == nil && branch != "" {
		terminal.Info(deps.Stdout, "Branch  : %s", branch)
	}

	if remoteURL, err := deps.Git.GetRemoteURL(ctx, "origin"); err != nil {
		terminal.Info(deps.Stdout, "Remote  : (no origin)")
	} else {
		info, parseErr := remote.Parse(remoteURL)
		if parseErr != nil {
			terminal.Info(deps.Stdout, "Remote  : %s", remoteURL)
		} else {
			terminal.Info(deps.Stdout, "Remote  : %s/%s (%s)", info.Owner, info.Repo, info.Protocol)
			terminal.Info(deps.Stdout, "URL     : %s", remoteURL)
		}
	}

	return nil
}
