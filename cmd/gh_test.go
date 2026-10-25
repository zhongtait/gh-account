package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/ghman"
	"github.com/zhongtait/gh-account/internal/utils"
)

func TestRunSystemGH(t *testing.T) {
	_ = runSystemGH(context.Background(), "gh", strings.NewReader(""), io.Discard, io.Discard, []string{"FOO=BAR"}, "--version")
	_ = runSystemGH(context.Background(), "", strings.NewReader(""), io.Discard, io.Discard, nil, "--version")
}

func TestGHCmdHelpAndEmptyArgs(t *testing.T) {
	commandSetup(t)

	oldLookPath := ghLookPath
	oldExec := ghExec
	oldEnsure := ghEnsure
	defer func() {
		ghLookPath = oldLookPath
		ghExec = oldExec
		ghEnsure = oldEnsure
	}()

	ghLookPath = func(file string) (string, error) {
		return "/usr/bin/gh", nil
	}

	called := false
	var capturedBin string
	var capturedArgs []string
	ghExec = func(ctx context.Context, binary string, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
		called = true
		capturedBin = binary
		capturedArgs = args
		return nil
	}

	ghCmd := newGHCmd()

	// 1. Empty args runs gh directly
	if err := ghCmd.RunE(ghCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !called || len(capturedArgs) != 0 || capturedBin != "/usr/bin/gh" {
		t.Fatalf("expected 0 args, got called=%v args=%v bin=%s", called, capturedArgs, capturedBin)
	}

	// 2. ghEnsure error
	ghEnsure = func(ctx context.Context) (string, error) {
		return "", errors.New("download failed")
	}
	if err := ghCmd.RunE(ghCmd, nil); err == nil || !strings.Contains(err.Error(), "ensure GitHub CLI (gh): download failed") {
		t.Fatalf("expected download failed error, got: %v", err)
	}
	ghEnsure = ensureGH

	// 3. Help flags
	if err := ghCmd.RunE(ghCmd, []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if err := ghCmd.RunE(ghCmd, []string{"-h"}); err != nil {
		t.Fatal(err)
	}
}

func TestGHCmdInstallAndPath(t *testing.T) {
	store, _, _, out := commandSetup(t)

	oldLookPath := ghLookPath
	oldExec := ghExec
	defer func() {
		ghLookPath = oldLookPath
		ghExec = oldExec
	}()

	ghCmd := newGHCmd()

	// 1. Path when system gh available
	ghLookPath = func(file string) (string, error) {
		return "/usr/local/bin/gh", nil
	}
	out.Reset()
	if err := ghCmd.RunE(ghCmd, []string{"path"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/usr/local/bin/gh (system)") {
		t.Fatalf("unexpected path output: %s", out.String())
	}

	// 2. Path when managed gh available
	ghLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	mgr := ghman.NewManager(store.Dir, io.Discard, io.Discard)
	managedPath := mgr.ManagedPath()
	_ = os.MkdirAll(filepath.Dir(managedPath), 0o755)
	_ = os.WriteFile(managedPath, []byte("echo"), 0o755)
	out.Reset()
	if err := ghCmd.RunE(ghCmd, []string{"path"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "(managed)") {
		t.Fatalf("unexpected path output: %s", out.String())
	}

	// 3. Path when neither available
	_ = os.Remove(managedPath)
	if err := ghCmd.RunE(ghCmd, []string{"path"}); err == nil || !strings.Contains(err.Error(), "GitHub CLI (gh) not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}

	// 4. Install command
	oldNewManager := ghNewManager
	defer func() { ghNewManager = oldNewManager }()
	ghNewManager = func(configDir string, stdout, stderr io.Writer) *ghman.Manager {
		m := ghman.NewManager(configDir, stdout, stderr)
		m.BaseURL = "http://127.0.0.1:1" // Will fail download gracefully
		return m
	}
	if err := ghCmd.RunE(ghCmd, []string{"install"}); err == nil {
		t.Fatal("expected install network error")
	}
}

func TestGHCmdAccountAndExecution(t *testing.T) {
	store, _, gh, _ := commandSetup(t)

	oldLookPath := ghLookPath
	oldExec := ghExec
	oldEnsure := ghEnsure
	defer func() {
		ghLookPath = oldLookPath
		ghExec = oldExec
		ghEnsure = oldEnsure
	}()

	ghLookPath = func(file string) (string, error) {
		return "/usr/bin/gh", nil
	}

	var capturedEnv []string
	var capturedArgs []string
	ghExec = func(ctx context.Context, binary string, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
		capturedEnv = extraEnv
		capturedArgs = args
		return nil
	}

	ghCmd := newGHCmd()

	// 1. Extract account flag error
	if err := ghCmd.RunE(ghCmd, []string{"--account"}); err == nil || !strings.Contains(err.Error(), "--account requires an argument") {
		t.Fatalf("expected missing arg error, got: %v", err)
	}

	// 2. gh not found with args
	ghEnsure = func(ctx context.Context) (string, error) {
		return "", errors.New("not found")
	}
	if err := ghCmd.RunE(ghCmd, []string{"pr", "list"}); err == nil || !strings.Contains(err.Error(), "ensure GitHub CLI (gh): not found") {
		t.Fatalf("expected not found error, got: %v", err)
	}
	ghEnsure = ensureGH

	// 3. Uninitialized store with account flag
	emptyStore := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = emptyStore
	if err := ghCmd.RunE(ghCmd, []string{"--account", "work", "pr", "list"}); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	// Uninitialized store without account flag passes through without extraEnv
	capturedEnv = nil
	if err := ghCmd.RunE(ghCmd, []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedEnv) != 0 {
		t.Fatalf("expected no extra env for uninitialized store, got: %v", capturedEnv)
	}
	deps.Store = oldStore

	// 4. Initialized store, account not found
	if err := ghCmd.RunE(ghCmd, []string{"--account", "nonexistent", "pr", "list"}); err == nil || !strings.Contains(err.Error(), "account \"nonexistent\" not found") {
		t.Fatalf("expected account not found error, got: %v", err)
	}

	// 5. Account exists but has no token
	if err := store.UpsertAccount("work", config.Account{
		Login:    "workuser",
		Hostname: "github.com",
		GitName:  "Work User",
		Email:    "work@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ghCmd.RunE(ghCmd, []string{"--account", "work", "pr", "list"}); err == nil || !strings.Contains(err.Error(), "has no OAuth token") {
		t.Fatalf("expected no token error, got: %v", err)
	}

	// 6. Account exists and has valid token
	if err := store.SaveAuth(config.AuthFile{
		Active: "github.com:workuser",
		Credentials: map[string]config.Credential{
			"github.com:workuser": {
				Hostname:    "github.com",
				Login:       "workuser",
				AccessToken: "gho_work_token_123",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	capturedEnv = nil
	if err := ghCmd.RunE(ghCmd, []string{"--account", "work", "pr", "list"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedEnv) != 3 || capturedEnv[0] != "GH_TOKEN=gho_work_token_123" {
		t.Fatalf("expected injected env, got: %v", capturedEnv)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "pr" || capturedArgs[1] != "list" {
		t.Fatalf("expected pr list args, got: %v", capturedArgs)
	}

	// 7. Active account auto-injected without --account flag
	gh.login = "workuser"
	gh.host = "github.com"
	capturedEnv = nil
	if err := ghCmd.RunE(ghCmd, []string{"repo", "view"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedEnv) != 3 || capturedEnv[0] != "GH_TOKEN=gho_work_token_123" {
		t.Fatalf("expected injected env for active account, got: %v", capturedEnv)
	}

	// 8. No active account runs gh without extraEnv
	gh.login = ""
	capturedEnv = nil
	if err := ghCmd.RunE(ghCmd, []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if len(capturedEnv) != 0 {
		t.Fatalf("expected no extra env when no active account, got: %v", capturedEnv)
	}

	// 9. Store error on GetCredential
	authPath := utils.AuthPath(store.Dir)
	_ = os.WriteFile(authPath, []byte("invalid: [\n"), 0o600)
	if err := ghCmd.RunE(ghCmd, []string{"--account", "work", "status"}); err == nil || !strings.Contains(err.Error(), "parse auth.yaml") {
		t.Fatalf("expected parse auth.yaml error, got: %v", err)
	}
}
