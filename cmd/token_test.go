package cmd

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestTokenCmdErrorsAndOutputs(t *testing.T) {
	store, _, gh, out := commandSetup(t)

	oldCopy := clipboardCopy
	defer func() { clipboardCopy = oldCopy }()

	tokenCmd := newTokenCmd()

	// 1. Not initialized error
	emptyStore := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = emptyStore
	if err := tokenCmd.RunE(tokenCmd, nil); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	deps.Store = oldStore

	// 2. Account not found with explicit alias
	if err := tokenCmd.RunE(tokenCmd, []string{"nonexistent"}); err == nil || !strings.Contains(err.Error(), "account \"nonexistent\" not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}

	// 3. No active account
	gh.login = ""
	if err := store.UpsertAccount("alice", config.Account{
		Login:    "alice",
		Hostname: "github.com",
		GitName:  "Alice",
		Email:    "alice@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tokenCmd.RunE(tokenCmd, nil); err == nil || !strings.Contains(err.Error(), "no active account found") {
		t.Fatalf("expected no active account error, got: %v", err)
	}

	// 4. Account found but no credential
	if err := tokenCmd.RunE(tokenCmd, []string{"alice"}); err == nil || !strings.Contains(err.Error(), "has no OAuth token") {
		t.Fatalf("expected no OAuth token error, got: %v", err)
	}

	// 5. Valid token output for explicit alias
	if err := store.SaveAuth(config.AuthFile{
		Active: "github.com:alice",
		Credentials: map[string]config.Credential{
			"github.com:alice": {
				Hostname:    "github.com",
				Login:       "alice",
				AccessToken: "gho_secret_alice_token",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := tokenCmd.RunE(tokenCmd, []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gho_secret_alice_token") {
		t.Fatalf("expected token in output, got: %q", out.String())
	}

	// 6. Valid token output for active account (no args)
	gh.login = "alice"
	out.Reset()
	if err := tokenCmd.RunE(tokenCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gho_secret_alice_token") {
		t.Fatalf("expected token in output for active account, got: %q", out.String())
	}

	// 7. Clipboard copy success
	var copiedText string
	clipboardCopy = func(text string) error {
		copiedText = text
		return nil
	}
	out.Reset()
	copyCmd := newTokenCmd()
	copyCmd.Flags().Set("copy", "true")
	if err := copyCmd.RunE(copyCmd, []string{"alice"}); err != nil {
		t.Fatal(err)
	}
	if copiedText != "gho_secret_alice_token" {
		t.Fatalf("expected copied token %q, got %q", "gho_secret_alice_token", copiedText)
	}
	if !strings.Contains(out.String(), "Token copied to clipboard") {
		t.Fatalf("expected success message, got: %q", out.String())
	}

	// 8. Clipboard copy error
	clipboardCopy = func(text string) error {
		return errors.New("clipboard failure")
	}
	if err := copyCmd.RunE(copyCmd, []string{"alice"}); err == nil || !strings.Contains(err.Error(), "clipboard failure") {
		t.Fatalf("expected clipboard failure error, got: %v", err)
	}
}

func TestTokenCmdStoreFailures(t *testing.T) {
	store, _, _, _ := commandSetup(t)

	// Corrupt accounts.yaml to test LoadAccounts error
	accountsPath := utils.AccountsPath(store.Dir)
	_ = os.WriteFile(accountsPath, []byte("invalid: [\n"), 0o600)

	tokenCmd := newTokenCmd()
	if err := tokenCmd.RunE(tokenCmd, []string{"alice"}); err == nil {
		t.Fatal("expected LoadAccounts error")
	}

	// Restore valid accounts in fresh store, corrupt auth.yaml to test GetCredential error
	store2, _, _, _ := commandSetup(t)
	if err := store2.UpsertAccount("bob", config.Account{
		Login:    "bob",
		Hostname: "github.com",
		GitName:  "Bob",
		Email:    "bob@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	authPath := utils.AuthPath(store2.Dir)
	_ = os.WriteFile(authPath, []byte("invalid: [\n"), 0o600)

	if err := tokenCmd.RunE(tokenCmd, []string{"bob"}); err == nil {
		t.Fatal("expected GetCredential error")
	}
}
