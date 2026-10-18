package cmd

import (
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
)

func TestStatusCmd(t *testing.T) {
	_, _, gh, out := commandSetup(t)

	// 1. Uninitialized error
	emptyStore := config.NewStore(t.TempDir())
	oldStore := deps.Store
	deps.Store = emptyStore
	cmd := newStatusCmd()
	if err := cmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "config not initialized") {
		t.Fatalf("expected uninitialized error, got: %v", err)
	}
	deps.Store = oldStore

	// 2. Status error from GitHub client
	oldLogin := gh.login
	gh.login = ""
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected status error")
	}

	// 3. Status success
	gh.login = oldLogin
	out.Reset()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "logged in to github.com as alice") {
		t.Fatalf("unexpected status output: %q", out.String())
	}
}
