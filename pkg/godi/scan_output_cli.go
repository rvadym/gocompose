package godi

import (
	"fmt"
	"sort"
	"strings"
)

// FormatScanOutputCLI formats a dependency graph as a colorized CLI report.
func FormatScanOutputCLI(graph DependencyGraph) string {
	var output strings.Builder

	writeSeparator(&output)
	fmt.Fprintf(&output, "\n%sDependency graph%s %s(%d constructors)%s\n",
		Cyan, Reset, BrightBlack, len(graph.Nodes), Reset)
	for _, node := range graph.Nodes {
		fmt.Fprintf(&output, "\n- %s%s%s\n", BrightWhite, node.Function, Reset)
		fmt.Fprintf(&output, "  %s%s:%d%s\n", BrightBlack, node.File, node.Line, Reset)
		for _, interfaceName := range node.Provides {
			fmt.Fprintf(&output, "  %s<-%s %sprovides%s %s\n",
				Yellow, Reset, Green, Reset, interfaceName)
		}
		if len(node.Dependencies) > 0 {
			fmt.Fprintf(&output, "  %sdependencies%s\n", Cyan, Reset)
			for _, dependency := range node.Dependencies {
				fmt.Fprintf(&output,
					"    • %s \n"+
						"        %s\n"+
						"        %s %s\n",
					Green+dependency.Parameter+Reset,
					dependency.Type,
					Cyan+"->"+Reset,
					dependency.Provider,
				)
			}
		}
	}
	if len(graph.Nodes) == 0 {
		fmt.Fprintf(&output, "  %s(no tagged constructor functions found)%s\n", BrightBlack, Reset)
	}

	writeSeparator(&output)
	fmt.Fprintf(&output, "\n%sUnresolved dependencies%s %s(%d)%s\n\n",
		Red, Reset, BrightBlack, len(graph.Unresolved), Reset)
	if len(graph.Unresolved) == 0 {
		fmt.Fprintf(&output, "  %s(none)%s\n", Green, Reset)
	} else {
		grouped := make(map[string][]UnresolvedDependency)
		functions := make([]string, 0)
		for _, dependency := range graph.Unresolved {
			if _, exists := grouped[dependency.Function]; !exists {
				functions = append(functions, dependency.Function)
			}
			grouped[dependency.Function] = append(grouped[dependency.Function], dependency)
		}
		sort.Strings(functions)
		for _, function := range functions {
			fmt.Fprintf(&output, "  %s\n", BrightWhite+function+Reset)
			for _, dependency := range grouped[function] {
				fmt.Fprintf(&output, "    %s : %s\n      %s\n",
					Red+dependency.Parameter+Reset,
					dependency.Type,
					BrightBlack+dependency.Reason+Reset,
				)
			}
		}
	}

	writeSeparator(&output)
	if len(graph.External) > 0 {
		fmt.Fprintf(&output, "\n%sExternal inputs (Deps)%s %s(%d)%s\n",
			Magenta, Reset, BrightBlack, len(graph.External), Reset)
		for _, dependency := range graph.External {
			fmt.Fprintf(&output, "  %s%s%s\n    %s%s%s %s\n",
				BrightWhite, dependency.Function, Reset,
				Yellow, dependency.Parameter, Reset,
				BrightBlack+dependency.Type+Reset,
			)
		}
	}
	writeSeparator(&output)
	return output.String()
}

func writeSeparator(output *strings.Builder) {
	fmt.Fprintf(output, "\n%s%s%s\n", BrightBlack, Separator, Reset)
}
