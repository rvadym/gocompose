package godi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/fs"
)

func TestLoadBindingsReturnsNilWhenFileMissing(t *testing.T) {
	bindings, err := LoadBindings(fs.OS{}, filepath.Join(t.TempDir(), bindingsFileName))
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	if bindings != nil {
		t.Fatalf("LoadBindings() = %v, want nil", bindings)
	}
}

func TestLoadBindingsParsesYAML(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, bindingsFileName, `bindings:
  - interface: example.com/project/app/contracts.Reader
    constructor: example.com/vendor/impl.NewReader
`)

	bindings, err := LoadBindings(fs.OS{}, filepath.Join(root, bindingsFileName))
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("LoadBindings() returned %d bindings, want 1", len(bindings))
	}
	want := Binding{
		Interface:   "example.com/project/app/contracts.Reader",
		Constructor: "example.com/vendor/impl.NewReader",
	}
	if bindings[0] != want {
		t.Fatalf("LoadBindings()[0] = %+v, want %+v", bindings[0], want)
	}
}

func TestLoadBindingsRejectsMalformedEntries(t *testing.T) {
	cases := map[string]string{
		"missing dot":     "bindings:\n  - interface: example.com/a/contracts\n    constructor: example.com/b.New\n",
		"invalid name":    "bindings:\n  - interface: example.com/a.1Reader\n    constructor: example.com/b.New\n",
		"empty field":     "bindings:\n  - interface: example.com/a.Reader\n    constructor: \"\"\n",
		"duplicate iface": "bindings:\n  - interface: example.com/a.Reader\n    constructor: example.com/b.New\n  - interface: example.com/a.Reader\n    constructor: example.com/c.Other\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, bindingsFileName, content)
			if _, err := LoadBindings(fs.OS{}, filepath.Join(root, bindingsFileName)); err == nil {
				t.Fatal("LoadBindings() error = nil, want error")
			}
		})
	}
}

func TestBindingNodesRejectsInterfaceOutsideApp(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "thirdparty/impl/impl.go", `package impl
type Reader struct{}
func (*Reader) Read() string { return "" }
func NewReader() *Reader { return &Reader{} }
`)

	bindings := []Binding{{
		Interface:   "example.com/project/app/contracts.Reader",
		Constructor: "example.com/project/thirdparty/impl.NewReader",
	}}
	_, err := bindingNodes(bindings, map[string]struct{}{}, root)
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("bindingNodes() error = %v, want an undeclared-interface error", err)
	}
}

func TestBindingNodesRejectsConstructorNotImplementingInterface(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeProjectFile(t, root, "thirdparty/impl/impl.go", `package impl
type Writer struct{}
func NewWriter() *Writer { return &Writer{} }
`)

	interfaceKey := "example.com/project/app/contracts.Reader"
	knownKey := interfaceKey
	bindings := []Binding{{
		Interface:   interfaceKey,
		Constructor: "example.com/project/thirdparty/impl.NewWriter",
	}}
	known := map[string]struct{}{knownKey: {}}
	_, err := bindingNodes(bindings, known, filepath.Join(root, "app"))
	if err == nil || !strings.Contains(err.Error(), "does not implement") {
		t.Fatalf("bindingNodes() error = %v, want a does-not-implement error", err)
	}
}

func TestGenerateContainerEndToEndWiresBindingToThirdPartyConstructor(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "app/module.go", "package app\n")
	writeProjectFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeProjectFile(t, root, "app/use_case/greet/interactor.go", `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader) (*Interactor, error) {
	return &Interactor{}, nil
}
type Interactor struct{}
`)
	writeProjectFile(t, root, "thirdparty/impl/impl.go", `package impl
type Reader struct{}
func NewReader() *Reader { return &Reader{} }
func (*Reader) Read() string { return "" }
`)
	writeProjectFile(t, root, "app/adapters/composition/"+bindingsFileName, `bindings:
  - interface: example.com/project/app/contracts.Reader
    constructor: example.com/project/thirdparty/impl.NewReader
`)

	runner := NewRunner(fs.OS{})
	bindings, err := LoadBindings(fs.OS{}, filepath.Join(root, "app", "adapters", "composition", bindingsFileName))
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	graph, err := runner.BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app"), Bindings: bindings})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if len(graph.Unresolved) > 0 {
		t.Fatalf("Unresolved = %+v, want none", graph.Unresolved)
	}
	outPath, err := runner.GenerateContainer(graph, GenerateOptions{OutputDir: filepath.Join(root, "app", "adapters", "composition")})
	if err != nil {
		t.Fatalf("GenerateContainer() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "impl.NewReader()") {
		t.Errorf("generated source does not call the bound constructor:\n%s", data)
	}

	assertGoBuildSucceeds(t, root)
}

