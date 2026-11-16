package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestAutoUsesDirectoryBindingWhenRemoteIsMissing(t *testing.T) {
	store, g, _, out := commandSetup(t)
	addAlice(t, store)
	workspace := t.TempDir()
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Directories = map[string]config.DirectoryBinding{workspace: {Account: "alice"}}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	originalGetwd := commandGetwd
	t.Cleanup(func() { commandGetwd = originalGetwd })
	commandGetwd = func() (string, error) { return workspace, nil }
	g.remoteErr = errors.New("no origin")

	out.Reset()
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "directory binding") {
		t.Fatalf("binding notice missing: %s", out.String())
	}

	// Without a binding or an alias the missing remote is still an error.
	cfg.Directories = nil
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil || !strings.Contains(err.Error(), "origin remote") {
		t.Fatalf("missing remote error = %v", err)
	}

	// An explicit alias wins even without a remote.
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err == nil {
		t.Fatal("expected error without binding")
	}
	aliasCmd := newAutoCmd()
	if err := aliasCmd.Flags().Set("alias", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := aliasCmd.RunE(aliasCmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAutoUsesDirectoryBindingWhenOwnerHasNoProfile(t *testing.T) {
	store, g, _, out := commandSetup(t)
	addAlice(t, store)
	workspace := t.TempDir()
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Directories = map[string]config.DirectoryBinding{workspace: {Account: "alice"}}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	originalGetwd := commandGetwd
	t.Cleanup(func() { commandGetwd = originalGetwd })
	commandGetwd = func() (string, error) { return workspace, nil }

	// The remote owner has no configured profile, so the binding decides.
	g.remote = "https://github.com/someone-else/repo.git"
	out.Reset()
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "alice") && !strings.Contains(out.String(), "Alice") {
		t.Fatalf("binding account not selected: %s", out.String())
	}

	// A matching remote account still takes precedence over the binding.
	if err := store.UpsertAccount("owner", config.Account{Login: "someone-else", Hostname: "github.com", GitName: "Owner", Email: "owner@example.com", Protocol: "https"}); err != nil {
		t.Fatal(err)
	}
	g.remote = "https://github.com/someone-else/repo.git"
	out.Reset()
	if err := newAutoCmd().RunE(newAutoCmd(), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "someone-else") {
		t.Fatalf("remote owner should win: %s", out.String())
	}
}

func TestAddDetectsProfileEmail(t *testing.T) {
	store, _, gh, out := commandSetup(t)
	gh.login = "added"
	gh.profile = githubProfileFor("added", "Added User", "added@example.com")
	deps.Stdin = strings.NewReader("added-alias\n\n")
	add := newAddCmd()
	if err := add.RunE(add, nil); err != nil {
		t.Fatal(err)
	}
	account, err := store.GetAccount("added-alias")
	if err != nil {
		t.Fatal(err)
	}
	if account.Email != "added@example.com" || account.GitName != "Added User" || account.Login != "added" {
		t.Fatalf("account = %+v", account)
	}
	if !strings.Contains(out.String(), "Detected GitHub email") {
		t.Fatalf("email detection not announced: %s", out.String())
	}
}

func TestAddFallsBackToLoginWhenProfileIsUnavailable(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	gh.profileErr = errors.New("profile unavailable")
	deps.Stdin = strings.NewReader("plain\nPlain\nplain@example.com\n\n")
	add := newAddCmd()
	if err := add.RunE(add, nil); err != nil {
		t.Fatal(err)
	}
	account, err := store.GetAccount("plain")
	if err != nil {
		t.Fatal(err)
	}
	if account.GitName != "Plain" || account.Email != "plain@example.com" {
		t.Fatalf("account = %+v", account)
	}
}

func TestStoreWarningWriterIsUsed(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials:\n  'github.com|broken':\n    hostname: github.com\n    login: broken\n    encrypted_token: v1:not-valid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var warnings bytes.Buffer
	store.Warning = &warnings
	if _, err := store.LoadAuth(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings.String(), "cannot be decrypted") {
		t.Fatalf("warning not written to the injected writer: %q", warnings.String())
	}

	// The warning is emitted only once per credential.
	warnings.Reset()
	if _, err := store.LoadAuth(); err != nil {
		t.Fatal(err)
	}
	if warnings.Len() != 0 {
		t.Fatalf("warning repeated: %q", warnings.String())
	}
}

func addAliceWithProtocol(t *testing.T, store *config.Store, protocol string) {
	t.Helper()
	if err := store.UpsertAccount("alice", config.Account{
		Login:    "alice",
		Hostname: "github.com",
		GitName:  "Alice",
		Email:    "alice@example.com",
		Protocol: protocol,
	}); err != nil {
		t.Fatal(err)
	}
}

func githubProfileFor(login, name, email string) github.UserProfile {
	return github.UserProfile{Login: login, Name: name, Email: email}
}

// Verify that the directory binding helper still resolves nested paths, which
// `gha auto` now depends on.
func TestMatchDirectoryDrivesAutoBinding(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	nested := base + string(os.PathSeparator) + "project"
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Directories = map[string]config.DirectoryBinding{base: {Account: "work"}}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	alias, _, found, err := store.MatchDirectory(nested)
	if err != nil || !found || alias != "work" {
		t.Fatalf("MatchDirectory = %q %v %v", alias, found, err)
	}
}
