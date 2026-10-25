package ghman

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zhongtait/gh-account/internal/terminal"
)

var (
	// ErrGHNotFound is returned when gh is not found in PATH or managed directory.
	ErrGHNotFound = errors.New("GitHub CLI (gh) not found")

	defaultGHVersion = "2.96.0"
	defaultBaseURL   = "https://github.com/cli/cli/releases/download"
)

// Manager handles finding, downloading, and installing the GitHub CLI (gh) binary.
type Manager struct {
	ConfigDir  string
	HTTPClient *http.Client
	Version    string
	BaseURL    string
	GOOS       string
	GOARCH     string
	LookPath   func(file string) (string, error)
	Stdout     io.Writer
	Stderr     io.Writer
}

// NewManager creates a new Manager instance.
func NewManager(configDir string, stdout, stderr io.Writer) *Manager {
	return &Manager{
		ConfigDir:  configDir,
		HTTPClient: &http.Client{Timeout: 90 * time.Second},
		LookPath:   exec.LookPath,
		Stdout:     stdout,
		Stderr:     stderr,
	}
}

func (m *Manager) goos() string {
	if m.GOOS != "" {
		return m.GOOS
	}
	return runtime.GOOS
}

func (m *Manager) goarch() string {
	if m.GOARCH != "" {
		return m.GOARCH
	}
	return runtime.GOARCH
}

func (m *Manager) version() string {
	if v := os.Getenv("GHA_GH_VERSION"); strings.TrimSpace(v) != "" {
		return strings.TrimPrefix(strings.TrimSpace(v), "v")
	}
	if m.Version != "" {
		return strings.TrimPrefix(m.Version, "v")
	}
	return defaultGHVersion
}

func (m *Manager) baseURL() string {
	if m.BaseURL != "" {
		return strings.TrimRight(m.BaseURL, "/")
	}
	return defaultBaseURL
}

func (m *Manager) binaryName() string {
	if m.goos() == "windows" {
		return "gh.exe"
	}
	return "gh"
}

// ManagedPath returns the absolute path where the managed gh executable is stored.
func (m *Manager) ManagedPath() string {
	return filepath.Join(m.ConfigDir, "bin", m.binaryName())
}

// FindGH looks for gh in system PATH or in the managed directory.
// Returns (path, isManaged, error).
func (m *Manager) FindGH() (string, bool, error) {
	lookPath := m.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	// 1. Check system PATH
	if sysPath, err := lookPath("gh"); err == nil && sysPath != "" {
		return sysPath, false, nil
	}

	// 2. Check managed directory
	managed := m.ManagedPath()
	if info, err := os.Stat(managed); err == nil && !info.IsDir() {
		return managed, true, nil
	}

	return "", false, ErrGHNotFound
}

// EnsureGH finds gh or automatically downloads and installs it.
func (m *Manager) EnsureGH(ctx context.Context) (string, error) {
	path, _, err := m.FindGH()
	if err == nil {
		return path, nil
	}
	return m.Install(ctx)
}

// Install downloads the official GitHub CLI release binary for the current platform and installs it.
func (m *Manager) Install(ctx context.Context) (string, error) {
	osName := m.goos()
	archName := m.goarch()
	version := m.version()

	var ghOS string
	switch osName {
	case "darwin":
		ghOS = "macOS"
	case "linux":
		ghOS = "linux"
	case "windows":
		ghOS = "windows"
	default:
		return "", fmt.Errorf("automatic gh download does not support operating system %q; install gh manually", osName)
	}

	var ghArch string
	switch archName {
	case "amd64":
		ghArch = "amd64"
	case "arm64":
		ghArch = "arm64"
	case "386":
		ghArch = "386"
	case "arm":
		ghArch = "armv6"
	default:
		return "", fmt.Errorf("automatic gh download does not support architecture %q; install gh manually", archName)
	}

	ext := ".tar.gz"
	if osName == "windows" {
		ext = ".zip"
	}

	archiveName := fmt.Sprintf("gh_%s_%s_%s%s", version, ghOS, ghArch, ext)
	downloadURL := fmt.Sprintf("%s/v%s/%s", m.baseURL(), version, archiveName)

	if m.Stdout != nil {
		terminal.Info(m.Stdout, "GitHub CLI (gh) not found; downloading official v%s for %s/%s...", version, osName, archName)
	}

	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create download request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download GitHub CLI from %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download GitHub CLI from %s: received HTTP %d", downloadURL, resp.StatusCode)
	}

	binDir := filepath.Join(m.ConfigDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create managed bin directory: %w", err)
	}

	targetPath := m.ManagedPath()
	binName := m.binaryName()

	var binaryData []byte
	if ext == ".zip" {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read response body: %w", err)
		}
		binaryData, err = extractBinaryFromZip(data, binName)
		if err != nil {
			return "", err
		}
	} else {
		var err error
		binaryData, err = extractBinaryFromTarGz(resp.Body, binName)
		if err != nil {
			return "", err
		}
	}

	// Write to temporary file first then atomic rename
	tmpFile, err := os.CreateTemp(binDir, "gh-download-*")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(binaryData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write binary: %w", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("chmod binary: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temporary binary: %w", err)
	}

	// Remove target if exists before rename (for Windows compatibility)
	_ = os.Remove(targetPath)
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return "", fmt.Errorf("install binary to %s: %w", targetPath, err)
	}

	if m.Stdout != nil {
		terminal.Success(m.Stdout, "GitHub CLI installed to %s", targetPath)
	}

	return targetPath, nil
}

func extractBinaryFromTarGz(r io.Reader, binName string) ([]byte, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("decompress gzip archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar archive: %w", err)
		}

		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if filepath.Base(header.Name) == binName {
				data, err := io.ReadAll(tr)
				if err != nil {
					return nil, fmt.Errorf("extract %s from tar: %w", binName, err)
				}
				return data, nil
			}
		}
	}

	return nil, fmt.Errorf("binary %s not found in tar.gz archive", binName)
}

func extractBinaryFromZip(data []byte, binName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("read zip archive: %w", err)
	}

	for _, f := range zr.File {
		if !f.FileInfo().IsDir() && filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open file in zip: %w", err)
			}
			defer rc.Close()

			content, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("extract %s from zip: %w", binName, err)
			}
			return content, nil
		}
	}

	return nil, fmt.Errorf("binary %s not found in zip archive", binName)
}
