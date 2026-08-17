package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gofrs/flock"
	"github.com/zhongtait/gh-account/internal/utils"
	"gopkg.in/yaml.v3"
)

// Store loads and persists gha configuration files.
type Store struct {
	Dir string
	mu  sync.RWMutex
}

type storeLocker interface {
	Lock() error
	Unlock() error
}

var (
	storeConfigDir       = utils.ConfigDir
	storeMkdirAll        = os.MkdirAll
	storeStat            = os.Stat
	storeReadFile        = os.ReadFile
	storeWriteFile       = os.WriteFile
	storeChmod           = os.Chmod
	storeRandReader      = rand.Reader
	storeAtomicWriteFile = utils.AtomicWriteFile
	storeAbs             = filepath.Abs
	storeExpandHome      = utils.ExpandHome
	storeYAMLMarshal     = yaml.Marshal
	storeNewCipher       = aes.NewCipher
	storeNewGCM          = cipher.NewGCM
	newStoreLock         = func(path string) storeLocker { return flock.New(path) }
)

// NewStore creates a store rooted at the given config directory.
func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

// DefaultStore creates a store using the default config location.
func DefaultStore() (*Store, error) {
	dir, err := storeConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStore(dir), nil
}

// EnsureInitialized creates the config directory and default files if needed.
func (s *Store) EnsureInitialized() error {
	return s.withWriteLock(func() error {
		if err := storeMkdirAll(s.Dir, 0o700); err != nil {
			return err
		}
		accountsPath := utils.AccountsPath(s.Dir)
		if _, err := storeStat(accountsPath); errors.Is(err, os.ErrNotExist) {
			if err := s.saveAccounts(DefaultAccounts()); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		configPath := utils.ConfigPath(s.Dir)
		if _, err := storeStat(configPath); errors.Is(err, os.ErrNotExist) {
			if err := s.saveConfig(DefaultConfig()); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		return nil
	})
}

// LoadAccounts reads accounts.yaml.
func (s *Store) LoadAccounts() (AccountsFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadAccounts()
}

func (s *Store) loadAccounts() (AccountsFile, error) {
	path := utils.AccountsPath(s.Dir)
	data, err := storeReadFile(path)
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
		account.Hostname = normalizeHostname(account.Hostname)
		file.Accounts[alias] = account
	}
	if err := validateAccounts(file); err != nil {
		return AccountsFile{}, fmt.Errorf("validate accounts.yaml: %w", err)
	}
	return file, nil
}

// SaveAccounts writes accounts.yaml.
func (s *Store) SaveAccounts(file AccountsFile) error {
	if file.Accounts == nil {
		file.Accounts = map[string]Account{}
	}
	if err := validateAccounts(file); err != nil {
		return err
	}
	return s.withWriteLock(func() error { return s.saveAccounts(file) })
}

func (s *Store) saveAccounts(file AccountsFile) error {
	return writeYAML(utils.AccountsPath(s.Dir), file)
}

// LoadConfig reads config.yaml.
func (s *Store) LoadConfig() (ConfigFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadConfig()
}

func (s *Store) loadConfig() (ConfigFile, error) {
	path := utils.ConfigPath(s.Dir)
	data, err := storeReadFile(path)
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
	return s.withWriteLock(func() error { return s.saveConfig(file) })
}

func (s *Store) saveConfig(file ConfigFile) error {
	return writeYAML(utils.ConfigPath(s.Dir), file)
}

// UpdateConfig performs a load-modify-save operation while holding the store's
// in-process and cross-process write locks.
func (s *Store) UpdateConfig(update func(*ConfigFile) error) error {
	return s.withWriteLock(func() error {
		file, err := s.loadConfig()
		if err != nil {
			return err
		}
		if err := update(&file); err != nil {
			return err
		}
		if file.DefaultScope == "" {
			file.DefaultScope = "local"
		}
		return s.saveConfig(file)
	})
}

// LoadAuth reads the local OAuth credential store. Missing auth.yaml is
// treated as an empty store so existing installations upgrade seamlessly.
func (s *Store) LoadAuth() (AuthFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadAuth()
}

func (s *Store) loadAuth() (AuthFile, error) {
	path := utils.AuthPath(s.Dir)
	data, err := storeReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultAuth(), nil
	}
	if err != nil {
		return AuthFile{}, err
	}
	var file AuthFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return AuthFile{}, fmt.Errorf("parse auth.yaml: %w", err)
	}
	if file.Credentials == nil {
		file.Credentials = map[string]Credential{}
	}
	for key, credential := range file.Credentials {
		if credential.EncryptedToken == "" {
			continue
		}
		plain, decryptErr := s.decryptToken(key, credential.EncryptedToken)
		if decryptErr != nil {
			return AuthFile{}, fmt.Errorf("decrypt auth credential %q: %w", key, decryptErr)
		}
		credential.AccessToken = plain
		file.Credentials[key] = credential
	}
	return file, nil
}

// GetCredential returns the stored OAuth credential for a GitHub identity.
func (s *Store) GetCredential(hostname, login string) (Credential, bool, error) {
	hostname = normalizeHostname(hostname)
	login = strings.TrimSpace(login)
	if login == "" {
		return Credential{}, false, nil
	}
	auth, err := s.LoadAuth()
	if err != nil {
		return Credential{}, false, err
	}
	for _, credential := range auth.Credentials {
		if strings.EqualFold(normalizeHostname(credential.Hostname), hostname) && strings.EqualFold(strings.TrimSpace(credential.Login), login) {
			return credential, strings.TrimSpace(credential.AccessToken) != "", nil
		}
	}
	return Credential{}, false, nil
}

// SaveAuth writes OAuth credentials with owner-only permissions.
func (s *Store) SaveAuth(file AuthFile) error {
	return s.withWriteLock(func() error { return s.saveAuth(file) })
}

