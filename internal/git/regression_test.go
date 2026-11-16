package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func realGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	binary, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required for config integration tests")
	}
	command := exec.Command(binary, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func isolatedGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "global.gitconfig"))
	t.Setenv("GIT_CONFIG_COUNT", "0")
	realGit(t, root, "init", "-q", "-b", "main")
	return root
}

func writeConfigFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRewritesPreserveValuelessBooleans(t *testing.T) {
	root := isolatedGitRepo(t)
	path := filepath.Join(root, ".git", "config")
	initial, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, path, string(initial)+"\n[credential]\nhelper = old-helper\nuseHttpPath\n[commit]\ngpgsign\n[tag]\n# sign tags too\ngpgsign\n")
	client := NewNativeClient(root)
	for i := 0; i < 2; i++ {
		if err := client.SetCredentialHelper(context.Background(), ScopeLocal, "new-helper", "github.com|new"); err != nil {
			t.Fatal(err)
		}
		if err := client.SetRemoteURL(context.Background(), "origin", "https://github.com/new/repo.git"); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"commit.gpgsign", "tag.gpgsign", "credential.useHttpPath"} {
			if got := realGit(t, root, "config", "--local", "--bool", "--get", key); got != "true" {
				t.Fatalf("%s = %q after rewrite, want true", key, got)
			}
		}
	}
}

func TestRemoteRewriteReplacesEffectiveURLList(t *testing.T) {
	for _, mode := range []string{"local", "include", "conditional", "global", "explicit pushurl"} {
		t.Run(mode, func(t *testing.T) {
			root := isolatedGitRepo(t)
			path := filepath.Join(root, ".git", "config")
			initial, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			remoteConfig := "[remote \"origin\"]\nurl = https://github.com/old/repo.git\nurl = https://github.com/other/repo.git\nfetch = +refs/heads/*:refs/remotes/origin/*\n"
			local := string(initial)
			switch mode {
			case "local":
				local += remoteConfig
			case "global":
				writeConfigFixture(t, os.Getenv("GIT_CONFIG_GLOBAL"), remoteConfig)
			default:
				if mode == "explicit pushurl" {
					remoteConfig += "pushurl = https://github.com/publish/repo.git\n"
				}
				writeConfigFixture(t, filepath.Join(root, "remote.inc"), remoteConfig)
				if mode == "conditional" {
					local += "[includeIf \"onbranch:main\"]\npath = ../remote.inc\n"
				} else {
					local += "[include]\npath = ../remote.inc\n"
				}
			}
			writeConfigFixture(t, path, local)
			client := NewNativeClient(root)
			for _, owner := range []string{"new", "next", "next"} {
				url := "https://github.com/" + owner + "/repo.git"
				if err := client.SetRemoteURL(context.Background(), "origin", url); err != nil {
					t.Fatal(err)
				}
				if got, err := client.GetRemoteURL(context.Background(), "origin"); err != nil || got != url {
					t.Fatalf("NativeClient remote = %q, %v", got, err)
				}
				if got := realGit(t, root, "remote", "get-url", "--all", "origin"); got != url {
					t.Fatalf("fetch URLs = %q, want only %q", got, url)
				}
				pushURL := url
				if mode == "explicit pushurl" {
					pushURL = "https://github.com/publish/repo.git"
				}
				if got := realGit(t, root, "remote", "get-url", "--push", "--all", "origin"); got != pushURL {
					t.Fatalf("push URLs = %q, want only %q", got, pushURL)
				}
			}
			if got := realGit(t, root, "config", "--get", "remote.origin.fetch"); got != "+refs/heads/*:refs/remotes/origin/*" {
				t.Fatalf("fetch refspec changed: %q", got)
			}
			if mode == "include" || mode == "conditional" || mode == "explicit pushurl" {
				data, err := os.ReadFile(filepath.Join(root, "remote.inc"))
				if err != nil || string(data) != remoteConfig {
					t.Fatalf("shared include changed: %q, %v", data, err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || strings.Count(string(data), "url =") != 2 {
				t.Fatalf("rewrites accumulated URL entries: %q, %v", data, err)
			}
		})
	}
}

func TestGetRemoteURLHonorsEmptyReset(t *testing.T) {
	root := isolatedGitRepo(t)
	client := NewNativeClient(root)
	for _, tc := range []struct{ config, want string }{
		{"url = https://github.com/old/repo.git\nurl = \"\"\nurl = https://github.com/new/repo.git\nurl = https://github.com/other/repo.git\n", "https://github.com/new/repo.git"},
		{"url = https://github.com/old/repo.git\nurl = \"\"\n", ""},
	} {
		writeConfigFixture(t, filepath.Join(root, ".git", "config"), "[remote \"origin\"]\n"+tc.config)
		got, err := client.GetRemoteURL(context.Background(), "origin")
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("remote = %q, %v; want %q", got, err, tc.want)
		}
	}
}

func TestRemoteRewriteRejectsUnsupportedGitWithoutWriting(t *testing.T) {
	root := isolatedGitRepo(t)
	path := filepath.Join(root, ".git", "config")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	original := gitConfigCommand
	t.Cleanup(func() { gitConfigCommand = original })
	for _, tc := range []struct {
		version string
		err     error
	}{
		{"git version 2.45.4", nil},
		{"unknown", nil},
		{"", errors.New("Git unavailable")},
	} {
		gitConfigCommand = func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.version), tc.err }
		if err := NewNativeClient(root).SetRemoteURL(context.Background(), "origin", "https://github.com/new/repo.git"); err == nil || !strings.Contains(err.Error(), "Git 2.46") {
			t.Fatalf("unsupported Git accepted: %v", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("config changed after rejected rewrite: %q, %v", after, err)
		}
	}
}
