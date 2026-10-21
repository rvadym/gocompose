package cmd

import (
	"github.com/rvadym/gocompose/pkg/godi"
	"github.com/spf13/cobra"
)

const (
	DefaultProjectPath = godi.ProjectPathCWD
	DefaultAppsPath    = godi.AppsPathCWD
)

const (
	FlagVerbosity1 = "v"
	FlagDryRun     = "dry-run"

	FlagProjectPath = "project-path"
	FlagAppsPath    = "apps-path"
	FlagApp         = "app"
)

func AddFlagVerbosity1(c *cobra.Command, target *bool) {
	c.Flags().BoolVar(target, FlagVerbosity1, false, "Verbosity level 1.")
}

func AddFlagDryRun(c *cobra.Command, target *bool) {
	c.Flags().BoolVar(target, FlagDryRun, false, "Dry run mode")
}

func AddFlagProjectPath(c *cobra.Command, target *string) {
	c.Flags().StringVar(
		target, FlagProjectPath, DefaultProjectPath,
		"Path of the project to search for apps",
	)
}

func AddFlagAppsPath(c *cobra.Command, target *string) {
	c.Flags().StringVar(
		target, FlagAppsPath, DefaultAppsPath,
		"Path of the apps relative to the project root",
	)
}

func AddFlagApp(c *cobra.Command, target *[]string) {
	c.Flags().StringArrayVar(
		target, FlagApp, nil,
		"Name of an app package to generate the DI container for",
	)
}
