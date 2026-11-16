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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestTarGz(binName string, content []byte) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	header := &tar.Header{
		Name:     "bin/" + binName,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func createTestZip(binName string, content []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	w, err := zw.Create("bin/" + binName)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(content); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func serveReleaseAsset(w http.ResponseWriter, r *http.Request, assetName string, assetData []byte) {
	if strings.HasSuffix(r.URL.Path, "_checksums.txt") {
		sum := sha256.Sum256(assetData)
		_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + assetName + "\n"))
		return
	}
	if strings.HasSuffix(r.URL.Path, assetName) {
		_, _ = w.Write(assetData)
		return
	}
	http.NotFound(w, r)
}

func TestFindGH(t *testing.T) {
	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)

	// 1. System PATH hit
	m.LookPath = func(file string) (string, error) {
		if file == "gh" {
			return "/usr/local/bin/gh", nil
		}
		return "", errors.New("not found")
	}
	path, isManaged, err := m.FindGH()
	if err != nil || path != "/usr/local/bin/gh" || isManaged {
		t.Fatalf("expected /usr/local/bin/gh, false, nil; got %s, %v, %v", path, isManaged, err)
	}

	// 2. System PATH miss, managed binary present
	m.LookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	managedBin := m.ManagedPath()
	if err := os.WriteFile(managedBin, []byte("#!/bin/sh\necho gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, isManaged, err = m.FindGH()
	if err != nil || path != managedBin || !isManaged {
		t.Fatalf("expected %s, true, nil; got %s, %v, %v", managedBin, path, isManaged, err)
	}

	// 3. Both missing
	_ = os.Remove(managedBin)
	_, _, err = m.FindGH()
	if !errors.Is(err, ErrGHNotFound) {
		t.Fatalf("expected ErrGHNotFound, got: %v", err)
	}
}

func TestEnsureGHExtant(t *testing.T) {
	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)
	m.LookPath = func(file string) (string, error) {
		return "/mock/gh", nil
	}
	path, err := m.EnsureGH(context.Background())
	if err != nil || path != "/mock/gh" {
		t.Fatalf("EnsureGH: got %q, %v", path, err)
	}
}

func TestInstallMacOSZipSuccess(t *testing.T) {
	dummyContent := []byte("#!/bin/sh\necho hello-gh\n")
	zipData, err := createTestZip("gh", dummyContent)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, "gh_2.96.1_macOS_arm64.zip", zipData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)
	m.BaseURL = server.URL
	m.GOOS = "darwin"
	m.GOARCH = "arm64"
	m.Version = "v2.96.1"
	m.LookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	path, err := m.Install(context.Background())
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if path != m.ManagedPath() {
		t.Fatalf("expected %s, got %s", m.ManagedPath(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, dummyContent) {
		t.Fatalf("unexpected binary content: %s", string(data))
	}
}

func TestInstallZipSuccess(t *testing.T) {
	dummyContent := []byte("fake-windows-binary")
	zipData, err := createTestZip("gh.exe", dummyContent)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, "gh_2.96.1_windows_amd64.zip", zipData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)
	m.BaseURL = server.URL
	m.GOOS = "windows"
	m.GOARCH = "amd64"
	m.Version = "2.96.1"

	path, err := m.Install(context.Background())
	if err != nil {
		t.Fatalf("Install zip failed: %v", err)
	}
	if path != m.ManagedPath() {
		t.Fatalf("expected %s, got %s", m.ManagedPath(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, dummyContent) {
		t.Fatalf("unexpected binary content: %s", string(data))
	}
}

func TestInstallHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"

	_, err := m.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP 500 error, got: %v", err)
	}
}

func TestInstallArchiveMissingBinary(t *testing.T) {
	tarData, err := createTestTarGz("other-tool", []byte("xyz"))
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, "gh_2.96.1_linux_amd64.tar.gz", tarData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"
	m.Version = "2.96.1"

	_, err = m.Install(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not found in tar.gz archive") {
		t.Fatalf("expected not found error, got: %v", err)
	}
}

func TestInstallUnsupportedPlatform(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)

	m.GOOS = "solaris"
	m.GOARCH = "amd64"
	if _, err := m.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "does not support operating system") {
		t.Fatalf("expected unsupported os error, got: %v", err)
	}

	m.GOOS = "linux"
	m.GOARCH = "mips64"
	if _, err := m.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "does not support architecture") {
		t.Fatalf("expected unsupported arch error, got: %v", err)
	}
}

