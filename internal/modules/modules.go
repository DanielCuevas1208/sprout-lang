// Package modules loads Sprout programs across multiple source files.
//
// A module is a file that exports its top-level declarations. The Loader
// resolves import statements against the filesystem, parses every file once,
// reports missing modules and import cycles, and orders the graph so every
// module runs after the modules it imports.
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// Import is one import statement in a unit.
type Import struct {
	Spec   string // module specifier as written
	Alias  string // binding name in the importing file
	Pos    source.Pos
	Target int // index into Graph.Units, or -1 when unresolved
}

// Unit is one parsed source file in a graph.
type Unit struct {
	Path string // path used to open the file, for display
	Abs  string // cleaned absolute path, used to deduplicate files
	File *source.File
	Prog *ast.Program

	Imports []*Import
	Diags   []diag.Diagnostic

	// parseErrors reports whether the parser rejected this file. Such a unit
	// is not worth checking, so its checks are skipped.
	parseErrors bool
}

// Graph is a loaded module graph in dependency order.
//
// Every module appears before the units that import it. The entry unit is
// last. A unit's import targets always have a smaller index than the unit.
type Graph struct {
	Units []*Unit
	Entry int
}

// Loader loads module graphs from the filesystem.
type Loader struct {
	// SearchPaths are extra directories to search for modules, in order.
	// They are tried after the entry directory.
	SearchPaths []string

	// entryDir is the directory of the entry file. Bare module specifiers
	// resolve against it after the importing file's directory.
	entryDir string
}

// Load reads the module graph rooted at entry.
//
// The entry path may be relative to the working directory. Load returns an
// error only when the entry file itself cannot be opened. Problems inside
// the graph, such as missing modules and cycles, become diagnostics on the
// units that import them.
func (l *Loader) Load(entry string) (*Graph, error) {
	abs, err := filepath.Abs(entry)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", entry, err)
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return nil, fmt.Errorf("cannot open %s: no such file", entry)
	}

	l.entryDir = filepath.Dir(abs)
	g := &Graph{}
	cache := make(map[string]*Unit)
	l.loadUnit(g, cache, nil, abs, entry)
	g.Entry = len(g.Units) - 1
	g.check()
	return g, nil
}

// loadUnit parses one file and, depth-first, the files it imports.
func (l *Loader) loadUnit(g *Graph, cache map[string]*Unit, stack []*Unit, abs, display string) *Unit {
	if u, ok := cache[abs]; ok {
		return u
	}

	u := &Unit{Path: display, Abs: abs}
	cache[abs] = u // reserve before recursion so cycles are visible

	text, err := os.ReadFile(abs)
	if err != nil {
		u.Diags = append(u.Diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Message:  fmt.Sprintf("cannot read %s: %v", display, err),
		})
		return u
	}
	u.File = source.NewFile(display, string(text))
	prog, diags := parser.Parse(u.File)
	u.Prog = prog
	u.Diags = append(u.Diags, diags...)
	u.parseErrors = hasErrors(diags)

	stack = append(stack, u)
	for _, stmt := range prog.Stmts {
		imp, ok := stmt.(*ast.ImportStmt)
		if !ok {
			continue
		}
		record := &Import{Spec: imp.Path, Alias: imp.Alias.Name, Pos: imp.KwPos, Target: -1}
		u.Imports = append(u.Imports, record)

		target, err := l.resolve(filepath.Dir(abs), imp.Path)
		if err != nil {
			u.Diags = append(u.Diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				File:     u.File,
				Pos:      imp.KwPos,
				Message:  err.Error(),
			})
			continue
		}

		if inProgress := findInStack(stack, target); inProgress != nil {
			record.Target = -1
			u.Diags = append(u.Diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				File:     u.File,
				Pos:      imp.KwPos,
				Message:  fmt.Sprintf("import cycle: %s", cyclePath(stack, inProgress, imp.Path)),
			})
			continue
		}

		if child, ok := cache[target]; ok {
			record.Target = unitIndex(g.Units, child)
			continue
		}
		child := l.loadUnit(g, cache, stack, target, imp.Path)
		record.Target = unitIndex(g.Units, child)
	}

	g.Units = append(g.Units, u)
	return u
}

// resolve finds the file that a specifier names, or an error.
func (l *Loader) resolve(fromDir, spec string) (string, error) {
	var bases []string
	switch {
	case filepath.IsAbs(spec):
		bases = []string{spec}
	case strings.HasPrefix(spec, "."):
		bases = []string{filepath.Join(fromDir, spec)}
	default:
		bases = []string{filepath.Join(fromDir, spec)}
		if l.entryDir != "" {
			bases = append(bases, filepath.Join(l.entryDir, spec))
		}
		for _, sp := range l.SearchPaths {
			bases = append(bases, filepath.Join(sp, spec))
		}
	}

	for _, base := range bases {
		for _, cand := range candidates(base) {
			if info, err := os.Stat(cand); err == nil && !info.IsDir() {
				if abs, err := filepath.Abs(cand); err == nil {
					return abs, nil
				}
			}
		}
	}
	return "", fmt.Errorf("cannot find module %q", spec)
}

// candidates lists the file shapes a module specifier may name.
func candidates(base string) []string {
	return []string{
		base,
		base + ".spr",
		filepath.Join(base, "main.spr"),
	}
}

// findInStack returns the stack unit whose file matches abs, or nil.
func findInStack(stack []*Unit, abs string) *Unit {
	for _, u := range stack {
		if u.Abs == abs {
			return u
		}
	}
	return nil
}

// cyclePath renders a cycle as "a -> b -> a".
//
// stack lists the units currently being loaded, from the entry inward.
// start is the unit the cycle returns to; backSpec is the specifier of the
// import that closes the loop.
func cyclePath(stack []*Unit, start *Unit, backSpec string) string {
	var parts []string
	started := false
	for _, u := range stack {
		if u == start {
			started = true
		}
		if started {
			parts = append(parts, u.Path)
		}
	}
	parts = append(parts, backSpec)
	return strings.Join(parts, " -> ")
}

func unitIndex(units []*Unit, u *Unit) int {
	for i, x := range units {
		if x == u {
			return i
		}
	}
	return -1
}

// check runs the static checker over every unit with module knowledge.
func (g *Graph) check() {
	for _, u := range g.Units {
		if u.parseErrors || u.Prog == nil {
			continue
		}
		imports := make(map[string][]string)
		for _, imp := range u.Imports {
			if imp.Target >= 0 {
				imports[imp.Alias] = ExportedNames(g.Units[imp.Target].Prog)
			}
		}
		u.Diags = append(u.Diags, checker.CheckModules(u.File, u.Prog, imports)...)
	}
}

// ExportedNames lists the names a program exports, in source order.
//
// A module exports its top-level let, const, and function declarations.
// Import aliases are private and are not included.
func ExportedNames(prog *ast.Program) []string {
	var names []string
	for _, s := range prog.Stmts {
		switch n := s.(type) {
		case *ast.LetStmt:
			names = append(names, n.Name.Name)
		case *ast.FnStmt:
			names = append(names, n.Name.Name)
		}
	}
	return names
}

// Diags returns every diagnostic in the graph, unit by unit.
func (g *Graph) Diags() []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, u := range g.Units {
		out = append(out, u.Diags...)
	}
	return out
}

// HasErrors reports whether any diagnostic is an error.
func (g *Graph) HasErrors() bool {
	for _, d := range g.Diags() {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}
