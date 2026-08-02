package config

// Account describes a managed GitHub identity.
type Account struct {
	Login    string `yaml:"login"`
	GitName  string `yaml:"git_name"`
	Email    string `yaml:"email"`
	Protocol string `yaml:"protocol"`
}

// AccountsFile is the on-disk accounts.yaml structure.
type AccountsFile struct {
	Accounts map[string]Account `yaml:"accounts"`
}

// DirectoryBinding maps a workspace path to an account alias.
type DirectoryBinding struct {
	Account string `yaml:"account"`
}

// ConfigFile is the on-disk config.yaml structure.
type ConfigFile struct {
	DefaultScope string                      `yaml:"default_scope"`
	UpdateRemote bool                        `yaml:"update_remote"`
	Directories  map[string]DirectoryBinding `yaml:"directories"`
}

// DefaultAccounts returns an empty accounts document.
func DefaultAccounts() AccountsFile {
	return AccountsFile{Accounts: map[string]Account{}}
}

// DefaultConfig returns the default application configuration.
func DefaultConfig() ConfigFile {
	return ConfigFile{
		DefaultScope: "local",
		UpdateRemote: false,
		Directories:  map[string]DirectoryBinding{},
	}
}
