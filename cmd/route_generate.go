package cmd

import (
	"fmt"

	"github.com/rvadym/gocompose/pkg/goroute"
	"github.com/spf13/cobra"
)

func newRouteGenerateCommand() *cobra.Command {
	var verbosity1, dryRun bool
	projectPath := DefaultProjectPath
	appsPath := DefaultAppsPath
	var apps []string

	routeGenerateCmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"g"},
		Short:   "Generate routes",
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

			if err := goroute.NewDefaultRunner().Run(projectPath, appsPath, apps, verbosity1); err != nil {
				return err
			}

			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run mode. Done!")
			}

			return nil
		},
	}

	// Execution
	AddFlagVerbosity1(routeGenerateCmd, &verbosity1)
	AddFlagDryRun(routeGenerateCmd, &dryRun)

	// Scope
	AddFlagProjectPath(routeGenerateCmd, &projectPath)
	AddFlagAppsPath(routeGenerateCmd, &appsPath)
	AddFlagApp(routeGenerateCmd, &apps)

	return routeGenerateCmd
}
