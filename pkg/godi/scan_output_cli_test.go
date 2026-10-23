package godi

import (
	"strings"
	"testing"
)

func TestFormatScanOutputCLI(t *testing.T) {
	output := FormatScanOutputCLI(DependencyGraph{
		Nodes: []GraphNode{{
			File:     "consumer.go",
			Line:     12,
			Function: "example.com/app.NewConsumer",
			Provides: []string{"example.com/contracts.Reader"},
			Dependencies: []DependencyEdge{{
				Parameter: "reader",
				Type:      "example.com/contracts.Reader",
				Provider:  "example.com/adapters.NewReader",
			}},
		}},
		Unresolved: []UnresolvedDependency{{
			Function:  "example.com/app.NewConsumer",
			Parameter: "writer",
			Type:      "example.com/contracts.Writer",
			Reason:    "no tagged provider",
		}, {
			Function:  "example.com/app.NewConsumer",
			Parameter: "publisher",
			Type:      "example.com/contracts.Publisher",
			Reason:    "no tagged provider",
		}},
		External: []ExternalDependency{{
			Function:  "example.com/app.NewConsumer",
			Parameter: "logger",
			Type:      "example.com/log.Logger",
		}},
	})

	for _, want := range []string{
		"Dependency graph",
		"example.com/app.NewConsumer",
		"example.com/contracts.Reader",
		"reader",
		"example.com/adapters.NewReader",
		"Unresolved dependencies",
		"writer",
		"no tagged provider",
		"External inputs (Deps)",
		"logger",
		Cyan,
		Red,
		Reset,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("FormatScanOutputCLI() output missing %q:\n%s", want, output)
		}
	}
	if got := strings.Count(output, "example.com/app.NewConsumer"); got != 3 {
		t.Errorf("constructor should appear once in graph, unresolved, and external sections; got %d mentions\n%s", got, output)
	}
	if got := strings.Count(output, BrightWhite+"example.com/app.NewConsumer"+Reset); got != 3 {
		t.Errorf("constructor should be printed once in each relevant section; got %d headings\n%s", got, output)
	}
}
