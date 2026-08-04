package utils

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, stderr string, err error)
}

// RealRunner runs commands on the host system.
type RealRunner struct{}

// Run executes a command and captures stdout/stderr.
func (RealRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if err != nil {
		if errOut != "" {
			return out, errOut, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, errOut)
		}
		return out, errOut, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, errOut, nil
}

// LookPath checks whether a binary exists in PATH.
func LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
