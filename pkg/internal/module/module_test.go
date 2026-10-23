package module

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type moduleCase struct {
	name       string
	files      map[string]string
	start      string
	wantRoot   string
	wantModule string
	wantErr    bool
}

type mapReader map[string]string

func (reader mapReader) ReadFile(path string) ([]byte, error) {
	content, ok := reader[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(content), nil
}

func (mapReader) Abs(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(string(filepath.Separator), path), nil
}

func TestFindModule(t *testing.T) {
	root, err := filepath.Abs("virtual-project")
	if err != nil {
		t.Fatal(err)
	}
	relativeRoot := filepath.Join(string(filepath.Separator), "virtual-project")
	tests := []moduleCase{
		{
			name:       "go.mod in start directory",
			files:      map[string]string{filepath.Join(root, "go.mod"): "module example.com/project\n\ngo 1.21\n"},
			start:      root,
			wantRoot:   root,
			wantModule: "example.com/project",
		},
		{
			name:       "go.mod in ancestor directory",
			files:      map[string]string{filepath.Join(root, "go.mod"): "module example.com/project\n"},
			start:      filepath.Join(root, "a", "b", "c"),
			wantRoot:   root,
			wantModule: "example.com/project",
		},
		{
			name: "nearest go.mod wins",
			files: map[string]string{
				filepath.Join(root, "go.mod"):          "module example.com/outer\n",
				filepath.Join(root, "inner", "go.mod"): "module example.com/inner\n",
			},
			start:      filepath.Join(root, "inner", "pkg"),
			wantRoot:   filepath.Join(root, "inner"),
			wantModule: "example.com/inner",
		},
		{
			name:       "module directive after other lines and with CRLF",
			files:      map[string]string{filepath.Join(root, "go.mod"): "// comment\r\nmodule example.com/crlf\r\n\r\ngo 1.21\r\n"},
			start:      root,
			wantRoot:   root,
			wantModule: "example.com/crlf",
		},
		{
			name:    "go.mod without module directive",
			files:   map[string]string{filepath.Join(root, "go.mod"): "go 1.21\n"},
			start:   root,
			wantErr: true,
		},
		{
			name:    "empty go.mod",
			files:   map[string]string{filepath.Join(root, "go.mod"): ""},
			start:   root,
			wantErr: true,
		},
		{
			name:    "no go.mod above start",
			start:   filepath.Join(root, "a", "b"),
			wantErr: true,
		},
		{
			name:       "relative start path",
			files:      map[string]string{filepath.Join(relativeRoot, "go.mod"): "module example.com/relative\n"},
			start:      "virtual-project/pkg",
			wantRoot:   relativeRoot,
			wantModule: "example.com/relative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := mapReader(tc.files)

			gotRoot, gotModule, err := NewFinder(reader).FindModule(tc.start)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("findModule(%q) = (%q, %q, nil), want error", tc.start, gotRoot, gotModule)
				}
				return
			}
			if err != nil {
				t.Fatalf("findModule(%q) error: %v", tc.start, err)
			}
			if gotRoot != tc.wantRoot {
				t.Errorf("root = %q, want %q", gotRoot, tc.wantRoot)
			}
			if gotModule != tc.wantModule {
				t.Errorf("module = %q, want %q", gotModule, tc.wantModule)
			}
		})
	}
}

func TestFindModuleReturnsReadError(t *testing.T) {
	t.Parallel()

	wantErr := os.ErrPermission
	_, _, err := NewFinder(errReader{err: wantErr}).FindModule("virtual-project")
	if !errors.Is(err, wantErr) {
		t.Errorf("findModule() error = %v, want %v", err, wantErr)
	}
}

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs("virtual-root-only")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "go.mod"): "module example.com/root\n",
	}

	got, err := NewFinder(mapReader(files)).FindModuleRoot(filepath.Join(root, "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindModuleRoot() = %q, want %q", got, root)
	}
}

type errReader struct {
	err error
}

func (reader errReader) ReadFile(string) ([]byte, error) {
	return nil, reader.err
}

func (errReader) Abs(path string) (string, error) {
	return filepath.Abs(path)
}