func TestBindInterfaceCreatesAndAppendsBindings(t *testing.T) {
	root := newBindingProject(t)
	runner := NewDefaultRunner()

	path, err := runner.BindInterface(root, "internal/app", "catalog", Binding{
		Interface:   "example.com/project/internal/app/catalog/contracts.Reader",
		Constructor: "example.com/vendor/impl.NewReader",
	})
	if err != nil {
		t.Fatalf("BindInterface() error = %v", err)
	}

	bindings, err := LoadBindings(fs.OS{}, path)
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].Constructor != "example.com/vendor/impl.NewReader" {
		t.Fatalf("LoadBindings() = %v, want one NewReader binding", bindings)
	}

	if _, err := runner.BindInterface(root, "internal/app", "catalog", Binding{
		Interface:   "example.com/project/internal/app/catalog/contracts.Writer",
		Constructor: "example.com/vendor/impl.NewWriter",
	}); err != nil {
		t.Fatalf("BindInterface() second binding error = %v", err)
	}

	bindings, err = LoadBindings(fs.OS{}, path)
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	if len(bindings) != 2 {
		t.Fatalf("LoadBindings() len = %d, want 2", len(bindings))
	}
}

func TestBindInterfaceRejectsDuplicateInterface(t *testing.T) {
	root := newBindingProject(t)
	runner := NewDefaultRunner()
	binding := Binding{
		Interface:   "example.com/project/internal/app/catalog/contracts.Reader",
		Constructor: "example.com/vendor/impl.NewReader",
	}
	if _, err := runner.BindInterface(root, "internal/app", "catalog", binding); err != nil {
		t.Fatalf("BindInterface() error = %v", err)
	}
	if _, err := runner.BindInterface(root, "internal/app", "catalog", binding); err == nil {
		t.Fatal("BindInterface() error = nil, want already-bound error")
	}
}

func TestBindInterfaceRejectsMalformedName(t *testing.T) {
	root := newBindingProject(t)
	_, err := NewDefaultRunner().BindInterface(root, "internal/app", "catalog", Binding{
		Interface:   "Reader",
		Constructor: "example.com/vendor/impl.NewReader",
	})
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("BindInterface() error = %v, want format error", err)
	}
}

func newBindingProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	if err := os.MkdirAll(filepath.Join(root, "internal", "app", "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBindingOverridesAnnotatedProviderAndWarns(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	writeProjectFile(t, root, "app/module.go", "package app\n")
	writeProjectFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface{ Read() string }
`)
	writeProjectFile(t, root, "app/adapters/annotated/reader.go", `package annotated
import "example.com/project/app/contracts"
//di:example.com/project/app/contracts.Reader
func NewAnnotatedReader() *AnnotatedReader { return &AnnotatedReader{} }
type AnnotatedReader struct{}
func (*AnnotatedReader) Read() string { return "annotated" }
`)
	writeProjectFile(t, root, "thirdparty/impl/impl.go", `package impl
type Reader struct{}
func NewReader() *Reader { return &Reader{} }
func (*Reader) Read() string { return "" }
`)
	writeProjectFile(t, root, "app/use_case/greet/interactor.go", `package greet
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader) (*Interactor, error) {
	return &Interactor{}, nil
}
type Interactor struct{}
`)
	writeProjectFile(t, root, "app/adapters/composition/"+bindingsFileName, `bindings:
  - interface: example.com/project/app/contracts.Reader
    constructor: example.com/project/thirdparty/impl.NewReader
`)

	bindings, err := LoadBindings(fs.OS{}, filepath.Join(root, "app", "adapters", "composition", bindingsFileName))
	if err != nil {
		t.Fatalf("LoadBindings() error = %v", err)
	}
	graph, err := NewRunner(fs.OS{}).BuildDependencyGraph(ScanOptions{Root: filepath.Join(root, "app"), Bindings: bindings})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}

	if len(graph.Unresolved) > 0 {
		t.Fatalf("Unresolved = %+v, want none: binding should win over annotation", graph.Unresolved)
	}
	if len(graph.Warnings) != 1 || !strings.Contains(graph.Warnings[0], "annotated/reader.go") ||
		!strings.Contains(graph.Warnings[0], "NewReader") {
		t.Fatalf("Warnings = %q, want one warning naming the annotated provider and the binding", graph.Warnings)
	}
}
