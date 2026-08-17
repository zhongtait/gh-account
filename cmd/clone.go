package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/git"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

// cloneGit is a variable to keep the command testable without changing the
// native Git client, which intentionally does not shell out to git.
var cloneGit = runGitClone

var (
	cloneSelectAccount  = selectCloneAccount
	cloneRun            = cloneWithAccount
	cloneDestinationFor = cloneDestination
	cloneLogin          = runLoginForClone
	cloneAccountByAlias = func(store *config.Store, alias string) (config.Account, error) { return store.GetAccount(alias) }
	cloneLocalGit       = func(destination string) cloneConfigurator { return git.NewNativeClient(destination) }
)

type cloneConfigurator interface {
	SetIdentity(context.Context, git.Scope, git.Identity) error
	SetCredentialHelper(context.Context, git.Scope, string, string) error
}

func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <repository> [auto|use <account>]",
		Short: "Clone a GitHub repository with a selected account",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 && len(args) != 2 && len(args) != 3 {
				return errors.New("usage: gha clone <repository> [auto|use <account>]")
			}
			if len(args) == 2 && strings.ToLower(strings.TrimSpace(args[1])) != "auto" {
				return errors.New("the optional mode must be auto or use <account>")
			}
			if len(args) == 3 && strings.ToLower(strings.TrimSpace(args[1])) != "use" {
				return errors.New("the optional account selector must be: use <account>")
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 2 {
				return completeAccountAliases(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			ctx := commandContext(cmd)
			info, err := remote.Parse(args[0])
			if err != nil {
				return fmt.Errorf("repository is not a supported GitHub remote: %w", err)
			}

			alias := ""
			explicit := len(args) == 3
			if explicit {
				alias = strings.TrimSpace(args[2])
			}
			alias, account, err := cloneSelectAccount(ctx, cmd, info, alias, explicit)
			if err != nil {
				return err
			}

			cloneErr := cloneRun(ctx, args[0], info, account)
			if cloneErr != nil && !explicit && isHTTPRemote(info) && isCloneAuthFailure(cloneErr) {
				terminal.Warn(deps.Stdout, "Account %s cannot access this repository; starting GitHub login", alias)
				loginAlias, loginErr := cloneLogin(cmd, info)
				if loginErr != nil {
					return fmt.Errorf("%w; automatic login failed: %v", cloneErr, loginErr)
				}
				account, err = cloneAccountByAlias(deps.Store, loginAlias)
				if err != nil {
					return err
				}
				alias = loginAlias
				cloneErr = cloneRun(ctx, args[0], info, account)
			}
			if cloneErr != nil {
				return cloneErr
			}

			destination, err := cloneDestinationFor(args[0], info)
			if err != nil {
				return err
			}
			localGit := cloneLocalGit(destination)
			if err := localGit.SetIdentity(ctx, git.ScopeLocal, git.Identity{Name: account.GitName, Email: account.Email}); err != nil {
				return fmt.Errorf("configure cloned repository identity: %w", err)
			}
			if strings.EqualFold(info.Protocol, "https") || strings.EqualFold(info.Protocol, "http") {
				helper := credentialHelperCommand(deps.Store, account)
				if err := localGit.SetCredentialHelper(ctx, git.ScopeLocal, helper, credentialAccountKey(account)); err != nil {
					return fmt.Errorf("configure cloned repository credentials: %w", err)
				}
			}
			terminal.Success(deps.Stdout, "Cloned %s with account %s", args[0], terminal.Bold(alias))
			terminal.Info(deps.Stdout, "Local git identity: %s <%s>", account.GitName, account.Email)
			return nil
		},
	}
}

