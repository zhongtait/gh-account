package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newAutoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auto [--repo]",
		Short: "Automatically switch account based on directory bindings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			alias, prefix, found, err := deps.Store.MatchDirectory(cwd)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("no directory binding matched %s", cwd)
			}

			terminal.Info(deps.Stdout, "Matched %s -> %s", prefix, alias)
			useCmd := newUseCmd()
			useCmd.SetContext(commandContext(cmd))
			useCmd.SetOut(deps.Stdout)
			useCmd.SetErr(deps.Stderr)
			useCmd.SetArgs([]string{alias})
			// Ensure persistent pre-run style deps remain available.
			return useCmd.RunE(useCmd, []string{alias})
		},
	}
}
