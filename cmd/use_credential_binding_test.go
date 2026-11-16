package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
)

func TestUseSSHProfileRefreshesCredentialBinding(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	if err := store.UpsertAccount("sshacc", config.Account{
		Login:    "sshacc",
		Hostname: "github.com",
		GitName:  "SSH User",
		Email:    "ssh@example.com",
		Protocol: "ssh",
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		scope  git.Scope
		remote string
	}{
		{"existing HTTPS remote", git.ScopeLocal, "https://github.com/old/repo.git"},
		{"existing SSH remote", git.ScopeLocal, "git@github.com:old/repo.git"},
		{"global binding", git.ScopeGlobal, "https://github.com/old/repo.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			gitDir := filepath.Join(root, ".git")
			if err := os.MkdirAll(gitDir, 0o755); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(gitDir, "config")
			if err := os.WriteFile(configPath, []byte("[remote \"origin\"]\nurl = "+tc.remote+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.scope == git.ScopeGlobal {
				configPath = filepath.Join(root, "global.gitconfig")
				t.Setenv("GIT_CONFIG_GLOBAL", configPath)
			}
			if err := store.SaveConfig(config.ConfigFile{DefaultScope: string(tc.scope)}); err != nil {
				t.Fatal(err)
			}
			client := git.NewNativeClient(root)
			deps.Git = client
			if err := client.SetCredentialHelper(context.Background(), tc.scope, "old-helper", "github.com|old"); err != nil {
				t.Fatal(err)
			}
			if err := runCommand(t, newUseCmd(), "sshacc"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "old-helper") || strings.Contains(string(data), "github.com|old") || !strings.Contains(string(data), "account-key = github.com|sshacc") || !strings.Contains(string(data), "credential-helper --account-key 'github.com|sshacc'") {
				t.Fatalf("credential binding was not switched:\n%s", data)
			}
			if got, err := client.GetRemoteURL(context.Background(), "origin"); err != nil || got != tc.remote {
				t.Fatalf("remote unexpectedly changed: %q, %v", got, err)
			}
		})
	}
}
