package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhongtait/gh-account/internal/utils"
	"gopkg.in/yaml.v3"
)

// Store loads and persists gha configuration files.
type Store struct {
	Dir string
}

// NewStore creates a store rooted at the given config directory.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// DefaultStore creates a store using the default config location.
func DefaultStore() (*Store, error) {
	dir, err := utils.ConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStore(dir), nil
}

// EnsureInitialized creates the config directory and default files if needed.
func (s *Store) EnsureInitialized() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}

	accountsPath := utils.AccountsPath(s.Dir)
	if _, err := os.Stat(accountsPath); errors.Is(err, os.ErrNotExist) {
		if err := s.SaveAccounts(DefaultAccounts()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	configPath := utils.ConfigPath(s.Dir)
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := s.SaveConfig(DefaultConfig()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// LoadAccounts reads accounts.yaml.
func (s *Store) LoadAccounts() (AccountsFile, error) {
	path := utils.AccountsPath(s.Dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return AccountsFile{}, err
	}
	var file AccountsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return AccountsFile{}, fmt.Errorf("parse accounts.yaml: %w", err)
	}
	if file.Accounts == nil {
		file.Accounts = map[string]Account{}
	}
	for alias, account := range file.Accounts {
		account.Protocol = normalizeProtocol(account.Protocol)
		file.Accounts[alias] = account
	}
	return file, nil
}

// SaveAccounts writes accounts.yaml.
func (s *Store) SaveAccounts(file AccountsFile) error {
	if file.Accounts == nil {
		file.Accounts = map[string]Account{}
	}
	return writeYAML(utils.AccountsPath(s.Dir), file)
}

// LoadConfig reads config.yaml.
func (s *Store) LoadConfig() (ConfigFile, error) {
	path := utils.ConfigPath(s.Dir)
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var file ConfigFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return ConfigFile{}, fmt.Errorf("parse config.yaml: %w", err)
	}
	if file.DefaultScope == "" {
		file.DefaultScope = "local"
	}
	if file.Directories == nil {
		file.Directories = map[string]DirectoryBinding{}
	}
	return file, nil
}

// SaveConfig writes config.yaml.
func (s *Store) SaveConfig(file ConfigFile) error {
	if file.DefaultScope == "" {
		file.DefaultScope = "local"
	}
	if file.Directories == nil {
		file.Directories = map[string]DirectoryBinding{}
	}
	return writeYAML(utils.ConfigPath(s.Dir), file)
}

// GetAccount returns an account by alias.
func (s *Store) GetAccount(alias string) (Account, error) {
	file, err := s.LoadAccounts()
	if err != nil {
		return Account{}, err
	}
	account, ok := file.Accounts[alias]
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", alias)
	}
	return account, nil
}

// UpsertAccount creates or updates an account alias.
func (s *Store) UpsertAccount(alias string, account Account) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return errors.New("alias is required")
	}
	if strings.TrimSpace(account.Login) == "" {
		return errors.New("login is required")
	}
	if strings.TrimSpace(account.GitName) == "" {
		return errors.New("git_name is required")
	}
	if strings.TrimSpace(account.Email) == "" {
		return errors.New("email is required")
	}
	account.Protocol = normalizeProtocol(account.Protocol)

	file, err := s.LoadAccounts()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			file = DefaultAccounts()
		} else {
			return err
		}
	}
	file.Accounts[alias] = account
	return s.SaveAccounts(file)
}

// RemoveAccount deletes an account alias.
func (s *Store) RemoveAccount(alias string) error {
	file, err := s.LoadAccounts()
	if err != nil {
		return err
	}
	if _, ok := file.Accounts[alias]; !ok {
		return fmt.Errorf("account %q not found", alias)
	}
	delete(file.Accounts, alias)
	return s.SaveAccounts(file)
}

// ListAliases returns sorted account aliases.
func (s *Store) ListAliases() ([]string, error) {
	file, err := s.LoadAccounts()
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(file.Accounts))
	for alias := range file.Accounts {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases, nil
}

// MatchDirectory finds the best directory binding for a path.
func (s *Store) MatchDirectory(path string) (string, string, bool, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return "", "", false, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", false, err
	}

	var (
		bestPrefix string
		bestAlias  string
		found      bool
	)

	for rawDir, binding := range cfg.Directories {
		expanded, err := utils.ExpandHome(rawDir)
		if err != nil {
			return "", "", false, err
		}
		absDir, err := filepath.Abs(expanded)
		if err != nil {
			return "", "", false, err
		}
		if absPath == absDir || strings.HasPrefix(absPath, absDir+string(os.PathSeparator)) {
			if !found || len(absDir) > len(bestPrefix) {
				bestPrefix = absDir
				bestAlias = binding.Account
				found = true
			}
		}
	}

	return bestAlias, bestPrefix, found, nil
}

func writeYAML(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizeProtocol(protocol string) string {
	p := strings.ToLower(strings.TrimSpace(protocol))
	switch p {
	case "", "https":
		return "https"
	case "ssh":
		return "ssh"
	default:
		return p
	}
}
