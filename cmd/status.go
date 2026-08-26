package cmd

import (
	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active GitHub OAuth account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}
			status, err := deps.GitHub.Status(commandContext(cmd))
			if err != nil {
				return err
			}
			terminal.Info(deps.Stdout, "%s", status)
			return nil
		},
	}
}