func selectCloneAccount(ctx context.Context, cmd *cobra.Command, info remote.Info, preferredAlias string, explicit bool) (string, config.Account, error) {
	file, err := deps.Store.LoadAccounts()
	if err != nil {
		return "", config.Account{}, err
	}

	if explicit {
		account, ok := file.Accounts[preferredAlias]
		if !ok {
			return "", config.Account{}, fmt.Errorf("account %q not found", preferredAlias)
		}
		if !sameGitHubAccount(info.Owner, info.Host, account.Login, account.Hostname) && strings.EqualFold(info.Protocol, "https") {
			// The repository owner may be an organization or another user, so a
			// mismatch is allowed when the selected account has a credential.
			checker, ok := deps.GitHub.(github.CredentialChecker)
			if !ok {
				return "", config.Account{}, errors.New("GitHub credential checking is unavailable")
			}
			has, checkErr := checker.HasCredential(ctx, account.Login, account.Hostname)
			if checkErr != nil {
				return "", config.Account{}, fmt.Errorf("check account credential: %w", checkErr)
			}
			if !has {
				return "", config.Account{}, fmt.Errorf("account %q is not logged in; run gha login %s", preferredAlias, preferredAlias)
			}
		}
		if strings.EqualFold(info.Protocol, "https") || strings.EqualFold(info.Protocol, "http") {
			if err := requireAccountCredential(ctx, account); err != nil {
				return "", config.Account{}, fmt.Errorf("account %q cannot clone this repository: %w", preferredAlias, err)
			}
		}
		return preferredAlias, account, nil
	}

	// Auto prefers an account whose login is the repository owner, then the
	// active account, then the only credential available for this host.
	var candidates []struct {
		alias   string
		account config.Account
	}
	checker, hasChecker := deps.GitHub.(github.CredentialChecker)
	activeLogin, activeHost, _ := currentGitHubIdentity(ctx)
	var activeAlias string
	var activeAccount config.Account
	for accountAlias, account := range file.Accounts {
		if !sameHost(info.Host, account.Hostname) {
			continue
		}
		if strings.EqualFold(info.Owner, account.Login) {
			if !hasChecker || hasCredential(ctx, checker, account) {
				return accountAlias, account, nil
			}
		}
		if !hasChecker || hasCredential(ctx, checker, account) {
			candidates = append(candidates, struct {
				alias   string
				account config.Account
			}{accountAlias, account})
		}
		if activeAlias == "" && sameGitHubAccount(activeLogin, activeHost, account.Login, account.Hostname) && (!hasChecker || hasCredential(ctx, checker, account)) {
			activeAlias = accountAlias
			activeAccount = account
		}
	}
	if activeAlias != "" {
		return activeAlias, activeAccount, nil
	}
	if len(candidates) == 1 {
		return candidates[0].alias, candidates[0].account, nil
	}
	if len(candidates) > 1 {
		return "", config.Account{}, fmt.Errorf("multiple logged-in accounts match host %s; use: gha clone %s use <account>", info.Host, info.Raw)
	}

	// No usable local credential: use the existing login flow and retry account
	// selection. This is the auto equivalent of running gha login.
	loginAlias, loginErr := cloneLogin(cmd, info)
	if loginErr != nil {
		return "", config.Account{}, loginErr
	}
	account, err := deps.Store.GetAccount(loginAlias)
	if err != nil {
		return "", config.Account{}, err
	}
	if strings.EqualFold(info.Protocol, "https") || strings.EqualFold(info.Protocol, "http") {
		if err := requireAccountCredential(ctx, account); err != nil {
			return "", config.Account{}, err
		}
	}
	return loginAlias, account, nil
}

func runLoginForClone(cmd *cobra.Command, info remote.Info) (string, error) {
	loginCmd := newLoginCmd()
	loginCmd.SetContext(commandContext(cmd))
	loginCmd.SetIn(deps.Stdin)
	loginCmd.SetOut(deps.Stdout)
	loginCmd.SetErr(deps.Stderr)
	_ = loginCmd.Flags().Set("hostname", info.Host)
	_ = loginCmd.Flags().Set("protocol", info.Protocol)
	if err := loginCmd.RunE(loginCmd, nil); err != nil {
		return "", fmt.Errorf("automatic GitHub login failed: %w", err)
	}
	login, hostname, err := currentGitHubIdentity(commandContext(cmd))
	if err != nil {
		return "", fmt.Errorf("login succeeded but active account is unavailable: %w", err)
	}
	file, err := deps.Store.LoadAccounts()
	if err != nil {
		return "", err
	}
	for alias, account := range file.Accounts {
		if sameGitHubAccount(login, hostname, account.Login, account.Hostname) {
			return alias, nil
		}
	}
	return "", fmt.Errorf("login succeeded for %s, but no account profile was saved", login)
}

func hasCredential(ctx context.Context, checker github.CredentialChecker, account config.Account) bool {
	has, err := checker.HasCredential(ctx, account.Login, account.Hostname)
	return err == nil && has
}

func requireAccountCredential(ctx context.Context, account config.Account) error {
	checker, ok := deps.GitHub.(github.CredentialChecker)
	if !ok {
		return errors.New("GitHub credential checking is unavailable")
	}
	has, err := checker.HasCredential(ctx, account.Login, account.Hostname)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("run gha login for account %q first", account.Login)
	}
	return nil
}

func cloneWithAccount(ctx context.Context, repository string, info remote.Info, account config.Account) error {
	var helper string
	if isHTTPRemote(info) {
		helper = credentialHelperCommand(deps.Store, account)
	}
	var cloneStderr bytes.Buffer
	stderr := io.MultiWriter(deps.Stderr, &cloneStderr)
	if err := cloneGit(ctx, repository, helper, deps.Stdout, stderr); err != nil {
		message := strings.TrimSpace(cloneStderr.String())
		if message != "" {
			return fmt.Errorf("git clone failed: %w: %s", err, message)
		}
		return fmt.Errorf("git clone failed: %w", err)
	}
	return nil
}

func isHTTPRemote(info remote.Info) bool {
	return strings.EqualFold(info.Protocol, "https") || strings.EqualFold(info.Protocol, "http")
}

func isCloneAuthFailure(err error) bool {
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"authentication failed",
		"could not read username",
		"repository not found",
		"http 401",
		"http 403",
		" 401 ",
		" 403 ",
		"permission to ",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func runGitClone(ctx context.Context, repository, helper string, stdout, stderr io.Writer) error {
	args := []string{}
	if helper != "" {
		// Clear inherited helpers before installing gha's selected helper.
		args = append(args, "-c", "credential.helper=", "-c", "credential.helper="+helper)
	}
	args = append(args, "clone", repository)
	command := exec.CommandContext(ctx, "git", args...)
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func cloneDestination(repository string, info remote.Info) (string, error) {
	// git clone defaults to the repository basename when no destination is
	// supplied. Resolve it here so local configuration is written to the same
	// directory without changing git's normal clone behavior.
	destination := info.Repo
	if destination == "" {
		return "", errors.New("unable to determine clone destination")
	}
	return filepath.Abs(destination)
}

func sameHost(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
