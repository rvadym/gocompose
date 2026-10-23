package godi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rvadym/gocompose/pkg/internal/fs"
	"github.com/rvadym/gocompose/pkg/internal/input"
	"github.com/rvadym/gocompose/pkg/internal/module"
	"github.com/rvadym/gocompose/pkg/internal/output"
)

const (
	ProjectPathCWD = "."
	AppsPathCWD    = "internal/app"
)

// Runner holds the filesystem dependency used throughout a DI generation run.
type Runner struct {
	fileSystem   fs.FileSystem
	moduleFinder *module.Finder
	printer      output.Printer
}

// NewDefaultRunner creates a runner bound to fileSystem.
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

// Scan parses DI directives using the runner's filesystem.
func (r *Runner) Scan(options ScanOptions) ([]Directive, error) {
	return scan(options, r.fileSystem)
}

// BuildDependencyGraph builds a dependency graph using the runner's filesystem.
func (r *Runner) BuildDependencyGraph(options ScanOptions) (DependencyGraph, error) {
	return buildDependencyGraph(options, r.fileSystem)
}

// GenerateContainer writes a generated container using the runner's filesystem for inputs.
func (r *Runner) GenerateContainer(graph DependencyGraph, options GenerateOptions) (string, error) {
	source, outPath, err := r.RenderContainer(graph, options)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outPath, source, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", outPath, err)
	}
	return outPath, nil
}

// RenderContainer returns generated container source without writing it to disk.
func (r *Runner) RenderContainer(graph DependencyGraph, options GenerateOptions) ([]byte, string, error) {
	return buildContainerSourceWithFS(graph, options, r.fileSystem)
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
	if err := r.Validate(
		projectPath, appsPath, apps,
	); err != nil {
		return err
	}

	/*moduleRoot*/
	_, err = r.moduleFinder.FindModuleRoot(projectPath)
	if err != nil {
		return err
	}

	for _, app := range apps {
		appName := r.fileSystem.Base(app)
		if err := r.processApp(appsPath, appName, app); err != nil {
			return err
		}
	}

	return nil
}

const yyy = "~~~~~~~~~~~"

func (r *Runner) processApp(appsPath, appName, appPath string) error {
	r.printer.Printf(
		"\n"+
			Magenta+yyy+Reset+
			Yellow+" Starting DI container generation for [%s] app "+Reset+
			Magenta+yyy+Reset+
			"\n",
		appName,
	)

	graph, err := r.BuildDependencyGraph(ScanOptions{
		Root: appPath,
	})
	if err != nil {
		return fmt.Errorf("scan [%s] app: %w", appName, err)
	}

	r.printer.Print(FormatScanOutputCLI(graph))

	if len(graph.Unresolved) > 0 {
		return fmt.Errorf("%d unresolved dependencies; fix them before generating the container", len(graph.Unresolved))
	}

	outPath, err := r.GenerateContainer(graph, GenerateOptions{
		OutputDir: filepath.Join(appsPath, appName, "adapters", "composition"),
	})
	if err != nil {
		return fmt.Errorf("generate container: %w", err)
	}
	r.printer.Printf("\nwrote %s\n", outPath)

	return nil
}
