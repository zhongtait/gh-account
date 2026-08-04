package config

// Account describes a managed GitHub identity.
type Account struct {
	Login    string `yaml:"login"`
	Hostname string `yaml:"hostname,omitempty"`
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
	DefaultScope  string                      `yaml:"default_scope"`
	UpdateRemote  bool                        `yaml:"update_remote"`
	OAuthClientID string                      `yaml:"oauth_client_id,omitempty"`
	Directories   map[string]DirectoryBinding `yaml:"directories,omitempty"`
}

// Credential is an OAuth credential stored for a GitHub account.
// It is kept separate from accounts.yaml so account metadata can be edited
// without exposing or rewriting access tokens.
type Credential struct {
	Hostname       string `yaml:"hostname"`
	Login          string `yaml:"login"`
	AccessToken    string `yaml:"access_token,omitempty"`
	EncryptedToken string `yaml:"encrypted_token,omitempty"`
	TokenType      string `yaml:"token_type,omitempty"`
	Scope          string `yaml:"scope,omitempty"`
}

// AuthFile is the local OAuth credential store.
type AuthFile struct {
	Active      string                `yaml:"active"`
	Credentials map[string]Credential `yaml:"credentials"`
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
	}
}

// DefaultAuth returns an empty OAuth credential document.
func DefaultAuth() AuthFile {
	return AuthFile{Credentials: map[string]Credential{}}
}
