package godi

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/fs"
)

func TestGenerateContainerEndToEndProducesCompilableCode(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "app/module.go", "package app\n")
	writeProjectFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeProjectFile(t, root, "app/adapters/reader.go", `package adapters
//di:Reader
func NewReader() *Reader { return &Reader{} }
type Reader struct{}
func (*Reader) Read() string { return "" }
`)
	writeProjectFile(t, root, "app/use_case/greet/interactor.go", `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader, limit int) (*Interactor, error) {
	return &Interactor{}, nil
}
type Interactor struct{}
`)

	runner := NewRunner(fs.OS{})
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	outPath, err := runner.GenerateContainer(graph, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateContainer() error = %v", err)
	}
	if filepath.Dir(outPath) != filepath.Join(root, "app") {
		t.Fatalf("outPath = %s, want it inside the scan root", outPath)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{
		"package app",
		"type Deps struct {",
		"Limit int",
		"type Container struct {",
		"Greet *greet.Interactor",
		"func BuildContainer(deps Deps) (*Container, error) {",
		"reader := adapters.NewReader()",
		"greet.New(reader, deps.Limit)",
		"if err != nil {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated source missing %q:\n%s", want, source)
		}
	}

	assertGoBuildSucceeds(t, root)
}

func TestGenerateContainerEndToEndRedirectsOutputDirToAvoidImportCycle(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "app/module.go", "package app\n")
	writeProjectFile(t, root, "app/adapters/thing.go", `package adapters
//di:wire
func NewThing() *Thing { return &Thing{} }
type Thing struct{}
`)

	outDir := filepath.Join(root, "app", "adapters", "composition")
	runner := NewRunner(fs.OS{})
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	outPath, err := runner.GenerateContainer(graph, GenerateOptions{OutputDir: outDir})
	if err != nil {
		t.Fatalf("GenerateContainer() error = %v", err)
	}
	if filepath.Dir(outPath) != outDir {
		t.Fatalf("outPath = %s, want it inside %s", outPath, outDir)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "package composition") {
		t.Errorf("expected generated package name %q, got:\n%s", "composition", data)
	}

	assertGoBuildSucceeds(t, root)
}

func writeProjectFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertGoBuildSucceeds(t *testing.T, moduleDir string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("skipping compile verification: %v (no go toolchain on PATH, e.g. inside a sandboxed test runner)", err)
		return
	}
	cmd := exec.Command(goBin, "build", "./...")
	cmd.Dir = moduleDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated code does not compile: %v\n%s", err, out)
	}
}
