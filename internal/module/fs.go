// Package module loads and caches Sprout source files as modules.
//
// A module is a .spr file whose top-level declarations become its exports.
// Both execution engines use a Loader to resolve an import spec to a file,
// run that file, and cache the result by canonical path. The package stays
// independent of the engines so tests can inject an in-memory filesystem.
package module

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// FS abstracts the file operations the loader needs.
//
// The interface keeps the loader testable. Production code uses OS. Tests
// use MemFS, which stores files in memory.
type FS interface {
	// ReadFile returns the contents of the named file.
	ReadFile(name string) ([]byte, error)
	// Join joins path elements with the platform separator.
	Join(elem ...string) string
	// Dir returns the directory part of a path.
	Dir(name string) string
	// Base returns the last element of a path.
	Base(name string) string
	// IsAbs reports whether a path is absolute.
	IsAbs(name string) bool
	// Clean normalizes a path.
	Clean(name string) string
	// Exists reports whether a file exists.
	Exists(name string) bool
}

// OS reads from the real filesystem.
type OS struct{}

// ReadFile reads a file from the real filesystem.
func (OS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

// Join joins path elements with the platform separator.
func (OS) Join(elem ...string) string { return filepath.Join(elem...) }

// Dir returns the directory part of a path.
func (OS) Dir(name string) string { return filepath.Dir(name) }

// Base returns the last element of a path.
func (OS) Base(name string) string { return filepath.Base(name) }

// IsAbs reports whether a path is absolute.
func (OS) IsAbs(name string) bool { return filepath.IsAbs(name) }

// Clean normalizes a path.
func (OS) Clean(name string) string { return filepath.Clean(name) }

// Exists reports whether a file exists.
func (OS) Exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// MemFS is an in-memory filesystem for tests.
//
// Paths use forward slashes. Add seeds the tree before a load.
type MemFS struct {
	files map[string]string
}

// NewMemFS returns an empty in-memory filesystem.
func NewMemFS() *MemFS {
	return &MemFS{files: make(map[string]string)}
}

// Add stores text under path.
func (m *MemFS) Add(path, text string) { m.files[path] = text }

// ReadFile returns the text stored under name.
func (m *MemFS) ReadFile(name string) ([]byte, error) {
	text, ok := m.files[name]
	if !ok {
		return nil, fmt.Errorf("open %s: no such file", name)
	}
	return []byte(text), nil
}

// Join joins path elements with forward slashes.
func (m *MemFS) Join(elem ...string) string { return path.Join(elem...) }

// Dir returns the directory part of a path.
func (m *MemFS) Dir(name string) string { return path.Dir(name) }

// Base returns the last element of a path.
func (m *MemFS) Base(name string) string { return path.Base(name) }

// IsAbs reports whether a path is absolute.
func (m *MemFS) IsAbs(name string) bool { return path.IsAbs(name) }

// Clean normalizes a path.
func (m *MemFS) Clean(name string) string { return path.Clean(name) }

// Exists reports whether text was stored under name.
func (m *MemFS) Exists(name string) bool {
	_, ok := m.files[name]
	return ok
}
