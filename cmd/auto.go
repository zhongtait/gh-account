package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/remote"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newAutoCmd() *cobra.Command {
	var (
		alias    string
		login    string
		gitName  string
		email    string
		protocol string
	)

	cmd := &cobra.Command{
		Use:   "auto [flags]",
		Short: "Automatically sync identity from the current GitHub repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireInitialized(); err != nil {
				return err
			}

			ctx := commandContext(cmd)
			remoteURL, err := deps.Git.GetRemoteURL(ctx, "origin")
			if err != nil {
				return fmt.Errorf("unable to read origin remote: %w", err)
			}
			info, err := remote.Parse(remoteURL)
			if err != nil {
				return fmt.Errorf("origin is not a supported GitHub remote: %w", err)
			}

			accountAlias, account, found, err := accountForRemote(info, alias)
			if err != nil {
				return err
			}
			loggedIn := false
			if found {
				checker, ok := deps.GitHub.(github.CredentialChecker)
				if ok {
					loggedIn, err = checker.HasCredential(ctx, account.Login, account.Hostname)
					if err != nil {
						return fmt.Errorf("check local GitHub credential: %w", err)
					}
				}
			}

			if !found || !loggedIn {
				if found {
					terminal.Warn(deps.Stdout, "No local OAuth credential for %s/%s; enter account details manually", info.Host, info.Owner)
				} else {
					terminal.Warn(deps.Stdout, "No configured account matches %s/%s; enter account details manually", info.Host, info.Owner)
				}
				reader := bufio.NewReader(deps.Stdin)
				choice, err := promptAutoChoice(reader)
				if err != nil {
					return err
				}
				if choice == "1" {
					accountAlias, err = runLoginForAuto(cmd, info)
					if err != nil {
						return err
					}
					return runUseForAuto(cmd, accountAlias)
				}
				if choice != "2" {
					return fmt.Errorf("invalid choice %q; expected 1 or 2", choice)
				}
				accountAlias, account, err = promptAutoAccount(reader, info, accountAlias, account, alias, login, gitName, email, protocol)
				if err != nil {
					return err
				}
				if err := deps.Store.UpsertAccount(accountAlias, account); err != nil {
					return err
				}
				terminal.Success(deps.Stdout, "Saved account %s", terminal.Bold(accountAlias))
			}

			return runUseForAuto(cmd, accountAlias)
		},
	}

	cmd.Flags().StringVar(&alias, "alias", "", "account alias for manual account setup")
	cmd.Flags().StringVar(&login, "login", "", "GitHub login for manual account setup")
	cmd.Flags().StringVar(&gitName, "git-name", "", "git user.name for manual account setup")
	cmd.Flags().StringVar(&email, "email", "", "git user.email for manual account setup")
	cmd.Flags().StringVar(&protocol, "protocol", "", "git protocol for manual account setup (https|ssh)")
	return cmd
}

func promptAutoChoice(reader *bufio.Reader) (string, error) {
	fmt.Fprintln(deps.Stdout, "1) Login with GitHub OAuth (gha login)")
	fmt.Fprintln(deps.Stdout, "2) Enter account details manually")
	fmt.Fprint(deps.Stdout, "Choose [1/2]: ")
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		choice = "1"
	}
	return choice, nil
}

