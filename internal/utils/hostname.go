package utils

import "strings"

// NormalizeHostname trims URL prefixes and trailing slashes from a GitHub
// hostname, defaulting to github.com for empty values.
func NormalizeHostname(hostname string) string {
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
	return strings.ToLower(hostname)
}
