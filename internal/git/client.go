package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type gitLockFile interface {
	Chmod(os.FileMode) error
	WriteString(string) (int, error)
	Sync() error
	Close() error
}

var (
	gitAbs         = filepath.Abs
	gitStat        = os.Stat
	gitReadFile    = os.ReadFile
	gitUserHomeDir = os.UserHomeDir
	gitMkdirAll    = os.MkdirAll
	gitOpenFile    = func(name string, flag int, perm os.FileMode) (gitLockFile, error) {
		return os.OpenFile(name, flag, perm)
	}
	gitRemove        = os.Remove
	gitRename        = os.Rename
	gitConfigCommand = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, "git", args...)
		command.Dir = dir
		return command.Output()
	}
)

// Scope controls whether git config is written locally or globally.
type Scope string

const (
	ScopeLocal  Scope = "local"
	ScopeGlobal Scope = "global"
)

// Identity is the commit author identity.
type Identity struct {
	Name  string
	Email string
}

// Client abstracts git operations used by gha.
type Client interface {
	IsRepo(ctx context.Context) (bool, error)
	TopLevel(ctx context.Context) (string, error)
	GetIdentity(ctx context.Context, scope Scope) (Identity, error)
	SetIdentity(ctx context.Context, scope Scope, identity Identity) error
	GetRemoteURL(ctx context.Context, name string) (string, error)
	SetRemoteURL(ctx context.Context, name, url string) error
	CurrentBranch(ctx context.Context) (string, error)
}

// CredentialClient configures Git's credential helper for a scope.
type CredentialClient interface {
	SetCredentialHelper(ctx context.Context, scope Scope, command, accountKey string) error
}

// NativeClient implements the subset of Git repository/config operations
// required by gha. It reads and writes ordinary config files directly, and
// invokes Git to resolve conditional includes and check URL reset support.
type NativeClient struct {
	Dir string
}

// NewNativeClient creates a Git client rooted at dir.
func NewNativeClient(dir string) *NativeClient {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return &NativeClient{Dir: dir}
}

// IsRepo reports whether Dir is inside a Git work tree.
func (c *NativeClient) IsRepo(ctx context.Context) (bool, error) {
	_, _, found, err := c.repo()
	return found, err
}

// TopLevel returns the repository root.
func (c *NativeClient) TopLevel(ctx context.Context) (string, error) {
	root, _, found, err := c.repo()
	if err != nil {
		return "", fmt.Errorf("check git repository: %w", err)
	}
	if !found {
		return "", errors.New("not inside a git repository")
	}
	return root, nil
}

// GetIdentity reads user.name and user.email for the given scope. It resolves
// [include] directives in the selected config file the way Git does, so an
// identity inherited from an included file is reported instead of "(unset)".
func (c *NativeClient) GetIdentity(ctx context.Context, scope Scope) (Identity, error) {
	file, err := c.loadConfigResolved(scope)
	if err != nil {
		return Identity{}, fmt.Errorf("load git config: %w", err)
	}
	identity := Identity{Name: file.get("user.name"), Email: file.get("user.email")}
	if file.hasConditionalIncludes() {
		nameValues, err := c.configValuesWithGit(ctx, scope, "user.name")
		if err != nil {
			return Identity{}, err
		}
		emailValues, err := c.configValuesWithGit(ctx, scope, "user.email")
		if err != nil {
			return Identity{}, err
		}
		identity.Name = lastValue(nameValues)
		identity.Email = lastValue(emailValues)
	}
	return identity, nil
}

