// Package module loads and runs Sprout modules.
//
// An import expression hands its path to a Loader. The loader resolves the
// path against the importing file, reads the source, and runs it with the
// same engine kind as the caller. The module's top-level declarations become
// its exports. Loaders cache modules by path, so each module runs once.
//
// The two engine kinds get separate runners. The interpreter runner reads
// the globals by name. The virtual machine runner snapshots the exports map
// that the compiler records on the program.
package module

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// Runner executes a parsed module program and returns its namespace value.
type Runner func(l *Loader, file *source.File, prog *ast.Program) (*object.Module, error)

// Loader loads modules once and caches them by resolved path.
type Loader struct {
	cache   map[string]*object.Module
	loading map[string]bool
	run     Runner
	order   []string
}

// NewLoader returns a loader that runs modules with run.
func NewLoader(run Runner) *Loader {
	return &Loader{
		cache:   make(map[string]*object.Module),
		loading: make(map[string]bool),
		run:     run,
	}
}

// NewInterpLoader returns a loader that runs modules on the interpreter.
func NewInterpLoader(stdin io.Reader, stdout, stderr io.Writer) *Loader {
	return NewLoader(func(l *Loader, file *source.File, prog *ast.Program) (*object.Module, error) {
		iv := interp.NewWithIO(stdin, stdout, stderr)
		iv.SetModuleLoader(l)
		if _, rerr := iv.Exec(file, prog); rerr != nil {
			return nil, wrapRunError(file.Name, rerr)
		}
		exports := make(map[string]object.Object, len(TopLevelNames(prog)))
		for _, name := range TopLevelNames(prog) {
			v, err := iv.Globals().Get(name)
			if err != nil {
				return nil, fmt.Errorf("module '%s': %v", friendlyName(file.Name), err)
			}
			exports[name] = v
		}
		return &object.Module{Name: file.Name, Exports: exports}, nil
	})
}

// NewVMLoader returns a loader that runs modules on the bytecode VM.
func NewVMLoader(stdin io.Reader, stdout, stderr io.Writer) *Loader {
	return NewLoader(func(l *Loader, file *source.File, prog *ast.Program) (*object.Module, error) {
		machine := vm.NewWithIO(stdin, stdout, stderr)
		machine.SetModuleLoader(l)
		m, _, rerr := machine.RunModule(file, prog)
		if rerr != nil {
			return nil, wrapRunError(file.Name, rerr)
		}
		return m, nil
	})
}

// TopLevelNames lists the top-level declarations of prog, sorted.
//
// Every top-level let, const, and fn statement becomes a module export.
func TopLevelNames(prog *ast.Program) []string {
	var names []string
	for _, s := range prog.Stmts {
		switch n := s.(type) {
		case *ast.LetStmt:
			names = append(names, n.Name.Name)
		case *ast.FnStmt:
			names = append(names, n.Name.Name)
		}
	}
	sort.Strings(names)
	return names
}

// LoadModule resolves path against from and runs the module.
//
// It implements runtime.ModuleLoader, so an engine can install a Loader
// directly as its import hook. A module runs once; later imports of the same
// path return the cached namespace value.
func (l *Loader) LoadModule(from *source.File, path string) (*object.Module, error) {
	resolved := resolve(from, path)
	if m, ok := l.cache[resolved]; ok {
		return m, nil
	}
	if l.loading[resolved] {
		return nil, fmt.Errorf("circular import of '%s'", path)
	}
	l.loading[resolved] = true
	defer delete(l.loading, resolved)

	text, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("cannot read module '%s': %v", path, err)
	}
	file := source.NewFile(resolved, string(text))
	prog, diags := parser.Parse(file)
	if d := firstError(diags); d != nil {
		return nil, fmt.Errorf("parse error in module '%s': %s (at %d:%d)", path, d.Message, d.Pos.Line, d.Pos.Column)
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		d := diags[0]
		return nil, fmt.Errorf("error in module '%s': %s (at %d:%d)", path, d.Message, d.Pos.Line, d.Pos.Column)
	}

	m, err := l.run(l, file, prog)
	if err != nil {
		return nil, err
	}
	l.cache[resolved] = m
	l.order = append(l.order, resolved)
	return m, nil
}

// Files returns the modules loaded so far, in load order.
func (l *Loader) Files() []string {
	out := make([]string, len(l.order))
	copy(out, l.order)
	return out
}

// resolve turns an import path into an absolute file path.
//
// Relative paths resolve against the directory of the importing file. A file
// with no directory, such as the REPL, resolves against the working
// directory.
func resolve(from *source.File, path string) string {
	p := filepath.FromSlash(path)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	base := "."
	if from != nil && from.Name != "" {
		if dir := filepath.Dir(from.Name); dir != "." {
			base = dir
		}
	}
	return filepath.Clean(filepath.Join(base, p))
}

// firstError returns the first error diagnostic in ds, or nil.
func firstError(ds []diag.Diagnostic) *diag.Diagnostic {
	for i := range ds {
		if ds[i].Severity == diag.SeverityError {
			return &ds[i]
		}
	}
	return nil
}

