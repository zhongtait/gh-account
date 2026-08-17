package remote

import (
	"fmt"
	"net/url"
	"strings"
)

// Info describes a parsed GitHub remote.
type Info struct {
	Raw      string
	Protocol string
	Host     string
	Owner    string
	Repo     string
}

// Parse inspects a git remote URL.
func Parse(raw string) (Info, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Info{}, fmt.Errorf("empty remote url")
	}

	if strings.HasPrefix(raw, "git@") {
		// git@github.com:owner/repo.git
		withoutPrefix := strings.TrimPrefix(raw, "git@")
		parts := strings.SplitN(withoutPrefix, ":", 2)
		if len(parts) != 2 {
			return Info{}, fmt.Errorf("invalid ssh remote: %s", raw)
		}
		host := parts[0]
		path := strings.TrimSuffix(parts[1], ".git")
		owner, repo, err := splitOwnerRepo(path)
		if err != nil {
			return Info{}, err
		}
		return Info{Raw: raw, Protocol: "ssh", Host: host, Owner: owner, Repo: repo}, nil
	}

	if strings.HasPrefix(raw, "ssh://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Info{}, err
		}
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		owner, repo, err := splitOwnerRepo(path)
		if err != nil {
			return Info{}, err
		}
		return Info{Raw: raw, Protocol: "ssh", Host: u.Host, Owner: owner, Repo: repo}, nil
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Info{}, err
		}
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		owner, repo, err := splitOwnerRepo(path)
		if err != nil {
			return Info{}, err
		}
		protocol := "https"
		if u.Scheme == "http" {
			protocol = "http"
		}
		return Info{Raw: raw, Protocol: protocol, Host: u.Host, Owner: owner, Repo: repo}, nil
	}

	return Info{}, fmt.Errorf("unsupported remote url: %s", raw)
}

// RewriteOwner rebuilds a remote URL with a new owner while preserving protocol/host/repo.
func RewriteOwner(raw, newOwner string) (string, error) {
	info, err := Parse(raw)
	if err != nil {
		return "", err
	}
	info.Owner = newOwner
	return Build(info)
}

// Build creates a remote URL from structured info.
func Build(info Info) (string, error) {
	if info.Host == "" || info.Owner == "" || info.Repo == "" {
		return "", fmt.Errorf("incomplete remote info")
	}
	switch strings.ToLower(info.Protocol) {
	case "ssh":
		return fmt.Sprintf("git@%s:%s/%s.git", info.Host, info.Owner, info.Repo), nil
	case "http":
		return fmt.Sprintf("http://%s/%s/%s.git", info.Host, info.Owner, info.Repo), nil
	case "https", "":
		return fmt.Sprintf("https://%s/%s/%s.git", info.Host, info.Owner, info.Repo), nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", info.Protocol)
	}
}

func splitOwnerRepo(path string) (string, string, error) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid owner/repo path: %s", path)
	}
	return parts[0], parts[1], nil
}
