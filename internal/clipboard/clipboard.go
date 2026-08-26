package clipboard

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

type clipboardCommand interface {
	StdinPipe() (io.WriteCloser, error)
	Start() error
	Wait() error
}

var (
	clipboardGOOS     = runtime.GOOS
	clipboardLookPath = exec.LookPath
	newCommand        = func(name string, args ...string) clipboardCommand { return exec.Command(name, args...) }
)

func hasTool(name string) bool {
	_, err := clipboardLookPath(name)
	return err == nil
}

// Copy copies the given text to the system clipboard.
// It returns an error if the clipboard operation fails.
func Copy(text string) error {
	var cmd clipboardCommand

	switch clipboardGOOS {
	case "darwin":
		cmd = newCommand("pbcopy")
	case "linux":
		// Prefer wl-copy on Wayland sessions: xclip/xsel only talk to
		// the X11 clipboard and silently lose content under Wayland.
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := clipboardLookPath("wl-copy"); err == nil {
				cmd = newCommand("wl-copy")
			}
		}
		if cmd == nil {
			// Fall back to X11 tools: xclip first, then xsel, then wl-copy.
			switch {
			case hasTool("xclip"):
				cmd = newCommand("xclip", "-selection", "clipboard")
			case hasTool("xsel"):
				cmd = newCommand("xsel", "--clipboard", "--input")
			case hasTool("wl-copy"):
				cmd = newCommand("wl-copy")
			default:
				return fmt.Errorf("no clipboard utility found (install xclip, xsel, or wl-clipboard)")
			}
		}
	case "windows":
		cmd = newCommand("clip")
	default:
		return fmt.Errorf("clipboard not supported on %s", clipboardGOOS)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start clipboard command: %w", err)
	}

	if _, err := stdin.Write([]byte(text)); err != nil {
		stdin.Close()
		return fmt.Errorf("write to clipboard: %w", err)
	}

	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("clipboard command failed: %w", err)
	}

	return nil
}
