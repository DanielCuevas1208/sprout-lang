package module

import (
	"os"
	"path/filepath"
)

// joinPath joins a directory and an import specifier.
func joinPath(dir, spec string) string {
	if dir == "" {
		return filepath.FromSlash(spec)
	}
	return filepath.Join(dir, filepath.FromSlash(spec))
}

// exists reports whether a file exists at p.
func exists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// cleanPath returns the absolute, cleaned form of p.
func cleanPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// absoluteSpec returns spec when it names an absolute path, or "".
func absoluteSpec(spec string) string {
	if filepath.IsAbs(filepath.FromSlash(spec)) {
		return filepath.FromSlash(spec)
	}
	return ""
}

// BaseName strips the directory and ".spr" suffix from a path.
func BaseName(p string) string {
	base := filepath.Base(p)
	if len(base) > 4 && base[len(base)-4:] == ".spr" {
		return base[:len(base)-4]
	}
	return base
}
