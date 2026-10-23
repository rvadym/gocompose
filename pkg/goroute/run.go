package goroute

import (
	"fmt"
	"path/filepath"

	"github.com/rvadym/gocompose/pkg/internal/fs"
	"github.com/rvadym/gocompose/pkg/internal/input"
	"github.com/rvadym/gocompose/pkg/internal/module"
	"github.com/rvadym/gocompose/pkg/internal/output"
)

// Runner holds the dependencies used throughout a route generation run.
type Runner struct {
	fileSystem   fs.FileSystem
	moduleFinder *module.Finder
	printer      output.Printer
}

// NewDefaultRunner creates a runner bound to the operating system filesystem.
func NewDefaultRunner() *Runner {
	filesystem := fs.Default()
	return &Runner{
		fileSystem:   filesystem,
		moduleFinder: module.NewFinder(filesystem),
		printer:      output.DefaultPrinter(),
	}
}

// NewRunner creates a runner bound to fileSystem.
func NewRunner(fileSystem fs.FileSystem) *Runner {
	return &Runner{
		fileSystem:   fileSystem,
		moduleFinder: module.NewFinder(fileSystem),
		printer:      output.DefaultPrinter(),
	}
}

// Normalize resolves app paths and discovers apps using the runner's filesystem.
func (r *Runner) Normalize(projectPath, appsPath string, apps []string) (string, string, []string, error) {
	return input.NormalizePaths(projectPath, appsPath, apps, r.fileSystem)
}

// Validate validates paths using the runner's filesystem.
func (r *Runner) Validate(projectPath, appsPath string, apps []string) error {
	return input.ValidatePaths(projectPath, appsPath, apps, r.fileSystem)
}

func (r *Runner) Run(
	projectPath string,
	appsPath string,
	apps []string,
	verbosity1 bool,
) error {

	// Normalize
	projectPath, appsPath, apps, err := r.Normalize(
		projectPath, appsPath, apps,
	)
	if err != nil {
		return err
	}

	// Validate
	if err := r.Validate(projectPath, appsPath, apps); err != nil {
		return err
	}

	/*moduleRoot*/
	if _, err := r.moduleFinder.FindModuleRoot(projectPath); err != nil {
		return err
	}

	generator := NewGenerator(r.fileSystem)
	for _, app := range apps {
		appName := r.fileSystem.Base(app)
		outPath, err := generator.Generate(Options{
			Root:      app,
			OutputDir: filepath.Join(app, "adapters", "composition"),
		})
		if err != nil {
			return fmt.Errorf("generate routes for [%s] app: %w", appName, err)
		}
		r.printer.Printf("wrote %s\n", outPath)
	}

	return nil
}
