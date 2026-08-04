// Package module loads, resolves, and bundles Sprout module files.
//
// A Sprout program can import other files with the import statement. The
// module package turns an entry file into a graph of parsed files, checks
// for import cycles, and records each module's exported names. The checker,
// the interpreter, the compiler, and the build tool all consume this graph.
//
// Module resolution is file based. A specifier that starts with "." resolves
// relative to the importing file. Any other specifier is tried against the
// importing file's directory and then against the project's library
// directories. A missing ".spr" suffix is added automatically.
package module

import (
	"path"
	"strings"
)

// Resolver finds the file that an import specifier names.
type Resolver struct {
	// LibDirs lists project directories that bare specifiers search.
	// They are consulted after the importing file's own directory.
	LibDirs []string
}

// NewResolver returns a resolver that searches the given library directories.
func NewResolver(libDirs []string) *Resolver {
	return &Resolver{LibDirs: libDirs}
}

// Resolve returns the canonical path of the file named by spec.
//
// The search starts from dir, the directory of the importing file. A missing
// ".spr" suffix is added before checking each candidate.
func (r *Resolver) Resolve(spec, dir string) (string, bool) {
	cands := r.candidates(spec, dir)
	for _, c := range cands {
		if exists(c) {
			return cleanPath(c), true
		}
	}
	return "", false
}

// candidates lists the files to try, in order.
func (r *Resolver) candidates(spec, dir string) []string {
	if abs := absoluteSpec(spec); abs != "" {
		return []string{withSuffix(abs)}
	}

	withDot := strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")
	if withDot {
		return []string{withSuffix(joinPath(dir, spec))}
	}

	// A bare specifier searches the importing directory first, then the
	// project's library directories.
	base := joinPath(dir, spec)
	cands := []string{withSuffix(base)}
	for _, lib := range r.LibDirs {
		cands = append(cands, withSuffix(joinPath(lib, spec)))
	}
	return cands
}

// ModuleName returns the name a module binds when it has no alias.
func ModuleName(spec string) string {
	base := path.Base(strings.ReplaceAll(spec, "\\", "/"))
	base = strings.TrimSuffix(base, ".spr")
	if base == "" || base == "." || base == "/" {
		return "module"
	}
	return base
}

func withSuffix(p string) string {
	if strings.HasSuffix(p, ".spr") {
		return p
	}
	return p + ".spr"
}
