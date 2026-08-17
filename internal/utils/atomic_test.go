package utils

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type fakeAtomicFile struct {
	name       string
	chmodErr   error
	writeErr   error
	syncErr    error
	closeErr   error
	shortWrite bool
	writeCount int
}

func (f *fakeAtomicFile) Name() string            { return f.name }
func (f *fakeAtomicFile) Chmod(os.FileMode) error { return f.chmodErr }
func (f *fakeAtomicFile) Write(data []byte) (int, error) {
	f.writeCount++
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(data) - 1, nil
	}
	return len(data), nil
}
func (f *fakeAtomicFile) Sync() error  { return f.syncErr }
func (f *fakeAtomicFile) Close() error { return f.closeErr }

func TestAtomicWriteFileReplacesContentAndPreservesPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	if err := AtomicWriteFile(path, []byte("first\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(path, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second\n" {
		t.Fatalf("content = %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Fatalf("permissions = %o, want 640", info.Mode().Perm())
		}
	}
}

func TestAtomicWriteFileRejectsInvalidDestinations(t *testing.T) {
	root := t.TempDir()
	parentFile := filepath.Join(root, "parent")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(filepath.Join(parentFile, "config"), []byte("data"), 0o600); err == nil {
		t.Fatal("AtomicWriteFile succeeded below a regular file")
	}

	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(directory, []byte("data"), 0o600); err == nil {
		t.Fatal("AtomicWriteFile replaced a directory")
	}
}

func TestAtomicWriteFileErrorStages(t *testing.T) {
	original := struct {
		mkdirAll, stat, createTemp, remove, rename, chmod any
	}{atomicMkdirAll, atomicStat, atomicCreateTemp, atomicRemove, atomicRename, atomicChmod}
	defer func() {
		atomicMkdirAll = original.mkdirAll.(func(string, os.FileMode) error)
		atomicStat = original.stat.(func(string) (os.FileInfo, error))
		atomicCreateTemp = original.createTemp.(func(string, string) (atomicTempFile, error))
		atomicRemove = original.remove.(func(string) error)
		atomicRename = original.rename.(func(string, string) error)
		atomicChmod = original.chmod.(func(string, os.FileMode) error)
	}()
	forced := errors.New("forced")
	atomicMkdirAll = func(string, os.FileMode) error { return forced }
	if err := AtomicWriteFile("path", nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("mkdir error = %v", err)
	}
	atomicMkdirAll = os.MkdirAll
	atomicStat = func(string) (os.FileInfo, error) { return nil, forced }
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("stat error = %v", err)
	}
	atomicStat = os.Stat
	atomicCreateTemp = func(string, string) (atomicTempFile, error) { return nil, forced }
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("create error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) {
		return &fakeAtomicFile{name: "tmp", chmodErr: forced}, nil
	}
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("chmod temp error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) {
		return &fakeAtomicFile{name: "tmp", writeErr: forced}, nil
	}
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("write error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) {
		return &fakeAtomicFile{name: "tmp", shortWrite: true}, nil
	}
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), []byte("data"), 0o600); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) {
		return &fakeAtomicFile{name: "tmp", syncErr: forced}, nil
	}
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("sync error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) {
		return &fakeAtomicFile{name: "tmp", closeErr: forced}, nil
	}
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("close error = %v", err)
	}
	atomicCreateTemp = func(string, string) (atomicTempFile, error) { return &fakeAtomicFile{name: "tmp"}, nil }
	atomicRename = func(string, string) error { return forced }
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("rename error = %v", err)
	}
	atomicRename = os.Rename
	atomicRename = func(string, string) error { return nil }
	atomicChmod = func(string, os.FileMode) error { return forced }
	if err := AtomicWriteFile(filepath.Join(t.TempDir(), "file"), nil, 0o600); !errors.Is(err, forced) {
		t.Fatalf("destination chmod error = %v", err)
	}
}