// SetIdentity writes user.name and user.email for the given scope.
func (c *NativeClient) SetIdentity(ctx context.Context, scope Scope, identity Identity) error {
	if err := c.updateConfig(scope, func(file *gitConfig) {
		file.setIdentity(identity)
	}); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// SetCredentialHelper configures gha as the only credential helper for the
// selected Git config scope. An empty helper value resets helpers inherited
// from broader scopes, so a stale system credential cannot win first.
func (c *NativeClient) SetCredentialHelper(ctx context.Context, scope Scope, command, accountKey string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("credential helper command is required")
	}
	if strings.TrimSpace(accountKey) == "" {
		return errors.New("credential account key is required")
	}
	if err := c.updateConfig(scope, func(file *gitConfig) {
		// The empty helper must come after includes in this config scope so it
		// also resets helpers contributed by an included file. Append both keys
		// after every include so a later include cannot override the account.
		file.setMultiAfterIncludes("credential.helper", []string{"", strings.TrimSpace(command)})
		file.removeAll("gha.account-key")
		file.appendSection("gha", "account-key = "+quoteGitValue(strings.TrimSpace(accountKey)))
	}); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// GetRemoteURL returns the first remote URL after the last empty URL entry.
// Git treats an empty entry as a reset of URLs inherited from earlier config.
func (c *NativeClient) GetRemoteURL(ctx context.Context, name string) (string, error) {
	if name == "" {
		name = "origin"
	}
	file, err := c.loadConfigResolved(ScopeLocal)
	if err != nil {
		return "", fmt.Errorf("load git config: %w", err)
	}
	key := "remote." + strings.ToLower(strings.TrimSpace(name)) + ".url"
	values := file.getAll(key)
	if file.hasConditionalIncludes() {
		values, err = c.configValuesWithGit(ctx, ScopeLocal, key)
		if err != nil {
			return "", err
		}
	}
	value := firstRemoteURL(values)
	if value == "" {
		return "", fmt.Errorf("remote %q not found", name)
	}
	return value, nil
}

// SetRemoteURL replaces a remote's URL list, including inherited URLs. Explicit
// pushurl entries remain unchanged, as with git remote set-url without --push.
func (c *NativeClient) SetRemoteURL(ctx context.Context, name, remoteURL string) error {
	if name == "" {
		name = "origin"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("remote name is required")
	}
	if strings.TrimSpace(remoteURL) == "" {
		return errors.New("remote URL is required")
	}
	if err := requireRemoteURLReset(ctx); err != nil {
		return err
	}
	if err := c.updateConfig(ScopeLocal, func(file *gitConfig) {
		// Reset after all includes: prepending the new URL changes fetches but
		// leaves every inherited URL as an additional implicit push target.
		file.setMultiAfterIncludes("remote."+strings.ToLower(name)+".url", []string{"", strings.TrimSpace(remoteURL)})
	}); err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return nil
}

// Git before 2.46 treats an empty URL as another target instead of a reset.
// Check before writing so older installations cannot acquire a broken remote.
func requireRemoteURLReset(ctx context.Context) error {
	output, err := gitConfigCommand(ctx, "", "--version")
	if err != nil {
		return fmt.Errorf("rewriting remote URLs requires Git 2.46 or newer: %w", err)
	}
	match := gitVersionPattern.FindStringSubmatch(string(output))
	if match != nil {
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		if major > 2 || major == 2 && minor >= 46 {
			return nil
		}
	}
	return fmt.Errorf("rewriting remote URLs requires Git 2.46 or newer; found %q", strings.TrimSpace(string(output)))
}

// CurrentBranch returns the symbolic branch from HEAD. Detached HEAD returns
// an empty branch, matching the previous git CLI behavior.
func (c *NativeClient) CurrentBranch(ctx context.Context) (string, error) {
	_, gitDir, found, err := c.repo()
	if err != nil {
		return "", fmt.Errorf("check git repository: %w", err)
	}
	if !found {
		return "", errors.New("not inside a git repository")
	}
	data, err := gitReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", fmt.Errorf("read HEAD: %w", err)
	}
	head := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(head, prefix) {
		return strings.TrimPrefix(head, prefix), nil
	}
	return "", nil
}

func (c *NativeClient) repo() (root, gitDir string, found bool, err error) {
	start, err := gitAbs(c.Dir)
	if err != nil {
		return "", "", false, fmt.Errorf("get absolute path: %w", err)
	}
	if info, statErr := gitStat(start); statErr == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	for current := start; ; current = filepath.Dir(current) {
		marker := filepath.Join(current, ".git")
		info, statErr := gitStat(marker)
		if statErr == nil {
			if info.IsDir() {
				return current, marker, true, nil
			}
			data, readErr := gitReadFile(marker)
			if readErr != nil {
				return "", "", false, fmt.Errorf("read .git file: %w", readErr)
			}
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(strings.ToLower(line), "gitdir:") {
				value := strings.TrimSpace(line[len("gitdir:"):])
				if !filepath.IsAbs(value) {
					value = filepath.Join(current, value)
				}
				return current, filepath.Clean(value), true, nil
			}
			return "", "", false, errors.New("invalid .git file")
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", false, fmt.Errorf("stat .git: %w", statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", false, nil
		}
	}
}

