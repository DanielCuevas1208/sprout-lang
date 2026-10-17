package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// File is one parsed file in a module graph.
type File struct {
	Source *source.File
	Prog   *ast.Program
	// Path is the canonical absolute path of the file.
	Path string
	// Spec is the specifier used to reach this file. It is empty for the
	// entry file.
	Spec string
	// IsModule reports whether the file was loaded through an import.
	IsModule bool
	// Exports lists the exported names and their declaration positions.
	Exports map[string]source.Pos
	// ExportNames preserves declaration order for deterministic output.
	ExportNames []string
	// Imports holds the resolved import statements of the file.
	Imports []*Import
}

// Import is one import statement resolved to its target file.
type Import struct {
	Stmt   *ast.ImportStmt
	Target *File
}

// Graph is a set of files linked by their import statements.
type Graph struct {
	Entry    *File
	byPath   map[string]*File
	order    []*File
	resolver *Resolver
}

// NewGraph returns an empty graph with the given entry.
func NewGraph() *Graph {
	return &Graph{byPath: make(map[string]*File)}
}

// Resolve finds the file named by spec, searched from the file at fromPath.
func (g *Graph) Resolve(spec, fromPath string) (*File, bool) {
	if g.resolver == nil {
		return nil, false
	}
	p, ok := g.resolver.Resolve(spec, filepath.Dir(fromPath))
	if !ok {
		return nil, false
	}
	f, ok := g.byPath[p]
	return f, ok
}

// Files returns the graph files in discovery order, entry first.
func (g *Graph) Files() []*File { return g.order }

// Topo returns the files in dependency order, dependencies first.
//
// The entry file is always last. The build tool uses this order to emit
// module definitions before the code that references them.
func (g *Graph) Topo() []*File {
	var out []*File
	seen := make(map[*File]bool)
	var visit func(f *File)
	visit = func(f *File) {
		if seen[f] {
			return
		}
		seen[f] = true
		for _, im := range f.Imports {
			visit(im.Target)
		}
		out = append(out, f)
	}
	visit(g.Entry)
	return out
}

// ModuleName returns the canonical name of a file in the graph.
func (g *Graph) ModuleName(f *File) string {
	return ModuleName(f.Spec)
}

// Key returns a stable, machine-independent identity for a file.
//
// The build tool uses keys as cache names inside a bundle, so they must not
// depend on absolute paths.
func (g *Graph) Key(f *File) string {
	for i, x := range g.order {
		if x == f {
			return fmt.Sprintf("module%03d", i)
		}
	}
	return BaseName(f.Path)
}

// LoadOptions configures how an entry file is loaded.
type LoadOptions struct {
	// LibDirs lists project directories that bare specifiers search.
	LibDirs []string
}

// Load reads the entry file and every file it imports.
//
// It parses all files and reports import cycles and missing modules. It
// returns the graph, or nil, when the entry file itself cannot be parsed.
func Load(entryPath string, opts LoadOptions) (*Graph, []diag.Diagnostic) {
	g := NewGraph()
	g.resolver = NewResolver(opts.LibDirs)

	entryAbs := cleanPath(entryPath)
	entryFile, diags := readFile(entryAbs, "", false)
	if len(diags) > 0 {
		return nil, diags
	}
	g.byPath[entryAbs] = entryFile
	g.Entry = entryFile
	g.order = append(g.order, entryFile)

	loader := &loader{g: g, stack: []string{entryAbs}}
	diags = loader.load(entryFile)
	return g, diags
}

// loader walks the import edges of a graph.
type loader struct {
	g     *Graph
	stack []string // canonical paths currently being loaded
}

// load parses and links every file reachable from f.
//
// resolveImport already loads each new target and returns its diagnostics.
// Calling load on the target again here would re-enter already-loaded files
// and, once the load stack pops, miss cycles. So this loop only links.
func (l *loader) load(f *File) []diag.Diagnostic {
	var diags []diag.Diagnostic
	dir := filepath.Dir(f.Path)
	for _, stmt := range f.Prog.Stmts {
		imp, ok := stmt.(*ast.ImportStmt)
		if !ok {
			continue
		}
		target, errs := l.resolveImport(f, imp, dir)
		diags = append(diags, errs...)
		if target == nil {
			continue
		}
		f.Imports = append(f.Imports, &Import{Stmt: imp, Target: target})
	}
	return diags
}

