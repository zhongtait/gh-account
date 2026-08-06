package config

import (
	"fmt"
	"strings"
)

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

// Validate checks if an Account has all required fields and valid values.
func (a Account) Validate() error {
	if strings.TrimSpace(a.Login) == "" {
		return fmt.Errorf("login is required")
	}
	if strings.TrimSpace(a.GitName) == "" {
		return fmt.Errorf("git_name is required")
	}
	if strings.TrimSpace(a.Email) == "" {
		return fmt.Errorf("email is required")
	}
	// Validate email format
	if !strings.Contains(a.Email, "@") {
		return fmt.Errorf("email must be a valid email address")
	}
	// Validate protocol
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	if protocol != "" && protocol != "https" && protocol != "ssh" {
		return fmt.Errorf("protocol must be 'https' or 'ssh', got %q", a.Protocol)
	}
	return nil
}

// Validate checks if a Credential has required fields.
func (c Credential) Validate() error {
	if strings.TrimSpace(c.Hostname) == "" {
		return fmt.Errorf("hostname is required")
	}
	if strings.TrimSpace(c.Login) == "" {
		return fmt.Errorf("login is required")
	}
	if strings.TrimSpace(c.AccessToken) == "" && strings.TrimSpace(c.EncryptedToken) == "" {
		return fmt.Errorf("either access_token or encrypted_token is required")
	}
	return nil
}
