package godicb

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanFindsMultipleDirectivesOnFunction(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "wire.go", `package sample

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
`)

	directives, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(directives) != 2 {
		t.Fatalf("got %d directives, want 2: %#v", len(directives), directives)
	}

	got := []Directive{
		{
			File: filepath.Join(root, "wire.go"), Line: 10, Kind: FunctionDeclaration, Name: "NewService", Value: "wire",
			Parameters: []Parameter{
				{Name: "ctx", Type: "context.Context"},
				{Name: "cancel", Type: "context.Context"},
				{Name: "reader", Type: "example.com/project/contracts.Reader"},
				{Name: "values", Type: "...any"},
			},
			Results: []string{"*Service"},
		},
		{
			File: filepath.Join(root, "wire.go"), Line: 11, Kind: FunctionDeclaration, Name: "NewService", Value: "example.com/project/contracts.Service@dev",
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
	root := t.TempDir()
	withDir := filepath.Join(t.TempDir(), "single")
	withTree := filepath.Join(t.TempDir(), "nested")
	writeGoFile(t, root, "root.go", "package root\n//di:wire\nfunc Root() {}")
	writeGoFile(t, withDir, "extra.go", "package extra\n//di:example.com/contracts.Reader\nfunc NewReader() {}")
	writeGoFile(t, filepath.Join(withTree, "child"), "tree.go", "package child\n//di:example.com/contracts.Writer\nfunc NewWriter() {}")

	directives, err := Scan(ScanOptions{
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
	root := t.TempDir()
	writeGoFile(t, root, "bad.go", "package sample\n//di\nfunc NewService() {}")

	_, err := Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "malformed directive") {
		t.Fatalf("Scan() error = %v, want malformed directive error", err)
	}
}

func TestScanRejectsTypeLevelDirective(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "bad.go", "package sample\n//di:example.com/project/contracts.Reader\ntype Reader struct{}")

	_, err := Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "only annotate constructor/factory functions") {
		t.Fatalf("Scan() error = %v, want function-only directive error", err)
	}
}

func TestScanRejectsMissingOrEmptyInputDirectories(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "root.go", "package root")
	empty := t.TempDir()

	_, err := Scan(ScanOptions{Root: root, WithDirs: []string{empty}})
	if err == nil || !strings.Contains(err.Error(), "contains no Go files") {
		t.Fatalf("Scan() error = %v, want no Go files error", err)
	}
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
