package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/terminal"
	"github.com/zhongtait/gh-account/internal/utils"
)

// Dependencies are shared services for commands.
type Dependencies struct {
	Store  *config.Store
	Git    git.Client
	GitHub github.Client
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

var (
	// version 由构建脚本通过 ldflags 注入，开发环境默认显示 dev。
	version = "dev"

	deps Dependencies

	flagConfigDir    string
	flagGlobal       bool
	flagUpdateRemote bool
)

// Execute runs the root command.
func Execute() error {
	terminal.DisableColorIfNeeded()
	root := NewRootCommand()
	return root.Execute()
}

// NewRootCommand builds the CLI command tree.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "gha",
		Short:         "Manage multiple GitHub accounts with gh and git identity sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return setupDeps(cmd)
		},
	}

	root.PersistentFlags().StringVar(&flagConfigDir, "config-dir", "", "config directory (default: ~/.config/gha)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newAddCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newUseCmd())
	root.AddCommand(newCurrentCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newEditCmd())
	root.AddCommand(newRemoteCmd())
	root.AddCommand(newAutoCmd())
	root.AddCommand(newCompletionCmd())
	root.AddCommand(newVersionCmd())

	return root
}

func setupDeps(cmd *cobra.Command) error {
	dir := flagConfigDir
	var err error
	if dir == "" {
		dir, err = utils.ConfigDir()
		if err != nil {
			return err
		}
	} else {
		dir, err = utils.ExpandHome(dir)
		if err != nil {
			return err
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	deps = Dependencies{
		Store:  config.NewStore(dir),
		Git:    git.NewCLIClient(utils.RealRunner{}, cwd),
		GitHub: github.NewCLIClient(utils.RealRunner{}),
		Stdout: cmd.OutOrStdout(),
		Stderr: cmd.ErrOrStderr(),
		Stdin:  cmd.InOrStdin(),
	}
	return nil
}

func requireInitialized() error {
	accountsPath := utils.AccountsPath(deps.Store.Dir)
	if _, err := os.Stat(accountsPath); err != nil {
		return fmt.Errorf("config not initialized; run %s first", terminal.Bold("gha init"))
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if cmd != nil && cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "gha %s\n", version)
		},
	}
}
