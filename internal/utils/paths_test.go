package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExpandHome("~/project")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "project")
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if got, err := ExpandHome("~"); err != nil || got != home {
		t.Fatalf("ExpandHome(~) = %q, %v", got, err)
	}
	for _, path := range []string{"relative/path", ""} {
		got, err := ExpandHome(path)
		if err != nil || got != path {
			t.Fatalf("ExpandHome(%q) = %q, %v", path, got, err)
		}
	}
	if got, err := ExpandHome(`~\project`); err != nil || got != filepath.Join(home, "project") {
		t.Fatalf("ExpandHome(~\\project) = %q, %v", got, err)
	}
}

func TestHomeDirectoryErrorsAndWindowsConfigDir(t *testing.T) {
	oldHome, oldOS := userHomeDir, runtimeGOOS
	defer func() { userHomeDir, runtimeGOOS = oldHome, oldOS }()
	userHomeDir = func() (string, error) { return "", os.ErrPermission }
	if _, err := ExpandHome("~/project"); err == nil {
		t.Fatal("ExpandHome ignored a home directory error")
	}
	if _, err := ConfigDir(); err == nil {
		t.Fatal("ConfigDir ignored a home directory error")
	}
	userHomeDir = func() (string, error) { return "/home/test", nil }
	runtimeGOOS = "windows"
	t.Setenv("APPDATA", "C:\\Users\\test\\AppData\\Roaming")
	if got, err := ConfigDir(); err != nil || got != filepath.Join(os.Getenv("APPDATA"), "gha") {
		t.Fatalf("Windows ConfigDir = %q, %v", got, err)
	}
	t.Setenv("APPDATA", "")
	if got, err := ConfigDir(); err != nil || got != filepath.Join("/home/test", ".config", "gha") {
		t.Fatalf("Windows fallback ConfigDir = %q, %v", got, err)
	}
}

func TestConfigDirOverride(t *testing.T) {
	originalOS := runtimeGOOS
	t.Cleanup(func() { runtimeGOOS = originalOS })
	runtimeGOOS = "linux"
	t.Setenv("GH_GHA_CONFIG_DIR", "/tmp/gha-test")
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/gha-test" {
		t.Fatalf("got %s", got)
	}
	t.Setenv("GH_GHA_CONFIG_DIR", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err = ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "gha"); got != want {
		t.Fatalf("default ConfigDir = %q, want %q", got, want)
	}
}

func TestConfigPaths(t *testing.T) {
	dir := filepath.Join("tmp", "gha")
	if got := AccountsPath(dir); got != filepath.Join(dir, "accounts.yaml") {
		t.Fatalf("AccountsPath = %q", got)
	}
	if got := ConfigPath(dir); got != filepath.Join(dir, "config.yaml") {
		t.Fatalf("ConfigPath = %q", got)
	}
	if got := AuthPath(dir); got != filepath.Join(dir, "auth.yaml") {
		t.Fatalf("AuthPath = %q", got)
	}
	if got := AuthKeyPath(dir); got != filepath.Join(dir, "auth.key") {
		t.Fatalf("AuthKeyPath = %q", got)
	}
}