// wrapRunError adds module context to a runtime error.
//
// Both engines report runtime errors through their own error types. The
// message names the module so the import site stays readable.
func wrapRunError(name string, err error) error {
	display := friendlyName(name)
	switch rerr := err.(type) {
	case *interp.RunError:
		return moduleError(display, rerr.Message, rerr.Pos)
	case *vm.RunError:
		return moduleError(display, rerr.Message, rerr.Pos)
	}
	return err
}

// moduleError builds an error message that names the module and, when known,
// the position inside it.
func moduleError(name, message string, pos source.Pos) error {
	if pos.IsValid() {
		return fmt.Errorf("error in module '%s': %s (at %d:%d)", name, message, pos.Line, pos.Column)
	}
	return fmt.Errorf("error in module '%s': %s", name, message)
}

// friendlyName strips the directory and the ".spr" suffix from a file name.
func friendlyName(name string) string {
	base := name
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".spr")
}

// Graph resolves the transitive module graph of mainPath.
//
// It returns mainPath followed by its imports in depth-first order. A
// program and every module it needs can be copied into a build directory in
// that order. A circular import is an error, because such a program could
// never run.
func Graph(mainPath string) ([]string, error) {
	g := &graph{
		visited: make(map[string]bool),
		onStack: make(map[string]bool),
	}
	return g.walk(mainPath)
}

type graph struct {
	visited map[string]bool
	onStack map[string]bool
}

func (g *graph) walk(path string) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve '%s': %v", path, err)
	}
	abs = filepath.Clean(abs)
	if g.onStack[abs] {
		return nil, fmt.Errorf("circular import of '%s'", path)
	}
	if g.visited[abs] {
		return nil, nil
	}
	g.onStack[abs] = true
	defer delete(g.onStack, abs)
	g.visited[abs] = true

	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot read '%s': %v", path, err)
	}
	file := source.NewFile(abs, string(text))
	prog, diags := parser.Parse(file)
	if d := firstError(diags); d != nil {
		return nil, fmt.Errorf("parse error in '%s': %s (at %d:%d)", path, d.Message, d.Pos.Line, d.Pos.Column)
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		d := diags[0]
		return nil, fmt.Errorf("error in '%s': %s (at %d:%d)", path, d.Message, d.Pos.Line, d.Pos.Column)
	}

	files := []string{abs}
	for _, imp := range importPaths(prog) {
		sub, err := g.walk(filepath.Join(filepath.Dir(abs), filepath.FromSlash(imp)))
		if err != nil {
			return nil, err
		}
		files = append(files, sub...)
	}
	return files, nil
}

// importPaths collects every import path in prog, in source order.
func importPaths(prog *ast.Program) []string {
	var paths []string
	w := &importWalker{collect: func(p string) { paths = append(paths, p) }}
	w.stmts(prog.Stmts)
	return paths
}

// importWalker visits every expression in a program and reports the paths of
// the import expressions it finds.
type importWalker struct {
	collect func(path string)
}

func (w *importWalker) stmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *ast.LetStmt:
			if n.Value != nil {
				w.expr(n.Value)
			}
		case *ast.FnStmt:
			w.stmts(n.Body.Stmts)
		case *ast.IfStmt:
			w.expr(n.Cond)
			w.stmts(n.Then.Stmts)
			for _, b := range n.Elifs {
				w.expr(b.Cond)
				w.stmts(b.Body.Stmts)
			}
			if n.Else != nil {
				w.stmts(n.Else.Stmts)
			}
		case *ast.WhileStmt:
			w.expr(n.Cond)
			w.stmts(n.Body.Stmts)
		case *ast.ForInStmt:
			w.expr(n.Iterable)
			w.stmts(n.Body.Stmts)
		case *ast.ReturnStmt:
			if n.Value != nil {
				w.expr(n.Value)
			}
		case *ast.ExprStmt:
			w.expr(n.X)
		}
	}
}

func (w *importWalker) expr(e ast.Expr) {
	switch n := e.(type) {
	case *ast.ImportExpr:
		w.collect(n.Path)
	case *ast.MemberExpr:
		w.expr(n.Object)
	case *ast.UnaryExpr:
		w.expr(n.X)
	case *ast.BinaryExpr:
		w.expr(n.Left)
		w.expr(n.Right)
	case *ast.AssignExpr:
		w.expr(n.Target)
		w.expr(n.Value)
	case *ast.CallExpr:
		w.expr(n.Callee)
		for _, a := range n.Args {
			w.expr(a)
		}
	case *ast.IndexExpr:
		w.expr(n.X)
		w.expr(n.Index)
	case *ast.ListLit:
		for _, el := range n.Elems {
			w.expr(el)
		}
	case *ast.MapLit:
		for _, en := range n.Entries {
			w.expr(en.Key)
			w.expr(en.Value)
		}
	case *ast.FnExpr:
		w.stmts(n.Body.Stmts)
	}
}
