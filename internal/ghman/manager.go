package ghman

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// pinnedChecksums holds the SHA-256 of each official release archive for
// defaultGHVersion. Verifying against a hash compiled into gha means a
// compromised or spoofed release endpoint cannot swap the binary, which a
// checksum file downloaded from that same endpoint would not prevent. These
// values come from the GitHub release API (`digest` of each asset) and match
// gh_<version>_checksums.txt.
var pinnedChecksums = map[string]string{
	"gh_2.96.0_macOS_amd64.zip":    "4bd449df9ad639391bc62b8032546f0fe9edcd8526e06682a4f88abd8c5d163c",
	"gh_2.96.0_macOS_arm64.zip":    "f23a0c37d963aacc3bed703ccbd59b41c5ca22101fab7f00eb2b7cad23aba463",
	"gh_2.96.0_linux_amd64.tar.gz": "83d5c2ccad5498f58bf6368acb1ab32588cf43ab3a4b1c301bf36328b1c8bd60",
	"gh_2.96.0_linux_arm64.tar.gz": "06f86ec7103d41993b76cd78072f43595c34aaa56506d971d9860e67140bf909",
	"gh_2.96.0_linux_386.tar.gz":   "fda636f771e43d1f19e2d42e3c578cc9d8e453d23093a6a47a7fac88825196ec",
	"gh_2.96.0_linux_armv6.tar.gz": "14ff25ec64823415086214fb05b3e85aef21deba6a8d723a7ef8ead1cb977826",
	"gh_2.96.0_windows_amd64.zip":  "c2d6acc935cd2f00e2144d7e036d5cd82e6b6bd5594e8c75aa75ef2a4ed6aac3",
	"gh_2.96.0_windows_arm64.zip":  "c517e0b32c98a4ba90ac95af8d12cc3ac55781ab4ab72f9a91ce3de0541d2b09",
	"gh_2.96.0_windows_386.zip":    "575bd472f1811d7d9a7f94f27f11a2decb6ae6711b56fec337675234c336ffe6",
}

