package goroute

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/fs"
	"github.com/rvadym/gocompose/pkg/internal/testfs"
)

func TestBuildSourceMapsRoutesToContainerFields(t *testing.T) {
	root := virtualRoutePath(t, "route-project")
	outputDir := filepath.Join(root, "app/adapters/composition")
	fileSystem := routeProject(root, map[string]string{
		"api_index": "package api_index\n\n//router:get /index/:message\n//\n//di:wire\nfunc New() int { return 0 }\n",
		"api_ping":  "package api_ping\n\n// router:get /ping\n//router:post /ping\n//di:wire\nfunc New() int { return 0 }\n",
	})
	generator := NewGenerator(fileSystem)
	source, outPath, err := generator.Render(Options{Root: filepath.Join(root, "app"), OutputDir: outputDir})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(outputDir, outputFile); outPath != want {
		t.Fatalf("buildSource() path = %q, want %q", outPath, want)
	}
	src := string(source)
	for _, want := range []string{
		"package composition",
		`"example.com/project/internal/pkg/infra"`,
		"func GetRoutes(c *Container) (infra.HandlersConfigs, error) {",
		`Path:        "/index/:message",`,
		`Method:      "GET",`,
		`HandlerType: fmt.Sprintf("%T", c.ApiIndex),`,
		"Handler:     c.ApiIndex.Handle,",
		"if c.ApiPing == nil {",
		`Method:      "POST",`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated source missing %q:\n%s", want, src)
		}
	}
}

func TestScanErrors(t *testing.T) {
	tests := map[string]struct {
		handlers map[string]string
		want     string
	}{
		"missing wire": {
			map[string]string{"a": "package a\n//router:get /a\nfunc New() int { return 0 }\n"},
			"no //di:wire",
		},
		"bad path": {
			map[string]string{"a": "package a\n//router:GET a\n//di:wire\nfunc New() int { return 0 }\n"},
			"must start with '/'",
		},
		"missing method": {
			map[string]string{"a": "package a\n//router:/a\n//di:wire\nfunc New() int { return 0 }\n"},
			"want \"<method> /path\"",
		},
		"bad method": {
			map[string]string{"a": "package a\n//router:fetch /a\n//di:wire\nfunc New() int { return 0 }\n"},
			"invalid HTTP method",
		},
		"duplicate method and path": {
			map[string]string{
				"a": "package a\n//router:get /x\n//di:wire\nfunc New() int { return 0 }\n",
				"b": "package b\n//router:GET /x\n//di:wire\nfunc New() int { return 0 }\n",
			},
			"already declared",
		},
		"method": {
			map[string]string{"a": "package a\ntype T struct{}\n//router:get /a\n//di:wire\nfunc (T) New() int { return 0 }\n"},
			"not method",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			root := virtualRoutePath(t, "route-error-"+name)
			fileSystem := routeProject(root, tc.handlers)
			_, err := NewGenerator(fileSystem).Scan(filepath.Join(root, "app"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Scan() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestScanAllowsSamePathWithDifferentMethods(t *testing.T) {
	root := virtualRoutePath(t, "route-methods")
	fileSystem := routeProject(root, map[string]string{
		"api_get":  "package api_get\n//router:get /items\n//di:wire\nfunc New() int { return 0 }\n",
		"api_post": "package api_post\n//router:post /items\n//di:wire\nfunc New() int { return 0 }\n",
	})
	routes, err := NewGenerator(fileSystem).Scan(filepath.Join(root, "app"))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("Scan() returned %d routes, want 2: %#v", len(routes), routes)
	}
	if routes[0].key() != "GET /items" || routes[1].key() != "POST /items" {
		t.Fatalf("Scan() routes = %#v, want GET and POST /items", routes)
	}
}

func TestBuildSourceWithoutRoutesProducesEmptyRoutes(t *testing.T) {
	root := virtualRoutePath(t, "route-empty")
	fileSystem := routeProject(root, nil)
	source, _, err := NewGenerator(fileSystem).Render(Options{
		Root:      filepath.Join(root, "app"),
		OutputDir: filepath.Join(root, "app/adapters/composition"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func GetRoutes(c *Container) (infra.HandlersConfigs, error) {",
		"routes := make(infra.HandlersConfigs, 0, 0)",
		"return routes, nil",
	} {
		if !strings.Contains(string(source), want) {
			t.Errorf("generated empty routes missing %q:\n%s", want, source)
		}
	}
}

func virtualRoutePath(t *testing.T, name string) string {
	t.Helper()
	root, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func routeProject(root string, handlers map[string]string) fs.FileSystem {
	files := map[string]string{
		filepath.Join(root, "go.mod"):                                "module example.com/project\n\ngo 1.21\n",
		filepath.Join(root, "app/adapters/composition/container.go"): "package composition\n\ntype Container struct{}\n",
	}
	for dir, source := range handlers {
		files[filepath.Join(root, "app/adapters/server", dir, "handler.go")] = source
	}
	return testfs.Map{Files: files}
}
