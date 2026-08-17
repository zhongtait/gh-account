package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeGitLock struct {
	chmodErr, writeErr, syncErr, closeErr error
	shortWrite                            bool
}

func (f *fakeGitLock) Chmod(os.FileMode) error { return f.chmodErr }
func (f *fakeGitLock) WriteString(s string) (int, error) {
	if f.shortWrite {
		return len(s) - 1, nil
	}
	return len(s), f.writeErr
}
func (f *fakeGitLock) Sync() error  { return f.syncErr }
func (f *fakeGitLock) Close() error { return f.closeErr }

func restoreGitHooks(t *testing.T) {
	t.Helper()
	abs, stat, read := gitAbs, gitStat, gitReadFile
	home, mkdir, open := gitUserHomeDir, gitMkdirAll, gitOpenFile
	remove, rename := gitRemove, gitRename
	t.Cleanup(func() {
		gitAbs, gitStat, gitReadFile = abs, stat, read
		gitUserHomeDir, gitMkdirAll, gitOpenFile = home, mkdir, open
		gitRemove, gitRename = remove, rename
	})
}

func TestInjectedRepositoryAndPathErrors(t *testing.T) {
	restoreGitHooks(t)
	c := NewNativeClient(t.TempDir())
	gitAbs = func(string) (string, error) { return "", errors.New("abs") }
	if _, _, _, err := c.repo(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatal(err)
	}
	gitAbs = filepath.Abs

	root := t.TempDir()
	marker := filepath.Join(root, ".git")
	if err := os.WriteFile(marker, []byte("gitdir: elsewhere"), 0o600); err != nil {
		t.Fatal(err)
	}
	c = NewNativeClient(root)
	originalRead := gitReadFile
	gitReadFile = func(path string) ([]byte, error) {
		if path == marker {
			return nil, errors.New("read")
		}
		return originalRead(path)
	}
	if _, _, _, err := c.repo(); err == nil || !strings.Contains(err.Error(), "read .git") {
		t.Fatal(err)
	}
	gitReadFile = originalRead

	originalStat := gitStat
	gitStat = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, ".git") {
			return nil, os.ErrPermission
		}
		return originalStat(path)
	}
	if _, _, _, err := c.repo(); err == nil || !strings.Contains(err.Error(), "stat .git") {
		t.Fatal(err)
	}
	gitStat = originalStat

	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	gitUserHomeDir = func() (string, error) { return "", errors.New("home") }
	if _, err := c.configPath(ScopeGlobal); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatal(err)
	}

	gitReadFile = func(string) ([]byte, error) { return nil, os.ErrPermission }
	if _, err := commonGitDir(root); err == nil || !strings.Contains(err.Error(), "commondir") {
		t.Fatal(err)
	}
}

func TestInjectedSaveConfigErrors(t *testing.T) {
	restoreGitHooks(t)
	c := NewNativeClient(t.TempDir())
	path := filepath.Join(t.TempDir(), "config")
	t.Setenv("GIT_CONFIG_GLOBAL", path)
	file := gitConfig{lines: []string{"[user]", "name = Alice"}}

	originalMkdir := gitMkdirAll
	gitMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), "create config directory") {
		t.Fatal(err)
	}
	gitMkdirAll = originalMkdir

	originalStat := gitStat
	gitStat = func(name string) (os.FileInfo, error) {
		if name == path {
			return nil, os.ErrPermission
		}
		return originalStat(name)
	}
	if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), "stat git config") {
		t.Fatal(err)
	}
	gitStat = originalStat

	originalRead := gitReadFile
	gitReadFile = func(name string) ([]byte, error) {
		if name == path {
			return nil, os.ErrPermission
		}
		return originalRead(name)
	}
	if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), "read git config") {
		t.Fatal(err)
	}
	gitReadFile = originalRead

	originalOpen := gitOpenFile
	gitOpenFile = func(string, int, os.FileMode) (gitLockFile, error) { return nil, errors.New("open") }
	if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), "create git config lock") {
		t.Fatal(err)
	}
	gitOpenFile = originalOpen

	cases := []struct {
		lock *fakeGitLock
		want string
	}{
		{&fakeGitLock{chmodErr: errors.New("chmod")}, "permissions"},
		{&fakeGitLock{writeErr: errors.New("write")}, "write git config"},
		{&fakeGitLock{shortWrite: true}, "short write"},
		{&fakeGitLock{syncErr: errors.New("sync")}, "sync git config"},
		{&fakeGitLock{closeErr: errors.New("close")}, "close git config"},
	}
	for _, tc := range cases {
		gitOpenFile = func(string, int, os.FileMode) (gitLockFile, error) { return tc.lock, nil }
		if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
	gitOpenFile = func(string, int, os.FileMode) (gitLockFile, error) { return &fakeGitLock{}, nil }
	gitRename = func(string, string) error { return errors.New("rename") }
	if err := c.saveConfig(ScopeGlobal, file); err == nil || !strings.Contains(err.Error(), "replace git config") {
		t.Fatal(err)
	}
}
