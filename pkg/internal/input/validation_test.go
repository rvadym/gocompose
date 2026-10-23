package input

import (
	"errors"
	"testing"

	"github.com/rvadym/gocompose/pkg/internal/testfs"
)

func TestValidatePathsAcceptsExistingDirectories(t *testing.T) {
	t.Parallel()

	err := ValidatePaths(
		"/project",
		"/project/apps",
		[]string{"/project/apps/identity"},
		testfs.Map{Dirs: []string{
			"/project",
			"/project/apps",
			"/project/apps/identity",
		}},
	)
	if err != nil {
		t.Fatalf("ValidatePaths() error = %v", err)
	}
}

func TestValidatePathsErrors(t *testing.T) {
	t.Parallel()

	const (
		projectPath = "/project"
		appsPath    = "/project/apps"
	)
	tests := []struct {
		name        string
		projectPath string
		appsPath    string
		apps        []string
		fileSystem  testfs.Map
		want        error
	}{
		{
			name:        "missing project path",
			projectPath: "/missing",
			appsPath:    appsPath,
			apps:        []string{"/project/apps/identity"},
			fileSystem: testfs.Map{
				Dirs: []string{appsPath, "/project/apps/identity"},
			},
			want: ErrProjectPathDoesNotExist,
		},
		{
			name:        "missing apps path",
			projectPath: projectPath,
			appsPath:    "/project/missing",
			fileSystem: testfs.Map{
				Dirs: []string{projectPath},
			},
			want: ErrAppsPathDoesNotExist,
		},
		{
			name:        "missing app",
			projectPath: projectPath,
			appsPath:    appsPath,
			apps:        []string{"/project/apps/unknown"},
			fileSystem: testfs.Map{
				Dirs: []string{projectPath, appsPath},
			},
			want: ErrAppPathDoesNotExist,
		},
		{
			name:        "no apps",
			projectPath: projectPath,
			appsPath:    appsPath,
			fileSystem: testfs.Map{
				Dirs: []string{projectPath, appsPath},
			},
			want: ErrNoAppsFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePaths(tc.projectPath, tc.appsPath, tc.apps, tc.fileSystem)
			if err == nil || !errors.Is(err, tc.want) {
				t.Fatalf("ValidatePaths() error = %v, want %v", err, tc.want)
			}
		})
	}
}