const (
	maxArchiveSize         = 128 << 20
	maxChecksumFileSize    = 1 << 20
	maxBinarySize          = 256 << 20
	maxExpandedArchiveSize = 512 << 20
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

// verifyPinnedChecksum compares a downloaded archive against the hash compiled
// into gha, which the download server cannot influence. It reports whether a
// pinned digest applied: custom versions have no compiled digest, so callers
// fall back to the release checksum file for those. A mirror of the pinned
// release must serve the same immutable asset bytes, otherwise installation is
// refused even when the mirror also supplies a matching checksum file.
func (m *Manager) verifyPinnedChecksum(assetName string, assetData []byte) (bool, error) {
	if m.version() != defaultGHVersion {
		return false, nil
	}
	expected, ok := pinnedChecksums[assetName]
	if !ok {
		return false, fmt.Errorf("no pinned SHA-256 is available for %s; set GHA_GH_VERSION to a supported release", assetName)
	}
	actualSum := sha256.Sum256(assetData)
	actual := hex.EncodeToString(actualSum[:])
	if actual != expected {
		return false, fmt.Errorf("SHA-256 checksum mismatch for %s: downloaded content does not match the pinned release", assetName)
	}
	return true, nil
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

// releaseTarget maps a Go OS/architecture pair onto the names GitHub CLI uses
// in its release assets.
func releaseTarget(osName, archName string) (ghOS, ghArch string, err error) {
	switch osName {
	case "darwin":
		ghOS = "macOS"
	case "linux":
		ghOS = "linux"
	case "windows":
		ghOS = "windows"
	default:
		return "", "", fmt.Errorf("automatic gh download does not support operating system %q; install gh manually", osName)
	}

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
		return "", "", fmt.Errorf("automatic gh download does not support architecture %q; install gh manually", archName)
	}

	return ghOS, ghArch, nil
}

// archiveExt returns the release archive extension. GitHub CLI publishes .zip
// for macOS and Windows and .tar.gz for Linux; publishing a .tar.gz name for
// macOS would download nothing and, once pinned checksums exist, also fail the
// checksum lookup.
func archiveExt(osName string) string {
	if osName == "darwin" || osName == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// releaseArchiveName builds the release asset name for a platform. It is the
// single place that decides an archive name, so tests can assert that every
// supported platform maps to an asset with a pinned checksum.
func releaseArchiveName(osName, archName, version string) (string, error) {
	ghOS, ghArch, err := releaseTarget(osName, archName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("gh_%s_%s_%s%s", version, ghOS, ghArch, archiveExt(osName)), nil
}

// Install downloads the official GitHub CLI release binary for the current platform and installs it.
func (m *Manager) Install(ctx context.Context) (string, error) {
	osName := m.goos()
	archName := m.goarch()
	version := m.version()

	archiveName, err := releaseArchiveName(osName, archName, version)
	if err != nil {
		return "", err
	}
	ext := archiveExt(osName)
	downloadURL := fmt.Sprintf("%s/v%s/%s", m.baseURL(), version, archiveName)

	if m.Stdout != nil {
		terminal.Info(m.Stdout, "GitHub CLI (gh) not found; downloading official v%s for %s/%s...", version, osName, archName)
	}

	client := m.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	archiveData, err := downloadReleaseAsset(ctx, client, downloadURL, "GitHub CLI", maxArchiveSize)
	if err != nil {
		return "", err
	}
	// The compiled digest is the strongest check: it cannot be influenced by
	// the server that served the archive. Only when no digest is pinned (a
	// version selected through GHA_GH_VERSION) fall back to the release's own
	// checksum file, which is weaker but still catches truncated downloads.
	pinned, err := m.verifyPinnedChecksum(archiveName, archiveData)
	if err != nil {
		return "", err
	}
	if !pinned {
		checksumURL := fmt.Sprintf("%s/v%s/gh_%s_checksums.txt", m.baseURL(), version, version)
		checksumData, err := downloadReleaseAsset(ctx, client, checksumURL, "GitHub CLI checksum file", maxChecksumFileSize)
		if err != nil {
			return "", err
		}
		if err := verifyReleaseChecksum(checksumData, archiveName, archiveData); err != nil {
			return "", err
		}
	}

	binDir := filepath.Join(m.ConfigDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create managed bin directory: %w", err)
	}

	targetPath := m.ManagedPath()
	binName := m.binaryName()

	var binaryData []byte
	if ext == ".zip" {
		binaryData, err = extractBinaryFromZip(archiveData, binName)
		if err != nil {
			return "", err
		}
	} else {
		binaryData, err = extractBinaryFromTarGz(bytes.NewReader(archiveData), binName)
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

	if err := replaceManagedBinary(tmpPath, targetPath); err != nil {
		return "", fmt.Errorf("install binary to %s: %w", targetPath, err)
	}

	if m.Stdout != nil {
		terminal.Success(m.Stdout, "GitHub CLI installed to %s", targetPath)
	}

	return targetPath, nil
}

func downloadReleaseAsset(ctx context.Context, client *http.Client, assetURL, label string, maxSize int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", label, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s from %s: %w", label, assetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s from %s: received HTTP %d", label, assetURL, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s response body: %w", label, err)
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("%s exceeds the maximum allowed size of %d bytes", label, maxSize)
	}
	return data, nil
}

func verifyReleaseChecksum(checksumData []byte, assetName string, assetData []byte) error {
	var expected string
	for _, line := range strings.Split(string(checksumData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		if expected != "" {
			return fmt.Errorf("checksum file contains multiple entries for %s", assetName)
		}
		decoded, err := hex.DecodeString(fields[0])
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("checksum file contains an invalid SHA-256 value for %s", assetName)
		}
		expected = strings.ToLower(fields[0])
	}
	if expected == "" {
		return fmt.Errorf("checksum file does not contain an entry for %s", assetName)
	}

	actualSum := sha256.Sum256(assetData)
	actual := hex.EncodeToString(actualSum[:])
	if actual != expected {
		return fmt.Errorf("SHA-256 checksum mismatch for %s", assetName)
	}
	return nil
}

func replaceManagedBinary(tmpPath, targetPath string) error {
	if err := os.Rename(tmpPath, targetPath); err == nil {
		return nil
	} else {
		targetInfo, statErr := os.Stat(targetPath)
		if statErr != nil {
			return err
		}
		if targetInfo.IsDir() {
			return fmt.Errorf("existing target is a directory")
		}
	}

	backupFile, err := os.CreateTemp(filepath.Dir(targetPath), "gh-backup-*")
	if err != nil {
		return fmt.Errorf("prepare backup of existing binary: %w", err)
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close backup placeholder: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare backup path: %w", err)
	}
	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("preserve existing binary before replacement: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			return fmt.Errorf("replace failed: %w; restoring previous binary from %s also failed: %v", err, backupPath, restoreErr)
		}
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("new binary installed, but previous binary backup %s could not be removed: %w", backupPath, err)
	}
	return nil
}

func extractBinaryFromTarGz(r io.Reader, binName string) ([]byte, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("decompress gzip archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(io.LimitReader(gzr, maxExpandedArchiveSize))
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
				if header.Size > maxBinarySize {
					return nil, fmt.Errorf("extracted binary %s exceeds the maximum allowed size of %d bytes", binName, maxBinarySize)
				}
				data, err := io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
				if err != nil {
					return nil, fmt.Errorf("extract %s from tar: %w", binName, err)
				}
				if int64(len(data)) > maxBinarySize {
					return nil, fmt.Errorf("extracted binary %s exceeds the maximum allowed size of %d bytes", binName, maxBinarySize)
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
			if f.UncompressedSize64 > maxBinarySize {
				return nil, fmt.Errorf("extracted binary %s exceeds the maximum allowed size of %d bytes", binName, maxBinarySize)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open file in zip: %w", err)
			}
			defer rc.Close()

			content, err := io.ReadAll(io.LimitReader(rc, maxBinarySize+1))
			if err != nil {
				return nil, fmt.Errorf("extract %s from zip: %w", binName, err)
			}
			if int64(len(content)) > maxBinarySize {
				return nil, fmt.Errorf("extracted binary %s exceeds the maximum allowed size of %d bytes", binName, maxBinarySize)
			}
			return content, nil
		}
	}

	return nil, fmt.Errorf("binary %s not found in zip archive", binName)
}
