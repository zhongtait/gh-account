package utils

import (
	"regexp"
)

var (
	// GitHub token patterns
	tokenPatterns = []*regexp.Regexp{
		regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),                        // Personal access token
		regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`),                        // OAuth token
		regexp.MustCompile(`ghu_[a-zA-Z0-9]{36}`),                        // User token
		regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`),                        // Server token
		regexp.MustCompile(`ghr_[a-zA-Z0-9]{36}`),                        // Refresh token
		regexp.MustCompile(`github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59}`), // Fine-grained PAT
	}
)

// RedactToken masks a token, showing only the prefix and last 4 characters.
func RedactToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	prefix := token[:4]
	suffix := token[len(token)-4:]
	return prefix + "..." + suffix
}

// RedactSensitiveData redacts tokens from error messages and logs.
func RedactSensitiveData(text string) string {
	result := text
	for _, pattern := range tokenPatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			return RedactToken(match)
		})
	}
	// Also redact authorization headers - match everything after the colon
	result = regexp.MustCompile(`(?i)(Authorization|Bearer):\s+.*`).ReplaceAllString(result, "$1: ***")
	return result
}
