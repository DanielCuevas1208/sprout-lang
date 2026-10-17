package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Manifest describes a Sprout project.
//
// The manifest lives in a file named sprout.toml at the project root. The
// entry is the program that the run and build commands execute. The lib
// list names directories that bare import specifiers search.
type Manifest struct {
	Name    string   `toml:"name"`
	Version string   `toml:"version"`
	Entry   string   `toml:"entry"`
	Lib     []string `toml:"lib"`
}

// DefaultManifest returns the manifest a new project starts with.
func DefaultManifest(name string) Manifest {
	return Manifest{Name: name, Version: "0.1.0", Entry: "main.spr", Lib: []string{"lib"}}
}

// LoadManifest reads sprout.toml from dir.
//
// Missing fields fall back to sensible defaults so a minimal manifest works.
func LoadManifest(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "sprout.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}
	if m.Entry == "" {
		m.Entry = "main.spr"
	}
	return &m, nil
}

// FindManifest walks upward from dir looking for sprout.toml.
//
// It returns the project directory and its manifest. Walking upward lets
// nested commands run from any folder inside a project.
func FindManifest(dir string) (string, *Manifest, error) {
	for d := dir; ; d = filepath.Dir(d) {
		m, err := LoadManifest(d)
		if err == nil {
			return d, m, nil
		}
		if os.IsNotExist(err) {
			parent := filepath.Dir(d)
			if parent == d {
				return "", nil, fmt.Errorf("no sprout.toml found in %s or its parents", dir)
			}
			continue
		}
		return "", nil, err
	}
}

// Write saves the manifest into dir.
func (m *Manifest) Write(dir string) error {
	var b strings.Builder
	if err := toml.NewEncoder(&b).Encode(m); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "sprout.toml"), []byte(b.String()), 0o644)
}

// EntryPath returns the absolute path of the entry program.
func (m *Manifest) EntryPath(dir string) string {
	return filepath.Join(dir, filepath.FromSlash(m.Entry))
}

// LibDirs returns the library directories as absolute paths.
func (m *Manifest) LibDirs(dir string) []string {
	dirs := make([]string, 0, len(m.Lib))
	for _, lib := range m.Lib {
		dirs = append(dirs, filepath.Join(dir, filepath.FromSlash(lib)))
	}
	return dirs
}
