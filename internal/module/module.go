// Package module loads and runs Sprout modules.
//
// An import expression hands its path to a Loader. The loader resolves the
// path against the importing file, reads the source, and runs it with the
// same engine kind as the caller. The module's top-level declarations become
// its exports. Loaders cache modules by path so each module runs once.
//
// The two engine kinds get separate runners. The interpreter snapshot reads
// the global environment by name. The virtual machine reuses the exports map
// that the compiler records, and reads the entry environment by slot.
package module

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

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
			return nil, rerr
		}
		exports := make(map[string]object.Object)
		for _, name := range TopLevelNames(prog) {
			v, err := iv.Globals().Get(name)
			if err != nil {
				return nil, fmt.Errorf("module '%s': %v", file.Name, err)
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
			return nil, rerr
		}
		return m, nil
	})
}

// TopLevelNames lists the top-level declarations of prog, sorted.
//
// Every top-level let, const, and fn becomes a module export.
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
// It implements runtime.ModuleLoader, so engines can install a Loader
// directly as their import hook.
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
		return nil, wrapRunError(path, err)
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

// wrapRunError adds module context to a runtime error.
func wrapRunError(path string, err error) error {
	switch rerr := err.(type) {
	case *interp.RunError:
		return fmt.Errorf("error in module '%s': %s (at %d:%d)", path, rerr.Message, rerr.Pos.Line, rerr.Pos.Column)
	case *vm.RunError:
		return fmt.Errorf("error in module '%s': %s (at %d:%d)", path, rerr.Message, rerr.Pos.Line, rerr.Pos.Column)
	}
	return err
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

// Graph resolves the transitive module graph of mainPath.
//
// It returns mainPath followed by its imports in depth-first order, so a
// program and every module it needs can be copied into a build directory.
func Graph(mainPath string) ([]string, error) {
	g := &graph{visited: make(map[string]bool)}
	return g.walk(mainPath)
}

type graph struct {
	visited map[string]bool
}

func (g *graph) walk(path string) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve '%s': %v", path, err)
	}
	abs = filepath.Clean(abs)
	if g.visited[abs] {
		return nil, nil
	}
	g.visited[abs] = true

	text, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("cannot read '%s': %v", path, err)
	}
	file := source.NewFile(abs, string(text))
	prog, diags := parser.Parse(file)
	if d := firstError(diags); d != nil {
		return nil, fmt.Errorf("parse error in '%s': %s", path, d.Message)
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return nil, fmt.Errorf("error in '%s': %s", path, diags[0].Message)
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
	var walkExpr func(e ast.Expr)
	var walkStmts func(stmts []ast.Stmt)

	walkStmts = func(stmts []ast.Stmt) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.LetStmt:
				if n.Value != nil {
					walkExpr(n.Value)
				}
			case *ast.FnStmt:
				walkStmts(n.Body.Stmts)
			case *ast.IfStmt:
				walkExpr(n.Cond)
				walkStmts(n.Then.Stmts)
				for _, b := range n.Elifs {
					walkExpr(b.Cond)
					walkStmts(b.Body.Stmts)
				}
				if n.Else != nil {
					walkStmts(n.Else.Stmts)
				}
			case *ast.WhileStmt:
				walkExpr(n.Cond)
				walkStmts(n.Body.Stmts)
			case *ast.ForInStmt:
				walkExpr(n.Iterable)
				walkStmts(n.Body.Stmts)
			case *ast.ReturnStmt:
				if n.Value != nil {
					walkExpr(n.Value)
				}
			case *ast.ExprStmt:
				walkExpr(n.X)
			}
		}
	}

	walkExpr = func(e ast.Expr) {
		switch n := e.(type) {
		case *ast.ImportExpr:
			paths = append(paths, n.Path)
		case *ast.MemberExpr:
			walkExpr(n.Object)
		case *ast.UnaryExpr:
			walkExpr(n.X)
		case *ast.BinaryExpr:
			walkExpr(n.Left)
			walkExpr(n.Right)
		case *ast.AssignExpr:
			walkExpr(n.Target)
			walkExpr(n.Value)
		case *ast.CallExpr:
			walkExpr(n.Callee)
			for _, a := range n.Args {
				walkExpr(a)
			}
		case *ast.IndexExpr:
			walkExpr(n.X)
			walkExpr(n.Index)
		case *ast.ListLit:
			for _, el := range n.Elems {
				walkExpr(el)
			}
		case *ast.MapLit:
			for _, en := range n.Entries {
				walkExpr(en.Key)
				walkExpr(en.Value)
			}
		case *ast.FnExpr:
			walkStmts(n.Body.Stmts)
		}
	}

	walkStmts(prog.Stmts)
	return paths
}
