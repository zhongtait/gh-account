# gh-account

[![Unit tests](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/zhongtait/gh-account/actions/workflows/ci.yml)
[![Package artifacts](https://github.com/zhongtait/gh-account/actions/workflows/package.yml/badge.svg?branch=master)](https://github.com/zhongtait/gh-account/actions/workflows/package.yml)
[![Release](https://github.com/zhongtait/gh-account/actions/workflows/release.yml/badge.svg)](https://github.com/zhongtait/gh-account/actions/workflows/release.yml)
[![Latest release](https://img.shields.io/github/v/release/zhongtait/gh-account?sort=semver)](https://github.com/zhongtait/gh-account/releases/latest)

`gha` 是一个使用纯 Go GitHub OAuth 和 Git 配置操作的多账号管理工具。它把 GitHub OAuth 登录状态、Git 提交身份和仓库 remote 设置集中到一个可复用的命令行流程中。

## 功能

- 管理多个 GitHub 账号配置
- 切换本地 OAuth 登录账号并同步 Git `user.name` / `user.email`
- 通过 Git credential helper 让 HTTPS 的 `pull`、`fetch` 和 `push` 使用当前账号
- 按本地或全局范围写入 Git 身份
- 可选地按账号协议更新 `origin` remote
- 根据当前 GitHub 仓库的 `origin` remote 自动选择账号
- 提供登录、退出、编辑、诊断和 Shell completion
- 通过指定账号或自动选择账号克隆 GitHub 仓库，并把账号凭据和 Git 身份写入新仓库的本地配置

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
go test ./... -cover      # 单元测试与覆盖率（当前为 100%）
go test -race ./...       # 竞态检测
go vet ./...              # 静态检查
test -z "$(gofmt -l .)"  # 格式检查
make build                # 构建 gha
```

GitHub Actions 的 [CI workflow](.github/workflows/ci.yml) 会在 Pull Request 以及 `main`/`master` 推送时执行格式检查、`go vet`、单元测试，强制保持 100% statement coverage，并在 Ubuntu、macOS 和 Windows 上构建。

## 版本与发布

- 推送到 `main`/`master` 会生成临时 Package Artifact，不会自动创建 GitHub Release；Artifact 默认保留 14 天。
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
| `gha use <alias>` | 切换账号并同步 Git 身份 |
| `gha current` | 查看当前账号、Git 和 remote 状态 |
| `gha doctor` | 检查 Git、配置和 OAuth 登录状态 |
| `gha sync` | 根据当前 GitHub 账号同步 Git 身份 |
| `gha auto` | 根据当前仓库 origin 自动选择账号，未登录时手动填写 |
| `gha login <alias>` | 登录并保存账号配置 |
| `gha clone <repo> use <alias>` | 使用指定账号克隆仓库并写入仓库本地配置 |
| `gha clone <repo>` | 自动选择账号；没有可用凭据时进入 OAuth 登录 |
| `gha logout <alias>` | 删除本地保存的指定 OAuth credential |
| `gha edit [alias]` | 用 `$EDITOR` 编辑账号配置 |
| `gha remote` | 查看或处理 remote |
| `gha completion <shell>` | 生成补全脚本 |
| `gha version` | 显示版本 |

常用选项：

```bash
gha use personal --global
gha use personal --update-remote
gha sync --global

# 使用指定账号克隆私有仓库
gha clone https://github.com/OWNER/PRIVATE_REPO.git use personal

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

## 项目结构

```text
gh-account/
├── cmd/                 # Cobra 命令与 cmd/gha 可执行入口
├── internal/            # 配置、原生 Git、OAuth 和 remote 服务
├── configs/              # 配置示例
├── scripts/              # 发布构建脚本
├── .github/workflows/    # CI 与 Release workflow
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
