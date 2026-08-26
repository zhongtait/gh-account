package cmd

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
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

func TestCredentialHelperCommandResolvesPathLookup(t *testing.T) {
	originalLookPath, originalExecutable := credentialLookPath, credentialExecutable
	t.Cleanup(func() { credentialLookPath, credentialExecutable = originalLookPath, originalExecutable })

	store := config.NewStore(t.TempDir())

	// Prefer the binary that is currently running, even if PATH contains a
	// different gha executable.
	credentialLookPath = func(string) (string, error) { return "C:/bin/gha.exe", nil }
	credentialExecutable = func() (string, error) { return "C:/current/gha.exe", nil }
	if got := credentialHelperCommand(store, config.Account{Hostname: "github.com", Login: "u"}); !strings.Contains(got, "!'C:/current/gha.exe' ") {
		t.Fatalf("expected current executable, got %q", got)
	}

	// If the executable path is unavailable, fall back to PATH lookup.
	credentialExecutable = func() (string, error) { return "", errors.New("not found") }
	credentialLookPath = func(string) (string, error) { return "gha.exe", nil }
	if got := credentialHelperCommand(store, config.Account{Hostname: "github.com", Login: "u"}); strings.Contains(got, "!'gha.exe' ") {
		t.Fatalf("relative gha path was stored: %q", got)
	}

	// gha missing from PATH: fall back to the running binary's path.
	credentialLookPath = func(string) (string, error) { return "", errors.New("not found") }
	credentialExecutable = func() (string, error) { return filepath.Join("D", "tools", "gha.exe"), nil }
	got := credentialHelperCommand(store, config.Account{Hostname: "github.com", Login: "u"})
	if !strings.Contains(got, "!'"+filepath.ToSlash(filepath.Join("D", "tools", "gha.exe"))+"' ") {
		t.Fatalf("expected fallback to executable path, got %q", got)
	}
}
func TestCredentialHelperCommandUsesConfiguredStore(t *testing.T) {
	storeDir := filepath.Join(t.TempDir(), "gha config")
	store := config.NewStore(storeDir)
	command := credentialHelperCommand(store, config.Account{Hostname: "github.com", Login: "personal-user"})
	absoluteDir, err := filepath.Abs(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	expectedConfigDir := "--config-dir " + shellQuote(filepath.ToSlash(absoluteDir))
	if !strings.Contains(command, expectedConfigDir) || !strings.Contains(command, "--account-key 'github.com|personal-user'") {
		t.Fatalf("unexpected helper command: %s", command)
	}
}
