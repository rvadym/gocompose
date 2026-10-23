package cmd

import (
	"fmt"

	"github.com/rvadym/gocompose/pkg/godi"
	"github.com/spf13/cobra"
)

func newDIGenerateCommand() *cobra.Command {
	var verbosity1, dryRun bool
	projectPath := DefaultProjectPath
	appsPath := DefaultAppsPath
	var apps []string

	diGenerateCmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate dependency injection container(s)",
		RunE: func(cmd *cobra.Command, args []string) error {

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run mode. No changes will be persisted.")
			}

			if verbosity1 {
				fmt.Println()
				fmt.Fprintf(cmd.OutOrStdout(), "Flag --%s -> %v\n", FlagProjectPath, projectPath)
				fmt.Fprintf(cmd.OutOrStdout(), "Flag --%s -> %v\n", FlagAppsPath, appsPath)
				fmt.Fprintf(cmd.OutOrStdout(), "Flag --%s -> %v\n", FlagApp, apps)
				fmt.Println()
			}

			if err := godi.NewDefaultRunner().Run(projectPath, appsPath, apps, verbosity1); err != nil {
				return err
			}

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run mode. Done!")
			}

			return nil
		},
	}

	// Execution
	AddFlagVerbosity1(diGenerateCmd, &verbosity1)
	AddFlagDryRun(diGenerateCmd, &dryRun)

	// Scope
	AddFlagProjectPath(diGenerateCmd, &projectPath)
	AddFlagAppsPath(diGenerateCmd, &appsPath)
	AddFlagApp(diGenerateCmd, &apps)

	return diGenerateCmd
}
