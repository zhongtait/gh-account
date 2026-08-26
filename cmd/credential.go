package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/config"
)

var (
	credentialExecutable = os.Executable
	credentialAbs        = filepath.Abs
	credentialLookPath   = exec.LookPath
)

// newCredentialHelperCmd implements Git's credential helper protocol. Git
// appends the operation (get, store, or erase) to the configured command.
func newCredentialHelperCmd() *cobra.Command {
	var accountKey string

	cmd := &cobra.Command{
		Use:    "credential-helper <get|store|erase>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "get" {
				return nil
			}

			request, err := readCredentialRequest(deps.Stdin)
			if err != nil {
				return err
			}
			if request["protocol"] != "https" && request["protocol"] != "http" {
				return nil
			}

			hostname := strings.TrimSpace(request["host"])
			credentialHost, login, ok := strings.Cut(strings.TrimSpace(accountKey), "|")
			if !ok || strings.TrimSpace(credentialHost) == "" || strings.TrimSpace(login) == "" {
				return nil
			}
			if !strings.EqualFold(strings.TrimSpace(hostname), strings.TrimSpace(credentialHost)) {
				return nil
			}

			credential, found, err := deps.Store.GetCredential(credentialHost, login)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}
			return writeCredentialResponse(cmd.OutOrStdout(), request["protocol"], credential)
		},
	}
	cmd.Flags().StringVar(&accountKey, "account-key", "", "stored GitHub credential key")
	return cmd
}

func readCredentialRequest(reader io.Reader) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func writeCredentialResponse(writer io.Writer, protocol string, credential config.Credential) error {
	if _, err := fmt.Fprintf(writer, "protocol=%s\nhost=%s\nusername=%s\npassword=%s\n\n", protocol, credential.Hostname, credential.Login, credential.AccessToken); err != nil {
		return err
	}
	return nil
}

func credentialHelperCommand(store *config.Store, account config.Account) string {
	// Pin the helper to the currently running binary. Git may invoke it later
	// from an IDE, GUI, or service with a different PATH, and PATH may also
	// contain an older gha binary than the one the user just ran.
	var executable string
	if self, selfErr := credentialExecutable(); selfErr == nil && strings.TrimSpace(self) != "" {
		executable = filepath.ToSlash(self)
	} else if path, err := credentialLookPath("gha"); err == nil && strings.TrimSpace(path) != "" {
		if !filepath.IsAbs(path) {
			if absolute, absErr := filepath.Abs(path); absErr == nil {
				path = absolute
			}
		}
		executable = filepath.ToSlash(path)
	} else {
		executable = "gha"
	}
	configDir := store.Dir
	if absolute, err := credentialAbs(configDir); err == nil {
		configDir = absolute
	}
	configDir = filepath.ToSlash(configDir)
	return "!" + shellQuote(executable) + " --config-dir " + shellQuote(configDir) + " credential-helper --account-key " + shellQuote(credentialAccountKey(account))
}

func credentialAccountKey(account config.Account) string {
	hostname := strings.TrimSpace(account.Hostname)
	if hostname == "" {
		hostname = "github.com"
	}
	hostname = strings.TrimPrefix(strings.TrimPrefix(hostname, "https://"), "http://")
	hostname = strings.TrimSuffix(hostname, "/")
	return hostname + "|" + strings.TrimSpace(account.Login)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