// UpdateAuth performs a load-modify-save operation while holding the store's
// in-process and cross-process write locks.
func (s *Store) UpdateAuth(update func(*AuthFile) error) error {
	return s.withWriteLock(func() error {
		file, err := s.loadAuth()
		if err != nil {
			return err
		}
		if err := update(&file); err != nil {
			return err
		}
		return s.saveAuth(file)
	})
}

func (s *Store) saveAuth(file AuthFile) error {

	if file.Credentials == nil {
		file.Credentials = map[string]Credential{}
	}
	toSave := AuthFile{Active: file.Active, Credentials: map[string]Credential{}}
	for key, credential := range file.Credentials {
		if credential.AccessToken != "" {
			encrypted, encryptErr := s.encryptToken(key, credential.AccessToken)
			if encryptErr != nil {
				return encryptErr
			}
			credential.EncryptedToken = encrypted
			credential.AccessToken = ""
		}
		toSave.Credentials[key] = credential
	}
	data, err := storeYAMLMarshal(toSave)
	if err != nil {
		return err
	}
	path := utils.AuthPath(s.Dir)
	// AtomicWriteFile preserves an existing file's mode. Tighten legacy or
	// manually altered auth files before replacement so the new file is never
	// created with permissions broader than owner-only.
	if err := storeChmod(path, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("set auth file permissions: %w", err)
	}
	return storeAtomicWriteFile(path, data, 0o600)
}

func (s *Store) authKey(create bool) ([]byte, error) {
	path := utils.AuthKeyPath(s.Dir)
	data, err := storeReadFile(path)
	if err == nil {
		if len(data) != 32 {
			return nil, errors.New("auth.key must contain 32 bytes")
		}
		if chmodErr := storeChmod(path, 0o600); chmodErr != nil {
			return nil, fmt.Errorf("set auth key permissions: %w", chmodErr)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, fmt.Errorf("read auth key: %w", err)
	}
	data = make([]byte, 32)
	if _, err := io.ReadFull(storeRandReader, data); err != nil {
		return nil, fmt.Errorf("generate auth key: %w", err)
	}
	if err := storeMkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}
	if err := storeWriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write auth key: %w", err)
	}
	if err := storeChmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("set auth key permissions: %w", err)
	}
	return data, nil
}

func (s *Store) encryptToken(aad, token string) (string, error) {
	key, err := s.authKey(true)
	if err != nil {
		return "", fmt.Errorf("get auth key: %w", err)
	}
	block, err := storeNewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := storeNewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(storeRandReader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(token), []byte(aad))
	return "v1:" + base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (s *Store) decryptToken(aad, encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "v1:") {
		return "", errors.New("unsupported encrypted token format")
	}
	key, err := s.authKey(false)
	if err != nil {
		return "", fmt.Errorf("get auth key: %w", err)
	}
	data, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	block, err := storeNewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := storeNewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("encrypted token is truncated")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return "", errors.New("invalid auth key or encrypted token")
	}
	return string(plain), nil
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
	account.Protocol = normalizeProtocol(account.Protocol)
	account.Hostname = normalizeHostname(account.Hostname)
	account.Login = strings.TrimSpace(account.Login)
	account.GitName = strings.TrimSpace(account.GitName)
	account.Email = strings.TrimSpace(account.Email)
	if err := account.Validate(); err != nil {
		return err
	}

	return s.withWriteLock(func() error {
		file, err := s.loadAccounts()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				file = DefaultAccounts()
			} else {
				return err
			}
		}
		file.Accounts[alias] = account
		return s.saveAccounts(file)
	})
}

// RemoveAccount deletes an account alias.
func (s *Store) RemoveAccount(alias string) error {
	return s.withWriteLock(func() error {
		file, err := s.loadAccounts()
		if err != nil {
			return err
		}
		if _, ok := file.Accounts[alias]; !ok {
			return fmt.Errorf("account %q not found", alias)
		}
		delete(file.Accounts, alias)
		return s.saveAccounts(file)
	})
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

	absPath, err := storeAbs(path)
	if err != nil {
		return "", "", false, err
	}

	var (
		bestPrefix string
		bestAlias  string
		found      bool
	)

	for rawDir, binding := range cfg.Directories {
		expanded, err := storeExpandHome(rawDir)
		if err != nil {
			return "", "", false, err
		}
		absDir, err := storeAbs(expanded)
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
	data, err := storeYAMLMarshal(value)
	if err != nil {
		return err
	}
	return storeAtomicWriteFile(path, data, 0o644)
}

func (s *Store) withWriteLock(fn func() error) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := storeMkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	fileLock := newStoreLock(filepath.Join(s.Dir, ".gha.lock"))
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("lock config store: %w", err)
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); err == nil && unlockErr != nil {
			err = fmt.Errorf("unlock config store: %w", unlockErr)
		}
	}()
	return fn()
}

func validateAccounts(file AccountsFile) error {
	for alias, account := range file.Accounts {
		if strings.TrimSpace(alias) == "" {
			return errors.New("account alias is required")
		}
		if err := account.Validate(); err != nil {
			return fmt.Errorf("account %q: %w", alias, err)
		}
	}
	return nil
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

func normalizeHostname(hostname string) string {
	hostname = strings.TrimSpace(hostname)
	lower := strings.ToLower(hostname)
	if strings.HasPrefix(lower, "https://") {
		hostname = hostname[len("https://"):]
	} else if strings.HasPrefix(lower, "http://") {
		hostname = hostname[len("http://"):]
	}
	hostname = strings.TrimSuffix(hostname, "/")
	if hostname == "" {
		return "github.com"
	}
	return hostname
}