func (c *NativeClient) configPath(scope Scope) (string, error) {
	switch scope {
	case ScopeLocal:
		_, gitDir, found, err := c.repo()
		if err != nil {
			return "", fmt.Errorf("check git repository: %w", err)
		}
		if !found {
			return "", errors.New("not inside a git repository")
		}
		commonDir, err := commonGitDir(gitDir)
		if err != nil {
			return "", err
		}
		return filepath.Join(commonDir, "config"), nil
	case ScopeGlobal:
		if path := strings.TrimSpace(os.Getenv("GIT_CONFIG_GLOBAL")); path != "" {
			return path, nil
		}
		home, err := gitUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			return filepath.Join(xdg, "git", "config"), nil
		}
		return filepath.Join(home, ".gitconfig"), nil
	default:
		return "", fmt.Errorf("unsupported git scope %q", scope)
	}
}

func (c *NativeClient) loadConfig(scope Scope) (gitConfig, error) {
	path, err := c.configPath(scope)
	if err != nil {
		return gitConfig{}, fmt.Errorf("get config path: %w", err)
	}
	config, err := readGitConfig(path)
	if err != nil {
		return gitConfig{}, fmt.Errorf("read git config from %s: %w", path, err)
	}
	return config, nil
}

// loadConfigResolved reads the scope's config and inlines the files named by
// its unconditional [include] directives. Conditional includes are queried
// through Git itself so condition matching follows Git's current worktree and
// branch semantics.
func (c *NativeClient) loadConfigResolved(scope Scope) (gitConfig, error) {
	path, err := c.configPath(scope)
	if err != nil {
		return gitConfig{}, fmt.Errorf("get config path: %w", err)
	}
	return readGitConfigResolved(path, 0)
}

// configValuesWithGit asks Git to resolve a key when the selected config tree
// contains conditional includes. Git owns the full includeIf condition syntax,
// including gitdir, worktree, onbranch, and hasconfig conditions.
func (c *NativeClient) configValuesWithGit(ctx context.Context, scope Scope, key string) ([]string, error) {
	var args []string
	switch scope {
	case ScopeLocal:
		args = []string{"config", "--local", "--includes", "--null", "--get-all", key}
	case ScopeGlobal:
		args = []string{"config", "--global", "--includes", "--null", "--get-all", key}
	default:
		return nil, fmt.Errorf("unsupported git scope %q", scope)
	}
	output, err := gitConfigCommand(ctx, c.Dir, args...)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 && len(output) == 0 {
				return nil, nil
			}
			if detail := strings.TrimSpace(string(exitErr.Stderr)); detail != "" {
				return nil, fmt.Errorf("resolve conditional git config %s: %s", key, detail)
			}
		}
		return nil, fmt.Errorf("resolve conditional git config %s: %w", key, err)
	}
	if len(output) == 0 {
		return nil, nil
	}
	values := strings.Split(string(output), "\x00")
	if values[len(values)-1] == "" {
		values = values[:len(values)-1]
	}
	return values, nil
}

func firstRemoteURL(values []string) string {
	first := ""
	for _, value := range values {
		if value == "" {
			first = ""
		} else if first == "" {
			first = value
		}
	}
	return first
}

func lastValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
func (c *NativeClient) updateConfig(scope Scope, update func(*gitConfig)) error {
	path, err := c.configPath(scope)
	if err != nil {
		return fmt.Errorf("get config path: %w", err)
	}
	if err := gitMkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	lockPath := path + ".lock"
	lock, err := gitOpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create git config lock %s: %w", lockPath, err)
	}
	defer gitRemove(lockPath)
	perm := os.FileMode(0o644)
	if info, statErr := gitStat(path); statErr == nil {
		perm = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		lock.Close()
		return fmt.Errorf("stat git config %s: %w", path, statErr)
	}
	if err := lock.Chmod(perm); err != nil {
		lock.Close()
		return fmt.Errorf("set git config lock permissions: %w", err)
	}
	file, err := readGitConfig(path)
	if err != nil {
		lock.Close()
		return fmt.Errorf("read git config from %s: %w", path, err)
	}
	update(&file)
	data := strings.Join(file.lines, "\n")
	if !strings.HasSuffix(data, "\n") {
		data += "\n"
	}
	written, err := lock.WriteString(data)
	if err != nil {
		lock.Close()
		return fmt.Errorf("write git config lock: %w", err)
	}
	if written != len(data) {
		lock.Close()
		return fmt.Errorf("write git config lock: %w", io.ErrShortWrite)
	}
	if err := lock.Sync(); err != nil {
		lock.Close()
		return fmt.Errorf("sync git config lock: %w", err)
	}
	if err := lock.Close(); err != nil {
		return fmt.Errorf("close git config lock: %w", err)
	}
	if err := gitRename(lockPath, path); err != nil {
		return fmt.Errorf("replace git config %s: %w", path, err)
	}
	return nil
}

func commonGitDir(gitDir string) (string, error) {
	data, err := gitReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, os.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", fmt.Errorf("read git commondir: %w", err)
	}
	common := strings.TrimSpace(string(data))
	if common == "" {
		return "", errors.New("git commondir is empty")
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}
	return filepath.Clean(common), nil
}

type gitConfig struct {
	lines []string
}