func runLoginForAuto(cmd *cobra.Command, info remote.Info) (string, error) {
	loginCmd := newLoginCmd()
	loginCmd.SetContext(commandContext(cmd))
	loginCmd.SetIn(deps.Stdin)
	loginCmd.SetOut(deps.Stdout)
	loginCmd.SetErr(deps.Stderr)
	if err := loginCmd.Flags().Set("hostname", info.Host); err != nil {
		return "", err
	}
	if info.Protocol != "" {
		if err := loginCmd.Flags().Set("protocol", info.Protocol); err != nil {
			return "", err
		}
	}
	if err := loginCmd.RunE(loginCmd, nil); err != nil {
		return "", err
	}

	login, hostname, err := currentGitHubIdentity(commandContext(cmd))
	if err != nil {
		return "", fmt.Errorf("login succeeded but active GitHub account is unavailable: %w", err)
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

func accountForRemote(info remote.Info, preferredAlias string) (string, config.Account, bool, error) {
	file, err := deps.Store.LoadAccounts()
	if err != nil {
		return "", config.Account{}, false, err
	}
	if preferredAlias != "" {
		if candidate, ok := file.Accounts[preferredAlias]; ok && sameGitHubAccount(info.Owner, info.Host, candidate.Login, candidate.Hostname) {
			return preferredAlias, candidate, true, nil
		}
	}
	var (
		matchedAlias string
		matched      config.Account
	)
	for candidateAlias, candidate := range file.Accounts {
		if !sameGitHubAccount(info.Owner, info.Host, candidate.Login, candidate.Hostname) {
			continue
		}
		if matchedAlias != "" {
			return "", config.Account{}, false, fmt.Errorf("multiple accounts match %s/%s; use --alias to specify one", info.Host, info.Owner)
		}
		matchedAlias = candidateAlias
		matched = candidate
	}
	return matchedAlias, matched, matchedAlias != "", nil
}

func promptAutoAccount(reader *bufio.Reader, info remote.Info, existingAlias string, existing config.Account, aliasFlag, loginFlag, gitNameFlag, emailFlag, protocolFlag string) (string, config.Account, error) {
	loginDefault := info.Owner
	if existing.Login != "" {
		loginDefault = existing.Login
	}
	login := loginFlag
	if login == "" {
		login = promptWithDefault(reader, "GitHub Login", loginDefault)
	}
	if strings.TrimSpace(login) == "" {
		return "", config.Account{}, fmt.Errorf("GitHub login is required")
	}

	aliasDefault := existingAlias
	if aliasDefault == "" {
		aliasDefault = suggestAlias(login)
	}
	accountAlias := aliasFlag
	if accountAlias == "" {
		accountAlias = promptWithDefault(reader, "Alias", aliasDefault)
	}
	if strings.TrimSpace(accountAlias) == "" {
		return "", config.Account{}, fmt.Errorf("alias is required")
	}

	gitNameDefault := login
	if existing.GitName != "" {
		gitNameDefault = existing.GitName
	}
	gitName := gitNameFlag
	if gitName == "" {
		gitName = promptWithDefault(reader, "Git Name", gitNameDefault)
	}

	email := emailFlag
	if email == "" && existing.Email != "" {
		email = existing.Email
	}
	if email == "" {
		email = promptWithDefault(reader, "Email", "")
	}

	protocolDefault := info.Protocol
	if existing.Protocol != "" {
		protocolDefault = existing.Protocol
	}
	if protocolDefault == "" {
		protocolDefault = "https"
	}
	protocol := protocolFlag
	if protocol == "" {
		protocol = promptWithDefault(reader, "Protocol [https/ssh]", protocolDefault)
	}

	return accountAlias, config.Account{
		Login:    strings.TrimSpace(login),
		Hostname: info.Host,
		GitName:  strings.TrimSpace(gitName),
		Email:    strings.TrimSpace(email),
		Protocol: strings.TrimSpace(protocol),
	}, nil
}

func promptWithDefault(reader *bufio.Reader, label, defaultValue string) string {
	if defaultValue == "" {
		fmt.Fprintf(deps.Stdout, "%s: ", label)
	} else {
		fmt.Fprintf(deps.Stdout, "%s [%s]: ", label, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return ""
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

func runUseForAuto(cmd *cobra.Command, alias string) error {
	useCmd := newUseCmd()
	useCmd.SetContext(commandContext(cmd))
	useCmd.SetOut(deps.Stdout)
	useCmd.SetErr(deps.Stderr)
	return useCmd.RunE(useCmd, []string{alias})
}
