package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var gitExec = runSystemGit

func runSystemGit(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func newGitCmd() *cobra.Command {
	var flagAccount string

	cmd := &cobra.Command{
		Use:                "git [flags] <command> [args...]",
		Short:              "Run git commands with GitHub account integration",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return gitExec(commandContext(cmd), deps.Stdin, deps.Stdout, deps.Stderr)
			}
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return cmd.Help()
			}

			accountAlias, gitArgs, err := extractAccountFlag(args)
			if err != nil {
				return err
			}

			if len(gitArgs) == 0 {
				return runGitWithAccount(cmd, accountAlias, nil)
			}

			if gitArgs[0] == "clone" {
				return runGitCloneWrapper(cmd, accountAlias, gitArgs[1:])
			}

			if targetCmd := findGHASubcommand(cmd, gitArgs[0]); targetCmd != nil {
				return runGHASubcommand(cmd, targetCmd, gitArgs[1:])
			}

			return runGitWithAccount(cmd, accountAlias, gitArgs)
		},
	}

	cmd.Flags().StringVarP(&flagAccount, "account", "a", "", "specify the account alias to use for this git command")
	return cmd
}

func extractAccountFlag(args []string) (string, []string, error) {
	var accountAlias string
	var gitArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--account" || arg == "-a" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires an argument", arg)
			}
			accountAlias = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--account=") {
			accountAlias = strings.TrimPrefix(arg, "--account=")
		} else if strings.HasPrefix(arg, "-a=") {
			accountAlias = strings.TrimPrefix(arg, "-a=")
		} else {
			gitArgs = append(gitArgs, arg)
		}
	}

	return strings.TrimSpace(accountAlias), gitArgs, nil
}

func runGitCloneWrapper(cmd *cobra.Command, accountAlias string, cloneArgs []string) error {
	if len(cloneArgs) > 0 && (cloneArgs[0] == "--help" || cloneArgs[0] == "-h") {
		return newCloneCmd().Help()
	}

	finalArgs := cloneArgs
	if accountAlias != "" {
		hasExplicit := false
		for _, a := range cloneArgs {
			if strings.EqualFold(a, "use") {
				hasExplicit = true
				break
			}
		}
		if !hasExplicit {
			finalArgs = append(append([]string{}, cloneArgs...), "use", accountAlias)
		}
	}

	cloneCmd := newCloneCmd()
	cloneCmd.SetContext(commandContext(cmd))
	cloneCmd.SetIn(deps.Stdin)
	cloneCmd.SetOut(deps.Stdout)
	cloneCmd.SetErr(deps.Stderr)

	if err := cloneCmd.Args(cloneCmd, finalArgs); err != nil {
		return err
	}
	return cloneCmd.RunE(cloneCmd, finalArgs)
}

func runGitWithAccount(cmd *cobra.Command, accountAlias string, gitArgs []string) error {
	ctx := commandContext(cmd)
	if accountAlias == "" {
		return gitExec(ctx, deps.Stdin, deps.Stdout, deps.Stderr, gitArgs...)
	}

	if err := requireInitialized(); err != nil {
		return err
	}

	acc, err := deps.Store.GetAccount(accountAlias)
	if err != nil {
		return err
	}

	extraArgs := []string{
		"-c", "user.name=" + acc.GitName,
		"-c", "user.email=" + acc.Email,
	}
	helper := credentialHelperCommand(deps.Store, acc)
	extraArgs = append(extraArgs, "-c", "credential.helper=", "-c", "credential.helper="+helper)

	argsToRun := append(extraArgs, gitArgs...)
	return gitExec(ctx, deps.Stdin, deps.Stdout, deps.Stderr, argsToRun...)
}

func findGHASubcommand(cmd *cobra.Command, name string) *cobra.Command {
	if name == "git" || name == "add" || name == "init" || name == "remote" || name == "status" {
		return nil
	}
	if !cmd.HasParent() {
		return nil
	}
	root := cmd.Root()
	for _, c := range root.Commands() {
		if c.Name() == "git" {
			continue
		}
		if c.Name() == name || c.HasAlias(name) {
			return c
		}
	}
	return nil
}

func runGHASubcommand(parent *cobra.Command, target *cobra.Command, args []string) error {
	target.SetContext(commandContext(parent))
	target.SetIn(deps.Stdin)
	target.SetOut(deps.Stdout)
	target.SetErr(deps.Stderr)
	if err := target.ParseFlags(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return target.Help()
		}
		return err
	}
	flagsArgs := target.Flags().Args()
	if target.Args != nil {
		if err := target.Args(target, flagsArgs); err != nil {
			return err
		}
	}
	if target.RunE != nil {
		return target.RunE(target, flagsArgs)
	}
	if target.Run != nil {
		target.Run(target, flagsArgs)
	}
	return nil
}
