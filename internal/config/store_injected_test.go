package config

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/utils"
)

type fakeStoreLock struct {
	lockErr   error
	unlockErr error
}

func (f *fakeStoreLock) Lock() error   { return f.lockErr }
func (f *fakeStoreLock) Unlock() error { return f.unlockErr }

func restoreStoreHooks(t *testing.T) {
	t.Helper()
	configDir, mkdir, stat := storeConfigDir, storeMkdirAll, storeStat
	read, write, chmod := storeReadFile, storeWriteFile, storeChmod
	randReader, atomicWrite := storeRandReader, storeAtomicWriteFile
	abs, expand, marshal := storeAbs, storeExpandHome, storeYAMLMarshal
	newCipher, newGCM, newLock := storeNewCipher, storeNewGCM, newStoreLock
	t.Cleanup(func() {
		storeConfigDir, storeMkdirAll, storeStat = configDir, mkdir, stat
		storeReadFile, storeWriteFile, storeChmod = read, write, chmod
		storeRandReader, storeAtomicWriteFile = randReader, atomicWrite
		storeAbs, storeExpandHome, storeYAMLMarshal = abs, expand, marshal
		storeNewCipher, storeNewGCM, newStoreLock = newCipher, newGCM, newLock
	})
}

func TestInjectedDefaultAndInitializationErrors(t *testing.T) {
	restoreStoreHooks(t)
	storeConfigDir = func() (string, error) { return "", errors.New("config dir") }
	if _, err := DefaultStore(); err == nil {
		t.Fatal("DefaultStore ignored config error")
	}
	storeConfigDir = utils.ConfigDir

	originalMkdir := storeMkdirAll
	calls := 0
	storeMkdirAll = func(path string, perm os.FileMode) error {
		calls++
		if calls == 2 {
			return errors.New("inner mkdir")
		}
		return originalMkdir(path, perm)
	}
	if err := NewStore(t.TempDir()).EnsureInitialized(); err == nil || !strings.Contains(err.Error(), "inner mkdir") {
		t.Fatal(err)
	}
	storeMkdirAll = originalMkdir

	originalStat, originalAtomic := storeStat, storeAtomicWriteFile
	for _, failBase := range []string{"accounts.yaml", "config.yaml"} {
		store := NewStore(t.TempDir())
		storeStat = func(path string) (os.FileInfo, error) {
			if filepath.Base(path) == failBase {
				return nil, os.ErrPermission
			}
			return originalStat(path)
		}
		if err := store.EnsureInitialized(); err == nil {
			t.Errorf("stat %s error ignored", failBase)
		}
		storeStat = originalStat
	}
	for _, failBase := range []string{"accounts.yaml", "config.yaml"} {
		store := NewStore(t.TempDir())
		storeAtomicWriteFile = func(path string, data []byte, perm os.FileMode) error {
			if filepath.Base(path) == failBase {
				return errors.New("atomic " + failBase)
			}
			return originalAtomic(path, data, perm)
		}
		if err := store.EnsureInitialized(); err == nil || !strings.Contains(err.Error(), "atomic") {
			t.Errorf("write %s: %v", failBase, err)
		}
	}
	storeAtomicWriteFile, storeStat = originalAtomic, originalStat
}

