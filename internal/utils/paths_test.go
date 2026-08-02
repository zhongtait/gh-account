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
}

func TestConfigDirOverride(t *testing.T) {
	t.Setenv("GH_GHA_CONFIG_DIR", "/tmp/gha-test")
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/gha-test" {
		t.Fatalf("got %s", got)
	}
}
