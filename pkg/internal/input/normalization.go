// Package input normalizes and validates project and application inputs.
package input

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rvadym/gocompose/pkg/internal/fs"
)

var (
	ErrCannotResolveProjectPath = errors.New("cannot resolve project path")
	ErrDiscoveryFailed          = errors.New("apps discovery failed")
)

// NormalizePaths resolves projectPath, appsPath, and requestedApps using
// fileSystem. When requestedApps is empty, it discovers non-hidden app
// directories beneath appsPath.
func NormalizePaths(
	projectPath string,
	appsPath string,
	requestedApps []string,
	fileSystem fs.FileSystem,
) (string, string, []string, error) {
	originalProjectPath := projectPath
	projectPath, err := fileSystem.Abs(projectPath)
	if err != nil {
		return "", "", nil, fmt.Errorf("%w: `%s`: %v", ErrCannotResolveProjectPath, originalProjectPath, err)
	}

	if appsPath == "" {
		appsPath = projectPath
	} else if !filepath.IsAbs(appsPath) {
		appsPath = filepath.Join(projectPath, appsPath)
	}
	appsPath = filepath.Clean(appsPath)

	if len(requestedApps) == 0 {
		var err error
		requestedApps, err = discover(appsPath, fileSystem)
		if err != nil {
			return "", "", nil, err
		}
	}

	resolvedApps := make([]string, len(requestedApps))
	for i, app := range requestedApps {
		if app != "" {
			if filepath.IsAbs(app) {
				app = filepath.Clean(app)
			} else {
				app = filepath.Join(appsPath, app)
			}
		}
		resolvedApps[i] = app
	}

	return projectPath, appsPath, resolvedApps, nil
}

func discover(appsPath string, fileSystem fs.FileSystem) ([]string, error) {
	apps := make([]string, 0)
	if !fileSystem.IsDirectory(appsPath) {
		return apps, nil
	}

	entries, err := fileSystem.ReadDir(appsPath)
	if err != nil {
		return apps, fmt.Errorf("%w: %v", ErrDiscoveryFailed, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if fileSystem.IsDirectory(filepath.Join(appsPath, entry.Name())) {
			apps = append(apps, entry.Name())
		}
	}
	return apps, nil
}
