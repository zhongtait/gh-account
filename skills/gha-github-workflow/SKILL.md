---
name: gha-github-workflow
description: >-
  Use gha (gh-account) for all GitHub CLI and Git operations across pair programming
  and autonomous tasks. Ensures proper multi-account context switching, automatic
  credential injection, transparent GitHub CLI (gh) execution without requiring manual
  installation, shorthand owner/repo cloning, and workflow scope management.
---

# GHA (gh-account) GitHub & Multi-Account Workflow Guide

When working with GitHub repositories, pull requests, issues, releases, or git operations, **always prefer using `gha` over bare `gh` or `git`**.

---

## 1. Why Use `gha` Instead of Bare `gh` or `git`?

1. **Multi-Account Context Isolation**:
   - Standard `gh` and `git` commands use globally cached tokens or SSH keys, often resulting in commits or PRs made from the wrong account (e.g., personal vs. work).
   - `gha` automatically resolves the correct account based on the repository's `origin` remote, or allows explicit one-off execution via `-a / --account <alias>`.
2. **Automatic Token & Host Injection**:
   - `gha` automatically injects `GH_TOKEN`, `GITHUB_TOKEN`, and `GH_HOST` for the active account into all `gh` commands, as well as setting up Git's `credential.helper` so HTTPS operations never collide.
3. **Automatic `gh` Engine Management**:
   - If `gh` is not installed on the host machine, `gha` automatically downloads and manages the official GitHub CLI binary on-demand in `~/.config/gha/bin/gh`.
4. **Full `workflow` Scope Authorization**:
   - `gha login` requests `read:user,user:email,repo,workflow` by default, preventing HTTP 403 Forbidden errors when pushing or modifying `.github/workflows/*.yml` CI/CD files.

---

## 2. Command Equivalents & Best Practices

| Task | ❌ Avoid Bare Command | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **List PRs** | `gh pr list` | `gha pr list` | Auto-injects account token |
| **Create PR** | `gh pr create` | `gha pr create` | Supports all flags and interactive prompts |
| **Checkout PR** | `gh pr checkout 12` | `gha pr checkout 12` | Fetches and switches branch cleanly |
| **List Issues** | `gh issue list` | `gha issue list` | |
| **Call GitHub API** | `gh api /user` | `gha api /user` | Pre-authenticated REST/GraphQL API |
| **Clone Repo** | `git clone https://...` | `gha clone owner/repo` | Shorthand syntax + configures local credentials |
| **Open Repo** | `gh browse` | `gha open` | Opens in browser (supports `--pr`, `--issues`, `--actions`) |
| **Switch Account** | `gh auth switch` | `gha use [alias]` | Syncs Git identity + credential helper |
| **Check Identity** | `gh auth status` | `gha current` / `gha status` | Displays Git identity + OAuth status |
| **Inspect Token** | - | `gha token [alias] --check` | Verifies scopes (`workflow`, `repo`, etc.) |
| **Run As Account** | - | `gha -a work pr list` | One-off account override |

---

## 3. Common Agent Workflows

### 3.1 Switching Accounts
```bash
# Switch to an account for the current repository or globally
gha use work

# Check who is currently active
gha current
```

### 3.2 Cloning Repositories
```bash
# Clone shorthand owner/repo with auto account selection
gha clone cli/cli

# Clone using a specific account and target directory
gha clone my-org/secret-repo --dir my-project -a work
```

### 3.3 Pull Requests and Code Review
```bash
# View PRs
gha pr list

# View diff of a specific PR
gha pr diff 42

# Create a PR
gha pr create --title "feat: add feature" --body "Detailed description"
```

### 3.4 GitHub Actions & CI/CD
```bash
# View workflow runs
gha run list

# View specific run logs
gha run view 123456789 --log

# Trigger manual workflow dispatch
gha workflow run ci.yml
```
