package input

import (
	"errors"
	"fmt"

	"github.com/rvadym/gocompose/pkg/internal/fs"
)

var (
	ErrProjectPathDoesNotExist   = errors.New("project path does not exist")
	ErrProjectPathIsNotDirectory = errors.New("project path is not a directory")

	ErrCannotResolveAppsPath  = errors.New("cannot resolve apps path")
	ErrAppsPathDoesNotExist   = errors.New("apps path does not exist")
	ErrAppsPathIsNotDirectory = errors.New("apps path is not a directory")

	ErrCannotResolveAppPath  = errors.New("cannot resolve app path")
	ErrAppPathDoesNotExist   = errors.New("app path does not exist")
	ErrAppPathIsNotDirectory = errors.New("app path is not a directory")

	ErrNoAppsFound = errors.New("no apps found")
)

// ValidatePaths checks the normalized project, apps, and app paths.
func ValidatePaths(projectPath, appsPath string, apps []string, fileSystem fs.FileSystem) error {
	var errs []error

	if err := validateProjectPath(projectPath, fileSystem); err != nil {
		errs = append(errs, err)
	}
	appsPathErr := validateAppsPath(appsPath, fileSystem)
	if appsPathErr != nil {
		errs = append(errs, appsPathErr)
	}
	if len(apps) == 0 && appsPathErr == nil {
		errs = append(errs, fmt.Errorf("%w in `%s`", ErrNoAppsFound, appsPath))
	}
	if err := validateApps(apps, fileSystem); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateProjectPath(projectPath string, fileSystem fs.FileSystem) error {
	if len(projectPath) == 0 {
		return fmt.Errorf("%w: `%s`", ErrCannotResolveProjectPath, projectPath)
	}
	if !fileSystem.Exists(projectPath) {
		return fmt.Errorf("%w: `%s`", ErrProjectPathDoesNotExist, projectPath)
	}
	if !fileSystem.IsDirectory(projectPath) {
		return fmt.Errorf("%w: `%s`", ErrProjectPathIsNotDirectory, projectPath)
	}
	return nil
}

func validateAppsPath(appsPath string, fileSystem fs.FileSystem) error {
	if len(appsPath) == 0 {
		return fmt.Errorf("%w: `%s`", ErrCannotResolveAppsPath, appsPath)
	}
	if !fileSystem.Exists(appsPath) {
		return fmt.Errorf("%w: `%s`", ErrAppsPathDoesNotExist, appsPath)
	}
	if !fileSystem.IsDirectory(appsPath) {
		return fmt.Errorf("%w: `%s`", ErrAppsPathIsNotDirectory, appsPath)
	}
	return nil
}

func validateApps(apps []string, fileSystem fs.FileSystem) error {
	var errs []error
	for _, app := range apps {
		if len(app) == 0 {
			errs = append(errs, fmt.Errorf("%w: `%s`", ErrCannotResolveAppPath, app))
			continue
		}
		if !fileSystem.Exists(app) {
			errs = append(errs, fmt.Errorf("%w: `%s`", ErrAppPathDoesNotExist, app))
			continue
		}
		if !fileSystem.IsDirectory(app) {
			errs = append(errs, fmt.Errorf("%w: `%s`", ErrAppPathIsNotDirectory, app))
		}
	}
	return errors.Join(errs...)
}
