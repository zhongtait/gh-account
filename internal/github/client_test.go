package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zhongtait/gh-account/internal/config"
)

type fakeTransport struct {
	responses []string
	statuses  []int
	requests  []*http.Request
}

func (f *fakeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, request)
	index := len(f.requests) - 1
	status := http.StatusOK
	if index < len(f.statuses) && f.statuses[index] != 0 {
		status = f.statuses[index]
	}
	body := "{}"
	if index < len(f.responses) {
		body = f.responses[index]
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    request,
	}, nil
}

func TestLoginDeviceFlowStoresCredential(t *testing.T) {
	transport := &fakeTransport{responses: []string{
		`{"device_code":"device","user_code":"ABCD-EFGH","verification_uri":"https://github.com/login/device","verification_uri_complete":"https://github.com/login/device?user_code=ABCD-EFGH","expires_in":600,"interval":1}`,
		`{"error":"authorization_pending"}`,
		`{"access_token":"secret-token","token_type":"bearer","scope":"read:user,repo"}`,
		`{"login":"personal-user"}`,
	}}
	store := config.NewStore(t.TempDir())
	client := NewOAuthClient(store, io.Discard, "client-id")
	client.HTTP = &http.Client{Transport: transport}
	client.OpenBrowser = func(string) error { return nil }

	if err := client.Login(context.Background(), "github.com", "https"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	auth, err := store.LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	credential, ok := auth.Credentials[credentialKey("github.com", "personal-user")]
	if !ok {
		t.Fatalf("credential was not stored: %+v", auth)
	}
	if credential.AccessToken != "secret-token" || auth.Active == "" {
		t.Fatalf("unexpected auth file: %+v", auth)
	}
	if len(transport.requests) != 4 {
		t.Fatalf("expected device, two token, and user requests; got %d", len(transport.requests))
	}
	if got := transport.requests[0].URL.Path; got != "/login/device/code" {
		t.Fatalf("unexpected device endpoint: %s", got)
	}
	if got := transport.requests[1].URL.Path; got != "/login/oauth/access_token" {
		t.Fatalf("unexpected token endpoint: %s", got)
	}
	if got := transport.requests[3].Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestCurrentLoginAndSwitchUser(t *testing.T) {
	transport := &fakeTransport{responses: []string{`{"login":"work-user"}`}}
	store := config.NewStore(t.TempDir())
	auth := config.DefaultAuth()
	auth.Credentials[credentialKey("github.com", "work-user")] = config.Credential{
		Hostname: "github.com", Login: "work-user", AccessToken: "work-token",
	}
	auth.Credentials[credentialKey("github.com", "personal-user")] = config.Credential{
		Hostname: "github.com", Login: "personal-user", AccessToken: "personal-token",
	}
	auth.Active = credentialKey("github.com", "work-user")
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	client := NewOAuthClient(store, io.Discard, "client-id")
	client.HTTP = &http.Client{Transport: transport}

	login, err := client.CurrentLogin(context.Background())
	if err != nil || login != "work-user" {
		t.Fatalf("CurrentLogin = %q, %v", login, err)
	}
	if err := client.SwitchUser(context.Background(), "personal-user"); err != nil {
		t.Fatalf("SwitchUser: %v", err)
	}
	updated, err := store.LoadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Active != credentialKey("github.com", "personal-user") {
		t.Fatalf("unexpected active credential: %q", updated.Active)
	}
}

func TestLogoutRemovesOnlyRequestedCredential(t *testing.T) {
	store := config.NewStore(t.TempDir())
	auth := config.DefaultAuth()
	auth.Credentials[credentialKey("github.com", "one")] = config.Credential{Hostname: "github.com", Login: "one", AccessToken: "one-token"}
	auth.Credentials[credentialKey("github.com", "two")] = config.Credential{Hostname: "github.com", Login: "two", AccessToken: "two-token"}
	auth.Active = credentialKey("github.com", "one")
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	client := NewOAuthClient(store, io.Discard, "client-id")
	if err := client.Logout(context.Background(), "one", "github.com"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	updated, err := store.LoadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.Credentials[credentialKey("github.com", "one")]; ok {
		t.Fatal("requested credential was not removed")
	}
	if _, ok := updated.Credentials[credentialKey("github.com", "two")]; !ok {
		t.Fatal("other credential was removed")
	}
	if updated.Active != "" {
		t.Fatalf("active credential = %q, want empty", updated.Active)
	}
}

func TestOAuthClientMissingCredentialsAndClientID(t *testing.T) {
	store := config.NewStore(t.TempDir())
	client := NewOAuthClient(store, io.Discard, "")
	if _, err := client.CurrentLogin(context.Background()); err == nil {
		t.Fatal("CurrentLogin succeeded without an active credential")
	}
	if err := client.Login(context.Background(), "github.com", "https"); err == nil {
		t.Fatal("Login succeeded without an OAuth client ID")
	}
	if err := client.Logout(context.Background(), "alice", "github.com"); err == nil {
		t.Fatal("Logout succeeded without a stored credential")
	}
}

func TestSwitchUserRejectsAmbiguousLogin(t *testing.T) {
	store := config.NewStore(t.TempDir())
	auth := config.DefaultAuth()
	for _, host := range []string{"github.com", "ghe.example"} {
		key := credentialKey(host, "alice")
		auth.Credentials[key] = config.Credential{Hostname: host, Login: "alice", AccessToken: host + "-token"}
	}
	if err := store.SaveAuth(auth); err != nil {
		t.Fatal(err)
	}
	client := NewOAuthClient(store, io.Discard, "client-id")
	if err := client.SwitchUser(context.Background(), "alice"); err == nil {
		t.Fatal("SwitchUser accepted ambiguous credentials")
	}
	if err := client.SwitchUserAtHost(context.Background(), "alice", "ghe.example"); err != nil {
		t.Fatal(err)
	}
	updated, err := store.LoadAuth()
	if err != nil {
		t.Fatal(err)
	}
	if updated.Active != credentialKey("ghe.example", "alice") {
		t.Fatalf("active credential = %q", updated.Active)
	}
}
