package terminal

import (
	"fmt"
	"io"
	"os"
)

var (
	// EnableColor controls ANSI color output.
	EnableColor = true
)

const (
	reset  = "\033[0m"
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	bold   = "\033[1m"
)

func colorize(code, message string) string {
	if !EnableColor {
		return message
	}
	return code + message + reset
}

// Success prints a success line.
func Success(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "%s %s\n", colorize(green, "✓"), fmt.Sprintf(format, args...))
}

// Error prints an error line.
func Error(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "%s %s\n", colorize(red, "✗"), fmt.Sprintf(format, args...))
}

// Info prints an info line.
func Info(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "%s %s\n", colorize(cyan, "ℹ"), fmt.Sprintf(format, args...))
}

// Warn prints a warning line.
func Warn(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "%s %s\n", colorize(yellow, "!"), fmt.Sprintf(format, args...))
}

// Bold returns bold text when color is enabled.
func Bold(text string) string {
	return colorize(bold, text)
}

// DisableColorIfNeeded turns off color when stdout is not a TTY or NO_COLOR is set.
func DisableColorIfNeeded() {
	if os.Getenv("NO_COLOR") != "" {
		EnableColor = false
		return
	}
	if fi, err := os.Stdout.Stat(); err == nil {
		if (fi.Mode() & os.ModeCharDevice) == 0 {
			EnableColor = false
		}
	}
}
