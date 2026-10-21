// Package testfs provides in-memory filesystems for unit tests.
package testfs

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing/fstest"
)

// Map is a read-only filesystem backed by in-memory file and directory maps.
type Map struct {
	Files map[string]string
	Dirs  []string
}

func (m Map) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(m.asMapFS(), clean(name))
}

func (m Map) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(m.asMapFS(), clean(name))
}

func (m Map) ReadFile(name string) ([]byte, error) {
	return fs.ReadFile(m.asMapFS(), clean(name))
}

func (Map) Abs(path string) (string, error) {
	// `filepath.IsAbs` and `filepath.Clean` only inspect and
	// normalize the path string;
	// they don’t access the filesystem.
	// `filepath.Abs` is different: for relative paths,
	// it consults the process working directory.
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(string(filepath.Separator), path), nil
}

func (Map) Base(path string) string {
	return filepath.Base(path)
}

func (m Map) Exists(name string) bool {
	_, err := m.Stat(name)
	return err == nil
}

func (m Map) IsDirectory(name string) bool {
	info, err := m.Stat(name)
	return err == nil && info.IsDir()
}

func (m Map) asMapFS() fstest.MapFS {
	result := make(fstest.MapFS, len(m.Files)+len(m.Dirs))
	for name, content := range m.Files {
		result[clean(name)] = &fstest.MapFile{Data: []byte(content)}
	}
	for _, name := range m.Dirs {
		result[clean(name)] = &fstest.MapFile{Mode: fs.ModeDir}
	}
	return result
}

func clean(name string) string {
	name = filepath.ToSlash(filepath.Clean(name))
	name = strings.TrimLeft(name, "/")
	if name == "" {
		return "."
	}
	return name
}
