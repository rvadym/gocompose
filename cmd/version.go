package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	GitCommit = "none"
	BuildDate = "unknown"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display build version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "Version:    %s\n", Version)
			fmt.Fprintf(cmd.OutOrStdout(), "Git commit: %s\n", GitCommit)
			fmt.Fprintf(cmd.OutOrStdout(), "Build date: %s\n", BuildDate)
		},
	}
}