func TestEnsureGHWhenMissing(t *testing.T) {
	dummyContent := []byte("#!/bin/sh\necho ensured\n")
	tarData, err := createTestTarGz("gh", dummyContent)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, "gh_2.96.1_linux_amd64.tar.gz", tarData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"
	m.Version = "2.96.1"
	m.LookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}

	path, err := m.EnsureGH(context.Background())
	if err != nil {
		t.Fatalf("EnsureGH missing failed: %v", err)
	}
	if path != m.ManagedPath() {
		t.Fatalf("expected %s, got %s", m.ManagedPath(), path)
	}
}

func TestArchMappings(t *testing.T) {
	for _, tc := range []struct {
		os, arch string
	}{
		{"linux", "386"},
		{"linux", "arm"},
	} {
		m := NewManager(t.TempDir(), nil, nil)
		m.GOOS = tc.os
		m.GOARCH = tc.arch
		m.BaseURL = "http://127.0.0.1:1"
		_, err := m.Install(context.Background())
		// It should attempt to download and fail connecting to 127.0.0.1:1, meaning OS/arch validation passed!
		if err == nil || strings.Contains(err.Error(), "does not support") {
			t.Fatalf("expected arch %s to be supported, got err: %v", tc.arch, err)
		}
	}
}

func TestCorruptArchives(t *testing.T) {
	_, err := extractBinaryFromTarGz(bytes.NewReader([]byte("not-gzip")), "gh")
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}

	_, err = extractBinaryFromZip([]byte("not-zip"), "gh.exe")
	if err == nil {
		t.Fatal("expected error for invalid zip")
	}
}

func TestDefaultBaseURLAndArch(t *testing.T) {
	m := NewManager(t.TempDir(), nil, nil)
	if m.baseURL() != defaultBaseURL {
		t.Fatalf("expected defaultBaseURL, got: %s", m.baseURL())
	}
	if m.goarch() == "" {
		t.Fatal("expected non-empty goarch")
	}
}

func TestVersionEnvOverride(t *testing.T) {
	t.Setenv("GHA_GH_VERSION", "v2.50.0")
	m := NewManager(t.TempDir(), nil, nil)
	if v := m.version(); v != "2.50.0" {
		t.Fatalf("expected 2.50.0, got %s", v)
	}
}

func TestPinnedChecksumsCoverSupportedTargets(t *testing.T) {
	for _, tc := range []struct{ os, arch, asset string }{
		{"darwin", "amd64", "gh_2.96.0_macOS_amd64.zip"},
		{"darwin", "arm64", "gh_2.96.0_macOS_arm64.zip"},
		{"linux", "amd64", "gh_2.96.0_linux_amd64.tar.gz"},
		{"linux", "arm64", "gh_2.96.0_linux_arm64.tar.gz"},
		{"linux", "386", "gh_2.96.0_linux_386.tar.gz"},
		{"linux", "arm", "gh_2.96.0_linux_armv6.tar.gz"},
		{"windows", "amd64", "gh_2.96.0_windows_amd64.zip"},
		{"windows", "arm64", "gh_2.96.0_windows_arm64.zip"},
		{"windows", "386", "gh_2.96.0_windows_386.zip"},
	} {
		if _, ok := pinnedChecksums[tc.asset]; !ok {
			t.Errorf("no pinned checksum for %s/%s (asset %s)", tc.os, tc.arch, tc.asset)
		}
	}
}

