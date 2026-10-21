package pkg

import (
	"fmt"

	"github.com/spf13/cobra"
)

func PrintCommandTree(cmd *cobra.Command, prefix string) {
	fmt.Println(prefix + cmd.Name())

	for _, sub := range cmd.Commands() {
		PrintCommandTree(sub, prefix+"  ")
	}
}
