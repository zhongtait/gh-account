package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/remote"
)

func TestReadCredentialRequestAndResponse(t *testing.T) {
	request, err := readCredentialRequest(strings.NewReader("protocol=https\nhost=github.com\ninvalid\npath=owner/repo\r\n\nignored=x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if request["protocol"] != "https" || request["host"] != "github.com" || request["path"] != "owner/repo" {
		t.Fatalf("unexpected request: %#v", request)
	}
	if _, ok := request["ignored"]; ok {
		t.Fatal("fields after the credential protocol terminator were parsed")
	}

	var output bytes.Buffer
	credential := config.Credential{Hostname: "github.com", Login: "octocat", AccessToken: "secret"}
	if err := writeCredentialResponse(&output, "https", credential); err != nil {
		t.Fatal(err)
	}
	want := "protocol=https\nhost=github.com\nusername=octocat\npassword=secret\n\n"
	if output.String() != want {
		t.Fatalf("response = %q, want %q", output.String(), want)
	}
}

func TestCredentialKeyAndShellQuote(t *testing.T) {
	account := config.Account{Hostname: "https://GHE.example/", Login: "Work User"}
	if got := credentialAccountKey(account); got != "GHE.example|Work User" {
		t.Fatalf("credentialAccountKey = %q", got)
	}
	if got := shellQuote("a'b"); got != `'a'"'"'b'` {
		t.Fatalf("shellQuote = %q", got)
	}
}

func TestCloneAuthFailureAndHostMatching(t *testing.T) {
	for _, message := range []string{"Authentication failed", "repository not found", "HTTP 403"} {
		if !isCloneAuthFailure(testError(message)) {
			t.Errorf("isCloneAuthFailure(%q) = false", message)
		}
	}
	if isCloneAuthFailure(testError("network timeout")) {
		t.Fatal("network timeout was classified as an authentication failure")
	}
	if !sameHost("GITHUB.COM", "github.com") {
		t.Fatal("sameHost should be case insensitive")
	}
	if sameGitHubAccount("alice", "github.com", "alice", "ghe.example") {
		t.Fatal("different GitHub hosts matched as the same account")
	}
	if !sameGitHubAccount("alice", "", "alice", "") {
		t.Fatal("empty hosts should default to github.com")
	}

	info, _ := remote.Parse("https://github.com/acme/repo.git")
	if !isHTTPRemote(info) {
		t.Fatal("HTTPS remote was not classified as HTTP")
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }

func testError(message string) error { return stringError(message) }
