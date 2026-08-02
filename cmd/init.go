package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
	"github.com/zhongtait/gh-account/internal/utils"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize gha configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.EnsureInitialized(); err != nil {
				return err
			}
			terminal.Success(deps.Stdout, "Initialized config directory: %s", deps.Store.Dir)
			terminal.Info(deps.Stdout, "Accounts file: %s", utils.AccountsPath(deps.Store.Dir))
			terminal.Info(deps.Stdout, "Config file: %s", utils.ConfigPath(deps.Store.Dir))
			fmt.Fprintln(deps.Stdout)
			terminal.Info(deps.Stdout, "Next: add an account with %s", terminal.Bold("gha add"))
			return nil
		},
	}
}
