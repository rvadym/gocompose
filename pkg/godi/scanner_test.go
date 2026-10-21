package godi

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/testfs"
)

func TestScanFindsMultipleDirectivesOnFunction(t *testing.T) {
	root := virtualPath(t, "scan-project")
	file := filepath.Join(root, "wire.go")
	fileSystem := testfs.Map{Files: map[string]string{
		file: `package sample

import (
	"context"
	contracts "example.com/project/contracts"
)

// NewService creates the service.
//
//di:wire
//di:example.com/project/contracts.Service@dev
func NewService(ctx, cancel context.Context, reader contracts.Reader, values ...any) *Service { return nil }

// directory-scoped documentation is not a DI directive.
func DocumentedHelper() {}

// This is prose, not a directive: //di:Ignored
var example = "//di:AlsoIgnored"
`,
	}}

	directives, err := NewRunner(fileSystem).Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(directives) != 2 {
		t.Fatalf("got %d directives, want 2: %#v", len(directives), directives)
	}

	got := []Directive{
		{
			File: file, Line: 10, Kind: FunctionDeclaration, Name: "NewService", Value: "wire",
			Parameters: []Parameter{
				{Name: "ctx", Type: "context.Context"},
				{Name: "cancel", Type: "context.Context"},
				{Name: "reader", Type: "example.com/project/contracts.Reader"},
				{Name: "values", Type: "...any"},
			},
			Results: []string{"*Service"},
		},
		{
			File: file, Line: 11, Kind: FunctionDeclaration, Name: "NewService", Value: "example.com/project/contracts.Service@dev",
			Parameters: []Parameter{
				{Name: "ctx", Type: "context.Context"},
				{Name: "cancel", Type: "context.Context"},
				{Name: "reader", Type: "example.com/project/contracts.Reader"},
				{Name: "values", Type: "...any"},
			},
			Results: []string{"*Service"},
		},
	}
	if !reflect.DeepEqual(directives, got) {
		t.Fatalf("Scan() directives = %#v, want %#v", directives, got)
	}
}

func TestScanIncludesExtraDirectoriesAndTrees(t *testing.T) {
	root := virtualPath(t, "scan-root")
	withDir := virtualPath(t, "scan-extra")
	withTree := virtualPath(t, "scan-tree")
	fileSystem := testfs.Map{Files: map[string]string{
		filepath.Join(root, "root.go"):              "package root\n//di:wire\nfunc Root() {}",
		filepath.Join(withDir, "extra.go"):          "package extra\n//di:example.com/contracts.Reader\nfunc NewReader() {}",
		filepath.Join(withTree, "child", "tree.go"): "package child\n//di:example.com/contracts.Writer\nfunc NewWriter() {}",
	}}

	directives, err := NewRunner(fileSystem).Scan(ScanOptions{
		Root:      root,
		WithDirs:  []string{withDir},
		WithTrees: []string{withTree},
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(directives) != 3 {
		t.Fatalf("got %d directives, want 3: %#v", len(directives), directives)
	}
	for i := 1; i < len(directives); i++ {
		if directives[i-1].File > directives[i].File {
			t.Fatalf("directives are not sorted by file: %#v", directives)
		}
	}
}

func TestScanRejectsMalformedDirective(t *testing.T) {
	root := virtualPath(t, "scan-malformed")
	fileSystem := testfs.Map{Files: map[string]string{
		filepath.Join(root, "bad.go"): "package sample\n//di\nfunc NewService() {}",
	}}

	_, err := NewRunner(fileSystem).Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "malformed directive") {
		t.Fatalf("Scan() error = %v, want malformed directive error", err)
	}
}

func TestScanRejectsTypeLevelDirective(t *testing.T) {
	root := virtualPath(t, "scan-type-level")
	fileSystem := testfs.Map{Files: map[string]string{
		filepath.Join(root, "bad.go"): "package sample\n//di:example.com/project/contracts.Reader\ntype Reader struct{}",
	}}

	_, err := NewRunner(fileSystem).Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "only annotate constructor/factory functions") {
		t.Fatalf("Scan() error = %v, want function-only directive error", err)
	}
}

func TestScanRejectsMissingOrEmptyInputDirectories(t *testing.T) {
	root := virtualPath(t, "scan-empty-with-dir")
	empty := filepath.Join(root, "empty")
	fileSystem := testfs.Map{
		Files: map[string]string{filepath.Join(root, "root.go"): "package root"},
		Dirs:  []string{empty},
	}

	_, err := NewRunner(fileSystem).Scan(ScanOptions{Root: root, WithDirs: []string{empty}})
	if err == nil || !strings.Contains(err.Error(), "contains no Go files") {
		t.Fatalf("Scan() error = %v, want no Go files error", err)
	}
}

func virtualPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
