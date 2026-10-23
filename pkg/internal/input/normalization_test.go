package input

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/testfs"
)

func TestNormalizePathsDiscoversAppsInVirtualProject(t *testing.T) {
	t.Parallel()

	fileSystem := testfs.Map{
		Files: map[string]string{
			"/project/internal/apps/README.md": "",
		},
		Dirs: []string{
			"/project",
			"/project/internal/apps",
			"/project/internal/apps/billing",
			"/project/internal/apps/.hidden",
		},
	}

	projectPath, appsPath, apps, err := NormalizePaths("project", "internal/apps", nil, fileSystem)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/project"; projectPath != want {
		t.Errorf("project path = %q, want %q", projectPath, want)
	}
	if want := "/project/internal/apps"; appsPath != want {
		t.Errorf("apps path = %q, want %q", appsPath, want)
	}
	wantApps := []string{filepath.Join(appsPath, "billing")}
	if !reflect.DeepEqual(apps, wantApps) {
		t.Errorf("apps = %v, want %v", apps, wantApps)
	}
}

func TestNormalizePathsPreservesAbsoluteAppsAndInput(t *testing.T) {
	t.Parallel()

	requested := []string{"/other/identity"}
	_, _, apps, err := NormalizePaths("/project", "apps", requested, testfs.Map{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/other/identity"}; !reflect.DeepEqual(apps, want) {
		t.Errorf("apps = %v, want %v", apps, want)
	}
	if want := []string{"/other/identity"}; !reflect.DeepEqual(requested, want) {
		t.Errorf("requested apps mutated to %v", requested)
	}
}
