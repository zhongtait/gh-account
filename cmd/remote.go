package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newRemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remote",
		Short: "Show origin remote details for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			isRepo, err := deps.Git.IsRepo(ctx)
			if err != nil {
				return err
			}
			if !isRepo {
				return fmt.Errorf("not inside a git repository")
			}

			url, err := deps.Git.GetRemoteURL(ctx, "origin")
			if err != nil {
				return fmt.Errorf("origin remote not found")
			}

			info, err := remote.Parse(url)
			if err != nil {
				terminal.Info(deps.Stdout, "URL: %s", url)
				return nil
			}

			fmt.Fprintln(deps.Stdout, terminal.Bold("Remote"))
			terminal.Info(deps.Stdout, "Name     : origin")
			terminal.Info(deps.Stdout, "Owner    : %s", info.Owner)
			terminal.Info(deps.Stdout, "Repo     : %s", info.Repo)
			terminal.Info(deps.Stdout, "Protocol : %s", info.Protocol)
			terminal.Info(deps.Stdout, "Host     : %s", info.Host)
			terminal.Info(deps.Stdout, "URL      : %s", info.Raw)
			return nil
		},
	}
}
