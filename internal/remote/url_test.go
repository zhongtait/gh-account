package remote

import "testing"

func TestParseAndRewriteHTTPS(t *testing.T) {
	info, err := Parse("https://github.com/work/demo.git")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Owner != "work" || info.Repo != "demo" || info.Protocol != "https" {
		t.Fatalf("unexpected info: %+v", info)
	}

	next, err := RewriteOwner("https://github.com/work/demo.git", "personal")
	if err != nil {
		t.Fatalf("RewriteOwner: %v", err)
	}
	if next != "https://github.com/personal/demo.git" {
		t.Fatalf("unexpected url: %s", next)
	}
}

func TestParseSSH(t *testing.T) {
	info, err := Parse("git@github.com:acme/app.git")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Protocol != "ssh" || info.User != "git" || info.Owner != "acme" || info.Repo != "app" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestParseRejectsNestedWebPaths(t *testing.T) {
	if _, err := Parse("https://github.com/acme/app/issues"); err == nil {
		t.Fatal("nested GitHub web path was accepted as a repository")
	}
}

func TestParseSCPEdgeCases(t *testing.T) {
	// Bracketed IPv6 literal host.
	info, err := Parse("git@[2001:db8::1]:acme/app.git")
	if err != nil {
		t.Fatalf("Parse ipv6 scp: %v", err)
	}
	if info.Protocol != "ssh" || info.User != "git" || info.Owner != "acme" || info.Repo != "app" {
		t.Fatalf("unexpected ipv6 info: %+v", info)
	}

	// Malformed bracketed host must be rejected.
	if _, err := Parse("git@[2001:db8::1:acme/app.git"); err == nil {
		t.Fatal("unclosed IPv6 literal was accepted")
	}

	// Empty path must be rejected.
	if _, err := Parse("git@github.com:"); err == nil {
		t.Fatal("empty SCP path was accepted")
	}
}
func TestParseSCPAndGitProtocol(t *testing.T) {
	info, err := Parse("deploy@github.com:acme/app.git")
	if err != nil {
		t.Fatalf("Parse scp: %v", err)
	}
	if info.Protocol != "ssh" || info.User != "deploy" || info.Host != "github.com" || info.Owner != "acme" || info.Repo != "app" {
		t.Fatalf("unexpected scp info: %+v", info)
	}
	if got, err := Build(info); err != nil || got != "deploy@github.com:acme/app.git" {
		t.Fatalf("Build custom-user scp = %q, %v", got, err)
	}
	if got, err := RewriteOwner("deploy@github.com:acme/app.git", "other"); err != nil || got != "deploy@github.com:other/app.git" {
		t.Fatalf("RewriteOwner custom-user scp = %q, %v", got, err)
	}

	withoutUser, err := Parse("github.com:acme/app.git")
	if err != nil {
		t.Fatalf("Parse scp without user: %v", err)
	}
	if withoutUser.Protocol != "ssh" || withoutUser.User != "" || withoutUser.Host != "github.com" || withoutUser.Owner != "acme" || withoutUser.Repo != "app" {
		t.Fatalf("unexpected scp-without-user info: %+v", withoutUser)
	}
	if got, err := Build(withoutUser); err != nil || got != "git@github.com:acme/app.git" {
		t.Fatalf("Build scp without user = %q, %v", got, err)
	}

	sshURL, err := Parse("ssh://deploy@git.example/acme/app.git")
	if err != nil {
		t.Fatalf("Parse ssh URL with user: %v", err)
	}
	if sshURL.User != "deploy" || sshURL.Host != "git.example" {
		t.Fatalf("unexpected ssh URL info: %+v", sshURL)
	}
	if got, err := Build(sshURL); err != nil || got != "deploy@git.example:acme/app.git" {
		t.Fatalf("Build ssh URL with user = %q, %v", got, err)
	}

	sshPort, err := Parse("ssh://deploy@git.example:2222/acme/app.git")
	if err != nil {
		t.Fatalf("Parse ssh URL with port: %v", err)
	}
	if got, err := Build(sshPort); err != nil || got != "ssh://deploy@git.example:2222/acme/app.git" {
		t.Fatalf("Build ssh URL with port = %q, %v", got, err)
	}

	gitInfo, err := Parse("git://github.com/acme/app.git")
	if err != nil {
		t.Fatalf("Parse git://: %v", err)
	}
	if gitInfo.Protocol != "git" || gitInfo.Owner != "acme" || gitInfo.Repo != "app" {
		t.Fatalf("unexpected git:// info: %+v", gitInfo)
	}

	got, err := Build(Info{Protocol: "git", Host: "github.com", Owner: "acme", Repo: "repo"})
	if err != nil {
		t.Fatalf("Build git://: %v", err)
	}
	if got != "git://github.com/acme/repo.git" {
		t.Fatalf("Build git:// = %q", got)
	}
}

func TestAccountProtocol(t *testing.T) {
	tests := map[string]string{
		"":        "https",
		"git":     "https",
		"http":    "https",
		"https":   "https",
		" SSH ":   "ssh",
		"invalid": "",
	}
	for input, want := range tests {
		if got := AccountProtocol(input); got != want {
			t.Errorf("AccountProtocol(%q) = %q, want %q", input, got, want)
		}
	}
}
func TestBuildProtocolsAndErrors(t *testing.T) {
	base := Info{Host: "github.com", Owner: "acme", Repo: "repo"}
	tests := []struct {
		protocol string
		want     string
	}{
		{protocol: "ssh", want: "git@github.com:acme/repo.git"},
		{protocol: "http", want: "http://github.com/acme/repo.git"},
		{protocol: "https", want: "https://github.com/acme/repo.git"},
		{protocol: "", want: "https://github.com/acme/repo.git"},
	}
	for _, test := range tests {
		base.Protocol = test.protocol
		got, err := Build(base)
		if err != nil || got != test.want {
			t.Errorf("Build(%q) = %q, %v; want %q", test.protocol, got, err, test.want)
		}
	}
	if _, err := Build(Info{Protocol: "https", Host: "github.com", Owner: "acme"}); err == nil {
		t.Fatal("Build accepted incomplete repository info")
	}
	if _, err := Build(Info{Protocol: "ftp", Host: "github.com", Owner: "acme", Repo: "repo"}); err == nil {
		t.Fatal("Build accepted unsupported protocol")
	}
}

func TestParseSupportedAndInvalidURLs(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		protocol string
		host     string
	}{
		{name: "http", url: "http://github.com/acme/repo", protocol: "http", host: "github.com"},
		{name: "ssh scheme", url: "ssh://git@git.example/acme/repo.git", protocol: "ssh", host: "git.example"},
	}
	for _, test := range tests {
		info, err := Parse(test.url)
		if err != nil {
			t.Errorf("Parse(%s): %v", test.name, err)
			continue
		}
		if info.Protocol != test.protocol || info.Host != test.host || info.Owner != "acme" || info.Repo != "repo" {
			t.Errorf("Parse(%s) = %+v", test.name, info)
		}
	}
	upper, err := Parse("SSH://deploy@git.example/acme/repo.GIT")
	if err != nil || upper.Protocol != "ssh" || upper.User != "deploy" || upper.Repo != "repo" {
		t.Fatalf("case-insensitive ssh URL = %+v, %v", upper, err)
	}
	for _, raw := range []string{"", "git@github.com", "git@github.com:only-owner", "ssh://", "ssh://%gh", "https://%gh", "https://github.com/acme", "ftp://github.com/acme/repo"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", raw)
		}
	}
	for _, raw := range []string{"https:///acme/repo", "https://github.com/acme/repo?ref=main"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", raw)
		}
	}
	if _, err := RewriteOwner("not-a-remote", "alice"); err == nil {
		t.Fatal("RewriteOwner accepted an invalid remote")
	}
}
