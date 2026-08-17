package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func TestAuthStoreIsPrivateAndRoundTrips(t *testing.T) {
	store := NewStore(t.TempDir())
	auth := DefaultAuth()
	auth.Active = "github.com|personal-user"
	auth.Credentials[auth.Active] = Credential{
		Hostname: "github.com", Login: "personal-user", AccessToken: "secret-token",
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatalf("SaveAuth: %v", err)
	}

	// Windows does not expose Unix owner/group/other permission bits through
	// FileMode, so Mode().Perm() cannot verify 0600 there.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(store.Dir, "auth.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("auth.yaml permissions = %o, want 600", info.Mode().Perm())
		}
		keyInfo, err := os.Stat(filepath.Join(store.Dir, "auth.key"))
		if err != nil {
			t.Fatal(err)
		}
		if keyInfo.Mode().Perm() != 0o600 {
			t.Fatalf("auth.key permissions = %o, want 600", keyInfo.Mode().Perm())
		}
	}
	data, err := os.ReadFile(filepath.Join(store.Dir, "auth.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-token") || !strings.Contains(string(data), "encrypted_token:") {
		t.Fatal("token was not encrypted in auth.yaml")
	}
	if err := os.Chmod(filepath.Join(store.Dir, "auth.yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatalf("SaveAuth after permission change: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(store.Dir, "auth.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("auth.yaml permissions after rewrite = %o, want 600", info.Mode().Perm())
		}
	}
	loaded, err := store.LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if loaded.Active != auth.Active || loaded.Credentials[auth.Active].AccessToken != "secret-token" {
		t.Fatalf("unexpected auth: %+v", loaded)
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

func TestUpsertAccountRejectsInvalidValues(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("bad", Account{
		Login: "bad", GitName: "Bad", Email: "bad.example.com", Protocol: "ftp",
	}); err == nil {
		t.Fatal("UpsertAccount accepted an invalid email/protocol")
	}
}

func TestLoadAccountsRejectsInvalidValues(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "accounts.yaml"), []byte("accounts:\n  bad:\n    login: bad\n    git_name: Bad\n    email: bad.example.com\n    protocol: ftp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAccounts(); err == nil {
		t.Fatal("LoadAccounts accepted invalid account data")
	}
}

func TestUpdateConfigAndAuthAreTransactional(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateConfig(func(cfg *ConfigFile) error {
		cfg.OAuthClientID = "client-id"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuthClientID != "client-id" {
		t.Fatalf("OAuthClientID = %q", cfg.OAuthClientID)
	}

	if err := store.UpdateAuth(func(auth *AuthFile) error {
		auth.Active = "github.com|alice"
		auth.Credentials[auth.Active] = Credential{
			Hostname: "github.com", Login: "alice", AccessToken: "token",
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	auth, err := store.LoadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.Active != "github.com|alice" || auth.Credentials[auth.Active].AccessToken != "token" {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}

func TestConcurrentAccountUpdatesPreserveAllEntries(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	const count = 8
	var waitGroup sync.WaitGroup
	waitGroup.Add(count)
	for index := 0; index < count; index++ {
		index := index
		go func() {
			defer waitGroup.Done()
			worker := NewStore(store.Dir)
			alias := "account-" + string(rune('a'+index))
			if err := worker.UpsertAccount(alias, Account{
				Login: alias, GitName: alias, Email: alias + "@example.com", Protocol: "https",
			}); err != nil {
				t.Errorf("UpsertAccount(%s): %v", alias, err)
			}
		}()
	}
	waitGroup.Wait()
	file, err := store.LoadAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Accounts) != count {
		t.Fatalf("account count = %d, want %d", len(file.Accounts), count)
	}
}