// TestReleaseArchiveNamesHavePinnedChecksums ties the archive naming to the
// pinned table, so a platform cannot silently produce an asset name that the
// default install path refuses. This is what the macOS .tar.gz regression broke.
func TestReleaseArchiveNamesHavePinnedChecksums(t *testing.T) {
	for _, tc := range []struct{ os, arch string }{
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"linux", "386"},
		{"linux", "arm"},
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"windows", "386"},
	} {
		name, err := releaseArchiveName(tc.os, tc.arch, defaultGHVersion)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.os, tc.arch, err)
		}
		if _, ok := pinnedChecksums[name]; !ok {
			t.Errorf("%s/%s names %s, which has no pinned checksum", tc.os, tc.arch, name)
		}
	}

	if got := archiveExt("darwin"); got != ".zip" {
		t.Errorf("macOS archive extension = %q, want .zip", got)
	}
	if got, err := releaseArchiveName("darwin", "arm64", "2.96.0"); err != nil || got != "gh_2.96.0_macOS_arm64.zip" {
		t.Errorf("macOS archive = %q, %v", got, err)
	}
	if _, err := releaseArchiveName("solaris", "amd64", "2.96.0"); err == nil {
		t.Error("unsupported OS accepted")
	}
	if _, err := releaseArchiveName("linux", "mips64", "2.96.0"); err == nil {
		t.Error("unsupported architecture accepted")
	}
}

// A mirror serving a tampered archive together with a self-consistent checksum
// file must still be rejected for the pinned version: the compiled digest is
// not something the download server can influence.
func TestInstallRejectsTamperedMirrorEndToEnd(t *testing.T) {
	tampered, err := createTestTarGz("gh", []byte("#!/bin/sh\necho evil\n"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, "gh_2.96.0_linux_amd64.tar.gz", tampered)
	}))
	defer server.Close()

	m := NewManager(t.TempDir(), nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"
	if _, err := m.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("mirror tampering not rejected: %v", err)
	}
	if _, err := os.Stat(m.ManagedPath()); !os.IsNotExist(err) {
		t.Fatalf("binary was installed despite rejection: %v", err)
	}
}

func TestInstallRejectsContentThatDoesNotMatchPinnedChecksum(t *testing.T) {
	const asset = "gh_2.96.0_linux_amd64.tar.gz"
	tarData, err := createTestTarGz("gh", []byte("#!/bin/sh\necho tampered\n"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveReleaseAsset(w, r, asset, tarData)
	}))
	defer server.Close()

	m := NewManager(t.TempDir(), nil, nil)
	m.GOOS = "linux"
	m.GOARCH = "amd64"
	// A custom endpoint must not bypass the compiled digest for the pinned
	// release, even if it serves a matching checksum file.
	m.BaseURL = server.URL
	if pinned, err := m.verifyPinnedChecksum(asset, tarData); err == nil || !strings.Contains(err.Error(), "checksum mismatch") || pinned {
		t.Fatalf("custom endpoint bypassed the pinned check: pinned=%v err=%v", pinned, err)
	}

	m.BaseURL = ""
	m.Version = "2.96.0"
	if pinned, err := m.verifyPinnedChecksum(asset, tarData); err == nil || !strings.Contains(err.Error(), "pinned") || pinned {
		t.Fatalf("tampered content accepted: pinned=%v err=%v", pinned, err)
	}
	if pinned, err := m.verifyPinnedChecksum("gh_9.9.9_linux_amd64.tar.gz", tarData); err == nil || !strings.Contains(err.Error(), "no pinned") || pinned {
		t.Fatalf("unknown asset accepted: pinned=%v err=%v", pinned, err)
	}

	// Content matching the pinned digest must be accepted and report that a
	// pinned check applied, so Install can skip the weaker checksum file.
	original, hadOriginal := pinnedChecksums[asset]
	sum := sha256.Sum256(tarData)
	pinnedChecksums[asset] = hex.EncodeToString(sum[:])
	t.Cleanup(func() {
		if hadOriginal {
			pinnedChecksums[asset] = original
			return
		}
		delete(pinnedChecksums, asset)
	})
	pinned, err := m.verifyPinnedChecksum(asset, tarData)
	if err != nil {
		t.Fatalf("matching content rejected: %v", err)
	}
	if !pinned {
		t.Fatal("matching pinned content did not report a pinned check")
	}

	// A non-default version has no compiled digest; Install must fall back to
	// the release checksum file.
	m.Version = "2.50.0"
	if pinned, err := m.verifyPinnedChecksum(asset, tarData); err != nil || pinned {
		t.Fatalf("non-default version: pinned=%v err=%v", pinned, err)
	}

	// Every shipped digest must be a valid SHA-256 hex string.
	for name, digest := range pinnedChecksums {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			t.Errorf("%s has an invalid SHA-256 value %q", name, digest)
		}
	}
}
