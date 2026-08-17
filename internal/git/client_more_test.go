package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeClientErrorsOutsideRepository(t *testing.T) {
	client := NewNativeClient(t.TempDir())
	ctx := context.Background()
	if _, err := client.TopLevel(ctx); err == nil {
		t.Fatal("TopLevel succeeded outside a repository")
	}
	if _, err := client.GetIdentity(ctx, ScopeLocal); err == nil {
		t.Fatal("GetIdentity succeeded outside a repository")
	}
	if err := client.SetIdentity(ctx, ScopeLocal, Identity{Name: "A", Email: "a@example.com"}); err == nil {
		t.Fatal("SetIdentity succeeded outside a repository")
	}
	if err := client.SetCredentialHelper(ctx, ScopeLocal, "helper", "key"); err == nil {
		t.Fatal("SetCredentialHelper succeeded outside a repository")
	}
	if _, err := client.GetRemoteURL(ctx, "origin"); err == nil {
		t.Fatal("GetRemoteURL succeeded outside a repository")
	}
	if err := client.SetRemoteURL(ctx, "origin", "https://github.com/a/b.git"); err == nil {
		t.Fatal("SetRemoteURL succeeded outside a repository")
	}
	if _, err := client.CurrentBranch(ctx); err == nil {
		t.Fatal("CurrentBranch succeeded outside a repository")
	}
}

