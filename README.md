# gh-account

<p align="center">
  <b>English</b> | <a href="README_zh.md">简体中文</a>
</p>

[![CI](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml)
[![Release](https://github.com/zhongtait/gh-account/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/zhongtait/gh-account/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/zhongtait/gh-account?sort=semver)](https://github.com/zhongtait/gh-account/releases/latest)

`gha` is a multi-account GitHub and Git management tool written in pure Go. It consolidates GitHub OAuth authentication, Git commit identity (`user.name` / `user.email`), and repository remote configurations into a unified, reusable command-line workflow.

## Features

- **Multi-Account Profile Management**: seamlessly manage multiple GitHub account configurations.
- **Automatic Email & Name Resolution**: upon successful login (OAuth Device Flow or PAT), automatically fetches your verified primary email (including GitHub privacy noreply emails) and display name via GitHub REST API—**no manual typing needed**, eliminating commit author typos.
- **Direct Token (PAT) Authentication**: supports direct authentication via `--token <PAT>` or `--with-token` (from stdin) for CI environments or existing Personal Access Tokens without opening a browser.
- **Token Validity & Scope Inspection**: `gha token --check` (or `--scopes`) queries the GitHub API in real time to display token validity, user identity, expiration, and authorized scopes (explicitly checking for `workflow` permissions).
- **Fuzzy Account Suggestions**: intelligent typo corrections based on Levenshtein distance (e.g., `account "persnal" not found; did you mean "personal"?`).
- **Auto-Managed GitHub CLI (gh) Engine**: automatically downloads the official `gh` binary on-demand to `~/.config/gha/bin/gh` if not present; full GitHub CLI commands (`pr`, `issue`, `repo`, `run`, `release`, `api`, etc.) work transparently with active credentials injected.
- **Shorthand Repository & Flexible Cloning**: clone using `owner/repo` shorthand, with support for `-a`/`--account` and `-d`/`--dir` flags.
- **Visual Auth Status**: `gha list` displays the OAuth token status (`logged in` / `no token`) for each configured account.
- **Transparent Git Proxying**: `gha git clone` is equivalent to `gha clone`; run any Git command directly (e.g., `gha push`, `gha commit`) with optional `-a`/`--account` for one-off identity overrides.
- **Quick Browser Shortcuts**: `gha open` (alias `browse`) opens the current repository, PRs (`--pr`), issues (`--issues`), or actions (`--actions`) in your default browser.
- **Interactive Account Switching**: `gha use` / `gha switch` supports interactive menus, while `gha whoami` and `gha status` report current identity and login status.
- **AI Agent Skill Integration**: built-in `skills/gha-github-workflow` guides AI coding assistants (Antigravity, Claude Code, Cursor, Copilot CLI) to prioritize `gha` for conflict-free multi-account workflows.
- **Credential Helper Isolation**: manages Git credential helpers for HTTPS repositories, preventing conflicts with the system keychain.
- **Local & Global Identity Scopes**: write Git identity locally per-repo or globally.
- **Diagnostic & Completion Support**: built-in `gha doctor` diagnostics and shell completions for Bash, Zsh, and Fish.

## Prerequisites

- GitHub CLI is **not** required beforehand; configuration and credentials are managed directly by Go.
- `git` must be installed and available in `PATH` for `gha clone` and Git operations.
- Building from source requires Go 1.26.1 or higher (as specified in `go.mod`).

First-time OAuth login requires a GitHub OAuth App public client ID with Device Flow enabled:

```bash
export GH_GHA_CLIENT_ID="your_oauth_app_client_id"
```

If neither the environment variable nor `--client-id` is provided, `gha login` prompts for it interactively and persists it in `config.yaml` (`oauth_client_id`) for future logins. The login flow uses GitHub Device Flow: `gha` displays a one-time verification code and URL, then attempts to open the default system browser automatically.

### Creating a GitHub OAuth Client ID

1. Log in to GitHub and open **Settings**.
2. Navigate to **Developer settings** → **OAuth Apps**.
3. Click **New OAuth App** (or **Register a new application**).
4. Fill in the application details:
   - **Application name**: e.g. `gha-account`
   - **Homepage URL**: repository homepage (e.g. `https://github.com/zhongtait/gh-account`)
   - **Authorization callback URL**: can be the same homepage (callback is not used by Device Flow)
5. Check **Enable Device Flow**, then click **Register application**.
6. Copy the **Client ID** from the application details page. (Never check in or expose the Client Secret; `gha` only needs the public Client ID).

Official references:
- [Creating an OAuth app](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app)
- [Authorizing OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)

Configure the Client ID:

```bash
# macOS / Linux
export GH_GHA_CLIENT_ID="your_client_id"
gha login personal
```

```powershell
# Windows PowerShell
$env:GH_GHA_CLIENT_ID = "your_client_id"
gha login personal
```

Or pass it per command:

```bash
gha --client-id "your_client_id" login personal
```

## Installation

### Download Release Binaries (Recommended)

Visit the [Releases](https://github.com/zhongtait/gh-account/releases) page, download the archive matching your OS and CPU architecture, extract it, and place `gha` (or `gha.exe` on Windows) into your `PATH`.

Verify checksums using `SHA256SUMS`:

```bash
sha256sum -c SHA256SUMS --ignore-missing
# Or on macOS: shasum -a 256 -c SHA256SUMS
```

Alternatively, use the automated installer script (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/zhongtait/gh-account/refs/heads/master/scripts/install.sh | bash
```

Install a specific release version:

```bash
curl -fsSL https://raw.githubusercontent.com/zhongtait/gh-account/refs/heads/master/scripts/install.sh | bash -s -- v1.0.0
```

#### Windows Defender False-Positive Note

Windows Defender might occasionally flag pre-compiled Go binaries due to:
1. **Static Go Compilation** - standalone binary without dynamic runtime DLLs.
2. **Network Operations** - connects to the GitHub API for OAuth authentication.
3. **Unsigned Binary** - lacks an expensive commercial code-signing certificate.

This is a **false positive**. You can add an exclusion in Windows Defender, inspect the [source code](https://github.com/zhongtait/gh-account), or compile from source via `go build ./cmd/gha`.

### Build from Source

```bash
git clone https://github.com/zhongtait/gh-account.git
cd gh-account
make build
install -m 755 bin/gha "${HOME}/.local/bin/gha"
```

Verify your installation:

```bash
gha version
gha doctor
```

## Development & Testing

Local development requires Go 1.26.1+. Standard checks:

```bash
go test ./... -race -cover # Unit tests, race detector, and coverage
go vet ./...              # Static analysis
test -z "$(gofmt -l .)"  # Code formatting check
make build                # Build gha binary
```

The GitHub Actions [CI workflow](.github/workflows/ci.yml) enforces 90%+ statement coverage, formatting checks, `go vet`, race detector tests, and cross-platform compilation (Ubuntu, macOS, Windows).

## Release & Versioning

- Pushing a semver tag (e.g. `v1.2.3`) triggers a formal GitHub Release:
  ```bash
  git tag v1.2.3
  git push origin v1.2.3
  ```
- Alternatively, trigger the [Release workflow](.github/workflows/release.yml) manually via `workflow_dispatch`.

## Configuration

Default configuration directory: `~/.config/gha/`:

```text
~/.config/gha/
├── accounts.yaml       # Account list and Git identity metadata
├── config.yaml         # Git scope, OAuth Client ID, and runtime settings
├── auth.yaml           # AES-GCM encrypted OAuth tokens (0600 permissions)
└── auth.key            # Local encryption key (0600 permissions)
```

Override the configuration directory via environment variable or global flag:

```bash
export GH_GHA_CONFIG_DIR="${HOME}/.config/gha-dev"
gha --config-dir "${HOME}/.config/gha-dev" list
```

Configuration templates are available at [`configs/accounts.example.yaml`](configs/accounts.example.yaml) and [`configs/config.example.yaml`](configs/config.example.yaml).

## Command Reference

| Command | Description |
| --- | --- |
| `gha init` | Initialize configuration directories and default files |
| `gha add` | Add a new account manually |
| `gha list` | List configured accounts and their OAuth status |
| `gha use [alias]` | Switch active account and sync Git identity (interactive menu if omitted; alias `switch`) |
| `gha current` | Show active account, Git identity, and remote info (alias `whoami`) |
| `gha status` | Inspect active account login status against GitHub servers |
| `gha git <command...>` / `gha <git-cmd>` | Transparent Git command proxy (`gha push`, `gha commit -m ...`, supports `-a`/`--account`) |
| `gha git clone <repo>` | Git-compatible clone, equivalent to `gha clone` |
| `gha token [alias]` | Display, copy (`-c`), or inspect (`--check`/`--scopes`) access token validity & permissions |
| `gha gh <command...>` | GitHub CLI integration: auto-injects `GH_TOKEN` and `GH_HOST` for the account |
| `gha pr` / `gha issue` / `gha run` / `gha api` | Transparent GitHub CLI command shortcuts |
| `gha open [alias]` | Open repository or account in browser (supports `--pr`, `--issues`, `--actions`, alias `browse`) |
| `gha doctor` | Diagnose Git, configuration, and OAuth authentication health |
| `gha sync` | Sync Git identity based on current GitHub account profile |
| `gha auto` | Auto-detect account from repository remote URL |
| `gha login <alias>` | Log in via Device Flow or `--token`/`--with-token` PAT; auto-resolves email & display name |
| `gha clone <repo>` | Clone repository and inject credential helper (supports `-a`/`--account`, `-d`/`--dir`) |
| `gha logout <alias>` | Remove stored local credential for an account |
| `gha edit [alias]` | Edit account configuration in `$EDITOR` |
| `gha remote` | View or update Git remote URLs |
| `gha completion <shell>` | Generate shell completion script (`bash`, `zsh`, `fish`) |
| `gha version` | Print version information |

### Quick Examples

```bash
# Interactive account switcher (or use `gha switch`)
gha use

# Switch globally and update origin remote
gha use personal --global
gha use personal --update-remote
gha sync --global

# Login with automatic email & name detection (no manual typing)
gha login personal               # Browser Device Flow; fetches name and primary email automatically
gha login work --token ghp_xxxx  # Direct PAT login for automation / CI
gha login work --with-token < pat.txt # Read PAT securely from stdin

# Inspect and copy access tokens
gha token                        # Print token for active account
gha token work                   # Print token for work account
gha token -c                     # Copy token to clipboard
gha token --check                # Verify validity and scopes via GitHub API (checks `workflow` scope)
gha token work --scopes          # Inspect scopes for work account

# Auto-managed GitHub CLI (gh) execution (no manual gh installation required)
gha gh pr list                   # List PRs under active account
gha --account work gh pr create  # Create PR as work account
gha pr list                      # Shortcut to gh pr list
gha repo list                    # Shortcut to gh repo list
gha issue list                   # Shortcut to gh issue list
gha release download v1.0.0      # Shortcut to download release asset
gha api /user                    # Call GitHub REST API
gha gh path                      # Show path of active gh binary (system or managed)
gha gh install                   # Force install / update managed GitHub CLI binary

# Git transparent proxying and shorthand syntax
gha clone cli/cli                                      # Shorthand owner/repo clone
gha clone cli/cli -a personal -d ./my-cli              # Specify account and destination directory
gha clone https://github.com/OWNER/REPO.git            # Equivalent to gha git clone ...
gha status                                       # Equivalent to gha git status
gha push origin main                             # Equivalent to gha git push origin main
gha commit -m "feat: updates"                    # Equivalent to gha git commit -m ...
gha log -n 5                                     # Equivalent to gha git log -n 5
gha diff                                         # Equivalent to gha git diff

# Run one-off Git command as a different account (without altering config)
gha --account work push origin main
gha git -a personal commit -m "feat: updates"

# GHA features accessible under `gha git`
gha git use work                                 # Equivalent to gha use work
gha git whoami                                   # Equivalent to gha whoami / gha current

# Open repository or account pages in default browser
gha open                                         # Open repository page
gha open --pr                                    # Open Pull Requests page
gha open --issues                                # Open Issues page
gha open --actions                               # Open GitHub Actions page
gha open personal                                # Open personal profile page

# Clone private repository with specific account
gha clone https://github.com/OWNER/PRIVATE_REPO.git use personal
gha clone OWNER/PRIVATE_REPO -a personal

# Clone repository with automatic account detection
gha clone https://github.com/OWNER/PRIVATE_REPO.git
```

## Shell Completion

```bash
# zsh
gha completion zsh > "${fpath[1]}/_gha"

# bash
gha completion bash > "${HOME}/.local/share/bash-completion/completions/gha"

# fish
gha completion fish > "${HOME}/.config/fish/completions/gha.fish"
```

## AI Agent Integration (Agent Skill)

This project includes a native Skill specification for AI coding assistants (such as Google Antigravity, Claude Code, Cursor, GitHub Copilot CLI):
- **Specification File**: [`skills/gha-github-workflow/SKILL.md`](skills/gha-github-workflow/SKILL.md)
- **Key Capabilities**:
  - Automatically guides AI agents to prioritize `gha` commands (`gha git push`, `gha pr create`, `gha run list`, etc.) instead of bare `gh` or `git`.
  - Intelligently recognizes multi-account contexts (`gha list` / `gha status`) to prevent 403 Forbidden errors and credential contamination in multi-tenant environments.

## Project Structure

```text
gh-account/
├── cmd/                 # Cobra command handlers and CLI entrypoint
├── internal/            # Core services: Git, OAuth, gh engine, and remote handlers
├── skills/              # AI Agent skill definitions (SKILL.md)
├── configs/             # Configuration templates
├── scripts/             # Release and build scripts
├── .github/workflows/   # CI and Release workflows
├── Makefile
└── go.mod
```

## License

MIT License. See [`LICENSE`](LICENSE) for details.
