package utils

import "testing"

func TestRedactToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{
			name:  "GitHub personal access token",
			token: "ghp_123456789012345678901234567890123456",
			want:  "ghp_...3456",
		},
		{
			name:  "GitHub OAuth token",
			token: "gho_123456789012345678901234567890123456",
			want:  "gho_...3456",
		},
		{
			name:  "short token",
			token: "short",
			want:  "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactToken(tt.token); got != tt.want {
				t.Errorf("RedactToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedactSensitiveData(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "redact personal access token",
			text: "Failed to authenticate with token ghp_123456789012345678901234567890123456",
			want: "Failed to authenticate with token ghp_...3456",
		},
		{
			name: "redact OAuth token",
			text: "Using token gho_123456789012345678901234567890123456",
			want: "Using token gho_...3456",
		},
		{
			name: "redact authorization header",
			text: "Authorization: Bearer token_value",
			want: "Authorization: ***",
		},
		{
			name: "redact multiple tokens",
			text: "Token1: ghp_111111111111111111111111111111111111 Token2: gho_222222222222222222222222222222222222",
			want: "Token1: ghp_...1111 Token2: gho_...2222",
		},
		{
			name: "no sensitive data",
			text: "This is a normal log message",
			want: "This is a normal log message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactSensitiveData(tt.text); got != tt.want {
				t.Errorf("RedactSensitiveData() = %v, want %v", got, tt.want)
			}
		})
	}
}
