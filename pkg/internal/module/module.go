// Package module provides shared Go module discovery helpers.
package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileReader is the filesystem capability required for module discovery.
type FileReader interface {
	ReadFile(string) ([]byte, error)
	Abs(string) (string, error)
}

// Finder discovers Go modules using a configured file reader.
type Finder struct {
	reader FileReader
}

// NewFinder creates a finder that reads go.mod files through reader.
func NewFinder(reader FileReader) *Finder {
	return &Finder{reader: reader}
}

// FindModuleRoot finds the nearest module root at or above start.
func (f *Finder) FindModuleRoot(start string) (string, error) {
	root, _, err := findModule(start, f.reader)
	return root, err
}

// FindModule finds the nearest module root and module path at or above start.
func (f *Finder) FindModule(start string) (string, string, error) {
	return findModule(start, f.reader)
}

func findModule(start string, reader FileReader) (string, string, error) {
	current, err := reader.Abs(start)
	if err != nil {
		return "", "", err
	}
	for {
		data, err := reader.ReadFile(filepath.Join(current, "go.mod"))
		if err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[0] == "module" {
					return current, fields[1], nil
				}
			}
			return "", "", fmt.Errorf("%s/go.mod has no module directive", current)
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("could not find go.mod above %s", start)
		}
		current = parent
	}
}
