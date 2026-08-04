package cmd

import (
	"context"
	"strings"
)

type currentIdentityClient interface {
	CurrentIdentity(ctx context.Context) (login string, hostname string, err error)
}

func currentGitHubIdentity(ctx context.Context) (login, hostname string, err error) {
	if client, ok := deps.GitHub.(currentIdentityClient); ok {
		return client.CurrentIdentity(ctx)
	}
	login, err = deps.GitHub.CurrentLogin(ctx)
	return login, "github.com", err
}

func sameGitHubAccount(login, hostname string, accountLogin, accountHostname string) bool {
	if !strings.EqualFold(strings.TrimSpace(login), strings.TrimSpace(accountLogin)) {
		return false
	}
	if strings.TrimSpace(accountHostname) == "" {
		accountHostname = "github.com"
	}
	if strings.TrimSpace(hostname) == "" {
		hostname = "github.com"
	}
	return strings.EqualFold(strings.TrimSpace(hostname), strings.TrimSpace(accountHostname))
}
