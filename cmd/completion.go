package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return cmd.Help()
			}
		},
	}
}

func completeAccountAliases(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if deps.Store == nil {
		if err := setupDeps(cmd); err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
	}
	aliases, err := deps.Store.ListAliases()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return aliases, cobra.ShellCompDirectiveNoFileComp
}
