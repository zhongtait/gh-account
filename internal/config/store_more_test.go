package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validAccount(login string) Account {
	return Account{Login: login, GitName: login, Email: login + "@example.com", Protocol: "https"}
}

type yamlErrorValue struct{}

func (yamlErrorValue) MarshalYAML() (any, error) { return nil, errors.New("marshal failed") }

func TestDefaultStoreAndInitializationErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GH_GHA_CONFIG_DIR", dir)
	store, err := DefaultStore()
	if err != nil || store.Dir != dir {
		t.Fatalf("DefaultStore = %+v, %v", store, err)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(filepath.Join(blocked, "child")).EnsureInitialized(); err == nil {
		t.Fatal("EnsureInitialized succeeded below a file")
	}
}

func TestAccountStoreErrorsAndRemoval(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.LoadAccounts(); err == nil {
		t.Fatal("LoadAccounts succeeded before initialization")
	}
	if _, err := store.GetAccount("missing"); err == nil {
		t.Fatal("GetAccount succeeded before initialization")
	}
	if _, err := store.ListAliases(); err == nil {
		t.Fatal("ListAliases succeeded before initialization")
	}
	if err := store.RemoveAccount("missing"); err == nil {
		t.Fatal("RemoveAccount succeeded before initialization")
	}
	if err := store.UpsertAccount("", validAccount("alice")); err == nil {
		t.Fatal("UpsertAccount accepted an empty alias")
	}
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount(" alice ", validAccount("alice")); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveAccount("alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveAccount("alice"); err == nil {
		t.Fatal("RemoveAccount accepted a missing alias")
	}
	if _, err := store.GetAccount("alice"); err == nil {
		t.Fatal("GetAccount returned a removed alias")
	}
}

func TestAccountFileNormalizationAndValidation(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Dir, "accounts.yaml")
	data := "accounts:\n  alice:\n    login: alice\n    hostname: https://ghe.example/\n    git_name: Alice\n    email: alice@example.com\n    protocol: SSH\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := store.LoadAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Accounts["alice"]; got.Protocol != "ssh" || got.Hostname != "ghe.example" {
		t.Fatalf("account not normalized: %+v", got)
	}
	if err := os.WriteFile(path, []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAccounts(); err == nil {
		t.Fatal("malformed accounts YAML was accepted")
	}
	if err := store.SaveAccounts(AccountsFile{}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAccounts(AccountsFile{Accounts: map[string]Account{"": validAccount("alice")}}); err == nil {
		t.Fatal("SaveAccounts accepted an empty alias")
	}
}

func TestConfigStoreDefaultsAndCallbacks(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveConfig(ConfigFile{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultScope != "local" || cfg.Directories == nil {
		t.Fatalf("config defaults not applied: %+v", cfg)
	}
	wantErr := errors.New("stop")
	if err := store.UpdateConfig(func(*ConfigFile) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("UpdateConfig error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "config.yaml"), []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadConfig(); err == nil {
		t.Fatal("malformed config YAML was accepted")
	}
	if err := store.UpdateConfig(func(*ConfigFile) error { return nil }); err == nil {
		t.Fatal("UpdateConfig succeeded with malformed config")
	}
	if _, _, _, err := store.MatchDirectory("."); err == nil {
		t.Fatal("MatchDirectory succeeded with malformed config")
	}
}

func TestCredentialLookupAndAuthErrors(t *testing.T) {
	store := NewStore(t.TempDir())
	if credential, found, err := store.GetCredential("github.com", ""); err != nil || found || credential.Login != "" {
		t.Fatalf("empty lookup = %+v, %v, %v", credential, found, err)
	}
	auth := DefaultAuth()
	auth.Credentials["github.com|alice"] = Credential{Hostname: "github.com", Login: "Alice", AccessToken: "token"}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	credential, found, err := store.GetCredential("HTTPS://GITHUB.COM/", "alice")
	if err != nil || !found || credential.AccessToken != "token" {
		t.Fatalf("credential lookup = %+v, %v, %v", credential, found, err)
	}
	if _, found, err := store.GetCredential("github.com", "missing"); err != nil || found {
		t.Fatalf("missing lookup found=%v err=%v", found, err)
	}

	wantErr := errors.New("stop")
	if err := store.UpdateAuth(func(*AuthFile) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("UpdateAuth error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "auth.yaml"), []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAuth(); err == nil {
		t.Fatal("malformed auth YAML was accepted")
	}
	if err := store.UpdateAuth(func(*AuthFile) error { return nil }); err == nil {
		t.Fatal("UpdateAuth succeeded with malformed auth")
	}
	if _, _, err := store.GetCredential("github.com", "alice"); err == nil {
		t.Fatal("GetCredential succeeded with malformed auth")
	}
}

func TestEncryptedTokenFailures(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.decryptToken("aad", "plain"); err == nil {
		t.Fatal("unencrypted token was accepted")
	}
	if _, err := store.decryptToken("aad", "v1:AA"); err == nil || !strings.Contains(err.Error(), "auth key") {
		t.Fatalf("missing key error = %v", err)
	}
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "auth.key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.authKey(false); err == nil {
		t.Fatal("short auth key was accepted")
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "auth.key"), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, encoded := range []string{"v1:%%%", "v1:AA"} {
		if _, err := store.decryptToken("aad", encoded); err == nil {
			t.Errorf("decryptToken(%q) unexpectedly succeeded", encoded)
		}
	}
	encrypted, err := store.encryptToken("aad", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.decryptToken("different-aad", encrypted); err == nil {
		t.Fatal("token decrypted with different associated data")
	}
}

func TestHelpersAndWriteFailures(t *testing.T) {
	if err := writeYAML(filepath.Join(t.TempDir(), "invalid.yaml"), yamlErrorValue{}); err == nil {
		t.Fatal("writeYAML marshaled an unsupported value")
	}
	store := NewStore(t.TempDir())
	wantErr := errors.New("callback")
	if err := store.withWriteLock(func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("withWriteLock error = %v", err)
	}
	if err := validateAccounts(AccountsFile{Accounts: map[string]Account{"bad": {Login: ""}}}); err == nil {
		t.Fatal("validateAccounts accepted an invalid account")
	}
	for input, want := range map[string]string{"": "https", " HTTPS ": "https", "SSH": "ssh", "ftp": "ftp"} {
		if got := normalizeProtocol(input); got != want {
			t.Errorf("normalizeProtocol(%q) = %q, want %q", input, got, want)
		}
	}
	for input, want := range map[string]string{"": "github.com", " http://ghe.example/ ": "ghe.example"} {
		if got := normalizeHostname(input); got != want {
			t.Errorf("normalizeHostname(%q) = %q, want %q", input, got, want)
		}
	}
}
