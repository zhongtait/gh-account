# gh-account

<p align="center">
  <a href="README.md">English</a> | <b>简体中文</b>
</p>

[![CI](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml)
[![Release](https://github.com/zhongtait/gh-account/actions/workflows/release.yml/badge.svg?event=push)](https://github.com/zhongtait/gh-account/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/zhongtait/gh-account?sort=semver)](https://github.com/zhongtait/gh-account/releases/latest)

`gha` 是一个使用纯 Go GitHub OAuth 和 Git 配置操作的多账号管理工具。它把 GitHub OAuth 登录状态、Git 提交身份和仓库 remote 设置集中到一个可复用的命令行流程中。

## 功能

- 管理多个 GitHub 账号配置
- 切换本地 OAuth 登录账号并同步 Git `user.name` / `user.email`
- **自动获取邮箱与真实姓名**：登录（OAuth 或 Token）成功后自动通过 GitHub API 读取主邮箱（含隐私邮箱识别）与个人昵称，**无需手动输入**，彻底杜绝提交邮箱输错导致的历史污染
- **支持 Token (PAT) 直接登录**：支持通过 `--token` 或 `--with-token`（从标准输入读取）直接导入 Personal Access Token，无需浏览器 Device Flow，非常适合已有 Token 或自动化脚本场景
- **Token 权限与状态自检**：`gha token --check`（或 `--scopes`）实时联网校验 Token 有效性与完整作用域（显式提示是否具备关键的 `workflow` 权限）
- **智能纠错建议**：当输错账号别名时（如 `gha use persnal`），基于编辑距离自动提示 `account "persnal" not found; did you mean "personal"?`
- **GitHub CLI (gh) 自动托管与透明透传**：系统未安装 `gh` 时**自动按需下载官方二进制**至 `~/.config/gha/bin/gh`，免去任何手动配置；全量 GitHub CLI 指令（`pr`、`issue`、`repo`、`run`、`release`、`api` 等）直接可用并自动注入选中多账号凭据
- **仓库简写与灵活克隆**：`gha clone owner/repo` 原生支持简写仓库格式，并支持 `-a`/`--account` 与 `-d`/`--dir` 参数指定账号与目标目录
- **直观账号认证状态**：`gha list` 清晰展示每个账号的 OAuth Token 登录状态（`logged in` / `no token`）
- **Git 命令透明兼容与代理**：`gha git clone` 等同于 `gha clone`；支持 `gha git <command>` 透明转发，并可通过 `--account` / `-a` 临时指定账号上下文
- **快速打开浏览器**：`gha open`（别名 `browse`）一键在系统浏览器中打开当前仓库或指定账号的 GitHub 页面
- **交互式账号切换与别名**：`gha use` / `gha switch` 支持交互式列出与选择账号，支持 `gha whoami` 与 `gha status` 快捷查看状态
- **AI Agent (智能体) 专属集成**：内置 `skills/gha-github-workflow`，让 AI 编程助手优先且原生使用 `gha`，避免与系统单账号 `gh` 产生权限冲突
- 通过 Git credential helper 让 HTTPS 的 `pull`、`fetch` 和 `push` 使用当前账号
- 按本地或全局范围写入 Git 身份
- 可选地按账号协议更新 `origin` remote
- 根据当前 GitHub 仓库的 `origin` remote 自动选择账号
- 提供登录、退出、编辑、诊断和 Shell completion

## 环境要求

- 不需要安装 GitHub CLI；常规账号和配置操作由程序直接读写 Git 文件
- 使用 `gha clone` 时需要额外安装 `git` 并确保它在 `PATH` 中
- 从源码构建需要 Go 1.26.1 或更高版本（以 `go.mod` 为准）

首次登录需要一个 GitHub OAuth App 的 public client ID，并在该 OAuth App 设置中启用 Device Flow。创建 OAuth App 后设置：

```bash
export GH_GHA_CLIENT_ID="你的 OAuth App Client ID"
```

如果没有设置环境变量或 `--client-id`，执行 `gha login` 时会交互式提示输入 Client ID，并保存到 `config.yaml` 的 `oauth_client_id` 字段，后续登录会自动复用。登录使用 GitHub Device Flow：`gha` 会在终端显示一次性验证码和验证地址，并尝试自动打开系统浏览器；如果无法打开，复制终端中的地址即可。OAuth 流程本身由 Go 代码通过 HTTPS 完成，不调用 `gh`。使用 `--no-browser` 可以关闭自动打开浏览器。GitHub OAuth App 不需要把 client secret 放进本地 CLI。

### 创建 GitHub OAuth Client ID

1. 登录 GitHub，打开 `Settings`。
2. 进入 `Developer settings` → `OAuth Apps`。
3. 点击 `New OAuth App`（首次创建时可能显示 `Register a new application`）。
4. 填写应用信息：
   - `Application name`：例如 `gha-account`。
   - `Homepage URL`：填写项目主页，例如 `https://github.com/zhongtait/gh-account`。
   - `Authorization callback URL`：Device Flow 不会使用回调，可以填写同一个项目主页。
5. 勾选 `Enable Device Flow`，然后点击 `Register application`。
6. 在应用详情页复制 `Client ID`。本项目只需要 Client ID，不要把 `Client Secret` 写入配置或提交到仓库。

官方文档：

- [Creating an OAuth app](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app)
- [Authorizing OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)

设置 Client ID：

```bash
# macOS / Linux
export GH_GHA_CLIENT_ID="你的 Client ID"
gha login personal
```

```powershell
# Windows PowerShell
$env:GH_GHA_CLIENT_ID = "你的 Client ID"
gha login personal
```

也可以只对单次命令生效：

```bash
gha --client-id "你的 Client ID" login personal
```

## 安装

### 下载 Release 二进制（推荐）

打开仓库的 [Releases](https://github.com/zhongtait/gh-account/releases) 页面，下载对应操作系统和架构的压缩包，解压后将 `gha`（Windows 为 `gha.exe`）放入 `PATH`。

每个 Release 同时提供 `SHA256SUMS`。下载后可以校验：

```bash
sha256sum -c SHA256SUMS --ignore-missing
# macOS 也可以使用：shasum -a 256 -c SHA256SUMS
```

也可以使用安装脚本自动识别 macOS/Linux 和 CPU 架构，并安装到 `~/.local/bin`：

```bash
curl -fsSL https://raw.githubusercontent.com/zhongtait/gh-account/refs/heads/master/scripts/install.sh | bash
```

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/zhongtait/gh-account/refs/heads/master/scripts/install.sh | bash -s -- v1.0.0
```

脚本会校验 `SHA256SUMS`，但不会自动修改 shell 配置文件；如果安装目录不在 `PATH`，按脚本输出将 `~/.local/bin` 加入 `~/.zshrc` 或 `~/.bashrc`。

#### Windows 安全提示

Windows Defender 可能会误报该程序为潜在威胁。这是因为：

1. **Go 静态编译** - 程序打包了所有依赖，没有外部 DLL
2. **网络操作** - 程序需要访问 GitHub API 进行 OAuth 认证
3. **未签名** - 目前没有代码签名证书

**这是误报**。你可以：

- 在 Windows Defender 中添加排除项
- 查看[源代码](https://github.com/zhongtait/gh-account)确认安全性
- 从源码自行编译：`go build ./cmd/gha`

我们已经向 Microsoft 提交了误报申请。

### 从源码安装

```bash
git clone https://github.com/zhongtait/gh-account.git
cd gh-account
make build
install -m 755 bin/gha "${HOME}/.local/bin/gha"
```

确认 `${HOME}/.local/bin` 已在 `PATH` 中，然后运行：

```bash
gha version
gha doctor
```

账号登录状态也可以通过 `gha login personal` 管理；编辑和退出账号分别使用 `gha edit personal` 与 `gha logout personal`。

## 开发与测试

本地开发需要 Go 1.26.1 或更高版本。常用检查命令：

```bash
go test ./... -race -cover # 单元测试、竞态检测与覆盖率
go vet ./...              # 静态检查
test -z "$(gofmt -l .)"  # 格式检查
make build                # 构建 gha
```

GitHub Actions 的 [CI workflow](.github/workflows/ci.yml) 会在 Pull Request 以及 `main`/`master` 推送时执行格式检查、`go vet`、单元测试，强制保持 90% 以上 statement coverage，运行 `go test -race`，并在 Ubuntu、macOS 和 Windows 上构建。

## 版本与发布

- 推送符合 `vX.Y.Z` 格式的 tag 会构建正式 Release。例如：
  ```bash
  git tag v1.2.3
  git push origin v1.2.3
  ```
- 也可以从 [Release workflow](.github/workflows/release.yml) 手动运行，填写版本号（例如 `v1.2.3`）；workflow 会在当前 commit 上创建 Release。
- 正式 Release 的版本号来自 tag 或手动输入；分支推送生成的临时包使用当前 commit SHA 的前 7 位。
- 每个正式 Release 都包含各平台压缩包和 `SHA256SUMS`，发布前会执行单元测试和 `go vet`。

## 配置

默认配置目录为 `~/.config/gha/`，包含：

```text
~/.config/gha/
├── accounts.yaml       # 账号和 Git 身份
├── config.yaml         # Git 范围、OAuth Client ID 与其他运行配置
├── auth.yaml           # 加密后的 OAuth token，权限为 0600
└── auth.key            # 本机加密密钥，权限为 0600
```

`auth.yaml` 中不会直接保存 access token，而是使用 `auth.key` 进行 AES-GCM 加密。请同时保护这两个文件；`gha logout` 删除的是本地 credential，不会撤销 GitHub 服务器端授权。

可以通过环境变量或全局参数指定其他目录：

```bash
export GH_GHA_CONFIG_DIR="${HOME}/.config/gha-dev"
gha --config-dir "${HOME}/.config/gha-dev" list
```

示例配置见 [`configs/accounts.example.yaml`](configs/accounts.example.yaml) 和 [`configs/config.example.yaml`](configs/config.example.yaml)。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `gha init` | 创建配置目录和默认文件 |
| `gha add` | 新增账号 |
| `gha list` | 列出账号 |
| `gha use [alias]` | 切换账号并同步 Git 身份（未指定参数时交互式选择，别名 `switch`） |
| `gha current` | 查看当前账号、Git 和 remote 状态（别名 `whoami`） |
| `gha status` | 查看当前账号在 GitHub 服务器端的登录状态 |
| `gha git <command...>` / `gha <git-command>` | Git 原生命令透明代理（`gha push` 等同于 `gha git push`，支持 `-a`/`--account` 指定临时账号） |
| `gha git clone <repo>` | Git 兼容克隆，等同于 `gha clone` |
| `gha token [alias]` | 查看、复制或检验 Token（支持 `-c` 复制剪贴板，`--check`/`--scopes` 校验有效性与权限作用域） |
| `gha gh <command...>` | GitHub CLI 深度集成：执行 `gh` 命令时自动注入对应账号的 `GH_TOKEN` 与 `GH_HOST` |
| `gha pr` / `gha issue` / `gha run` / `gha api` | GitHub CLI 快捷指令透传（自动注入当前激活账号凭据） |
| `gha open [alias]` | 在浏览器中打开当前仓库或账号主页（支持 `--pr`, `--issues`, `--actions`，别名 `browse`） |
| `gha doctor` | 检查 Git、配置和 OAuth 登录状态 |
| `gha sync` | 根据当前 GitHub 账号同步 Git 身份 |
| `gha auto` | 根据当前仓库 origin 自动选择账号，未登录时手动填写 |
| `gha login <alias>` | 登录并保存账号配置（支持 Device Flow 浏览器授权或 `--token`/`--with-token` PAT 直接登录；自动获取真实姓名与邮箱） |
| `gha clone <repo>` | 克隆仓库并自动注入本地凭据配置（支持 `-a`/`--account` 指定账号，`-d`/`--dir` 指定目录，或 `use <alias>` 语法） |
| `gha logout <alias>` | 删除本地保存的指定 OAuth credential |
| `gha edit [alias]` | 用 `$EDITOR` 编辑账号配置 |
| `gha remote` | 查看或处理 remote |
| `gha completion <shell>` | 生成补全脚本 |
| `gha version` | 显示版本 |

常用选项：

```bash
# 交互式切换账号（或直接 gha switch）
gha use

# 全局切换账号并更新 remote
gha use personal --global
gha use personal --update-remote
gha sync --global

# 账号登录与自动邮箱解析（无需手动输入 Email）
gha login personal               # 启动 Device Flow 授权，默认包含 repo, workflow, read:org, gist, user 核心权限
gha login work -s "repo,workflow,project" # 自定义或追加 OAuth 权限范围
gha login work --token ghp_xxxx  # 适用于已有 PAT 或 CI 场景，免浏览器直接验证并加密导入
gha login work --with-token < pat.txt # 从标准输入读取 Token

# 查看、提取与自检 OAuth / PAT Token
gha token                        # 打印当前激活账号的 Access Token
gha token work                   # 打印 work 账号的 Access Token
gha token -c                     # 复制当前账号 Token 到系统剪贴板
gha token --check                # 联网检测当前账号 Token 有效性与完整作用域（显式检查 workflow 和 read:org 权限）
gha token work --scopes          # 查看指定账号在 GitHub 拥有的 Scopes 权限

# GitHub CLI (gh) 自动托管与多账号上下文透传（本地无需提前安装 gh）
gha gh pr list                 # 自动以当前激活账号身份查看 PR 列表
gha --account work gh pr create # 临时指定 work 账号创建 PR
gha pr list                    # 快捷指令（直接无缝透传至 gh pr list）
gha repo list                  # 快捷指令（查看仓库列表）
gha issue list                 # 快捷指令（直接无缝透传至 gh issue list）
gha release download v1.0.0    # 快捷指令（下载 Release 资产）
gha api /user                  # 快捷调用 GitHub REST API
gha gh path                    # 查看当前生效的 gh 路径（system 还是 managed）
gha gh install                 # 主动下载安装/重装 gha 托管的官方 GitHub CLI 二进制

# Git 完整无感转移与简写支持（中间加不加 git 完全一致）
gha clone cli/cli                                      # 支持简写 owner/repo 克隆
gha clone cli/cli -a personal -d ./my-cli              # 指定克隆账号与目标目录
gha clone https://github.com/OWNER/REPO.git            # 等同于 gha git clone ...
gha status                                       # 等同于 gha git status
gha push origin main                             # 等同于 gha git push origin main
gha commit -m "feat: hello"                      # 等同于 gha git commit -m ...
gha log -n 5                                     # 等同于 gha git log -n 5
gha diff                                         # 等同于 gha git diff

# 临时以指定账号身份执行任意 Git 操作（不污染全局配置）
gha --account work push origin main
gha git -a personal commit -m "feat: updates"

# GHA 专属功能在 gha git 下同样可用
gha git use work                                 # 等同于 gha use work
gha git whoami                                   # 等同于 gha whoami / gha current

# 一键在浏览器中打开当前仓库、PR 列表、Issues、Actions 或个人主页
gha open                                         # 打开当前仓库页面
gha open --pr                                    # 打开当前仓库 PRs 页面
gha open --issues                                # 打开当前仓库 Issues 页面
gha open --actions                               # 打开当前仓库 Actions 流水线页面
gha open personal                                # 打开 personal 账号个人主页

# 使用指定账号克隆私有仓库
gha clone https://github.com/OWNER/PRIVATE_REPO.git use personal
gha clone OWNER/PRIVATE_REPO -a personal

# 自动选择账号（auto 可以省略）
gha clone https://github.com/OWNER/PRIVATE_REPO.git
```

`gha clone` 需要系统中存在 `git` 可执行文件。HTTPS 克隆时，程序只在 Git
进程中注入当前账号的 credential helper，不会把 access token 写入 remote URL；
克隆成功后，账号对应的 helper、`user.name` 和 `user.email` 会写入新仓库的
`.git/config`。`auto` 在现有凭据认证失败时会重新进入一次 OAuth 登录并重试。
SSH 地址仍使用 SSH key，不使用 OAuth token。

## Shell completion

```bash
# zsh
gha completion zsh > "${fpath[1]}/_gha"

# bash
gha completion bash > "${HOME}/.local/share/bash-completion/completions/gha"

# fish
gha completion fish > "${HOME}/.config/fish/completions/gha.fish"
```

## AI 编程助手与智能体集成 (Agent Skill)

本项目针对 AI 辅助编程（如 Google Antigravity、Claude Code、Cursor、GitHub Copilot CLI 等）内置了专属 Skill：
- **定义文件**：[`skills/gha-github-workflow/SKILL.md`](skills/gha-github-workflow/SKILL.md)
- **核心作用**：
  - 智能体在需要执行 Git 提交、推送、克隆、PR 查看/创建、Issue 管理、CI 运行状态跟踪时，会自动优先调用 `gha` 系列命令（如 `gha git push`、`gha pr create`、`gha run list` 等）。
  - 智能感知多账号状态（`gha list` / `gha status`），在多账号开发环境中自动选择或提示切换正确身份，彻底避免系统裸 `gh` 或单账号 Git 配置引起的 403 权限越权或身份污染。

## 项目结构

```text
gh-account/
├── cmd/                 # Cobra 命令与 cmd/gha 可执行入口
├── internal/            # 配置、原生 Git、OAuth、gh 引擎与 remote 服务
├── skills/              # 面向 AI Coding 智能体的技能规范 (SKILL.md)
├── configs/             # 配置示例
├── scripts/             # 发布构建脚本
├── .github/workflows/   # CI 与 Release workflow
├── Makefile
└── go.mod
```

说明：`gha edit` 仍会调用用户主动配置的 `$EDITOR`/`$VISUAL`，这是编辑器集成，不参与 OAuth、Git 或账号切换流程。

`gha auto` 不读取目录绑定来切换账号。它会读取当前仓库的 `origin`，例如 `https://github.com/zhongtait/gh-account.git` 会提取出 `github.com/zhongtait`，查找本地对应的 OAuth credential 和账号资料；如果没有本地登录 credential，则提示手动填写账号信息。

`gha use` 和 `gha auto` 还会为生效范围配置 Git credential helper。该 helper 从 `gha` 的加密凭据存储读取当前账号的 token，因此外部执行的 `git pull`、`git fetch` 和 `git push` 不会继续使用系统钥匙串中的其他 GitHub 账号。配置只保存 helper 命令和账号标识，不会把 token 写进 remote URL 或 Git 配置。

这套认证同步适用于 HTTPS remote。SSH remote 仍由 SSH key、`ssh-agent` 或 `core.sshCommand` 决定，不能通过 GitHub OAuth token 切换。

未登录时会提供两个选项：

1. 选择登录，执行与 `gha login` 相同的 OAuth 登录和账号资料保存流程，然后自动同步 Git 身份。
2. 选择手动指定，直接填写 GitHub Login、alias、Git Name、Email 和 protocol。

## 许可证

MIT，见 [`LICENSE`](LICENSE)。
