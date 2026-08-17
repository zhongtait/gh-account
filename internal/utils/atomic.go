package utils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type atomicTempFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

var (
	atomicMkdirAll   = os.MkdirAll
	atomicStat       = os.Stat
	atomicCreateTemp = func(dir, pattern string) (atomicTempFile, error) { return os.CreateTemp(dir, pattern) }
	atomicRemove     = os.Remove
	atomicRename     = os.Rename
	atomicChmod      = os.Chmod
)

// AtomicWriteFile replaces path atomically with data. Existing file
// permissions are preserved; perm is used when the file does not exist.
func AtomicWriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := atomicMkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	if info, err := atomicStat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination: %w", err)
	}

	temporary, err := atomicCreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer atomicRemove(temporaryPath)

	if err := temporary.Chmod(perm); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if written != len(data) {
		temporary.Close()
		return fmt.Errorf("write temporary file: %w", io.ErrShortWrite)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := atomicRename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	if err := atomicChmod(path, perm); err != nil {
		return fmt.Errorf("preserve destination permissions: %w", err)
	}
	return nil
}
