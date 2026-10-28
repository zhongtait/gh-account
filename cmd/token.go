package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/clipboard"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/terminal"
)

var clipboardCopy = clipboard.Copy

func newTokenCmd() *cobra.Command {
	var flagCopy bool
	var flagCheck bool

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

			if flagCheck {
				if inspector, ok := deps.GitHub.(github.TokenInspector); ok {
					info, err := inspector.InspectToken(commandContext(cmd), targetAccount.Hostname, token)
					if err != nil {
						terminal.Error(deps.Stdout, "Token check failed: %v", err)
						return err
					}
					terminal.Success(deps.Stdout, "Token is valid for %s (%s)", terminal.Bold(info.Login), targetAccount.Hostname)
					if len(info.Scopes) > 0 {
						terminal.Info(deps.Stdout, "Scopes: %s", strings.Join(info.Scopes, ", "))
						hasWorkflow := false
						hasReadOrg := false
						for _, s := range info.Scopes {
							if s == "workflow" {
								hasWorkflow = true
							}
							if s == "read:org" {
								hasReadOrg = true
							}
						}
						if hasWorkflow {
							terminal.Success(deps.Stdout, "✓ workflow scope present (Actions push permitted)")
						} else {
							terminal.Warn(deps.Stdout, "⚠ workflow scope missing (pushing .github/workflows/*.yml may fail)")
						}
						if hasReadOrg {
							terminal.Success(deps.Stdout, "✓ read:org scope present (PR comments & org access permitted)")
						} else {
							terminal.Warn(deps.Stdout, "⚠ read:org scope missing (viewing PR comments or org items may fail)")
						}
					} else {
						terminal.Info(deps.Stdout, "Scopes: (none or fine-grained PAT)")
					}
					return nil
				}
				terminal.Warn(deps.Stdout, "Token inspection is not supported by current GitHub client")
				return nil
			}

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
	cmd.Flags().BoolVar(&flagCheck, "check", false, "check token validity and scopes via GitHub API")
	cmd.Flags().BoolVar(&flagCheck, "scopes", false, "alias for --check")
	return cmd
}
