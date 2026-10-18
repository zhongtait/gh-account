package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestUseCmdInteractive(t *testing.T) {
	store, _, gh, _ := commandSetup(t)

	// 1. Uninitialized store error
	uninitStore := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = uninitStore
	useCmd := newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	deps.Store = oldStore

	// 2. Corrupt accounts.yaml returns error
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err == nil {
		t.Fatal("expected corrupt accounts error")
	}

	// 3. No accounts configured
	if err := store.SaveAccounts(config.DefaultAccounts()); err != nil {
		t.Fatal(err)
	}
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err != nil {
		t.Fatalf("expected success with no accounts, got: %v", err)
	}

	// Setup accounts: alice (active) and bob
	if err := store.UpsertAccount("alice", config.Account{
		Login:    "alice",
		Hostname: "github.com",
		GitName:  "Alice",
		Email:    "alice@example.com",
		Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertAccount("bob", config.Account{
		Login:    "bob",
		Hostname: "github.com",
		GitName:  "Bob",
		Email:    "bob@example.com",
		Protocol: "https",
	}); err != nil {
		t.Fatal(err)
	}
	gh.login = "alice"
	gh.host = "github.com"

	// 4. User inputs EOF / empty string (cancelled)
	deps.Stdin = strings.NewReader("")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err != nil {
		t.Fatalf("expected nil on EOF cancel, got: %v", err)
	}

	deps.Stdin = strings.NewReader("   \n")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err != nil {
		t.Fatalf("expected nil on empty line cancel, got: %v", err)
	}

	// 5. User selects by number: "1" (alice)
	deps.Stdin = strings.NewReader("1\n")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err != nil {
		t.Fatalf("expected switch by number 1 to succeed, got: %v", err)
	}

	// 6. User selects by number out of range: "99"
	deps.Stdin = strings.NewReader("99\n")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err == nil || !strings.Contains(err.Error(), "number out of range") {
		t.Fatalf("expected number out of range error, got: %v", err)
	}

	// 7. User selects by alias directly: "bob"
	deps.Stdin = strings.NewReader("bob\n")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err != nil {
		t.Fatalf("expected switch by alias name to succeed, got: %v", err)
	}

	// 8. User enters invalid selection: "nonexistent"
	deps.Stdin = strings.NewReader("nonexistent\n")
	useCmd = newUseCmd()
	if err := useCmd.RunE(useCmd, nil); err == nil || !strings.Contains(err.Error(), "account not found") {
		t.Fatalf("expected account not found error, got: %v", err)
	}
}