var (
	sectionPattern    = regexp.MustCompile(`^\s*\[([^]]+)\]\s*$`)
	keyPattern        = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*(?:=|\s+)\s*(.*?)\s*$`)
	gitVersionPattern = regexp.MustCompile(`^git version ([0-9]+)\.([0-9]+)\.`)
)

func readGitConfig(path string) (gitConfig, error) {
	data, err := gitReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return gitConfig{lines: []string{}}, nil
	}
	if err != nil {
		return gitConfig{}, fmt.Errorf("read file: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, strings.TrimSuffix(scanner.Text(), "\r"))
	}
	if err := scanner.Err(); err != nil {
		return gitConfig{}, fmt.Errorf("scan config file: %w", err)
	}
	return gitConfig{lines: lines}, nil
}

// maxIncludeDepth bounds [include] recursion so a self-including or mutually
// including config file cannot loop forever.
const maxIncludeDepth = 10

// readGitConfigResolved reads path and recursively inlines the files named by
// [include] directives. Values from included files appear at the point of the
// directive, so later entries (including the including file's own keys) keep
// Git's last-value-wins behavior. Missing include targets are ignored: Git
// treats them as empty during reads.
func readGitConfigResolved(path string, depth int) (gitConfig, error) {
	file, err := readGitConfig(path)
	if err != nil {
		return gitConfig{}, fmt.Errorf("read git config from %s: %w", path, err)
	}
	if depth >= maxIncludeDepth {
		return file, nil
	}
	resolved := gitConfig{lines: make([]string, 0, len(file.lines))}
	section := ""
	for _, line := range file.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			resolved.lines = append(resolved.lines, line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		keyMatch := keyPattern.FindStringSubmatch(trimmed)
		if section != "include" || keyMatch == nil || strings.ToLower(keyMatch[1]) != "path" {
			resolved.lines = append(resolved.lines, line)
			continue
		}
		includePath := unquoteGitValue(keyMatch[2])
		if includePath == "" {
			continue
		}
		if includePath == "~" || strings.HasPrefix(includePath, "~/") || strings.HasPrefix(includePath, `~\`) {
			expanded, expandErr := gitExpandHome(includePath)
			if expandErr != nil {
				resolved.lines = append(resolved.lines, line)
				continue
			}
			includePath = expanded
		}
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(path), includePath)
		}
		included, includeErr := readGitConfigResolved(includePath, depth+1)
		if includeErr != nil {
			// A missing or unreadable include is not fatal for reads: Git
			// simply contributes no values from it.
			continue
		}
		resolved.lines = append(resolved.lines, included.lines...)
	}
	return resolved, nil
}

func gitExpandHome(path string) (string, error) {
	home, err := gitUserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func (f gitConfig) getAll(wanted string) []string {
	section := ""
	var values []string
	for _, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(strings.TrimSpace(line), ";") {
			continue
		}
		if section+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			values = append(values, unquoteGitValue(match[2]))
		}
	}
	return values
}

func (f gitConfig) get(wanted string) string {
	section := ""
	value := ""
	for _, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(strings.TrimSpace(line), ";") {
			continue
		}
		if section+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			value = unquoteGitValue(match[2])
		}
	}
	return value
}

func (f *gitConfig) set(wanted, value string) {
	parts := strings.Split(wanted, ".")
	section := strings.Join(parts[:len(parts)-1], ".")
	key := parts[len(parts)-1]
	currentSection := ""
	last := -1
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			currentSection = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && currentSection+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			last = index
		}
	}
	formatted := key + " = " + quoteGitValue(value)
	if last >= 0 {
		f.lines[last] = formatted
		return
	}
	if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
		f.lines = append(f.lines, "")
	}
	f.lines = append(f.lines, "["+formatSection(section)+"]", formatted)
}

// mustAppendAfterIncludes reports whether any of the given keys currently sit
// before the last [include] or [includeIf] path. In that position an included
// file could override them, so a plain in-place update would not take effect.
func (f *gitConfig) mustAppendAfterIncludes(keys ...string) bool {
	lastInclude := f.lastIncludePathIndex()
	if lastInclude < 0 {
		return false
	}
	for _, key := range keys {
		if f.lastKeyIndex(key) < lastInclude {
			return true
		}
	}
	return false
}

// appendSection appends a new section, and its entries, at the end of the
// file. Writing it last makes the values win over anything an include
// directive contributed earlier.
func (f *gitConfig) appendSection(section string, entries ...string) {
	if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
		f.lines = append(f.lines, "")
	}
	f.lines = append(f.lines, "["+formatSection(section)+"]")
	f.lines = append(f.lines, entries...)
}

// setIdentity makes the selected identity effective when the config contains
// include directives. An included file is read at the directive's position,
// so an earlier user.name or user.email in the parent file can be overridden
// by the include. In that case append one parent [user] section after all
// includes; subsequent updates will replace these last values in place.
func (f *gitConfig) setIdentity(identity Identity) {
	if f.mustAppendAfterIncludes("user.name", "user.email") {
		f.appendSection("user",
			"name = "+quoteGitValue(identity.Name),
			"email = "+quoteGitValue(identity.Email),
		)
		return
	}
	f.set("user.name", identity.Name)
	f.set("user.email", identity.Email)
}

func (f *gitConfig) lastIncludePathIndex() int {
	section := ""
	last := -1
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = match[1]
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if isIncludeSection(section) && match != nil && strings.EqualFold(match[1], "path") && unquoteGitValue(match[2]) != "" {
			last = index
		}
	}
	return last
}

func isIncludeSection(section string) bool {
	section = canonicalSection(section)
	return section == "include" || strings.HasPrefix(section, "includeif.")
}

func (f *gitConfig) lastKeyIndex(wanted string) int {
	section := ""
	last := -1
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && section+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			last = index
		}
	}
	return last
}

// keyIndices returns every line index that defines wanted, in file order.
func (f *gitConfig) keyIndices(wanted string) []int {
	section := ""
	var indices []int
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			section = canonicalSection(match[1])
			continue
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && section+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			indices = append(indices, index)
		}
	}
	return indices
}

func (f gitConfig) hasConditionalIncludes() bool {
	for _, line := range f.lines {
		match := sectionPattern.FindStringSubmatch(line)
		if match != nil && strings.HasPrefix(canonicalSection(match[1]), "includeif.") {
			return true
		}
	}
	return false
}

// dropEmptySections removes section headers that no longer contain any keys,
// so rewriting a key does not leave a dangling "[remote "origin"]" behind.
func dropEmptySections(lines []string) []string {
	result := make([]string, 0, len(lines))
	for index, line := range lines {
		if sectionPattern.MatchString(line) {
			hasKey := false
			for _, next := range lines[index+1:] {
				if sectionPattern.MatchString(next) {
					break
				}
				// Preserve any non-comment content, including valueless boolean
				// keys and syntax the lightweight parser does not understand.
				trimmed := strings.TrimSpace(next)
				if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
					hasKey = true
					break
				}
			}
			if !hasKey {
				continue
			}
		}
		result = append(result, line)
	}
	return result
}

func (f *gitConfig) setMulti(wanted string, values []string) {
	parts := strings.Split(wanted, ".")
	section := strings.Join(parts[:len(parts)-1], ".")
	key := parts[len(parts)-1]

	filtered := make([]string, 0, len(f.lines)+len(values))
	currentSection := ""
	for _, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			currentSection = canonicalSection(match[1])
		}
		match := keyPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match != nil && currentSection+"."+strings.ToLower(match[1]) == strings.ToLower(wanted) {
			continue
		}
		filtered = append(filtered, line)
	}
	f.lines = filtered

	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, key+" = "+quoteGitValue(value))
	}

	targetSection := canonicalSection(section)
	sectionStart := -1
	sectionEnd := len(f.lines)
	currentSection = ""
	for index, line := range f.lines {
		if match := sectionPattern.FindStringSubmatch(line); match != nil {
			if sectionStart >= 0 && sectionEnd == len(f.lines) {
				sectionEnd = index
			}
			currentSection = canonicalSection(match[1])
			if currentSection == targetSection {
				sectionStart = index
				sectionEnd = len(f.lines)
			}
		}
	}
	if sectionStart < 0 {
		if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
			f.lines = append(f.lines, "")
		}
		f.lines = append(f.lines, "["+formatSection(section)+"]")
		f.lines = append(f.lines, formatted...)
		return
	}

	// Insert into the last matching section, after its existing contents.
	insertAt := sectionEnd
	f.lines = append(f.lines, make([]string, len(formatted))...)
	copy(f.lines[insertAt+len(formatted):], f.lines[insertAt:len(f.lines)-len(formatted)])
	copy(f.lines[insertAt:insertAt+len(formatted)], formatted)
}

// setMultiAfterIncludes removes the key's definitions in this file, then adds
// the replacement values at EOF. This is needed for credential.helper: its
// empty value resets every helper accumulated before it, including helpers
// supplied by included files.
func (f *gitConfig) setMultiAfterIncludes(wanted string, values []string) {
	f.removeAll(wanted)
	parts := strings.Split(wanted, ".")
	section := strings.Join(parts[:len(parts)-1], ".")
	key := parts[len(parts)-1]
	entries := make([]string, 0, len(values))
	for _, value := range values {
		entries = append(entries, key+" = "+quoteGitValue(value))
	}
	f.appendSection(section, entries...)
}

func (f *gitConfig) removeAll(wanted string) {
	indices := f.keyIndices(wanted)
	if len(indices) == 0 {
		return
	}
	remove := make(map[int]struct{}, len(indices))
	for _, index := range indices {
		remove[index] = struct{}{}
	}
	filtered := make([]string, 0, len(f.lines)-len(indices))
	for index, line := range f.lines {
		if _, drop := remove[index]; !drop {
			filtered = append(filtered, line)
		}
	}
	f.lines = dropEmptySections(filtered)
}

func canonicalSection(section string) string {
	section = strings.TrimSpace(section)
	if quote := strings.Index(section, `"`); quote >= 0 {
		section = strings.TrimSpace(section[:quote]) + "." + strings.Trim(section[quote:], `"`)
	}
	return strings.ToLower(strings.TrimSpace(section))
}

func formatSection(section string) string {
	parts := strings.SplitN(section, ".", 2)
	if len(parts) == 2 {
		return parts[0] + ` "` + parts[1] + `"`
	}
	return section
}

func quoteGitValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "#;\t\n\r") {
		return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	}
	return value
}

func unquoteGitValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		value = strings.TrimSuffix(strings.TrimPrefix(value, `"`), `"`)
		value = strings.ReplaceAll(strings.ReplaceAll(value, `\"`, `"`), `\\`, `\`)
	}
	return value
}

// ParseScope converts a CLI scope string into a Scope value.
func ParseScope(raw string, fallback Scope) (Scope, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		if fallback == "" {
			return ScopeLocal, nil
		}
		return fallback, nil
	}
	switch Scope(value) {
	case ScopeLocal, ScopeGlobal:
		return Scope(value), nil
	default:
		return "", fmt.Errorf("invalid scope %q (expected local or global)", raw)
	}
}
