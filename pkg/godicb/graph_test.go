package godicb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDependencyGraphResolvesProvidersAndReportsMissingInterfaces(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.26\n")
	writeGraphFile(t, root, "app/contracts/contracts.go", `package contracts
type Reader interface { Read() }
type Writer interface { Write() }
type Missing interface { Missing() }
`)
	writeGraphFile(t, root, "app/consumer/consumer.go", `package consumer
import "example.com/project/app/contracts"
//di:wire
func New(reader contracts.Reader, writer contracts.Writer, missing contracts.Missing, limit int) *Service {
	return nil
}
type Service struct{}
`)
	writeGraphFile(t, root, "adapters/reader.go", `package adapters
//di:Reader
//di:Writer
func NewReader() *Reader { return nil }
type Reader struct{}
func (*Reader) Read() {}
func (*Reader) Write() {}
`)

	graph, err := BuildDependencyGraph(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("BuildDependencyGraph() error = %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("got %d nodes, want consumer and provider: %#v", len(graph.Nodes), graph.Nodes)
	}
	var consumer GraphNode
	for _, node := range graph.Nodes {
		if node.Function == "example.com/project/app/consumer.New" {
			consumer = node
			break
		}
	}
	if consumer.Function == "" {
		t.Fatalf("consumer function missing from graph: %#v", graph.Nodes)
	}
	if len(consumer.Dependencies) != 2 {
		t.Fatalf("consumer dependencies = %#v, want Reader and Writer provider edges", consumer.Dependencies)
	}
	for _, dependency := range consumer.Dependencies {
		if dependency.Provider != "example.com/project/adapters.NewReader" {
			t.Fatalf("dependency %s resolved to %s, want NewReader", dependency.Parameter, dependency.Provider)
		}
	}
	if consumer.Dependencies[0].Type == consumer.Dependencies[1].Type {
		t.Fatalf("multiple interface tags did not resolve independently: %#v", consumer.Dependencies)
	}
	if len(graph.Unresolved) != 1 ||
		graph.Unresolved[0].Type != "example.com/project/app/contracts.Missing" ||
		graph.Unresolved[0].Reason != "no tagged provider" {
		t.Fatalf("unresolved dependencies = %#v, want missing contracts.Missing", graph.Unresolved)
	}
	if len(graph.External) != 1 || graph.External[0].Type != "int" {
		t.Fatalf("external dependencies = %#v, want int input", graph.External)
	}
}

func TestBuildDependencyGraphRejectsAmbiguousShortInterfaceTag(t *testing.T) {
	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/project\n\ngo 1.26\n")
	writeGraphFile(t, root, "contracts/one/contract.go", "package one\ntype Reader interface { ReadOne() }\n")
	writeGraphFile(t, root, "contracts/two/contract.go", "package two\ntype Reader interface { ReadTwo() }\n")
	writeGraphFile(t, root, "consumer/consumer.go", `package consumer
import (
	"example.com/project/contracts/one"
	"example.com/project/contracts/two"
)
//di:wire
func New(one one.Reader, two two.Reader) {}
`)
	writeGraphFile(t, root, "adapters/reader.go", `package adapters
//di:Reader
func NewReader() *Reader { return nil }
type Reader struct{}
func (*Reader) ReadOne() {}
func (*Reader) ReadTwo() {}
`)

	_, err := BuildDependencyGraph(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("BuildDependencyGraph() error = %v, want ambiguous short interface tag", err)
	}
}

func writeGraphFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
