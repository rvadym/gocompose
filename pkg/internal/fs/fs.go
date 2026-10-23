package fs

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FileSystem is the filesystem surface used by scanners and validators.
type FileSystem interface {
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	ReadFile(path string) ([]byte, error)
	Abs(path string) (string, error)
	Base(path string) string
	Exists(path string) bool
	IsDirectory(path string) bool
}

var defaultFileSystem FileSystem = OS{}

// Default returns the process-wide operating system filesystem.
func Default() FileSystem {
	return defaultFileSystem
}

// OS accesses the host filesystem.
type OS struct{}

func (OS) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (OS) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := make([]fs.DirEntry, len(entries))
	for i, entry := range entries {
		result[i] = entry
	}
	return result, nil
}

func (OS) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (OS) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func (OS) Base(path string) string {
	return filepath.Base(path)
}

// Exists reports whether path can be statted successfully.
func (OS) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDirectory reports whether path exists and is a directory.
func (OS) IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
