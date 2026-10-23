package goroute

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/fs"
)

func TestGenerateEndToEndReplacesStaleOutput(t *testing.T) {
	root := t.TempDir()
	writeRouteProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeRouteProjectFile(t, root, "app/adapters/composition/container.go", "package composition\n\ntype Container struct{}\n")
	writeRouteProjectFile(t, root, "app/adapters/composition/router.go", generatedMark+"\n\npackage composition\n// stale generated content\n")

	generator := NewGenerator(fs.OS{})
	out, err := generator.Generate(Options{
		Root:          filepath.Join(root, "app"),
		OutputDir:     filepath.Join(root, "app/adapters/composition"),
		HandlerImport: "example.com/project/internal/pkg/infra",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(root, "app/adapters/composition", outputFile)
	if out != wantPath {
		t.Fatalf("Generate() = %q, want %q", out, wantPath)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func GetRoutes(c *Container) (infra.HandlersConfigs, error) {",
		"routes := make(infra.HandlersConfigs, 0, 0)",
		"return routes, nil",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated empty routes missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "stale generated content") {
		t.Fatal("stale generated content remains")
	}
}

func TestRunnerRunGeneratesRoutesForRequestedApp(t *testing.T) {
	root := t.TempDir()
	writeRouteProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeRouteProjectFile(t, root, "internal/app/identity/adapters/composition/container.go", "package composition\ntype Container struct{}\n")
	writeRouteProjectFile(t, root, "internal/app/identity/adapters/server/identity/handler.go", `package identity
//router:get /health
//di:wire
func New() int { return 0 }
`)

	err := NewRunner(fs.OS{}).Run(root, "internal/app", []string{"identity"}, false)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "internal/app/identity/adapters/composition", outputFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`Path:        "/health",`,
		`Method:      "GET",`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated routes missing %q:\n%s", want, data)
		}
	}
}

func writeRouteProjectFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
