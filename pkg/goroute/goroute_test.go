package goroute

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T, handlers map[string]string) (root string) {
	t.Helper()
	root = t.TempDir()
	write(t, root, "go.mod", "module example.com/project\n\ngo 1.21\n")
	write(t, root, "app/adapters/composition/container.go", "package composition\n\ntype Container struct{}\n")
	for dir, src := range handlers {
		write(t, root, "app/adapters/server/"+dir+"/handler.go", src)
	}
	return root
}

func TestGenerateMapsRoutesToContainerFields(t *testing.T) {
	root := project(t, map[string]string{
		"api_index": "package api_index\n\n//router:get /index/:message\n//\n//di:wire\nfunc New() int { return 0 }\n",
		"api_ping":  "package api_ping\n\n// router:get /ping\n//router:post /ping\n//di:wire\nfunc New() int { return 0 }\n",
	})
	out, err := Generate(Options{Root: filepath.Join(root, "app"), OutputDir: filepath.Join(root, "app/adapters/composition")})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	src := string(data)
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
			root := project(t, tc.handlers)
			_, err := Scan(filepath.Join(root, "app"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Scan() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestScanAllowsSamePathWithDifferentMethods(t *testing.T) {
	root := project(t, map[string]string{
		"api_get":  "package api_get\n//router:get /items\n//di:wire\nfunc New() int { return 0 }\n",
		"api_post": "package api_post\n//router:post /items\n//di:wire\nfunc New() int { return 0 }\n",
	})
	routes, err := Scan(filepath.Join(root, "app"))
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

func TestGenerateWithoutRoutesWritesEmptyRoutes(t *testing.T) {
	root := project(t, nil)
	stale := filepath.Join(root, "app/adapters/composition/router.go")
	write(t, root, "app/adapters/composition/router.go", generatedMark+"\n\npackage composition\n// stale generated content\n")

	out, err := Generate(Options{Root: filepath.Join(root, "app"), OutputDir: filepath.Dir(stale)})
	if err != nil || out != stale {
		t.Fatalf("Generate() = %q, %v; want %q", out, err, stale)
	}
	data, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func GetRoutes(c *Container) (infra.HandlersConfigs, error) {",
		"routes := make(infra.HandlersConfigs, 0, 0)",
		"return routes, nil",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("generated empty routes missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "stale generated content") {
		t.Fatal("stale generated content remains")
	}
}
