package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
	"github.com/zhongtait/gh-account/internal/github"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newLoginCmd() *cobra.Command {
	var (
		hostname  string
		protocol  string
		gitName   string
		email     string
		token     string
		withToken bool
		scopes    string
	)

	cmd := &cobra.Command{
		Use:               "login [alias]",
		Short:             "Log in with GitHub OAuth or Personal Access Token and save the account profile",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountAliases,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := deps.Store.EnsureInitialized(); err != nil {
				return err
			}
			reader := bufio.NewReader(deps.Stdin)

			alias := ""
			if len(args) == 1 {
				alias = strings.TrimSpace(args[0])
			}

			existing := config.Account{}
			hasExisting := false
			if alias != "" {
				if acc, err := deps.Store.GetAccount(alias); err == nil {
					existing = acc
					hasExisting = true
					if hostname == "github.com" && acc.Hostname != "" {
						hostname = acc.Hostname
					}
					if protocol == "" {
						protocol = acc.Protocol
					}
					if gitName == "" {
						gitName = acc.GitName
					}
					if email == "" {
						email = acc.Email
					}
				}
			}
			if protocol == "" {
				protocol = "https"
			}

			ctx := commandContext(cmd)
			var loginName string
			var detectedProfile github.UserProfile

			if token != "" || withToken {
				if withToken && token == "" {
					terminal.Info(deps.Stdout, "Enter Personal Access Token: ")
					line, err := reader.ReadString('\n')
					if err != nil && len(line) == 0 {
						return fmt.Errorf("read token from stdin: %w", err)
					}
					token = strings.TrimSpace(line)
				}
				if tokenClient, ok := deps.GitHub.(github.TokenLoginClient); ok {
					prof, err := tokenClient.LoginWithToken(ctx, hostname, token)
					if err != nil {
						return fmt.Errorf("GitHub token login failed: %w", err)
					}
					loginName = prof.Login
					detectedProfile = prof
				} else {
					return errors.New("token login is not supported by GitHub client")
				}
			} else {
				if scopes != "" {
					if scopesClient, ok := deps.GitHub.(github.ScopesClient); ok {
						scopesClient.SetScopes(scopes)
					}
				}
				if err := ensureOAuthClientID(reader); err != nil {
					return err
				}
				if err := deps.GitHub.Login(ctx, hostname, protocol); err != nil {
					return fmt.Errorf("GitHub OAuth login failed: %w", err)
				}
				var err error
				loginName, err = deps.GitHub.CurrentLogin(ctx)
				if err != nil {
					return fmt.Errorf("login succeeded but failed to read active GitHub user: %w", err)
				}
				if profileClient, ok := deps.GitHub.(github.ProfileClient); ok {
					if prof, err := profileClient.CurrentProfile(ctx); err == nil {
						detectedProfile = prof
					}
				}
			}

			if strings.TrimSpace(loginName) == "" {
				return fmt.Errorf("login succeeded but active GitHub user is empty")
			}

			if alias == "" {
				alias = suggestAlias(loginName)
			}

			if gitName == "" {
				defaultName := existing.GitName
				if defaultName == "" && detectedProfile.Name != "" {
					defaultName = detectedProfile.Name
				}
				if defaultName == "" {
					defaultName = loginName
				}
				gitName = defaultName
			}
			if email == "" {
				defaultEmail := existing.Email
				if defaultEmail == "" && detectedProfile.Email != "" {
					defaultEmail = detectedProfile.Email
					terminal.Info(deps.Stdout, "Detected GitHub email: %s", terminal.Bold(defaultEmail))
				}
				var err error
				email, err = readValue(reader, "Email", defaultEmail)
				if err != nil {
					return err
				}
			}
			if strings.TrimSpace(email) == "" {
				return fmt.Errorf("email is required")
			}

			account := config.Account{
				Login:    loginName,
				Hostname: hostname,
				GitName:  gitName,
				Email:    email,
				Protocol: protocol,
			}
			if err := deps.Store.UpsertAccount(alias, account); err != nil {
				return err
			}

			if hasExisting {
				terminal.Success(deps.Stdout, "Logged in and updated account %s", terminal.Bold(alias))
			} else {
				terminal.Success(deps.Stdout, "Logged in and saved account %s", terminal.Bold(alias))
			}
			terminal.Info(deps.Stdout, "login=%s git_name=%s email=%s protocol=%s", account.Login, account.GitName, account.Email, account.Protocol)

			if inspector, ok := deps.GitHub.(github.TokenInspector); ok {
				var tokenToCheck string
				if token != "" {
					tokenToCheck = token
				} else if cred, found, err := deps.Store.GetCredential(hostname, loginName); err == nil && found {
					tokenToCheck = cred.AccessToken
				}
				if tokenToCheck != "" {
					if info, err := inspector.InspectToken(ctx, hostname, tokenToCheck); err == nil && len(info.Scopes) > 0 {
						terminal.Info(deps.Stdout, "Scopes: %s", strings.Join(info.Scopes, ", "))
						hasWorkflow := false
						hasReadOrg := false
						for _, s := range info.Scopes {
							if s == "workflow" {
								hasWorkflow = true
							}
							if s == "read:org" {
								hasReadOrg = true
							}
						}
						if !hasWorkflow {
							terminal.Warn(deps.Stdout, "⚠ workflow scope missing (pushing .github/workflows/*.yml may fail)")
						}
						if !hasReadOrg {
							terminal.Warn(deps.Stdout, "⚠ read:org scope missing (viewing PR comments or org items may fail)")
						}
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&hostname, "hostname", "github.com", "GitHub hostname")
	cmd.Flags().StringVar(&protocol, "protocol", "", "git protocol to prefer during login (https|ssh)")
	cmd.Flags().StringVar(&gitName, "git-name", "", "git user.name to store after login")
	cmd.Flags().StringVar(&email, "email", "", "git user.email to store after login")
	cmd.Flags().StringVarP(&scopes, "scopes", "s", "", "additional or custom OAuth scopes (comma-separated)")
	cmd.Flags().StringVar(&token, "token", "", "authenticate directly with a Personal Access Token (PAT)")
	cmd.Flags().BoolVar(&withToken, "with-token", false, "read Personal Access Token from stdin")
	return cmd
}

func suggestAlias(login string) string {
	login = strings.TrimSpace(login)
	if login == "" {
		return ""
	}
	return strings.ToLower(login)
}
