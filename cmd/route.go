package cmd

import "github.com/spf13/cobra"

func newRouteCommand() *cobra.Command {
	routeCmd := &cobra.Command{
		Use:   "route",
		Short: "Route manipulations",
	}

	routeCmd.AddCommand(newRouteGenerateCommand())

	return routeCmd
}
