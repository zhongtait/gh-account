package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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
	flagClientID     string
	flagNoBrowser    bool
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
		Short:         "Manage multiple GitHub accounts with OAuth and git identity sync",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return setupDeps(cmd)
		},
	}

	root.PersistentFlags().StringVar(&flagConfigDir, "config-dir", "", "config directory (default: ~/.config/gha)")
	root.PersistentFlags().StringVar(&flagClientID, "client-id", "", "GitHub OAuth App client ID (or GH_GHA_CLIENT_ID)")
	root.PersistentFlags().BoolVar(&flagNoBrowser, "no-browser", false, "do not open the OAuth verification page automatically")

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
	root.AddCommand(newCloneCmd())
	root.AddCommand(newCredentialHelperCmd())
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

	store := config.NewStore(dir)
	clientID := strings.TrimSpace(flagClientID)
	if clientID == "" {
		clientID = github.ClientIDFromEnv()
	}
	if clientID == "" {
		if cfg, configErr := store.LoadConfig(); configErr == nil {
			clientID = cfg.OAuthClientID
		}
	}

	oauthClient := github.NewOAuthClient(store, cmd.OutOrStdout(), clientID)
	if flagNoBrowser {
		oauthClient.OpenBrowser = nil
	}

	deps = Dependencies{
		Store:  store,
		Git:    git.NewNativeClient(cwd),
		GitHub: oauthClient,
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

func ensureOAuthClientID(reader *bufio.Reader) error {
	client, ok := deps.GitHub.(github.ClientIDClient)
	if !ok || strings.TrimSpace(client.ConfiguredClientID()) != "" {
		return nil
	}

	fmt.Fprint(deps.Stdout, "GitHub OAuth Client ID: ")
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return err
	}
	clientID := strings.TrimSpace(line)
	if clientID == "" {
		return fmt.Errorf("GitHub OAuth client ID is required")
	}
	client.SetClientID(clientID)

	cfg, err := deps.Store.LoadConfig()
	if err != nil {
		return err
	}
	cfg.OAuthClientID = clientID
	if err := deps.Store.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save GitHub OAuth client ID: %w", err)
	}
	return nil
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
