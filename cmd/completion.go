package cmd

import "github.com/spf13/cobra"

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			output := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(output)
			case "zsh":
				return cmd.Root().GenZshCompletion(output)
			case "fish":
				return cmd.Root().GenFishCompletion(output, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(output)
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
