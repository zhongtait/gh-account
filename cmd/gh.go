package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/ghman"
	"github.com/zhongtait/gh-account/internal/terminal"
)

var (
	ghLookPath   = exec.LookPath
	ghExec       = runSystemGH
	ghEnsure     = ensureGH
	ghNewManager = func(configDir string, stdout, stderr io.Writer) *ghman.Manager {
		return ghman.NewManager(configDir, stdout, stderr)
	}
)

func ghManager() *ghman.Manager {
	configDir := ""
	if deps.Store != nil {
		configDir = deps.Store.Dir
	}
	if configDir == "" {
		if dir, err := commandConfigDir(); err == nil {
			configDir = dir
		}
	}
	m := ghNewManager(configDir, deps.Stdout, deps.Stderr)
	m.LookPath = ghLookPath
	return m
}

func ensureGH(ctx context.Context) (string, error) {
	m := ghManager()
	return m.EnsureGH(ctx)
}

func runSystemGH(ctx context.Context, binary string, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
	if binary == "" {
		binary = "gh"
	}
	command := exec.CommandContext(ctx, binary, args...)
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
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}

			accountAlias, ghArgs, err := extractAccountFlag(args)
			if err != nil {
				return err
			}

			if len(ghArgs) > 0 {
				switch ghArgs[0] {
				case "install":
					m := ghManager()
					installedPath, err := m.Install(commandContext(cmd))
					if err != nil {
						return err
					}
					terminal.Success(deps.Stdout, "GitHub CLI installed to %s", installedPath)
					return nil
				case "path":
					m := ghManager()
					path, isManaged, err := m.FindGH()
					if err != nil {
						return err
					}
					source := "system"
					if isManaged {
						source = "managed"
					}
					fmt.Fprintf(deps.Stdout, "%s (%s)\n", path, source)
					return nil
				}
			}

			ghBin, err := ghEnsure(commandContext(cmd))
			if err != nil {
				return fmt.Errorf("ensure GitHub CLI (gh): %w", err)
			}

			if len(ghArgs) == 0 {
				return ghExec(commandContext(cmd), ghBin, deps.Stdin, deps.Stdout, deps.Stderr, nil)
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

			return ghExec(commandContext(cmd), ghBin, deps.Stdin, deps.Stdout, deps.Stderr, extraEnv, ghArgs...)
		},
	}

	cmd.Flags().StringVarP(&flagAccount, "account", "a", "", "specify the account alias to use for this gh command")
	return cmd
}
