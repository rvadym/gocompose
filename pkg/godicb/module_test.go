package godicb

import (
	"os"
	"path/filepath"
	"testing"
)

type moduleCase struct {
	name       string
	files      map[string]string
	dirs       []string
	start      string // relative to the temp root
	wantRoot   string // relative to the temp root
	wantModule string
	wantErr    bool
}

func moduleCases() []moduleCase {
	return []moduleCase{
		{
			name:       "go.mod in start directory",
			files:      map[string]string{"go.mod": "module example.com/project\n\ngo 1.21\n"},
			start:      ".",
			wantRoot:   ".",
			wantModule: "example.com/project",
		},
		{
			name:       "go.mod in ancestor directory",
			files:      map[string]string{"go.mod": "module example.com/project\n"},
			dirs:       []string{"a/b/c"},
			start:      "a/b/c",
			wantRoot:   ".",
			wantModule: "example.com/project",
		},
		{
			name: "nearest go.mod wins",
			files: map[string]string{
				"go.mod":       "module example.com/outer\n",
				"inner/go.mod": "module example.com/inner\n",
			},
			dirs:       []string{"inner/pkg"},
			start:      "inner/pkg",
			wantRoot:   "inner",
			wantModule: "example.com/inner",
		},
		{
			name:       "module directive after other lines and with CRLF",
			files:      map[string]string{"go.mod": "// comment\r\nmodule example.com/crlf\r\n\r\ngo 1.21\r\n"},
			start:      ".",
			wantRoot:   ".",
			wantModule: "example.com/crlf",
		},
		{
			name:    "go.mod without module directive",
			files:   map[string]string{"go.mod": "go 1.21\n"},
			start:   ".",
			wantErr: true,
		},
		{
			name:    "empty go.mod",
			files:   map[string]string{"go.mod": ""},
			start:   ".",
			wantErr: true,
		},
		{
			name:    "go.mod is a directory",
			dirs:    []string{"go.mod"},
			start:   ".",
			wantErr: true,
		},
		{
			name:    "no go.mod above start",
			dirs:    []string{"a/b"},
			start:   "a/b",
			wantErr: true,
		},
	}
}

func prepareModuleCase(t *testing.T, tc moduleCase) (root, start string) {
	t.Helper()
	root = t.TempDir()
	for _, dir := range tc.dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range tc.files {
		writeGraphFile(t, root, name, content)
	}
	return root, filepath.Join(root, tc.start)
}

func TestFindModule(t *testing.T) {
	t.Parallel()

	for _, tc := range moduleCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, start := prepareModuleCase(t, tc)

			gotRoot, gotModule, err := findModule(start)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("findModule(%q) = (%q, %q, nil), want error", start, gotRoot, gotModule)
				}
				return
			}
			if err != nil {
				t.Fatalf("findModule(%q) error: %v", start, err)
			}
			if want := filepath.Join(root, tc.wantRoot); gotRoot != want {
				t.Errorf("root = %q, want %q", gotRoot, want)
			}
			if gotModule != tc.wantModule {
				t.Errorf("module = %q, want %q", gotModule, tc.wantModule)
			}
		})
	}
}

func TestFindModuleResolvesRelativeStart(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGraphFile(t, root, "go.mod", "module example.com/relative\n")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	start, err := filepath.Rel(wd, root)
	if err != nil {
		t.Fatal(err)
	}

	gotRoot, gotModule, err := findModule(start)
	if err != nil {
		t.Fatal(err)
	}
	if gotRoot != root {
		t.Errorf("root = %q, want %q", gotRoot, root)
	}
	if gotModule != "example.com/relative" {
		t.Errorf("module = %q, want %q", gotModule, "example.com/relative")
	}
}

func TestFindModuleRoot(t *testing.T) {
	t.Parallel()

	for _, tc := range moduleCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root, start := prepareModuleCase(t, tc)

			got, err := FindModuleRoot(start)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("FindModuleRoot(%q) = %q, nil, want error", start, got)
				}
				if got != "" {
					t.Errorf("FindModuleRoot(%q) = %q on error, want empty", start, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindModuleRoot(%q) error: %v", start, err)
			}
			if want := filepath.Join(root, tc.wantRoot); got != want {
				t.Errorf("FindModuleRoot(%q) = %q, want %q", start, got, want)
			}
		})
	}
}
