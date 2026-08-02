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
		login    string
		gitName  string
		email    string
		protocol string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a GitHub account profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.EnsureInitialized(); err != nil {
				return err
			}

			reader := bufio.NewReader(deps.Stdin)

			var err error
			alias, err = readValue(reader, "Alias", alias)
			if err != nil {
				return err
			}
			login, err = readValue(reader, "GitHub Login", login)
			if err != nil {
				return err
			}
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

			account := config.Account{
				Login:    login,
				GitName:  gitName,
				Email:    email,
				Protocol: protocol,
			}
			if err := deps.Store.UpsertAccount(alias, account); err != nil {
				return err
			}

			terminal.Success(deps.Stdout, "Saved account %s", terminal.Bold(alias))
			terminal.Info(deps.Stdout, "login=%s git_name=%s email=%s protocol=%s", account.Login, account.GitName, account.Email, account.Protocol)
			return nil
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "account alias")
	cmd.Flags().StringVar(&login, "login", "", "GitHub login")
	cmd.Flags().StringVar(&gitName, "git-name", "", "git user.name")
	cmd.Flags().StringVar(&email, "email", "", "git user.email")
	cmd.Flags().StringVar(&protocol, "protocol", "https", "git protocol (https|ssh)")
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
