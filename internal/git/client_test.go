package git

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	responses map[string]string
	calls     []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if out, ok := f.responses[key]; ok {
		return out, "", nil
	}
	// fuzzy match ignoring -C <dir>
	for k, out := range f.responses {
		if strings.Contains(key, strings.TrimPrefix(k, "git ")) || strings.HasSuffix(key, strings.TrimPrefix(k, "git ")) {
			return out, "", nil
		}
	}
	return "", "", nil
}

func TestSetAndGetIdentityUsesScope(t *testing.T) {
	runner := &fakeRunner{responses: map[string]string{}}
	client := NewCLIClient(runner, "/tmp/repo")

	if err := client.SetIdentity(context.Background(), ScopeLocal, Identity{Name: "Tu Xiao", Email: "a@b.com"}); err != nil {
		t.Fatalf("SetIdentity: %v", err)
	}

	joined := strings.Join(runner.calls, "\n")
	if !strings.Contains(joined, "config --local user.name Tu Xiao") {
		t.Fatalf("missing name call: %s", joined)
	}
	if !strings.Contains(joined, "config --local user.email a@b.com") {
		t.Fatalf("missing email call: %s", joined)
	}
}

func TestParseScope(t *testing.T) {
	scope, err := ParseScope("global", ScopeLocal)
	if err != nil || scope != ScopeGlobal {
		t.Fatalf("expected global, got %s err=%v", scope, err)
	}
}
