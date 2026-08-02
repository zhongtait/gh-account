package cmd

import (
	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <alias>",
		Short:             "Remove a configured account",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}
			alias := args[0]
			if err := deps.Store.RemoveAccount(alias); err != nil {
				return err
			}
			terminal.Success(deps.Stdout, "Removed account %s", alias)
			return nil
		},
	}
}
