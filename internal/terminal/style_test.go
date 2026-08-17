package terminal

import (
	"bytes"
	"strings"
	"testing"
)

func TestStyles(t *testing.T) {
	original := EnableColor
	t.Cleanup(func() { EnableColor = original })
	EnableColor = false
	if got := Bold("text"); got != "text" {
		t.Fatal(got)
	}
	var output bytes.Buffer
	Success(&output, "saved %d", 1)
	Error(&output, "bad %s", "input")
	Info(&output, "info")
	Warn(&output, "warn")
	for _, want := range []string{"✓ saved 1", "✗ bad input", "ℹ info", "! warn"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %q", want, output.String())
		}
	}
	EnableColor = true
	if got := Bold("text"); !strings.Contains(got, bold) || !strings.Contains(got, reset) {
		t.Fatal(got)
	}
}

func TestDisableColorIfNeeded(t *testing.T) {
	original := EnableColor
	t.Cleanup(func() { EnableColor = original })
	EnableColor = true
	t.Setenv("NO_COLOR", "1")
	DisableColorIfNeeded()
	if EnableColor {
		t.Fatal("NO_COLOR was ignored")
	}

	EnableColor = true
	t.Setenv("NO_COLOR", "")
	DisableColorIfNeeded()
	if EnableColor {
		t.Fatal("non-TTY stdout was not detected")
	}
}
