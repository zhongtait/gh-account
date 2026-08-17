package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	userHomeDir = os.UserHomeDir
	runtimeGOOS = runtime.GOOS
)

// ExpandHome expands a leading ~ to the user home directory.
func ExpandHome(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := userHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ConfigDir returns the default gha config directory.
func ConfigDir() (string, error) {
	if custom := os.Getenv("GH_GHA_CONFIG_DIR"); custom != "" {
		return ExpandHome(custom)
	}

	home, err := userHomeDir()
	if err != nil {
		return "", err
	}

	if runtimeGOOS == "windows" {
		if base := os.Getenv("APPDATA"); base != "" {
			return filepath.Join(base, "gha"), nil
		}
	}

	return filepath.Join(home, ".config", "gha"), nil
}

// AccountsPath returns the accounts.yaml path under the config dir.
func AccountsPath(configDir string) string {
	return filepath.Join(configDir, "accounts.yaml")
}

// ConfigPath returns the config.yaml path under the config dir.
func ConfigPath(configDir string) string {
	return filepath.Join(configDir, "config.yaml")
}

// AuthPath returns the OAuth credential path under the config dir.
func AuthPath(configDir string) string {
	return filepath.Join(configDir, "auth.yaml")
}

// AuthKeyPath returns the local key used to encrypt OAuth tokens.
func AuthKeyPath(configDir string) string {
	return filepath.Join(configDir, "auth.key")
}
