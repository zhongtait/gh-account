package cmd

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			file, err := deps.Store.LoadAccounts()
			if err != nil {
				return err
			}
			if len(file.Accounts) == 0 {
				terminal.Info(deps.Stdout, "No accounts configured. Run %s", terminal.Bold("gha add"))
				return nil
			}

			activeLogin, activeHostname, _ := currentGitHubIdentity(commandContext(cmd))

			aliases := make([]string, 0, len(file.Accounts))
			for alias := range file.Accounts {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)

			w := tabwriter.NewWriter(deps.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tLOGIN\tEMAIL\tPROTOCOL\tAUTH\tACTIVE")
			for _, alias := range aliases {
				account := file.Accounts[alias]
				active := ""
				if activeLogin != "" && sameGitHubAccount(activeLogin, activeHostname, account.Login, account.Hostname) {
					active = "*"
				}
				auth := "no token"
				if cred, found, err := deps.Store.GetCredential(account.Hostname, account.Login); err == nil && found && strings.TrimSpace(cred.AccessToken) != "" {
					auth = "logged in"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", alias, account.Login, account.Email, account.Protocol, auth, active)
			}
			return w.Flush()
		},
	}
}
