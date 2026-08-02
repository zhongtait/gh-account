package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreInitLoadSave(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	if err := store.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "accounts.yaml")); err != nil {
		t.Fatalf("accounts.yaml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatalf("config.yaml missing: %v", err)
	}

	account := Account{
		Login:    "personal-user",
		GitName:  "Tu Xiao",
		Email:    "personal@example.com",
		Protocol: "https",
	}
	if err := store.UpsertAccount("personal", account); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}

	got, err := store.GetAccount("personal")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.Login != account.Login || got.Email != account.Email {
		t.Fatalf("unexpected account: %+v", got)
	}

	aliases, err := store.ListAliases()
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(aliases) != 1 || aliases[0] != "personal" {
		t.Fatalf("unexpected aliases: %v", aliases)
	}
}

func TestMatchDirectoryLongestPrefix(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.EnsureInitialized(); err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	personal := filepath.Join(dir, "Code", "Personal")
	nested := filepath.Join(personal, "demo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Directories = map[string]DirectoryBinding{
		filepath.Join(dir, "Code"):          {Account: "fallback"},
		filepath.Join(dir, "Code/Personal"): {Account: "personal"},
	}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	alias, _, found, err := store.MatchDirectory(nested)
	if err != nil {
		t.Fatalf("MatchDirectory: %v", err)
	}
	if !found || alias != "personal" {
		t.Fatalf("expected personal, got found=%v alias=%s", found, alias)
	}
}
