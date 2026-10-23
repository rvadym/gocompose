package cmd

import (
	"runtime"

	"github.com/spf13/cobra"
)

const nameCmd = "gocompose"

var rootCmd = &cobra.Command{
	Use:     nameCmd,
	Short:   "Go Compose Tools",
	Version: Version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	if runtime.GOOS == "windows" {
		rootCmd.Use += ".exe"
	}

	rootCmd.AddCommand(
		newDICommand(),
		newRouteCommand(),
		newVersionCommand(),
	)
}
