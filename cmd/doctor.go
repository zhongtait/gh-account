package cmd

import (
	"github.com/spf13/cobra"
	"github.com/zhongtait/gh-account/internal/doctor"
	"github.com/zhongtait/gh-account/internal/terminal"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose gha environment and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			report := doctor.Runner{
				Store:  deps.Store,
				Git:    deps.Git,
				GitHub: deps.GitHub,
			}.Run(commandContext(cmd))

			for _, check := range report.Checks {
				if check.OK {
					terminal.Success(deps.Stdout, "%s", check.Message)
				} else {
					terminal.Error(deps.Stdout, "%s", check.Message)
				}
			}

			if report.Healthy() {
				terminal.Success(deps.Stdout, "Everything looks good.")
				return nil
			}
			return nil
		},
	}
}
