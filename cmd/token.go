package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/clipboard"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/terminal"
)

var clipboardCopy = clipboard.Copy

func newTokenCmd() *cobra.Command {
	var flagCopy bool

	cmd := &cobra.Command{
		Use:   "token [alias]",
		Short: "Display or copy the GitHub OAuth access token for an account",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			accountsFile, err := deps.Store.LoadAccounts()
			if err != nil {
				return err
			}

			var targetAlias string
			var targetAccount config.Account

			if len(args) > 0 {
				targetAlias = strings.TrimSpace(args[0])
				acc, exists := accountsFile.Accounts[targetAlias]
				if !exists {
					return fmt.Errorf("account %q not found", targetAlias)
				}
				targetAccount = acc
			} else {
				activeLogin, activeHostname, _ := currentGitHubIdentity(commandContext(cmd))
				if activeLogin != "" {
					for alias, acc := range accountsFile.Accounts {
						if sameGitHubAccount(activeLogin, activeHostname, acc.Login, acc.Hostname) {
							targetAlias = alias
							targetAccount = acc
							break
						}
					}
				}
				if targetAlias == "" {
					return fmt.Errorf("no active account found; specify an alias or use %s", terminal.Bold("gha use <alias>"))
				}
			}

			cred, found, err := deps.Store.GetCredential(targetAccount.Hostname, targetAccount.Login)
			if err != nil {
				return err
			}
			if !found || strings.TrimSpace(cred.AccessToken) == "" {
				return fmt.Errorf("account %q (%s) has no OAuth token; run %s first", targetAlias, targetAccount.Login, terminal.Bold("gha login "+targetAlias))
			}

			token := cred.AccessToken

			if flagCopy {
				if err := clipboardCopy(token); err != nil {
					return fmt.Errorf("copy to clipboard: %w", err)
				}
				terminal.Success(deps.Stdout, "Token copied to clipboard for %s (%s)", targetAlias, targetAccount.Login)
				return nil
			}

			fmt.Fprintln(deps.Stdout, token)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&flagCopy, "copy", "c", false, "copy token to clipboard instead of printing")
	return cmd
}
