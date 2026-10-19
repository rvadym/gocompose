package godicb

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FindModuleRoot returns the directory of the nearest go.mod at or above start.
func FindModuleRoot(start string) (string, error) {
	root, _, err := findModule(start)
	return root, err
}

func findModule(start string) (string, string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(current, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
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
