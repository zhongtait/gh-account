package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetIdentityResolvesIncludes(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	included := filepath.Join(root, "identity.gitconfig")
	if err := os.WriteFile(included, []byte("[user]\n\tname = Included User\n\temail = included@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	config := "[include]\n\tpath = " + filepath.ToSlash(included) + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	identity, err := NewNativeClient(root).GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Included User" || identity.Email != "included@example.com" {
		t.Fatalf("identity = %+v", identity)
	}

	// Keys in the including file must win over included values.
	config += "\n[user]\n\tname = Local Wins\n"
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, err = NewNativeClient(root).GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Local Wins" || identity.Email != "included@example.com" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestGetIdentityResolvesRelativeAndHomeIncludes(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A relative include resolves against the including file's directory.
	if err := os.WriteFile(filepath.Join(gitDir, "extra.gitconfig"), []byte("[user]\n\tname = Relative\n\temail = relative@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[include]\n\tpath = extra.gitconfig\n[include]\n\tpath = missing.gitconfig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, err := NewNativeClient(root).GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Relative" || identity.Email != "relative@example.com" {
		t.Fatalf("identity = %+v", identity)
	}

	home := t.TempDir()
	originalHome := gitUserHomeDir
	t.Cleanup(func() { gitUserHomeDir = originalHome })
	gitUserHomeDir = func() (string, error) { return home, nil }
	if err := os.WriteFile(filepath.Join(home, "home.gitconfig"), []byte("[user]\n\tname = Home\n\temail = home@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[include]\n\tpath = ~/home.gitconfig\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, err = NewNativeClient(root).GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Home" || identity.Email != "home@example.com" {
		t.Fatalf("home include identity = %+v", identity)
	}
}

func TestGetIdentityStopsRecursiveIncludes(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	if err := os.WriteFile(configPath, []byte("[include]\n\tpath = config\n[user]\n\tname = Loop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	identity, err := NewNativeClient(root).GetIdentity(context.Background(), ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Loop" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestGetIdentityToleratesUnreadableInclude(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("[include]\n\tpath = "+filepath.Join(t.TempDir(), "gone")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := readGitConfigResolved(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if value := file.get("user.name"); value != "" {
		t.Fatalf("user.name = %q", value)
	}
}

// SetIdentity must make the identity win over an [include] that follows an
// earlier user section. Git resolves user.name to its LAST value, so writing
// in place before the include would be silently overridden.
func TestSetIdentityWinsOverIncludedIdentity(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	included := filepath.Join(root, "shared.gitconfig")
	if err := os.WriteFile(included, []byte("[user]\n\tname = Shared Old\n\temail = shared@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	base := "[user]\n\tname = Parent Old\n\temail = parent@example.com\n[include]\n\tpath = " + filepath.ToSlash(included) + "\n"
	if err := os.WriteFile(configPath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewNativeClient(root)
	ctx := context.Background()
	if err := client.SetIdentity(ctx, ScopeLocal, Identity{Name: "New Name", Email: "new@example.com"}); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetIdentity(ctx, ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New Name" || got.Email != "new@example.com" {
		t.Fatalf("included identity still wins: %+v", got)
	}

	// Repeated writes must stay stable instead of appending sections forever.
	for i := 0; i < 3; i++ {
		if err := client.SetIdentity(ctx, ScopeLocal, Identity{Name: "New Name", Email: "new@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "[user]"); count != 2 {
		t.Fatalf("expected exactly 2 [user] sections after rewrites, got %d:\n%s", count, data)
	}

	// When the winning key already sits after the include, it is edited in
	// place rather than appended.
	tail := "[include]\n\tpath = " + filepath.ToSlash(included) + "\n[user]\n\tname = Tail Old\n\temail = tail@example.com\n"
	if err := os.WriteFile(configPath, []byte(tail), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.SetIdentity(ctx, ScopeLocal, Identity{Name: "Tail New", Email: "tailnew@example.com"}); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetIdentity(ctx, ScopeLocal)
	if err != nil || got.Name != "Tail New" || got.Email != "tailnew@example.com" {
		t.Fatalf("identity = %+v, %v", got, err)
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "[user]"); count != 1 {
		t.Fatalf("in-place update appended a section:\n%s", data)
	}
}

// GetRemoteURL must match `git remote get-url`, which resolves the FIRST url.
func TestRemoteURLUsesFirstValue(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	cfg := "[remote \"origin\"]\n\turl = https://github.com/FIRST/repo.git\n\turl = https://github.com/SECOND/repo.git\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewNativeClient(root)
	got, err := client.GetRemoteURL(context.Background(), "origin")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/FIRST/repo.git" {
		t.Fatalf("GetRemoteURL = %q, want the first value", got)
	}
	if err := client.SetRemoteURL(context.Background(), "origin", "https://github.com/REWRITTEN/repo.git"); err != nil {
		t.Fatal(err)
	}
	got, err = client.GetRemoteURL(context.Background(), "origin")
	if err != nil || got != "https://github.com/REWRITTEN/repo.git" {
		t.Fatalf("SetRemoteURL left the first value unchanged: %q, %v", got, err)
	}
}

// SetRemoteURL must make the rewritten URL the first value Git resolves, even
// when origin is defined only in an included file.
func TestSetRemoteURLWinsOverIncludedRemote(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	included := filepath.Join(root, "remotes.gitconfig")
	if err := os.WriteFile(included, []byte("[remote \"origin\"]\n\turl = https://github.com/INCLUDED/repo.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	base := "[core]\n\trepositoryformatversion = 0\n[include]\n\tpath = " + filepath.ToSlash(included) + "\n"
	if err := os.WriteFile(configPath, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewNativeClient(root)
	ctx := context.Background()
	if err := client.SetRemoteURL(ctx, "origin", "https://github.com/REWRITTEN/repo.git"); err != nil {
		t.Fatal(err)
	}
	got, err := client.GetRemoteURL(ctx, "origin")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/REWRITTEN/repo.git" {
		t.Fatalf("GetRemoteURL = %q; the included value still wins", got)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "[remote"); count != 1 {
		t.Fatalf("expected one remote section, got %d:\n%s", count, data)
	}
}

func TestConditionalIncludesUseGitAndStayOverridden(t *testing.T) {
	root := t.TempDir()
	gitBinary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git executable is required to verify conditional include behavior")
	}
	init := exec.Command(gitBinary, "init", "-q", root)
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	gitDir := filepath.Join(root, ".git")
	included := filepath.Join(root, "conditional.gitconfig")
	if err := os.WriteFile(included, []byte("[user]\n\tname = Conditional User\n\temail = conditional@example.com\n[remote \"origin\"]\n\turl = https://github.com/CONDITIONAL/repo.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(gitDir, "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	gitDirSlash := filepath.ToSlash(gitDir)
	includedSlash := filepath.ToSlash(included)
	conditional := fmt.Sprintf("\n[user]\n\tname = Parent User\n\temail = parent@example.com\n[includeIf \"gitdir:%s\"]\n\tpath = %s\n", gitDirSlash, includedSlash)
	if err := os.WriteFile(configPath, append(data, []byte(conditional)...), 0o644); err != nil {
		t.Fatal(err)
	}

	client := NewNativeClient(root)
	ctx := context.Background()
	identity, err := client.GetIdentity(ctx, ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Conditional User" || identity.Email != "conditional@example.com" {
		t.Fatalf("identity = %+v; conditional include was not resolved", identity)
	}
	remoteURL, err := client.GetRemoteURL(ctx, "origin")
	if err != nil || remoteURL != "https://github.com/CONDITIONAL/repo.git" {
		t.Fatalf("conditional remote = %q, %v", remoteURL, err)
	}

	if err := client.SetIdentity(ctx, ScopeLocal, Identity{Name: "Selected User", Email: "selected@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := client.SetRemoteURL(ctx, "origin", "https://github.com/SELECTED/repo.git"); err != nil {
		t.Fatal(err)
	}
	identity, err = client.GetIdentity(ctx, ScopeLocal)
	if err != nil || identity.Name != "Selected User" || identity.Email != "selected@example.com" {
		t.Fatalf("identity after write = %+v, %v", identity, err)
	}
	remoteURL, err = client.GetRemoteURL(ctx, "origin")
	if err != nil || remoteURL != "https://github.com/SELECTED/repo.git" {
		t.Fatalf("remote after write = %q, %v", remoteURL, err)
	}
}
