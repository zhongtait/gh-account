package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// InteractiveRunner runs external commands attached to the current terminal.
type InteractiveRunner interface {
	RunInteractive(ctx context.Context, name string, args ...string) error
}

// RealInteractiveRunner runs commands with inherited stdio.
type RealInteractiveRunner struct{}

// RunInteractive executes a command interactively.
func (RealInteractiveRunner) RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
