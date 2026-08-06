# gh-account

GitHub 多账号管理 CLI 工具，支持在不同账号之间快速切换。

## 功能特性

- 🔐 原生 GitHub OAuth 认证（无需 GitHub CLI）
- 🔄 快速切换 GitHub 账号
- 🌐 支持多个 GitHub 实例（github.com 和 GitHub Enterprise）
- 📁 目录级账号自动切换
- 🔑 集成 Git credential helper
- 📋 自动复制 OAuth 验证码到剪贴板

## 安装

```bash
go install github.com/zhongtait/gh-account/cmd/gha@latest
```

### Windows 安全提示

Windows Defender 可能会误报该程序为潜在威胁。这是因为：

1. **Go 静态编译** - 程序打包了所有依赖，没有外部 DLL
2. **网络操作** - 程序需要访问 GitHub API 进行 OAuth 认证
3. **未签名** - 目前没有代码签名证书

**这是误报**。你可以：

- 在 Windows Defender 中添加排除项
- 查看[源代码](https://github.com/zhongtait/gh-account)确认安全性
- 从源码自行编译：`go build ./cmd/gha`

我们已经向 Microsoft 提交了误报申请。

## 使用

### 登录账号

```bash
gh-account login
```

验证码会自动复制到剪贴板，在浏览器中粘贴即可。

### 查看当前账号

```bash
gh-account whoami
```

### 切换账号

```bash
gh-account switch <username>
```

### 自动切换

设置目录级账号：

```bash
cd ~/work/company-project
gh-account switch work-account
```

下次在该目录下进行 Git 操作时，会自动使用 `work-account`。

### 登出账号

```bash
gh-account logout [username]
```

## 工作原理

1. 使用 GitHub Device Flow OAuth 获取访问令牌（scope: `read:user,repo`）
2. 将凭据存储在 `~/.config/gh-account/credentials.json`
3. 配置 Git credential helper 自动提供凭据
4. 根据当前目录自动选择对应账号

## 安全性

- 所有凭据加密存储在本地
- 支持敏感信息脱敏，错误日志不会泄露 token
- 配置文件权限设置为 0600
- 不会将凭据发送到 GitHub 以外的任何服务器

## 许可证

MIT
