package github

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

var (
	browserGOOS    = runtime.GOOS
	browserCommand = exec.Command
)

// OpenBrowser opens an HTTPS verification URL using the host OS. This is
// optional convenience functionality; login still works when it fails.
func OpenBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("invalid browser URL")
	}

	var command string
	var args []string
	switch browserGOOS {
	case "darwin":
		command, args = "open", []string{rawURL}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	case "linux", "freebsd", "openbsd", "netbsd":
		command, args = "xdg-open", []string{rawURL}
	default:
		return fmt.Errorf("browser opening is unsupported on %s", browserGOOS)
	}

	if err := browserCommand(command, args...).Start(); err != nil {
		return fmt.Errorf("start browser opener: %w", err)
	}
	return nil
}