// resolveImport resolves one import statement and loads its target file.
func (l *loader) resolveImport(from *File, imp *ast.ImportStmt, dir string) (*File, []diag.Diagnostic) {
	path, ok := l.g.resolver.Resolve(imp.Path, dir)
	if !ok {
		return nil, []diag.Diagnostic{missingModule(imp, from.Source)}
	}

	if target, ok := l.g.byPath[path]; ok {
		if isInStack(l.stack, path) {
			return nil, []diag.Diagnostic{cycle(imp, from.Source, l.stack, path)}
		}
		return target, nil
	}

	file, errs := readFile(path, imp.Path, true)
	if len(errs) > 0 {
		return nil, errs
	}

	l.g.byPath[path] = file
	l.g.order = append(l.g.order, file)
	l.stack = append(l.stack, path)
	subDiags := l.load(file)
	l.stack = l.stack[:len(l.stack)-1]
	return file, subDiags
}

// readFile reads and parses one Sprout file.
func readFile(path, spec string, isModule bool) (*File, []diag.Diagnostic) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Message:  fmt.Sprintf("cannot read module file %s", path),
		}}
	}
	sf := source.NewFile(path, string(text))
	prog, diags := parser.Parse(sf)
	if hasErrors(diags) {
		return nil, diags
	}
	f := &File{
		Source:   sf,
		Prog:     prog,
		Path:     path,
		Spec:     spec,
		IsModule: isModule,
		Exports:  make(map[string]source.Pos),
	}
	collectExports(f)
	return f, nil
}

// collectExports records the exported names of a file's top level.
//
// A name is recorded once. Exporting a method also exports its struct type,
// so a struct and a method on it both export the same name.
func collectExports(f *File) {
	add := func(name string, pos source.Pos) {
		if _, done := f.Exports[name]; done {
			return
		}
		f.Exports[name] = pos
		f.ExportNames = append(f.ExportNames, name)
	}
	for _, stmt := range f.Prog.Stmts {
		switch n := stmt.(type) {
		case *ast.LetStmt:
			if n.Export {
				add(n.Name.Name, n.Name.Position)
			}
		case *ast.StructStmt:
			if n.Export {
				add(n.Name.Name, n.Name.Position)
			}
		case *ast.FnStmt:
			if n.Export && n.Receiver != nil {
				// A method rides on its struct type. Exporting it also
				// exports the type so importers can reach the method.
				add(n.Receiver.Name, n.Receiver.Position)
				continue
			}
			if n.Export {
				add(n.Name.Name, n.Name.Position)
			}
		}
	}
}

func missingModule(imp *ast.ImportStmt, file *source.File) diag.Diagnostic {
	return diag.Diagnostic{
		Severity: diag.SeverityError,
		Message:  fmt.Sprintf("cannot find module '%s'", imp.Path),
		File:     file,
		Pos:      imp.PathPos,
	}
}

// cycle builds a diagnostic for an import cycle.
//
// The stack holds the canonical paths currently being loaded, from the entry
// down to the file that imports the repeating module.
func cycle(imp *ast.ImportStmt, file *source.File, stack []string, repeat string) diag.Diagnostic {
	names := make([]string, 0, len(stack)+1)
	for _, p := range stack {
		names = append(names, BaseName(p))
	}
	names = append(names, BaseName(repeat))
	return diag.Diagnostic{
		Severity: diag.SeverityError,
		Message:  fmt.Sprintf("import cycle: %s", strings.Join(names, " -> ")),
		File:     file,
		Pos:      imp.ImportPos,
	}
}

func isInStack(stack []string, path string) bool {
	for _, p := range stack {
		if p == path {
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