func TestNativeClientDefaultsAndRepositoryErrorWrappers(t *testing.T) {
	client := NewNativeClient("")
	if client.Dir == "" {
		t.Fatal("NewNativeClient did not default its directory")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	client = NewNativeClient(root)
	if _, err := client.TopLevel(context.Background()); err == nil {
		t.Fatal("TopLevel accepted an invalid .git marker")
	}
	if _, err := client.GetIdentity(context.Background(), ScopeLocal); err == nil {
		t.Fatal("GetIdentity accepted an invalid .git marker")
	}
	if err := client.SetIdentity(context.Background(), ScopeLocal, Identity{}); err == nil {
		t.Fatal("SetIdentity accepted an invalid .git marker")
	}
	if err := client.SetCredentialHelper(context.Background(), ScopeLocal, "helper", "key"); err == nil {
		t.Fatal("SetCredentialHelper accepted an invalid .git marker")
	}
	if _, err := client.GetRemoteURL(context.Background(), ""); err == nil {
		t.Fatal("GetRemoteURL accepted an invalid .git marker")
	}
	if err := client.SetRemoteURL(context.Background(), " ", "https://github.com/a/b.git"); err == nil {
		t.Fatal("SetRemoteURL accepted a blank remote name")
	}
	if _, err := client.CurrentBranch(context.Background()); err == nil {
		t.Fatal("CurrentBranch accepted an invalid .git marker")
	}
	filePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewNativeClient(filePath).IsRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeClientValidationAndConfigPaths(t *testing.T) {
	client := NewNativeClient(t.TempDir())
	ctx := context.Background()
	for _, test := range []struct {
		command func() error
		name    string
	}{
		{name: "credential command", command: func() error { return client.SetCredentialHelper(ctx, ScopeGlobal, "", "key") }},
		{name: "credential key", command: func() error { return client.SetCredentialHelper(ctx, ScopeGlobal, "helper", "") }},
		{name: "remote name", command: func() error { return client.SetRemoteURL(ctx, "", "") }},
		{name: "remote url", command: func() error { return client.SetRemoteURL(ctx, "origin", "") }},
	} {
		if err := test.command(); err == nil {
			t.Errorf("%s accepted invalid input", test.name)
		}
	}
	if _, err := client.configPath(Scope("bad")); err == nil {
		t.Fatal("configPath accepted an invalid scope")
	}
	if _, err := client.configPath(ScopeLocal); err == nil {
		t.Fatal("local configPath succeeded outside a repository")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global"))
	if path, err := client.configPath(ScopeGlobal); err != nil || path != os.Getenv("GIT_CONFIG_GLOBAL") {
		t.Fatalf("global configPath = %q, %v", path, err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	if path, err := client.configPath(ScopeGlobal); err != nil || path != filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "git", "config") {
		t.Fatalf("XDG global configPath = %q, %v", path, err)
	}
	if scope, err := ParseScope("", ScopeGlobal); err != nil || scope != ScopeGlobal {
		t.Fatalf("ParseScope fallback = %q, %v", scope, err)
	}
	if scope, err := ParseScope("", ""); err != nil || scope != ScopeLocal {
		t.Fatalf("ParseScope empty fallback = %q, %v", scope, err)
	}
	if _, err := ParseScope("invalid", ScopeLocal); err == nil {
		t.Fatal("ParseScope accepted invalid scope")
	}
}

func TestGitConfigParserEdgeCases(t *testing.T) {
	config := gitConfig{lines: []string{
		"# comment", "; comment", "[user]", "\tname = \"Alice\\\" Smith\"", "email a@example.com",
		"[remote \"origin\"]", "url = https://github.com/a/b.git", "unknown line",
	}}
	if got := config.get("user.name"); got != `Alice" Smith` {
		t.Fatalf("quoted user.name = %q", got)
	}
	if got := config.get("user.email"); got != "a@example.com" {
		t.Fatalf("user.email = %q", got)
	}
	if got := unquoteGitValue(`"a\\b"`); got != `a\b` {
		t.Fatalf("unquoteGitValue = %q", got)
	}
	if got := unquoteGitValue("plain"); got != "plain" {
		t.Fatalf("unquoteGitValue plain = %q", got)
	}
	if got := formatSection("remote.origin"); got != `remote "origin"` {
		t.Fatalf("formatSection subsection = %q", got)
	}
	if got := formatSection("user"); got != "user" {
		t.Fatalf("formatSection section = %q", got)
	}
	config.setMulti("credential.helper", []string{"", "helper"})
	config.setMulti("credential.helper", []string{"new"})
	if got := config.get("credential.helper"); got != "new" {
		t.Fatalf("setMulti result = %q", got)
	}
	config.set("user.name", "Alice")
	if got := config.get("user.name"); got != "Alice" {
		t.Fatalf("set result = %q", got)
	}
}

func TestGitConfigFileErrorsAndDetachedHead(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("deadbeef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := NewNativeClient(root)
	if branch, err := client.CurrentBranch(context.Background()); err != nil || branch != "" {
		t.Fatalf("detached branch = %q, %v", branch, err)
	}
	if _, err := readGitConfig(filepath.Join(root, "missing")); err != nil {
		t.Fatal(err)
	}
	if _, err := readGitConfig(gitDir); err == nil {
		t.Fatal("readGitConfig accepted a directory")
	}
	longLine := strings.Repeat("x", 70*1024)
	longPath := filepath.Join(root, "long-config")
	if err := os.WriteFile(longPath, []byte(longLine), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readGitConfig(longPath); err == nil {
		t.Fatal("readGitConfig accepted an overlong line")
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config.lock"), []byte("busy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.SetIdentity(context.Background(), ScopeLocal, Identity{Name: "A", Email: "a@example.com"}); err == nil {
		t.Fatal("SetIdentity ignored an existing config lock")
	}
	if err := client.SetCredentialHelper(context.Background(), ScopeLocal, "helper", "key"); err == nil {
		t.Fatal("SetCredentialHelper ignored an existing config lock")
	}
	if err := client.SetRemoteURL(context.Background(), "origin", "https://github.com/a/b.git"); err == nil {
		t.Fatal("SetRemoteURL ignored an existing config lock")
	}
}

func TestCurrentBranchReadErrorAndCommonDirErrors(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	client := NewNativeClient(root)
	if _, err := client.CurrentBranch(context.Background()); err == nil {
		t.Fatal("CurrentBranch succeeded without HEAD")
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := commonGitDir(gitDir); err == nil {
		t.Fatal("commonGitDir accepted an empty commondir")
	}
	if _, err := client.GetIdentity(context.Background(), ScopeLocal); err == nil {
		t.Fatal("GetIdentity accepted an empty commondir")
	}
}

func TestGitConfigMissingRemoteAndGlobalErrors(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[user]\nname = A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewNativeClient(root).GetRemoteURL(context.Background(), ""); err == nil {
		t.Fatal("GetRemoteURL succeeded without a remote")
	}
	client := NewNativeClient(root)
	t.Setenv("GIT_CONFIG_GLOBAL", gitDir)
	if _, err := client.GetIdentity(context.Background(), ScopeGlobal); err == nil {
		t.Fatal("GetIdentity read a directory as global config")
	}
	if err := client.saveConfig(Scope("bad"), gitConfig{}); err == nil {
		t.Fatal("saveConfig accepted invalid scope")
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(parentFile, "config"))
	if err := client.saveConfig(ScopeGlobal, gitConfig{}); err == nil {
		t.Fatal("saveConfig created a directory below a file")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if path, err := client.configPath(ScopeGlobal); err != nil || !strings.HasSuffix(path, ".gitconfig") {
		t.Fatalf("default global config path = %q, %v", path, err)
	}
}

func TestGitRepositoryMarkerErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("not a gitdir file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewNativeClient(root).IsRepo(context.Background()); err == nil {
		t.Fatal("invalid .git file was accepted")
	}
	if err := os.Remove(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("GITDIR: relative\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isRepo, err := NewNativeClient(root).IsRepo(context.Background())
	if err != nil || !isRepo {
		t.Fatalf("relative gitdir = %v, %v", isRepo, err)
	}
}
