package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMainSuccessAndRedactedError(t *testing.T) {
	originalExecute, originalOutput, originalExit := executeCLI, errorOutput, exitProcess
	t.Cleanup(func() { executeCLI, errorOutput, exitProcess = originalExecute, originalOutput, originalExit })
	executeCLI = func() error { return nil }
	main()

	var output bytes.Buffer
	exited := 0
	executeCLI = func() error { return errors.New("token=ghp_123456789012345678901234567890123456") }
	errorOutput = &output
	exitProcess = func(code int) { exited = code }
	main()
	if exited != 1 || !strings.Contains(output.String(), "ghp_...3456") {
		t.Fatalf("exit=%d output=%q", exited, output.String())
	}
}
