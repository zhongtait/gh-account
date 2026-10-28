---
name: gha-github-workflow
description: >-
  MANDATORY workflow skill for all GitHub CLI (gh), GitHub REST/GraphQL API, CI/CD Actions,
  Releases, and Git remote operations. Always route GitHub and git commands through gha (gh-account)
  to guarantee correct account context, automatic OAuth credential injection, and isolation without
  manual token handling or bare gh/curl invocations.
---

# GHA (gh-account) GitHub & Multi-Account Workflow Guide

When working with GitHub repositories, pull requests, issues, releases, GitHub Actions, or git operations, **AI coding agents MUST ALWAYS route operations through `gha`**.

---

## 1. Mandatory Operating Directives for AI Agents

To prevent credential contamination, 403 Forbidden errors, and account mismatch:

1. **NEVER call bare `gh` or explicit binary paths**:
   - ❌ **Forbidden**: `gh run list`, `/opt/homebrew/bin/gh auth token`, `/usr/local/bin/gh api ...`
   - ✅ **Required**: `gha run list`, `gha token [alias]`, `gha api ...`
2. **NEVER write custom Python/curl scripts to query the GitHub API**:
   - ❌ **Forbidden**: Extracting tokens via `gh auth token` and writing `curl -H "Authorization: Bearer ..."` or `urllib.request` scripts.
   - ✅ **Required**: Use `gha api <endpoint>`. `gha` automatically attaches the correct OAuth token, API headers, and host resolution.
3. **ALWAYS use `gha` for Actions (CI/CD) and Releases**:
   - ❌ **Forbidden**: Inspecting runs or releases via bare `gh` or custom REST calls.
   - ✅ **Required**: Use `gha run list`, `gha run view <id> --log`, `gha release view <tag>`, etc.
4. **Use Universal Passthrough `gha gh <subcommand>` when needed**:
   - Any GitHub CLI command can be executed transparently with `gha gh <subcommand> [args...]` (or directly via top-level commands like `gha pr`, `gha run`, `gha api`, `gha auth`, `gha release`).
5. **Multi-Account Context Isolation**:
   - `gha` automatically detects the account matching the current repository's `origin` remote.
   - For one-off account overrides, use `-a / --account <alias>` (e.g. `gha -a work pr list`).

---

## 2. Command Equivalents & Best Practices

### 2.1 CI/CD & GitHub Actions

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **List Runs** | `gh run list` | `gha run list` | View recent CI/CD workflow runs |
| **View Run Details / Logs** | Python script / `gh run view` | `gha run view <id> --log` | Inspect step failure logs directly |
| **Rerun Failed Jobs** | `gh run rerun <id>` | `gha run rerun <id> --failed` | Re-triggers failed Actions jobs |
| **Trigger Workflow** | `gh workflow run` | `gha workflow run <file.yml>` | Triggers `workflow_dispatch` |
| **Watch Run** | `gh run watch <id>` | `gha run watch <id>` | Streams live run progress in terminal |

### 2.2 Releases & Distribution

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **View Release** | Python / REST / `gh release view` | `gha release view [tag]` | Inspects release notes, assets, and checksums |
| **List Releases** | `gh release list` | `gha release list` | Lists all published versions and drafts |
| **Create Release** | `gh release create` | `gha release create <tag> [files]` | Automatically uses account with `repo` scope |
| **Download Assets** | `gh release download` | `gha release download [tag]` | Downloads release assets to local directory |

### 2.3 GitHub REST & GraphQL API

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **GET Request** | `curl -H "Authorization: ..."` | `gha api repos/{owner}/{repo}/...` | Auto-injects bearer token and host |
| **Mutation (POST/PUT)** | Custom Python / curl | `gha api repos/{owner}/{repo}/... -X PUT -F key=value` | Fully authenticated API mutations |
| **GraphQL Query** | `gh api graphql -f query=...` | `gha api graphql -f query='...'` | Executes GraphQL queries cleanly |

### 2.4 Pull Requests & Issues

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **List PRs** | `gh pr list` | `gha pr list` | Auto-injects account token |
| **Create PR** | `gh pr create` | `gha pr create` | Supports all flags and interactive prompts |
| **Checkout PR** | `gh pr checkout 12` | `gha pr checkout 12` | Fetches and switches branch cleanly |
| **View PR Diff** | `gh pr diff 42` | `gha pr diff 42` | View changes in a pull request |
| **List Issues** | `gh issue list` | `gha issue list` | View repository issues |
| **Create Issue** | `gh issue create` | `gha issue create` | Create issue with title and body |

### 2.5 Identity, Tokens & Accounts

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **Switch Account** | `gh auth switch` | `gha use [alias]` | Syncs Git identity + credential helper |
| **Check Identity** | `gh auth status` | `gha current` / `gha status` / `gha auth status` | Displays active account & token state |
| **Inspect Token & Scopes** | `gh auth token` / keychain | `gha token [alias] --check` | Verifies scopes (`workflow`, `repo`, etc.) |
| **Print Token Value** | Extracting via script | `gha token [alias]` | Prints OAuth token to stdout |
| **Add / Login Account** | `gh auth login` | `gha login` / `gha add` | Authorizes with `workflow` scope by default |

### 2.6 Repository, Browser & Universal Passthrough

| Task | ❌ Avoid Bare / Script | ✅ Preferred `gha` Command | Notes |
| :--- | :--- | :--- | :--- |
| **Clone Repo** | `git clone https://...` | `gha clone owner/repo` | Shorthand syntax + configures credentials |
| **Open in Browser** | `gh browse` | `gha open` | Opens repo (supports `--pr`, `--issues`, `--actions`) |
| **Universal Passthrough** | `gh <any-command>` | `gha gh <any-command> [args...]` | Transparent execution with credentials |
| **One-Off Account Override** | - | `gha -a <alias> <command>` | Run any command using a specific profile |

---

## 3. Practical AI Agent Workflows

### 3.1 Investigating CI/CD Failures
```bash
# 1. Check recent workflow runs
gha run list -L 5

# 2. View details or step logs for a failing run
gha run view 123456789 --log-failed

# 3. Rerun failed jobs after fixing code
gha run rerun 123456789 --failed
```

### 3.2 Inspecting and Verifying Releases
```bash
# 1. View latest release details and attached binary assets
gha release view v0.11.0

# 2. List all past releases
gha release list -L 10
```

### 3.3 Querying and Updating GitHub API
```bash
# Query workflow permissions (No need to extract tokens or use curl!)
gha api repos/owner/repo/actions/permissions/workflow

# Update repository settings with PUT
gha api repos/owner/repo/actions/permissions/workflow -X PUT -F default_workflow_permissions=write
```

### 3.4 Cloning and Working with Repositories
```bash
# Clone with automatic account resolution
gha clone owner/repo

# Clone with a specific account into a custom directory
gha clone my-org/secret-repo --dir my-project -a work

# Verify current repository account context
gha current
```
