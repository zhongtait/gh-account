package config

import (
	"testing"
)

func TestAccountValidate(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid account with https",
			account: Account{
				Login:    "testuser",
				Hostname: "github.com",
				GitName:  "Test User",
				Email:    "test@example.com",
				Protocol: "https",
			},
			wantErr: false,
		},
		{
			name: "valid account with ssh",
			account: Account{
				Login:    "testuser",
				Hostname: "github.com",
				GitName:  "Test User",
				Email:    "test@example.com",
				Protocol: "ssh",
			},
			wantErr: false,
		},
		{
			name: "valid account without protocol",
			account: Account{
				Login:   "testuser",
				GitName: "Test User",
				Email:   "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing login",
			account: Account{
				GitName:  "Test User",
				Email:    "test@example.com",
				Protocol: "https",
			},
			wantErr: true,
			errMsg:  "login is required",
		},
		{
			name: "missing git_name",
			account: Account{
				Login:    "testuser",
				Email:    "test@example.com",
				Protocol: "https",
			},
			wantErr: true,
			errMsg:  "git_name is required",
		},
		{
			name: "missing email",
			account: Account{
				Login:    "testuser",
				GitName:  "Test User",
				Protocol: "https",
			},
			wantErr: true,
			errMsg:  "email is required",
		},
		{
			name: "invalid email format",
			account: Account{
				Login:    "testuser",
				GitName:  "Test User",
				Email:    "invalid-email",
				Protocol: "https",
			},
			wantErr: true,
			errMsg:  "email must be a valid email address",
		},
		{
			name: "invalid protocol",
			account: Account{
				Login:    "testuser",
				GitName:  "Test User",
				Email:    "test@example.com",
				Protocol: "ftp",
			},
			wantErr: true,
			errMsg:  "protocol must be 'https' or 'ssh'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.account.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Account.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Account.Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestCredentialValidate(t *testing.T) {
	tests := []struct {
		name       string
		credential Credential
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid credential with access_token",
			credential: Credential{
				Hostname:    "github.com",
				Login:       "testuser",
				AccessToken: "gho_test123456789",
			},
			wantErr: false,
		},
		{
			name: "valid credential with encrypted_token",
			credential: Credential{
				Hostname:       "github.com",
				Login:          "testuser",
				EncryptedToken: "encrypted_data",
			},
			wantErr: false,
		},
		{
			name: "missing hostname",
			credential: Credential{
				Login:       "testuser",
				AccessToken: "gho_test123456789",
			},
			wantErr: true,
			errMsg:  "hostname is required",
		},
		{
			name: "missing login",
			credential: Credential{
				Hostname:    "github.com",
				AccessToken: "gho_test123456789",
			},
			wantErr: true,
			errMsg:  "login is required",
		},
		{
			name: "missing both tokens",
			credential: Credential{
				Hostname: "github.com",
				Login:    "testuser",
			},
			wantErr: true,
			errMsg:  "either access_token or encrypted_token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.credential.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Credential.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Credential.Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
