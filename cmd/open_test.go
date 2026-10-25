package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
)

func TestOpenCmdWithAlias(t *testing.T) {
	store, _, _, out := commandSetup(t)
	oldOpen := openBrowser
	defer func() { openBrowser = oldOpen }()

	var openedURL string
	openBrowser = func(url string) error {
		openedURL = url
		return nil
	}

	// 1. Uninitialized error
	uninitStore := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = uninitStore
	cmd := newOpenCmd()
	if err := cmd.RunE(cmd, []string{"alice"}); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	deps.Store = oldStore

	// 2. Account not found
	cmd = newOpenCmd()
	if err := cmd.RunE(cmd, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "account \"missing\" not found") {
		t.Fatalf("expected missing account error, got: %v", err)
	}

	// 3. Account with empty hostname defaults to github.com
	if err := store.UpsertAccount("nohost", config.Account{
		Login:    "octocat",
		GitName:  "Octocat",
		Email:    "octo@example.com",
		Hostname: "",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"nohost"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/octocat" {
		t.Fatalf("expected https://github.com/octocat, got %s", openedURL)
	}

	// 4. Account with custom hostname
	if err := store.UpsertAccount("ghe", config.Account{
		Login:    "enterprise_user",
		GitName:  "Enterprise",
		Email:    "user@ghe.corp",
		Hostname: "github.corp.com",
	}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{"ghe"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.corp.com/enterprise_user" {
		t.Fatalf("expected https://github.corp.com/enterprise_user, got %s", openedURL)
	}

	// 5. flagNoBrowser skips openBrowser
	openedURL = ""
	flagNoBrowser = true
	if err := cmd.RunE(cmd, []string{"ghe"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "" {
		t.Fatal("openBrowser was called despite flagNoBrowser")
	}
	flagNoBrowser = false

	// 6. openBrowser error prints warning but returns nil
	openBrowser = func(url string) error {
		return errors.New("cannot open browser")
	}
	out.Reset()
	if err := cmd.RunE(cmd, []string{"ghe"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "failed to open browser") {
		t.Fatalf("expected browser warning in output, got: %s", out.String())
	}
}

func TestOpenCmdRepoAndActiveAccount(t *testing.T) {
	store, g, gh, _ := commandSetup(t)
	oldOpen := openBrowser
	defer func() { openBrowser = oldOpen }()

	var openedURL string
	openBrowser = func(url string) error {
		openedURL = url
		return nil
	}

	cmd := newOpenCmd()

	// 1. Inside repo with origin remote
	g.isRepo = true
	g.remote = "https://github.com/owner/myrepo.git"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/owner/myrepo" {
		t.Fatalf("expected https://github.com/owner/myrepo, got %s", openedURL)
	}

	// 2. Inside repo with --repo flag
	openedURL = ""
	_ = cmd.Flags().Set("repo", "true")
	if err := cmd.RunE(cmd, []string{"ignored"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/owner/myrepo" {
		t.Fatalf("expected https://github.com/owner/myrepo, got %s", openedURL)
	}
	_ = cmd.Flags().Set("repo", "false")

	// 2a. Inside repo with --pr flag
	openedURL = ""
	_ = cmd.Flags().Set("pr", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/owner/myrepo/pulls" {
		t.Fatalf("expected https://github.com/owner/myrepo/pulls, got %s", openedURL)
	}
	_ = cmd.Flags().Set("pr", "false")

	// 2b. Inside repo with --issues flag
	openedURL = ""
	_ = cmd.Flags().Set("issues", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/owner/myrepo/issues" {
		t.Fatalf("expected https://github.com/owner/myrepo/issues, got %s", openedURL)
	}
	_ = cmd.Flags().Set("issues", "false")

	// 2c. Inside repo with --actions flag
	openedURL = ""
	_ = cmd.Flags().Set("actions", "true")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/owner/myrepo/actions" {
		t.Fatalf("expected https://github.com/owner/myrepo/actions, got %s", openedURL)
	}
	_ = cmd.Flags().Set("actions", "false")

	// 2d. In repo but GetRemoteURL fails or parse fails, fall back to active account
	g.isRepo = true
	g.remoteErr = errors.New("no remote")
	gh.login = "alice"
	gh.host = "github.com"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	g.remoteErr = nil
	g.remote = "invalid-url"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	// 3. Not in a repo, fallback to active account with custom host
	g.isRepo = false
	gh.login = "alice"
	gh.host = "github.enterprise.com"
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.enterprise.com/alice" {
		t.Fatalf("expected https://github.enterprise.com/alice, got %s", openedURL)
	}

	// 4. Not in a repo, fallback to active account with empty host (defaults to github.com)
	gh.host = ""
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/alice" {
		t.Fatalf("expected https://github.com/alice, got %s", openedURL)
	}

	// 5. Not in a repo, no active account
	gh.login = ""
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "not inside a GitHub repository") {
		t.Fatalf("expected error when no repo and no active account, got: %v", err)
	}

	// 6. Completion for alias
	if err := store.UpsertAccount("alice", config.Account{
		Login:    "alice",
		GitName:  "Alice",
		Email:    "alice@example.com",
		Hostname: "github.com",
	}); err != nil {
		t.Fatal(err)
	}
	values, _ := cmd.ValidArgsFunction(cmd, nil, "")
	if len(values) == 0 {
		t.Fatal("expected alias completions")
	}

	// 7. Shorthand repository owner/repo and flags
	openedURL = ""
	cmd = newOpenCmd()
	if err := cmd.RunE(cmd, []string{"cli/cli"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/cli/cli" {
		t.Fatalf("expected https://github.com/cli/cli, got %s", openedURL)
	}

	openedURL = ""
	cmd = newOpenCmd()
	cmd.Flags().Set("issues", "true")
	if err := cmd.RunE(cmd, []string{"cli/cli"}); err != nil {
		t.Fatal(err)
	}
	if openedURL != "https://github.com/cli/cli/issues" {
		t.Fatalf("expected https://github.com/cli/cli/issues, got %s", openedURL)
	}
}
