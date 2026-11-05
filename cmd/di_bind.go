package cmd

import (
	"fmt"

	"github.com/rvadym/gocompose/pkg/godi"
	"github.com/spf13/cobra"
)

func newDIBindCommand() *cobra.Command {
	projectPath := DefaultProjectPath
	appsPath := DefaultAppsPath
	var apps []string

	diBindCmd := &cobra.Command{
		Use:   "bind <interface> <implementation>",
		Short: "Bind a constructor that cannot be annotated to an app interface",
		Long: "Record a binding in the app's adapters/composition/godi.yaml.\n\n" +
			"Both arguments use the <import/path>.<Name> notation, for example:\n" +
			"  " + nameCmd + " di bind --app catalog \\\n" +
			"    gitlab.com/acme/app/internal/app/catalog/application/contract.CategoryReader \\\n" +
			"    github.com/vendor/impl.NewCategoryRepo",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(apps) != 1 {
				return fmt.Errorf("exactly one --%s is required", FlagApp)
			}
			binding := godi.Binding{Interface: args[0], Constructor: args[1]}
			path, err := godi.NewDefaultRunner().BindInterface(projectPath, appsPath, apps[0], binding)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Bound %s to %s in %s\n", binding.Interface, binding.Constructor, path)
			return nil
		},
	}

	// Scope
	AddFlagProjectPath(diBindCmd, &projectPath)
	AddFlagAppsPath(diBindCmd, &appsPath)
	AddFlagApp(diBindCmd, &apps)

	return diBindCmd
}
