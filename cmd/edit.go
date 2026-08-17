package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/terminal"
	"github.com/zhongtait/gh-account/internal/utils"
	"gopkg.in/yaml.v3"
)

var (
	editFile        = openInEditor
	findEditor      = exec.LookPath
	editMkdirTemp   = os.MkdirTemp
	editRemoveAll   = os.RemoveAll
	editMarshal     = yaml.Marshal
	editWriteFile   = os.WriteFile
	editReadFile    = os.ReadFile
	editUnmarshal   = yaml.Unmarshal
	editSaveAccount = func(store *config.Store, alias string, account config.Account) error {
		return store.UpsertAccount(alias, account)
	}
)

func newEditCmd() *cobra.Command {
	var editAll bool

	cmd := &cobra.Command{
		Use:               "edit [alias]",
		Short:             "Edit an account profile or the full accounts file in $EDITOR",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			editor := resolveEditor()
			if editor == "" {
				return fmt.Errorf("no editor found; set $EDITOR or $VISUAL")
			}

			if editAll || len(args) == 0 {
				path := utils.AccountsPath(deps.Store.Dir)
				if err := editFile(editor, path); err != nil {
					return err
				}
				if _, err := deps.Store.LoadAccounts(); err != nil {
					return fmt.Errorf("accounts.yaml is invalid after edit: %w", err)
				}
				terminal.Success(deps.Stdout, "Updated %s", path)
				return nil
			}

			alias := strings.TrimSpace(args[0])
			account, err := deps.Store.GetAccount(alias)
			if err != nil {
				return err
			}

			tmpDir, err := editMkdirTemp("", "gha-edit-*")
			if err != nil {
				return err
			}
			defer editRemoveAll(tmpDir)

			tmpFile := filepath.Join(tmpDir, alias+".yaml")
			data, err := editMarshal(account)
			if err != nil {
				return err
			}
			if err := editWriteFile(tmpFile, data, 0o644); err != nil {
				return err
			}

			if err := editFile(editor, tmpFile); err != nil {
				return err
			}

			edited, err := editReadFile(tmpFile)
			if err != nil {
				return err
			}
			var next config.Account
			if err := editUnmarshal(edited, &next); err != nil {
				return fmt.Errorf("invalid account yaml: %w", err)
			}
			if strings.TrimSpace(next.Login) == "" || strings.TrimSpace(next.GitName) == "" || strings.TrimSpace(next.Email) == "" {
				return fmt.Errorf("login, git_name, and email are required")
			}
			if next.Protocol == "" {
				next.Protocol = "https"
			}

			if err := editSaveAccount(deps.Store, alias, next); err != nil {
				return err
			}
			terminal.Success(deps.Stdout, "Updated account %s", terminal.Bold(alias))
			terminal.Info(deps.Stdout, "login=%s git_name=%s email=%s protocol=%s", next.Login, next.GitName, next.Email, next.Protocol)
			return nil
		},
	}

	cmd.Flags().BoolVar(&editAll, "all", false, "edit the full accounts.yaml file")
	return cmd
}

func resolveEditor() string {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	for _, candidate := range []string{"nvim", "vim", "vi", "nano", "code", "notepad"} {
		if path, err := findEditor(candidate); err == nil {
			return path
		}
	}
	return ""
}

func openInEditor(editor, path string) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("invalid editor: %q", editor)
	}
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor %s: %w", editor, err)
	}
	return nil
}
