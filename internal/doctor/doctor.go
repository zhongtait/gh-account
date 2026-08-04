package doctor

import (
	"context"
	"fmt"
	"os"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/utils"
)

// Check is a single doctor diagnostic result.
type Check struct {
	Name    string
	OK      bool
	Message string
}

// Report aggregates doctor checks.
type Report struct {
	Checks []Check
}

// Healthy reports whether all checks passed.
func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if !check.OK {
			return false
		}
	}
	return true
}

// Runner performs environment diagnostics.
type Runner struct {
	Store  *config.Store
	Git    git.Client
	GitHub github.Client
}

// Run executes diagnostics.
func (r Runner) Run(ctx context.Context) Report {
	var checks []Check
	checks = append(checks, Check{Name: "git", OK: true, Message: "native Git support available"})

	if r.Store == nil {
		checks = append(checks, Check{Name: "config", OK: false, Message: "config store is not configured"})
	} else if _, err := os.Stat(utils.AccountsPath(r.Store.Dir)); err != nil {
		checks = append(checks, Check{Name: "config", OK: false, Message: "config not initialized; run gha init"})
	} else if _, err := r.Store.LoadAccounts(); err != nil {
		checks = append(checks, Check{Name: "config", OK: false, Message: fmt.Sprintf("failed to load config: %v", err)})
	} else {
		checks = append(checks, Check{Name: "config", OK: true, Message: "config loaded"})
	}

	if r.Git != nil {
		isRepo, err := r.Git.IsRepo(ctx)
		if err != nil {
			checks = append(checks, Check{Name: "repository", OK: false, Message: err.Error()})
		} else if !isRepo {
			checks = append(checks, Check{Name: "repository", OK: false, Message: "not inside a git repository"})
		} else {
			checks = append(checks, Check{Name: "repository", OK: true, Message: "repository detected"})
			if _, err := r.Git.GetRemoteURL(ctx, "origin"); err != nil {
				checks = append(checks, Check{Name: "remote", OK: false, Message: "origin remote not found"})
			} else {
				checks = append(checks, Check{Name: "remote", OK: true, Message: "origin remote found"})
			}
		}
	}

	if r.GitHub != nil {
		if login, err := r.GitHub.CurrentLogin(ctx); err != nil {
			checks = append(checks, Check{Name: "token", OK: false, Message: "unable to read active GitHub login; run gha login"})
		} else if login == "" {
			checks = append(checks, Check{Name: "token", OK: false, Message: "no active GitHub login"})
		} else {
			checks = append(checks, Check{Name: "token", OK: true, Message: fmt.Sprintf("authenticated as %s", login)})
		}
	}

	return Report{Checks: checks}
}
