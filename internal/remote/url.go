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
	// User is the SSH user from an SCP-style or ssh:// remote. An empty user
	// means the remote did not specify one and Build will use Git's default
	// "git" user.
	User  string
	Host  string
	Owner string
	Repo  string
}

// Parse inspects a git remote URL.
func Parse(raw string) (Info, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Info{}, fmt.Errorf("empty remote url")
	}

	if !strings.Contains(raw, "://") && strings.Contains(raw, ":") {
		// SCP syntax: [user@]host:owner/repo.git. Bracketed IPv6 literals
		// ([::1]:owner/repo.git) and port suffixes (host:2222:...) must be
		// kept intact while splitting host from path.
		user := ""
		target := raw
		if at := strings.LastIndex(raw, "@"); at >= 0 {
			if at == 0 {
				return Info{}, fmt.Errorf("invalid ssh remote: %s", raw)
			}
			user = raw[:at]
			target = raw[at+1:]
		}
		host, path, ok := splitSCPTarget(target)
		if !ok {
			return Info{}, fmt.Errorf("invalid ssh remote: %s", raw)
		}
		path = trimGitSuffix(strings.Trim(path, "/"))
		owner, repo, err := splitOwnerRepo(path)
		if err != nil {
			return Info{}, err
		}
		return Info{Raw: raw, Protocol: "ssh", User: user, Host: host, Owner: owner, Repo: repo}, nil
	}

	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Info{}, err
		}
		if u.Host == "" {
			return Info{}, fmt.Errorf("remote host is required: %s", raw)
		}
		if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return Info{}, fmt.Errorf("remote query or fragment is not supported: %s", raw)
		}
		path := strings.Trim(u.Path, "/")
		path = trimGitSuffix(path)
		owner, repo, err := splitOwnerRepo(path)
		if err != nil {
			return Info{}, err
		}
		protocol := strings.ToLower(u.Scheme)
		switch protocol {
		case "ssh":
			user := ""
			if u.User != nil {
				user = u.User.Username()
			}
			return Info{Raw: raw, Protocol: protocol, User: user, Host: u.Host, Owner: owner, Repo: repo}, nil
		case "git", "http", "https":
			return Info{Raw: raw, Protocol: protocol, Host: u.Host, Owner: owner, Repo: repo}, nil
		default:
			return Info{}, fmt.Errorf("unsupported remote url: %s", raw)
		}
	}

	if !strings.Contains(raw, "://") && !strings.Contains(raw, ":") {
		path := trimGitSuffix(strings.Trim(raw, "/"))
		if owner, repo, err := splitOwnerRepo(path); err == nil {
			return Info{Raw: raw, Protocol: "https", Host: "github.com", Owner: owner, Repo: repo}, nil
		}
	}

	return Info{}, fmt.Errorf("unsupported remote url: %s", raw)
}

func trimGitSuffix(path string) string {
	if strings.HasSuffix(strings.ToLower(path), ".git") {
		return path[:len(path)-len(".git")]
	}
	return path
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
		user := strings.TrimSpace(info.User)
		if user == "" {
			user = "git"
		}
		// SCP syntax cannot represent an SSH port: writing
		// user@host:2222:owner/repo.git would make 2222 part of the path.
		// When the parsed host includes a port (or is an IPv6 literal), keep
		// the URL form so Git receives an unambiguous endpoint.
		if strings.Contains(info.Host, ":") {
			return fmt.Sprintf("ssh://%s@%s/%s/%s.git", user, info.Host, info.Owner, info.Repo), nil
		}
		return fmt.Sprintf("%s@%s:%s/%s.git", user, info.Host, info.Owner, info.Repo), nil
	case "http":
		return fmt.Sprintf("http://%s/%s/%s.git", info.Host, info.Owner, info.Repo), nil
	case "git":
		return fmt.Sprintf("git://%s/%s/%s.git", info.Host, info.Owner, info.Repo), nil
	case "https", "":
		return fmt.Sprintf("https://%s/%s/%s.git", info.Host, info.Owner, info.Repo), nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", info.Protocol)
	}
}

// AccountProtocol returns the protocol accepted by account profiles for a
// parsed remote protocol. The unauthenticated git:// and http:// transports
// are represented as HTTPS for account metadata and OAuth login purposes.
// An empty input uses the application's HTTPS default; unknown protocols are
// rejected by returning an empty string.
func AccountProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "", "git", "http", "https":
		return "https"
	case "ssh":
		return "ssh"
	default:
		return ""
	}
}

// splitSCPTarget splits an SCP-style target into its host and path parts.
// It understands bracketed IPv6 literals and ignores colons inside them.
func splitSCPTarget(target string) (host, path string, ok bool) {
	if strings.HasPrefix(target, "[") {
		end := strings.Index(target, "]")
		if end < 0 {
			return "", "", false
		}
		host = target[:end+1]
		rest := target[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", "", false
		}
		return host, rest[1:], true
	}
	parts := strings.SplitN(target, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
func splitOwnerRepo(path string) (string, string, error) {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid owner/repo path: %s", path)
	}
	return parts[0], parts[1], nil
}
