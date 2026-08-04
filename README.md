# gh-account

`gha` 是一个基于 [GitHub CLI](https://cli.github.com/) 的 GitHub 多账号管理工具。它把 GitHub CLI 当前登录账号、Git 提交身份和仓库 remote 设置集中到一个可复用的命令行流程中。

## 功能

- 管理多个 GitHub 账号配置
- 切换 `gh` 登录账号并同步 Git `user.name` / `user.email`
- 按本地或全局范围写入 Git 身份
- 可选地按账号协议更新 `origin` remote
- 根据目录自动选择账号
- 提供登录、退出、编辑、诊断和 Shell completion

## 环境要求

- 已安装并登录的 [GitHub CLI](https://cli.github.com/)
- Git
- 从源码构建需要 Go 1.26.1 或更高版本（以 `go.mod` 为准）

## 安装

### 下载 Release 二进制（推荐）

打开仓库的 [Releases](https://github.com/zhongtait/gh-account/releases) 页面，下载对应操作系统和架构的压缩包，解压后将 `gha`（Windows 为 `gha.exe`）放入 `PATH`。

每个 Release 同时提供 `SHA256SUMS`。下载后可以校验：

```bash
sha256sum -c SHA256SUMS --ignore-missing
# macOS 也可以使用：shasum -a 256 -c SHA256SUMS
```

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

## 配置

默认配置目录为 `~/.config/gha/`，包含：

```text
~/.config/gha/
├── accounts.yaml
└── config.yaml
```

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
| `gha doctor` | 检查 GitHub CLI、Git 和配置 |
| `gha sync` | 根据当前 GitHub 账号同步 Git 身份 |
| `gha auto` | 根据目录绑定自动选择账号 |
| `gha login <alias>` | 登录并保存账号配置 |
| `gha logout <alias>` | 退出指定账号 |
| `gha edit [alias]` | 用 `$EDITOR` 编辑账号配置 |
| `gha remote` | 查看或处理 remote |
| `gha completion <shell>` | 生成补全脚本 |
| `gha version` | 显示版本 |

常用选项：

```bash
gha use personal --global
gha use personal --update-remote
gha sync --global
```

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
├── internal/            # 配置、Git、GitHub CLI 和 remote 服务
├── configs/              # 配置示例
├── scripts/              # 发布构建脚本
├── .github/workflows/    # CI 与 Release workflow
├── Makefile
└── go.mod
```

## 许可证

MIT，见 [`LICENSE`](LICENSE)。
