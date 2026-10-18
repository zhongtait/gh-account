package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
)

var (
	ghLookPath = exec.LookPath
	ghExec     = runSystemGH
)

func runSystemGH(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
	command := exec.CommandContext(ctx, "gh", args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if len(extraEnv) > 0 {
		command.Env = append(os.Environ(), extraEnv...)
	}
	return command.Run()
}

func newGHCmd() *cobra.Command {
	var flagAccount string

	cmd := &cobra.Command{
		Use:                "gh [flags] <command> [args...]",
		Short:              "Run GitHub CLI (gh) commands with account integration",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if _, err := ghLookPath("gh"); err != nil {
					return errors.New("GitHub CLI (gh) not found in PATH; install it from https://cli.github.com")
				}
				return ghExec(commandContext(cmd), deps.Stdin, deps.Stdout, deps.Stderr, nil)
			}
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}

			accountAlias, ghArgs, err := extractAccountFlag(args)
			if err != nil {
				return err
			}

			if _, err := ghLookPath("gh"); err != nil {
				return errors.New("GitHub CLI (gh) not found in PATH; install it from https://cli.github.com")
			}

			var extraEnv []string
			if err := requireInitialized(); err == nil {
				accountsFile, err := deps.Store.LoadAccounts()
				if err == nil {
					var targetAccount config.Account
					var foundAccount bool

					if accountAlias != "" {
						acc, exists := accountsFile.Accounts[accountAlias]
						if !exists {
							return fmt.Errorf("account %q not found", accountAlias)
						}
						targetAccount = acc
						foundAccount = true
					} else {
						activeLogin, activeHostname, _ := currentGitHubIdentity(commandContext(cmd))
						if activeLogin != "" {
							for _, acc := range accountsFile.Accounts {
								if sameGitHubAccount(activeLogin, activeHostname, acc.Login, acc.Hostname) {
									targetAccount = acc
									foundAccount = true
									break
								}
							}
						}
					}

					if foundAccount {
						cred, found, err := deps.Store.GetCredential(targetAccount.Hostname, targetAccount.Login)
						if err != nil {
							return err
						}
						if found && strings.TrimSpace(cred.AccessToken) != "" {
							extraEnv = []string{
								"GH_TOKEN=" + cred.AccessToken,
								"GITHUB_TOKEN=" + cred.AccessToken,
								"GH_HOST=" + targetAccount.Hostname,
							}
						} else if accountAlias != "" {
							return fmt.Errorf("account %q (%s) has no OAuth token; run gha login %s first", accountAlias, targetAccount.Login, accountAlias)
						}
					}
				}
			} else if accountAlias != "" {
				return err
			}

			return ghExec(commandContext(cmd), deps.Stdin, deps.Stdout, deps.Stderr, extraEnv, ghArgs...)
		},
	}

	cmd.Flags().StringVarP(&flagAccount, "account", "a", "", "specify the account alias to use for this gh command")
	return cmd
}
