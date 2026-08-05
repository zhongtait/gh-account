package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
)

func TestCredentialHelperReturnsSelectedCredential(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	auth := config.DefaultAuth()
	auth.Credentials["github.com|personal-user"] = config.Credential{
		Hostname: "github.com", Login: "personal-user", AccessToken: "personal-token",
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}

	oldDeps := deps
	defer func() { deps = oldDeps }()
	var output bytes.Buffer
	deps = Dependencies{
		Store:  store,
		Stdin:  strings.NewReader("protocol=https\nhost=github.com\npath=owner/private.git\n\n"),
		Stdout: &output,
		Stderr: io.Discard,
	}

	cmd := newCredentialHelperCmd()
	cmd.SetOut(&output)
	if err := cmd.Flags().Set("account-key", "github.com|personal-user"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"get"}); err != nil {
		t.Fatalf("credential helper: %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "username=personal-user\n") || !strings.Contains(got, "password=personal-token\n") {
		t.Fatalf("unexpected credential response: %q", got)
	}
}

func TestCredentialHelperIgnoresOtherHostsAndOperations(t *testing.T) {
	store := config.NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	oldDeps := deps
	defer func() { deps = oldDeps }()
	var output bytes.Buffer
	deps = Dependencies{
		Store:  store,
		Stdin:  strings.NewReader("protocol=https\nhost=github.com\n\n"),
		Stdout: &output,
		Stderr: io.Discard,
	}

	cmd := newCredentialHelperCmd()
	cmd.SetOut(&output)
	if err := cmd.Flags().Set("account-key", "github.example.com|work-user"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"get"}); err != nil {
		t.Fatalf("credential helper: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected no credential for another host, got %q", output.String())
	}

	if err := cmd.RunE(cmd, []string{"erase"}); err != nil {
		t.Fatalf("erase operation: %v", err)
	}
}

func TestCredentialHelperCommandUsesConfiguredStore(t *testing.T) {
	store := config.NewStore("/tmp/gha config")
	command := credentialHelperCommand(store, config.Account{Hostname: "github.com", Login: "personal-user"})
	if !strings.Contains(command, "--config-dir '/tmp/gha config'") || !strings.Contains(command, "--account-key 'github.com|personal-user'") {
		t.Fatalf("unexpected helper command: %s", command)
	}
}
