package godicb

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateContainerProducesCompilableCode(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/module.go", "package app\n")
	writeGraphFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeGraphFile(t, root, "app/adapters/reader.go", `package adapters
//di:Reader
func NewReader() *Reader { return &Reader{} }
type Reader struct{}
func (*Reader) Read() string { return "" }
`)
	writeGraphFile(t, root, "app/use_case/greet/interactor.go", `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader, limit int) (*Interactor, error) {
	return &Interactor{}, nil
}
type Interactor struct{}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	outPath, err := GenerateContainer(graph, GenerateOptions{})
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

func TestGenerateContainerRedirectsOutputDirToAvoidImportCycle(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/module.go", "package app\n")
	writeGraphFile(t, root, "app/adapters/thing.go", `package adapters
//di:wire
func NewThing() *Thing { return &Thing{} }
type Thing struct{}
`)

	outDir := filepath.Join(root, "app", "adapters", "composition")
	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	outPath, err := GenerateContainer(graph, GenerateOptions{
		OutputDir: outDir,
	})
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
		t.Errorf("expected inferred package name %q, got:\n%s", "container", data)
	}

	assertGoBuildSucceeds(t, root)
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

func TestGenerateContainerRejectsAmbiguousProvider(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeGraphFile(t, root, "app/adapters/reader.go", `package adapters
//di:Reader
func NewReader() *Reader { return &Reader{} }
type Reader struct{}
func (*Reader) Read() string { return "" }
//di:Reader
func NewOtherReader() *OtherReader { return &OtherReader{} }
type OtherReader struct{}
func (*OtherReader) Read() string { return "" }
`)
	writeGraphFile(t, root, "app/use_case/greet/interactor.go", `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader) *Interactor { return &Interactor{} }
type Interactor struct{}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if _, err := GenerateContainer(graph, GenerateOptions{}); err == nil {
		t.Fatal("expected an error for an ambiguous interface provider")
	}
}

func TestGenerateContainerRejectsUntaggedWireOnParameterizedConstructor(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/contracts/contracts.go", `package contracts
type Thing interface{ Do() }
`)
	writeGraphFile(t, root, "app/adapters/thing.go", `package adapters
//di:example.com/project/app/contracts.Thing
func NewThing(limit int) *Thing { return &Thing{} }
type Thing struct{}
func (*Thing) Do() {}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if _, err := GenerateContainer(graph, GenerateOptions{}); err == nil {
		t.Fatal("expected an error: constructor has parameters but is not tagged //di:wire")
	}
}

func TestGenerateContainerRejectsSameNamedParamsOfDifferentLocalTypes(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/module.go", "package app\n")
	writeGraphFile(t, root, "app/use_case/first/interactor.go", `package first
type Config struct{}
//di:wire
func New(config Config) *Interactor { return &Interactor{} }
type Interactor struct{}
`)
	writeGraphFile(t, root, "app/use_case/second/interactor.go", `package second
type Config struct{}
//di:wire
func New(config Config) *Interactor { return &Interactor{} }
type Interactor struct{}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}

	_, err = GenerateContainer(graph, GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "conflicting types") {
		t.Fatalf("GenerateContainer() error = %v, want conflicting types error", err)
	}
}

func TestGenerateContainerNamesRepeatedResultTypesByPackage(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeGraphFile(t, root, "app/module.go", "package app\n")
	writeGraphFile(t, root, "app/server/handler/api_healthz/handler.go", `package api_healthz
//di:wire
func New() Handler { return Handler{} }
type Handler struct{}
`)
	writeGraphFile(t, root, "app/server/handler/api_index/handler.go", `package api_index
//di:wire
func New() Handler { return Handler{} }
type Handler struct{}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	outPath, err := GenerateContainer(graph, GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateContainer() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"apiHealthz := api_healthz.New()",
		"apiIndex := api_index.New()",
		"ApiHealthz: apiHealthz",
		"ApiIndex:   apiIndex",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated source missing %q:\n%s", want, data)
		}
	}
}
