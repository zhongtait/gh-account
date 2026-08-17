package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newAddCmd() *cobra.Command {
	var (
		alias    string
		hostname string
		gitName  string
		email    string
		protocol string
		manual   bool
	)

	cmd := &cobra.Command{
		Use:   "add [flags]",
		Short: "Add a GitHub account profile (logs in via browser or manual)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.EnsureInitialized(); err != nil {
				return err
			}
			reader := bufio.NewReader(deps.Stdin)

			loginName := ""
			ctx := commandContext(cmd)
			if !manual {
				if err := ensureOAuthClientID(reader); err != nil {
					return err
				}
				if loginErr := deps.GitHub.Login(ctx, hostname, "https"); loginErr != nil {
					return fmt.Errorf("login failed: %w", loginErr)
				}
				var getErr error
				loginName, getErr = deps.GitHub.CurrentLogin(ctx)
				if getErr != nil {
					return fmt.Errorf("failed to get login name after auth: %w", getErr)
				}
			}

			var err error
			alias, err = readValue(reader, "Alias", alias)
			if err != nil {
				return err
			}

			if !manual {
				gitName, err = readValue(reader, "Git Name", gitName)
				if err != nil {
					return err
				}
				email, err = readValue(reader, "Email", email)
				if err != nil {
					return err
				}
				protocol, err = readValue(reader, "Protocol [https/ssh]", protocol)
				if err != nil {
					return err
				}
				if protocol == "" {
					protocol = "https"
				}
			} else {
				// manual mode: prompt GitHub Login
				var login string
				login, err = readValue(reader, "GitHub Login", "")
				if err != nil {
					return err
				}
				loginName = login

				gitName, err = readValue(reader, "Git Name", gitName)
				if err != nil {
					return err
				}
				email, err = readValue(reader, "Email", email)
				if err != nil {
					return err
				}
				protocol, err = readValue(reader, "Protocol [https/ssh]", protocol)
				if err != nil {
					return err
				}
				if protocol == "" {
					protocol = "https"
				}
			}

			account := config.Account{
				Login:    loginName,
				Hostname: hostname,
				GitName:  gitName,
				Email:    email,
				Protocol: protocol,
			}
			if err := deps.Store.UpsertAccount(alias, account); err != nil {
				return err
			}

			terminal.Success(deps.Stdout, "Saved account %s", terminal.Bold(alias))
			terminal.Info(deps.Stdout, "login=%s git_name=%s email=%s protocol=%s", loginName, gitName, email, protocol)
			return nil
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "account alias")
	cmd.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub hostname")
	cmd.Flags().StringVar(&gitName, "git-name", "", "git user.name")
	cmd.Flags().StringVar(&email, "email", "", "git user.email")
	cmd.Flags().StringVar(&protocol, "protocol", "", "git protocol (https|ssh)")
	cmd.Flags().BoolVar(&manual, "manual", false, "skip browser login and prompt manually")
	return cmd
}

func readValue(reader *bufio.Reader, label, current string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current), nil
	}
	fmt.Fprintf(deps.Stdout, "%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
