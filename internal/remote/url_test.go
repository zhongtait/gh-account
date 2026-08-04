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
