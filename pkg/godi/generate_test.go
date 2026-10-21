package godi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/fs"
	"github.com/rvadym/gocompose/pkg/internal/testfs"
)

func TestGenerateContainerRejectsAmbiguousProvider(t *testing.T) {
	root := virtualPath(t, "generate-ambiguous")
	fileSystem := generatedProject(root, map[string]string{
		"app/contracts/contracts.go": `package contracts
type Reader interface{ Read() string }
`,
		"app/adapters/reader.go": `package adapters
//di:Reader
func NewReader() *Reader { return &Reader{} }
type Reader struct{}
func (*Reader) Read() string { return "" }
//di:Reader
func NewOtherReader() *OtherReader { return &OtherReader{} }
type OtherReader struct{}
func (*OtherReader) Read() string { return "" }
`,
		"app/use_case/greet/interactor.go": `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader) *Interactor { return &Interactor{} }
type Interactor struct{}
`,
	})

	runner := NewRunner(fileSystem)
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if _, _, err := runner.RenderContainer(graph, GenerateOptions{}); err == nil {
		t.Fatal("expected an error for an ambiguous interface provider")
	}
}

func TestGenerateContainerRejectsUntaggedWireOnParameterizedConstructor(t *testing.T) {
	root := virtualPath(t, "generate-untagged-wire")
	fileSystem := generatedProject(root, map[string]string{
		"app/contracts/contracts.go": `package contracts
type Thing interface{ Do() }
`,
		"app/adapters/thing.go": `package adapters
//di:example.com/project/app/contracts.Thing
func NewThing(limit int) *Thing { return &Thing{} }
type Thing struct{}
func (*Thing) Do() {}
`,
	})

	runner := NewRunner(fileSystem)
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if _, _, err := runner.RenderContainer(graph, GenerateOptions{}); err == nil {
		t.Fatal("expected an error: constructor has parameters but is not tagged //di:wire")
	}
}

func TestGenerateContainerRejectsSameNamedParamsOfDifferentLocalTypes(t *testing.T) {
	root := virtualPath(t, "generate-conflicting-params")
	fileSystem := generatedProject(root, map[string]string{
		"app/module.go": "package app\n",
		"app/use_case/first/interactor.go": `package first
type Config struct{}
//di:wire
func New(config Config) *Interactor { return &Interactor{} }
type Interactor struct{}
`,
		"app/use_case/second/interactor.go": `package second
type Config struct{}
//di:wire
func New(config Config) *Interactor { return &Interactor{} }
type Interactor struct{}
`,
	})

	runner := NewRunner(fileSystem)
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	_, _, err = runner.RenderContainer(graph, GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "conflicting types") {
		t.Fatalf("GenerateContainer() error = %v, want conflicting types error", err)
	}
}

func TestGenerateContainerNamesRepeatedResultTypesByPackage(t *testing.T) {
	root := virtualPath(t, "generate-repeated-types")
	fileSystem := generatedProject(root, map[string]string{
		"app/module.go": "package app\n",
		"app/server/handler/api_healthz/handler.go": `package api_healthz
//di:wire
func New() Handler { return Handler{} }
type Handler struct{}
`,
		"app/server/handler/api_index/handler.go": `package api_index
//di:wire
func New() Handler { return Handler{} }
type Handler struct{}
`,
	})

	runner := NewRunner(fileSystem)
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app")})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	source, _, err := runner.RenderContainer(graph, GenerateOptions{})
	if err != nil {
		t.Fatalf("buildContainerSource() error = %v", err)
	}
	for _, want := range []string{
		"apiHealthz := api_healthz.New()",
		"apiIndex := api_index.New()",
		"ApiHealthz: apiHealthz",
		"ApiIndex:   apiIndex",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated source missing %q:\n%s", want, source)
		}
	}
}

func generatedProject(root string, files map[string]string) fs.FileSystem {
	mapped := testfs.Map{Files: map[string]string{
		filepath.Join(root, "go.mod"): "module example.com/project\n\ngo 1.21\n",
	}}
	for name, content := range files {
		mapped.Files[filepath.Join(root, name)] = content
	}
	return mapped
}
