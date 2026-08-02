package github

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	out   string
	calls []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.out, "", nil
}

type fakeInteractive struct {
	calls []string
}

func (f *fakeInteractive) RunInteractive(ctx context.Context, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return nil
}

func TestCurrentLoginFromAPI(t *testing.T) {
	client := NewCLIClient(&fakeRunner{out: "personal-user"})
	login, err := client.CurrentLogin(context.Background())
	if err != nil {
		t.Fatalf("CurrentLogin: %v", err)
	}
	if login != "personal-user" {
		t.Fatalf("unexpected login: %s", login)
	}
}

func TestExtractActiveLogin(t *testing.T) {
	status := strings.TrimSpace(`
github.com
  ✓ Logged in to github.com account personal-user (keyring)
  - Active account: true
`)
	if got := extractActiveLogin(status); got != "personal-user" {
		t.Fatalf("expected personal-user, got %s", got)
	}
}

func TestLoginUsesInteractiveRunner(t *testing.T) {
	runner := &fakeRunner{}
	interactive := &fakeInteractive{}
	client := &CLIClient{Runner: runner, InteractiveRunner: interactive}
	if err := client.Login(context.Background(), "github.com", "ssh"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if len(interactive.calls) != 1 {
		t.Fatalf("expected interactive call, got %v", interactive.calls)
	}
	if !strings.Contains(interactive.calls[0], "auth login") {
		t.Fatalf("unexpected call: %s", interactive.calls[0])
	}
	if !strings.Contains(interactive.calls[0], "--git-protocol ssh") {
		t.Fatalf("missing protocol: %s", interactive.calls[0])
	}
}

func TestLogoutPassesUser(t *testing.T) {
	runner := &fakeRunner{}
	client := NewCLIClient(runner)
	if err := client.Logout(context.Background(), "personal-user", "github.com"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("unexpected calls: %v", runner.calls)
	}
	if !strings.Contains(runner.calls[0], "auth logout") || !strings.Contains(runner.calls[0], "--user personal-user") {
		t.Fatalf("unexpected logout call: %s", runner.calls[0])
	}
}
