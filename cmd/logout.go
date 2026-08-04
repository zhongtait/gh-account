package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newLogoutCmd() *cobra.Command {
	var hostname string

	cmd := &cobra.Command{
		Use:               "logout [alias]",
		Short:             "Log out a GitHub account via GitHub CLI",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)

			loginName := ""
			alias := ""
			if len(args) == 1 {
				alias = strings.TrimSpace(args[0])
				if err := requireInitialized(); err != nil {
					return err
				}
				account, err := deps.Store.GetAccount(alias)
				if err != nil {
					return err
				}
				loginName = account.Login
			}

			if err := deps.GitHub.Logout(ctx, loginName, hostname); err != nil {
				if loginName != "" {
					return fmt.Errorf("failed to logout %s: %w", loginName, err)
				}
				return fmt.Errorf("failed to logout: %w", err)
			}

			if alias != "" {
				terminal.Success(deps.Stdout, "Logged out GitHub account for %s (%s)", alias, loginName)
			} else if loginName != "" {
				terminal.Success(deps.Stdout, "Logged out GitHub account %s", loginName)
			} else {
				terminal.Success(deps.Stdout, "Logged out GitHub account")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub hostname")
	return cmd
}
