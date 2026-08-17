package cmd

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/utils"
)

func TestSetupDependenciesInjectedErrors(t *testing.T) {
	originalConfig, originalExpand, originalGetwd, originalStat := commandConfigDir, commandExpandHome, commandGetwd, commandStat
	t.Cleanup(func() {
		commandConfigDir, commandExpandHome, commandGetwd, commandStat = originalConfig, originalExpand, originalGetwd, originalStat
	})
	cmd := NewRootCommand()
	flagConfigDir = ""
	commandConfigDir = func() (string, error) { return "", errors.New("config") }
	if err := setupDeps(cmd); err == nil {
		t.Fatal("config dir error ignored")
	}
	commandConfigDir = originalConfig
	flagConfigDir = "~/config"
	commandExpandHome = func(string) (string, error) { return "", errors.New("expand") }
	if err := setupDeps(cmd); err == nil {
		t.Fatal("expand error ignored")
	}
	commandExpandHome = originalExpand
	flagConfigDir = t.TempDir()
	commandGetwd = func() (string, error) { return "", errors.New("cwd") }
	if err := setupDeps(cmd); err == nil {
		t.Fatal("cwd error ignored")
	}
	commandGetwd = originalGetwd

	store, _, _, _ := commandSetup(t)
	commandStat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	if err := requireInitialized(); err == nil {
		t.Fatal("stat error ignored")
	}
	commandStat = originalStat

	// Missing dependencies cause completion to initialize itself; an empty
	// config then reports the list error directive.
	deps = Dependencies{}
	flagConfigDir = t.TempDir()
	_, directive := completeAccountAliases(NewRootCommand(), nil, "")
	if directive == 0 {
		t.Fatal("completion did not report missing accounts")
	}
	deps = Dependencies{Store: store}
}

func TestEnsureOAuthClientIDPersistenceFailure(t *testing.T) {
	store, _, gh, _ := commandSetup(t)
	gh.clientID = ""
	if err := os.WriteFile(utils.ConfigPath(store.Dir), []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureOAuthClientID(bufio.NewReader(strings.NewReader("client\n"))); err == nil || !strings.Contains(err.Error(), "save GitHub OAuth") {
		t.Fatal(err)
	}
	deps.GitHub = loginOnlyGitHub{}
	if err := ensureOAuthClientID(bufio.NewReader(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
}

func TestEditInjectedFileErrorsAndEditorSearch(t *testing.T) {
	store, _, _, _ := commandSetup(t)
	addAlice(t, store)
	originalFile, originalFind := editFile, findEditor
	originalMkdir, originalRemove := editMkdirTemp, editRemoveAll
	originalMarshal, originalWrite, originalRead, originalUnmarshal := editMarshal, editWriteFile, editReadFile, editUnmarshal
	t.Cleanup(func() {
		editFile, findEditor = originalFile, originalFind
		editMkdirTemp, editRemoveAll = originalMkdir, originalRemove
		editMarshal, editWriteFile, editReadFile, editUnmarshal = originalMarshal, originalWrite, originalRead, originalUnmarshal
	})
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	findEditor = func(name string) (string, error) {
		if name == "vim" {
			return "/bin/vim", nil
		}
		return "", errors.New("missing")
	}
	if got := resolveEditor(); got != "/bin/vim" {
		t.Fatal(got)
	}
	findEditor = func(string) (string, error) { return "", errors.New("missing") }
	if got := resolveEditor(); got != "" {
		t.Fatal(got)
	}
	t.Setenv("EDITOR", "true")

	editMkdirTemp = func(string, string) (string, error) { return "", errors.New("mkdir") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("mkdir error ignored")
	}
	editMkdirTemp = originalMkdir
	editMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("marshal error ignored")
	}
	editMarshal = originalMarshal
	editWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("write error ignored")
	}
	editWriteFile = originalWrite
	editFile = func(string, string) error { return nil }
	editReadFile = func(string) ([]byte, error) { return nil, errors.New("read") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("read error ignored")
	}
	editReadFile = originalRead
	editUnmarshal = func([]byte, any) error { return errors.New("unmarshal") }
	if err := newEditCmd().RunE(newEditCmd(), []string{"alice"}); err == nil {
		t.Fatal("unmarshal error ignored")
	}
	editUnmarshal = originalUnmarshal

	// Full-file editing validates the file after the editor exits.
	editFile = func(_ string, path string) error { return os.WriteFile(path, []byte("accounts: ["), 0o600) }
	all := newEditCmd()
	_ = all.Flags().Set("all", "true")
	if err := all.RunE(all, nil); err == nil || !strings.Contains(err.Error(), "invalid after edit") {
		t.Fatal(err)
	}
}