func TestInjectedLoadSaveAndAccountErrors(t *testing.T) {
	restoreStoreHooks(t)
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("accounts: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.LoadAccounts()
	if err != nil || accounts.Accounts == nil {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	if err := os.WriteFile(utils.AccountsPath(store.Dir), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts, err = store.LoadAccounts()
	if err != nil || accounts.Accounts == nil {
		t.Fatalf("nil accounts = %+v, %v", accounts, err)
	}
	if _, err := store.LoadConfig(); err == nil {
		t.Fatal("missing config read succeeded")
	}
	if err := os.WriteFile(utils.ConfigPath(store.Dir), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := store.LoadConfig()
	if err != nil || cfg.DefaultScope != "local" {
		t.Fatalf("config defaults = %+v, %v", cfg, err)
	}
	if err := store.UpdateConfig(func(cfg *ConfigFile) error { cfg.DefaultScope = ""; return nil }); err != nil {
		t.Fatal(err)
	}

	originalRead := storeReadFile
	storeReadFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "auth.yaml" || filepath.Base(path) == "accounts.yaml" {
			return nil, os.ErrPermission
		}
		return originalRead(path)
	}
	if _, err := store.LoadAuth(); err == nil {
		t.Fatal("auth permission error ignored")
	}
	if err := store.UpsertAccount("alice", validAccount("alice")); err == nil {
		t.Fatal("upsert permission error ignored")
	}
	storeReadFile = originalRead

	originalAtomic := storeAtomicWriteFile
	storeAtomicWriteFile = func(string, []byte, os.FileMode) error { return errors.New("atomic") }
	if err := store.SaveAccounts(DefaultAccounts()); err == nil {
		t.Fatal("SaveAccounts ignored write failure")
	}
	if err := store.SaveConfig(DefaultConfig()); err == nil {
		t.Fatal("SaveConfig ignored write failure")
	}
	storeAtomicWriteFile = originalAtomic

	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials:\n  github.com|alice:\n    hostname: github.com\n    login: alice\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err := store.LoadAuth()
	if err != nil || auth.Credentials["github.com|alice"].AccessToken != "" {
		t.Fatalf("auth = %+v, %v", auth, err)
	}
	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err = store.LoadAuth()
	if err != nil || auth.Credentials == nil {
		t.Fatalf("nil auth = %+v, %v", auth, err)
	}
	if err := os.WriteFile(utils.AuthPath(store.Dir), []byte("credentials:\n  key:\n    encrypted_token: plain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadAuth(); err == nil || !strings.Contains(err.Error(), "decrypt auth credential") {
		t.Fatal(err)
	}
	missingStore := NewStore(t.TempDir())
	if err := missingStore.UpsertAccount("alice", validAccount("alice")); err != nil {
		t.Fatal(err)
	}
}

func TestInjectedAuthPersistenceAndCryptoErrors(t *testing.T) {
	restoreStoreHooks(t)
	store := NewStore(t.TempDir())
	if err := store.saveAuth(AuthFile{}); err != nil {
		t.Fatal(err)
	}
	auth := DefaultAuth()
	auth.Credentials["key"] = Credential{AccessToken: "token"}

	originalRead, originalMkdir := storeReadFile, storeMkdirAll
	storeReadFile = func(string) ([]byte, error) { return nil, os.ErrPermission }
	if _, err := store.authKey(true); err == nil || !strings.Contains(err.Error(), "read auth key") {
		t.Fatal(err)
	}
	storeReadFile = originalRead

	storeRandReader = strings.NewReader("")
	if _, err := store.authKey(true); err == nil || !strings.Contains(err.Error(), "generate auth key") {
		t.Fatal(err)
	}
	storeRandReader = strings.NewReader(strings.Repeat("k", 128))
	storeMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if _, err := store.authKey(true); err == nil || !strings.Contains(err.Error(), "create config") {
		t.Fatal(err)
	}
	storeMkdirAll = originalMkdir

	originalWrite, originalChmod := storeWriteFile, storeChmod
	storeWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if _, err := store.authKey(true); err == nil || !strings.Contains(err.Error(), "write auth key") {
		t.Fatal(err)
	}
	storeWriteFile = originalWrite
	storeChmod = func(string, os.FileMode) error { return errors.New("chmod") }
	if _, err := store.authKey(true); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatal(err)
	}
	storeChmod = originalChmod

	if err := os.MkdirAll(store.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(utils.AuthKeyPath(store.Dir), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	storeChmod = func(string, os.FileMode) error { return errors.New("chmod existing") }
	if _, err := store.authKey(false); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatal(err)
	}
	storeChmod = originalChmod

	storeNewCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("cipher") }
	if _, err := store.encryptToken("aad", "token"); err == nil || !strings.Contains(err.Error(), "create cipher") {
		t.Fatal(err)
	}
	if _, err := store.decryptToken("aad", "v1:AA"); err == nil || !strings.Contains(err.Error(), "create cipher") {
		t.Fatal(err)
	}
	storeNewCipher = aes.NewCipher
	storeNewGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("gcm") }
	if _, err := store.encryptToken("aad", "token"); err == nil || !strings.Contains(err.Error(), "create GCM") {
		t.Fatal(err)
	}
	if _, err := store.decryptToken("aad", "v1:AA"); err == nil || !strings.Contains(err.Error(), "create GCM") {
		t.Fatal(err)
	}
	storeNewGCM = cipher.NewGCM

	storeRandReader = strings.NewReader("")
	if _, err := store.encryptToken("aad", "token"); err == nil || !strings.Contains(err.Error(), "generate nonce") {
		t.Fatal(err)
	}
	storeRandReader = strings.NewReader(strings.Repeat("r", 128))
	storeNewCipher = aes.NewCipher

	storeReadFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "auth.key" {
			return nil, os.ErrPermission
		}
		return originalRead(path)
	}
	if err := store.saveAuth(auth); err == nil {
		t.Fatal("saveAuth ignored encryption error")
	}
	storeReadFile = originalRead

	originalMarshal := storeYAMLMarshal
	storeYAMLMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal") }
	if err := store.saveAuth(DefaultAuth()); err == nil {
		t.Fatal("saveAuth ignored marshal error")
	}
	if err := writeYAML("unused", DefaultConfig()); err == nil {
		t.Fatal("writeYAML ignored marshal error")
	}
	storeYAMLMarshal = originalMarshal
	originalAtomic := storeAtomicWriteFile
	storeAtomicWriteFile = func(string, []byte, os.FileMode) error { return errors.New("atomic") }
	if err := store.saveAuth(DefaultAuth()); err == nil {
		t.Fatal("saveAuth ignored atomic error")
	}
	storeAtomicWriteFile = originalAtomic
	storeChmod = func(path string, _ os.FileMode) error {
		if filepath.Base(path) == "auth.yaml" {
			return errors.New("chmod auth")
		}
		return originalChmod(path, 0o600)
	}
	if err := store.saveAuth(DefaultAuth()); err == nil || !strings.Contains(err.Error(), "auth file permissions") {
		t.Fatal(err)
	}
	storeChmod = originalChmod
}

func TestInjectedMatchDirectoryAndLockErrors(t *testing.T) {
	restoreStoreHooks(t)
	store := NewStore(t.TempDir())
	if err := store.EnsureInitialized(); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Directories = map[string]DirectoryBinding{}
	cfg.Directories["~/work"] = DirectoryBinding{Account: "alice"}
	if err := store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	originalAbs := storeAbs
	storeAbs = func(string) (string, error) { return "", errors.New("abs") }
	if _, _, _, err := store.MatchDirectory("."); err == nil {
		t.Fatal("path abs error ignored")
	}
	storeAbs = originalAbs
	originalExpand := storeExpandHome
	storeExpandHome = func(string) (string, error) { return "", errors.New("expand") }
	if _, _, _, err := store.MatchDirectory("."); err == nil {
		t.Fatal("expand error ignored")
	}
	storeExpandHome = originalExpand
	calls := 0
	storeAbs = func(path string) (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("binding abs")
		}
		return originalAbs(path)
	}
	if _, _, _, err := store.MatchDirectory("."); err == nil {
		t.Fatal("binding abs error ignored")
	}
	storeAbs = originalAbs

	newStoreLock = func(string) storeLocker { return &fakeStoreLock{lockErr: errors.New("lock")} }
	if err := store.withWriteLock(func() error { return nil }); err == nil || !strings.Contains(err.Error(), "lock config store") {
		t.Fatal(err)
	}
	newStoreLock = func(string) storeLocker { return &fakeStoreLock{unlockErr: errors.New("unlock")} }
	if err := store.withWriteLock(func() error { return nil }); err == nil || !strings.Contains(err.Error(), "unlock config store") {
		t.Fatal(err)
	}
}
