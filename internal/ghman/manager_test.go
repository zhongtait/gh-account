package ghman

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
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
		Name:     "gh_2.96.0_macOS_arm64/bin/" + binName,
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

	w, err := zw.Create("gh_2.96.0_windows_amd64/bin/" + binName)
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

func TestInstallTarGzSuccess(t *testing.T) {
	dummyContent := []byte("#!/bin/sh\necho hello-gh\n")
	tarData, err := createTestTarGz("gh", dummyContent)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".tar.gz") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)
	m.BaseURL = server.URL
	m.GOOS = "darwin"
	m.GOARCH = "arm64"
	m.Version = "v2.96.0"
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
		if !strings.HasSuffix(r.URL.Path, ".zip") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	out := new(bytes.Buffer)
	m := NewManager(tmpDir, out, out)
	m.BaseURL = server.URL
	m.GOOS = "windows"
	m.GOARCH = "amd64"
	m.Version = "2.96.0"

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
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"

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
		_, _ = w.Write(tarData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	m := NewManager(tmpDir, nil, nil)
	m.BaseURL = server.URL
	m.GOOS = "linux"
	m.GOARCH = "amd64"
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
