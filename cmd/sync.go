package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync git identity from the active GitHub login",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			ctx := commandContext(cmd)
			login, hostname, err := currentGitHubIdentity(ctx)
			if err != nil {
				return fmt.Errorf("unable to detect active GitHub login: %w", err)
			}

			file, err := deps.Store.LoadAccounts()
			if err != nil {
				return err
			}

			var alias string
			var account config.Account
			for name, candidate := range file.Accounts {
				if sameGitHubAccount(login, hostname, candidate.Login, candidate.Hostname) {
					alias = name
					account = candidate
					break
				}
			}
			if alias == "" {
				return fmt.Errorf("no configured account matches active login %q; run gha add", login)
			}

			cfg, err := deps.Store.LoadConfig()
			if err != nil {
				return err
			}
			scope, err := resolveScope(cfg)
			if err != nil {
				return err
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

			if err := deps.Git.SetIdentity(ctx, scope, git.Identity{Name: account.GitName, Email: account.Email}); err != nil {
				return err
			}

			terminal.Success(deps.Stdout, "Synced git identity from %s (%s)", alias, login)
			return printCurrentSummary(cmd, alias, account, scope)
		},
	}

	cmd.Flags().BoolVar(&flagGlobal, "global", false, "write git identity to global config")
	return cmd
}
