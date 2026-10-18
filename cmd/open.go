package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

var openBrowser = github.OpenBrowser

func newOpenCmd() *cobra.Command {
	var flagRepo bool
	var flagPR bool
	var flagIssues bool
	var flagActions bool

	cmd := &cobra.Command{
		Use:               "open [alias]",
		Aliases:           []string{"browse"},
		Short:             "Open GitHub repository or account profile in browser",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			targetURL := ""

			if len(args) == 1 && !flagRepo && !flagPR && !flagIssues && !flagActions {
				alias := strings.TrimSpace(args[0])
				if err := requireInitialized(); err != nil {
					return err
				}
				account, err := deps.Store.GetAccount(alias)
				if err != nil {
					return err
				}
				targetURL = fmt.Sprintf("https://%s/%s", account.Hostname, account.Login)
			} else {
				isRepo, _ := deps.Git.IsRepo(ctx)
				if isRepo {
					if remoteURL, err := deps.Git.GetRemoteURL(ctx, "origin"); err == nil {
						if info, err := remote.Parse(remoteURL); err == nil {
							targetURL = fmt.Sprintf("https://%s/%s/%s", info.Host, info.Owner, info.Repo)
							if flagPR {
								targetURL += "/pulls"
							} else if flagIssues {
								targetURL += "/issues"
							} else if flagActions {
								targetURL += "/actions"
							}
						}
					}
				}

				if targetURL == "" {
					if err := requireInitialized(); err == nil {
						if login, host, err := currentGitHubIdentity(ctx); err == nil && login != "" {
							if host == "" {
								host = "github.com"
							}
							targetURL = fmt.Sprintf("https://%s/%s", host, login)
						}
					}
				}

				if targetURL == "" {
					return fmt.Errorf("not inside a GitHub repository and no active account found; specify an alias: gha open <alias>")
				}
			}

			terminal.Info(deps.Stdout, "Opening %s", terminal.Bold(targetURL))
			if flagNoBrowser {
				return nil
			}
			if err := openBrowser(targetURL); err != nil {
				terminal.Warn(deps.Stdout, "failed to open browser: %v", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagRepo, "repo", false, "open current repository in browser")
	cmd.Flags().BoolVar(&flagPR, "pr", false, "open repository pull requests in browser")
	cmd.Flags().BoolVar(&flagIssues, "issues", false, "open repository issues in browser")
	cmd.Flags().BoolVar(&flagActions, "actions", false, "open repository actions in browser")
	return cmd
}
