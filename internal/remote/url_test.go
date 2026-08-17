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
	if info.Protocol != "ssh" || info.Owner != "acme" || info.Repo != "app" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestParseRejectsNestedWebPaths(t *testing.T) {
	if _, err := Parse("https://github.com/acme/app/issues"); err == nil {
		t.Fatal("nested GitHub web path was accepted as a repository")
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
	for _, raw := range []string{"", "git@github.com", "git@github.com:only-owner", "ssh://", "ssh://%gh", "https://%gh", "https://github.com/acme", "ftp://github.com/acme/repo"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", raw)
		}
	}
	if _, err := RewriteOwner("not-a-remote", "alice"); err == nil {
		t.Fatal("RewriteOwner accepted an invalid remote")
	}
}
