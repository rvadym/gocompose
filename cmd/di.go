package cmd

import "github.com/spf13/cobra"

func newDICommand() *cobra.Command {
	diCmd := &cobra.Command{
		Use:   "di",
		Short: "Dependency injection container manipulations",
	}

	diCmd.AddCommand(newDIGenerateCommand())

	return diCmd
}
