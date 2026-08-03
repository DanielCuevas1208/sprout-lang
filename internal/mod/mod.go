// Package mod loads and validates Sprout modules.
//
// A module is a ".spr" file. The package resolves import paths, parses and
// checks modules, and loads each module exactly once. The resolver provides
// the static side: exported names and diagnostics. The loader provides the
// runtime side: it runs a module body and builds the module value.
//
// Both execution engines share the loader through a Runner callback. The
// callback executes a module body in a fresh engine and returns the exported
// values. A module that imports another module recurses through the same
// loader, so every module in a project runs once.
package mod

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// resolve maps an import path to an absolute file path.
//
// A relative path resolves against the directory of fromFile, the file that
// contains the import. A path without a ".spr" suffix gets one.
func resolve(path, fromFile string) (string, error) {
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(fromFile), target)
	}
	if filepath.Ext(target) == "" {
		target += ".spr"
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// ExportedNames lists the top-level names that prog exports, in source order.
func ExportedNames(prog *ast.Program) []string {
	var names []string
	for _, s := range prog.Stmts {
		switch v := s.(type) {
		case *ast.LetStmt:
			if v.Public {
				names = append(names, v.Name.Name)
			}
		case *ast.FnStmt:
			if v.Public {
				names = append(names, v.Name.Name)
			}
		}
	}
	return names
}

// ExportedSet returns the exported names of prog as a set.
func ExportedSet(prog *ast.Program) map[string]bool {
	names := ExportedNames(prog)
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// info is one loaded module.
type info struct {
	path    string
	file    *source.File
	prog    *ast.Program
	exports map[string]bool
	diags   []diag.Diagnostic
}

// Resolver statically loads modules and their exported names.
//
// It implements checker.ModuleResolver. Each module is parsed and checked at
// most once. Diagnostics are returned only for modules that have not been
// reported yet, so a shared module is reported once.
type Resolver struct {
	cache    map[string]*info
	loading  []string
	reported map[string]bool
}

// NewResolver returns an empty resolver.
func NewResolver() *Resolver {
	return &Resolver{
		cache:    make(map[string]*info),
		reported: make(map[string]bool),
	}
}

// ModuleExports returns the exported names of the module at path, the
// diagnostics found while loading it, and an error when it cannot be loaded.
func (r *Resolver) ModuleExports(path, fromFile string) (map[string]bool, []diag.Diagnostic, error) {
	inf, err := r.load(path, fromFile)
	if err != nil {
		return nil, nil, err
	}
	return inf.exports, r.takeNewDiags(inf), nil
}

// LoadModule returns the parsed program of the module at path.
//
// It is the shared entry point for the resolver and the loader.
func (r *Resolver) LoadModule(path, fromFile string) (*source.File, *ast.Program, error) {
	inf, err := r.load(path, fromFile)
	if err != nil {
		return nil, nil, err
	}
	return inf.file, inf.prog, nil
}

// load reads, parses, and checks one module, recursively.
func (r *Resolver) load(path, fromFile string) (*info, error) {
	full, err := resolve(path, fromFile)
	if err != nil {
		return nil, err
	}
	if inf, ok := r.cache[full]; ok {
		return inf, nil
	}
	for _, p := range r.loading {
		if p == full {
			return nil, fmt.Errorf("import cycle: %s -> %s", strings.Join(r.loading, " -> "), full)
		}
	}

	r.loading = append(r.loading, full)
	defer func() { r.loading = r.loading[:len(r.loading)-1] }()

	text, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("cannot find module '%s' (looked for %s)", path, full)
	}
	file := source.NewFile(full, string(text))
	prog, parseDiags := parser.Parse(file)

	inf := &info{path: full, file: file, prog: prog}
	inf.diags = append(inf.diags, parseDiags...)
	if !hasErrors(parseDiags) {
		inf.diags = append(inf.diags, checker.CheckWith(file, prog, r)...)
	}
	inf.exports = ExportedSet(prog)
	r.cache[full] = inf
	return inf, nil
}

// takeNewDiags returns the diagnostics of inf, and only the first time.
//
// The checker appends the diagnostics of a module's imports into the module's
// own list, so inf.diags already covers every transitive module. Returning
// the list once keeps a shared module from being reported at each import.
func (r *Resolver) takeNewDiags(inf *info) []diag.Diagnostic {
	if r.reported[inf.path] {
		return nil
	}
	r.reported[inf.path] = true
	return inf.diags
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

// Runner executes a module body and returns its exported values.
type Runner func(file *source.File, prog *ast.Program) (map[string]object.Object, error)

// Loader loads and runs modules, memoizing the resulting values.
//
// An engine constructs a Loader with a Runner that executes a module body in
// a fresh instance of that engine. The engine shares the loader across the
// module runs it starts, so each module in a project runs once.
type Loader struct {
	resolver *Resolver
	runner   Runner
	cache    map[string]*object.Module
	loading  []string
}

// NewLoader returns a loader that runs modules with runner.
func NewLoader(runner Runner) *Loader {
	return &Loader{
		resolver: NewResolver(),
		runner:   runner,
		cache:    make(map[string]*object.Module),
	}
}

// Load returns the module at path, loading and running it on first use.
func (l *Loader) Load(path, fromFile string) (*object.Module, error) {
	file, prog, err := l.resolver.LoadModule(path, fromFile)
	if err != nil {
		return nil, err
	}
	if m, ok := l.cache[file.Name]; ok {
		return m, nil
	}
	for _, p := range l.loading {
		if p == file.Name {
			return nil, fmt.Errorf("import cycle: %s -> %s", strings.Join(l.loading, " -> "), file.Name)
		}
	}

	l.loading = append(l.loading, file.Name)
	defer func() { l.loading = l.loading[:len(l.loading)-1] }()

	exports, err := l.runner(file, prog)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(file.Name), ".spr")
	if name == "" {
		name = "mod"
	}
	m := object.NewModule(name, file.Name, exports)
	l.cache[file.Name] = m
	return m, nil
}

// File returns the loaded source file for name, if any.
//
// The resolver reads each module exactly once, so the loader can hand the
// original source text to diagnostics for code running inside a module.
func (l *Loader) File(name string) *source.File {
	if inf, ok := l.resolver.cache[name]; ok {
		return inf.file
	}
	return nil
}
