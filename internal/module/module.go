// Package module loads Sprout files as reusable modules.
//
// A module is a source file that runs once in an isolated scope. Its
// exported declarations form a namespace that importers reach through the
// module value. The loader resolves paths, caches loaded modules, and
// rejects circular imports.
//
// The loader is engine-agnostic. It reads, parses, and checks each file,
// then hands the syntax tree to a Runner that executes it. Both engines
// provide a Runner, so a program loads modules with the engine that runs it.
package module

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// Runner executes a parsed module and returns its exported values.
//
// The tree-walking interpreter and the bytecode virtual machine both
// implement Runner. Each runs the module with its own engine so that module
// functions are values of the same engine as the importer.
type Runner interface {
	RunModule(path string, file *source.File, prog *ast.Program) (map[string]object.Object, error)
}

// Loader resolves, caches, and runs modules.
//
// A Loader is not safe for concurrent use. Sprout is single-threaded, so
// this matches the language.
type Loader struct {
	runner Runner

	cache   map[string]*object.Module
	checked map[string]bool
	loading map[string]bool
}

// New returns a Loader that uses runner to execute modules.
//
// The runner may be nil when the loader only performs static checks.
func New(runner Runner) *Loader {
	return &Loader{
		runner:  runner,
		cache:   make(map[string]*object.Module),
		checked: make(map[string]bool),
		loading: make(map[string]bool),
	}
}

// Load loads the module at path, resolved against fromDir.
//
// A missing ".spr" extension is added when needed. Each module runs at most
// once; later loads return the same cached value.
func (l *Loader) Load(path, fromDir string) (*object.Module, error) {
	abs, err := l.resolve(path, fromDir)
	if err != nil {
		return nil, err
	}
	return l.load(abs)
}

// CheckGraph loads and statically checks path and every module it imports,
// without executing any of them.
func (l *Loader) CheckGraph(path, fromDir string) error {
	abs, err := l.resolve(path, fromDir)
	if err != nil {
		return err
	}
	return l.checkAbs(abs)
}

// CheckProgram statically checks every module imported by prog.
func (l *Loader) CheckProgram(file *source.File, prog *ast.Program) error {
	return l.checkImportsOf(file, prog)
}

// load runs the module at abs and caches the result.
func (l *Loader) load(abs string) (*object.Module, error) {
	if mod, ok := l.cache[abs]; ok {
		return mod, nil
	}
	if l.loading[abs] {
		return nil, fmt.Errorf("circular import of %q", abs)
	}
	l.loading[abs] = true
	defer delete(l.loading, abs)

	file, prog, err := loadProgram(abs)
	if err != nil {
		return nil, err
	}
	if l.runner == nil {
		return nil, fmt.Errorf("module %q cannot run without an execution engine", abs)
	}
	exports, err := l.runner.RunModule(abs, file, prog)
	if err != nil {
		return nil, err
	}
	mod := &object.Module{Exports: exports}
	l.cache[abs] = mod
	return mod, nil
}

// checkAbs parses and checks the module at abs and recurses into its imports.
func (l *Loader) checkAbs(abs string) error {
	if l.checked[abs] {
		return nil
	}
	if l.loading[abs] {
		return fmt.Errorf("circular import of %q", abs)
	}
	l.loading[abs] = true
	defer delete(l.loading, abs)

	file, prog, err := loadProgram(abs)
	if err != nil {
		return err
	}
	if err := l.checkImportsOf(file, prog); err != nil {
		return err
	}
	l.checked[abs] = true
	return nil
}

func (l *Loader) checkImportsOf(file *source.File, prog *ast.Program) error {
	dir := filepath.Dir(file.Name)
	for _, s := range prog.Stmts {
		if imp, ok := s.(*ast.ImportStmt); ok {
			if err := l.CheckGraph(imp.Path, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolve turns an import path and an importing directory into an absolute
// file path.
func (l *Loader) resolve(path, fromDir string) (string, error) {
	if path == "" {
		return "", errors.New("module path cannot be empty")
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(fromDir, target)
	}
	if filepath.Ext(target) == "" {
		target += ".spr"
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("cannot resolve module path %q: %v", path, err)
	}
	return filepath.Clean(abs), nil
}

// loadProgram reads, parses, and checks the module at abs.
func loadProgram(abs string) (*source.File, *ast.Program, error) {
	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read module %q: %v", abs, err)
	}
	file := source.NewFile(abs, string(text))
	prog, diags := parser.Parse(file)
	if e := firstError(diags); e != nil {
		return nil, nil, fmt.Errorf("module %s: %s", abs, e)
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return nil, nil, fmt.Errorf("module %s: %s", abs, diags[0].Message)
	}
	return file, prog, nil
}

// firstError returns the first error-level diagnostic, or nil.
func firstError(diags []diag.Diagnostic) error {
	for i := range diags {
		if diags[i].Severity == diag.SeverityError {
			return errors.New(diagMessage(&diags[i]))
		}
	}
	return nil
}

// diagMessage renders a diagnostic with its source position.
func diagMessage(d *diag.Diagnostic) string {
	if d.File != nil && d.Pos.IsValid() {
		return fmt.Sprintf("%s:%d:%d: %s", d.File.Name, d.Pos.Line, d.Pos.Column, d.Message)
	}
	return d.Message
}
