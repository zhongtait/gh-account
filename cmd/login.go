package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newLoginCmd() *cobra.Command {
	var (
		hostname string
		protocol string
		gitName  string
		email    string
	)

	cmd := &cobra.Command{
		Use:               "login [alias]",
		Short:             "Log in with GitHub CLI and save the account profile",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.EnsureInitialized(); err != nil {
				return err
			}

			alias := ""
			if len(args) == 1 {
				alias = strings.TrimSpace(args[0])
			}

			existing := config.Account{}
			hasExisting := false
			if alias != "" {
				if acc, err := deps.Store.GetAccount(alias); err == nil {
					existing = acc
					hasExisting = true
					if protocol == "" {
						protocol = acc.Protocol
					}
					if gitName == "" {
						gitName = acc.GitName
					}
					if email == "" {
						email = acc.Email
					}
				}
			}
			if protocol == "" {
				protocol = "https"
			}

			ctx := commandContext(cmd)
			if err := deps.GitHub.Login(ctx, hostname, protocol); err != nil {
				return fmt.Errorf("gh auth login failed: %w", err)
			}

			loginName, err := deps.GitHub.CurrentLogin(ctx)
			if err != nil {
				return fmt.Errorf("login succeeded but failed to read active GitHub user: %w", err)
			}
			if strings.TrimSpace(loginName) == "" {
				return fmt.Errorf("login succeeded but active GitHub user is empty")
			}

			reader := bufio.NewReader(deps.Stdin)
			if alias == "" {
				alias, err = readValue(reader, "Alias", suggestAlias(loginName))
				if err != nil {
					return err
				}
			}
			if strings.TrimSpace(alias) == "" {
				return fmt.Errorf("alias is required")
			}

			if gitName == "" {
				defaultName := existing.GitName
				if defaultName == "" {
					defaultName = loginName
				}
				gitName, err = readValue(reader, "Git Name", defaultName)
				if err != nil {
					return err
				}
			}
			if email == "" {
				email, err = readValue(reader, "Email", existing.Email)
				if err != nil {
					return err
				}
			}
			if strings.TrimSpace(gitName) == "" {
				return fmt.Errorf("git name is required")
			}
			if strings.TrimSpace(email) == "" {
				return fmt.Errorf("email is required")
			}

			account := config.Account{
				Login:    loginName,
				GitName:  gitName,
				Email:    email,
				Protocol: protocol,
			}
			if err := deps.Store.UpsertAccount(alias, account); err != nil {
				return err
			}

			if hasExisting {
				terminal.Success(deps.Stdout, "Logged in and updated account %s", terminal.Bold(alias))
			} else {
				terminal.Success(deps.Stdout, "Logged in and saved account %s", terminal.Bold(alias))
			}
			terminal.Info(deps.Stdout, "login=%s git_name=%s email=%s protocol=%s", account.Login, account.GitName, account.Email, account.Protocol)
			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub hostname")
	cmd.Flags().StringVar(&protocol, "protocol", "", "git protocol to prefer during login (https|ssh)")
	cmd.Flags().StringVar(&gitName, "git-name", "", "git user.name to store after login")
	cmd.Flags().StringVar(&email, "email", "", "git user.email to store after login")
	return cmd
}

func suggestAlias(login string) string {
	login = strings.TrimSpace(login)
	if login == "" {
		return ""
	}
	return strings.ToLower(login)
}
