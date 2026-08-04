// Package module loads a Sprout program and its imports into one bundle.
//
// A bundle is the complete set of source files behind one entry point. The
// loader reads the entry file, resolves its import statements relative to the
// importing file, and repeats the process until every reachable module is
// loaded. It rejects missing files, duplicate imports, and import cycles.
//
// Both execution engines consume a bundle. The interpreter walks the syntax
// trees. The compiler turns every module into a function that returns a map
// of its exports.
package module

import (
	"os"
	"path/filepath"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// File is one source file in a bundle.
type File struct {
	// Index is the position of the file in Bundle.Files.
	Index int
	// Path is the resolved path of the file, relative when possible.
	Path string
	// Key is a canonical identity for cycle detection and dedup.
	Key    string
	Main   bool
	Source *source.File
	Prog   *ast.Program
	// Exports lists the names that importing files can bind.
	Exports []string
}

// Bundle is a program and every module it imports.
//
// Files[0] is always the entry file. The remaining files are modules, each
// appearing exactly once. Modules lists the module file indices in the order
// they were discovered. The engines initialize a module lazily on first
// import, so the listing order does not affect behavior.
type Bundle struct {
	Files   []*File
	Modules []int
}

// Main returns the entry file of the bundle.
func (b *Bundle) Main() *File { return b.Files[0] }

// Source returns the source file of the entry file.
func (b *Bundle) Source() *source.File { return b.Files[0].Source }

// loader builds a bundle by depth-first search over the import graph.
type loader struct {
	files  []*File
	byKey  map[string]*File
	module []int
	diags  []diag.Diagnostic
	stack  []string // paths currently being loaded, for cycle messages
}

// Load reads the program at rootPath and resolves its imports.
//
// The returned bundle always holds a file for rootPath, even when loading
// fails, so callers can render diagnostics with the full graph.
func Load(rootPath string) (*Bundle, []diag.Diagnostic) {
	l := &loader{byKey: make(map[string]*File)}
	l.add(rootPath, "", source.Pos{}, true)
	return &Bundle{Files: l.files, Modules: l.module}, l.diags
}

// add loads the file named by raw and all of its imports.
//
// importer is the path of the file that names raw, or "" for the entry
// file. pos points at the import statement in the importing file. The
// return value is nil when the file cannot be read.
func (l *loader) add(raw, importer string, pos source.Pos, isMain bool) *File {
	display := resolvePath(importer, raw)
	key := cleanKey(display)
	if f, ok := l.byKey[key]; ok {
		if f.isOnStack(l.stack) {
			l.diags = append(l.diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				Message:  "import cycle: " + cycleMessage(l.stack, display),
				File:     currentFile(importer, l.files),
				Pos:      pos,
			})
			return nil
		}
		return f
	}

	text, err := os.ReadFile(display)
	if err != nil {
		msg := "cannot find module '" + raw + "'"
		if importer == "" {
			msg = "cannot read file '" + raw + "'"
		}
		l.diags = append(l.diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Message:  msg,
			File:     currentFile(importer, l.files),
			Pos:      pos,
		})
		return nil
	}

	file := source.NewFile(display, string(text))
	prog, pdiags := parser.Parse(file)
	for i := range pdiags {
		l.diags = append(l.diags, pdiags[i])
	}

	f := &File{
		Index:  len(l.files),
		Path:   display,
		Key:    key,
		Main:   isMain,
		Prog:   prog,
		Source: file,
	}
	f.collectExports()
	l.files = append(l.files, f)
	l.byKey[key] = f

	// The entry file is always Files[0]. Every other file is a module and is
	// recorded after its own imports are loaded.
	if !isMain {
		l.module = append(l.module, f.Index)
	}

	l.stack = append(l.stack, key)
	l.resolveImports(f)
	l.stack = l.stack[:len(l.stack)-1]

	return f
}

// resolveImports resolves every import statement of f.
func (l *loader) resolveImports(f *File) {
	seen := make(map[string]bool)
	for _, stmt := range f.Prog.Stmts {
		imp, ok := stmt.(*ast.ImportStmt)
		if !ok {
			continue
		}
		resolved := resolvePath(f.Path, imp.Path)
		key := cleanKey(resolved)
		if seen[key] {
			l.diags = append(l.diags, diag.Diagnostic{
				Severity: diag.SeverityError,
				Message:  "duplicate import of '" + imp.Path + "'",
				File:     f.Source,
				Pos:      imp.Pos(),
			})
			imp.Module = -1
			continue
		}
		seen[key] = true
		target := l.add(imp.Path, f.Path, imp.Pos(), false)
		if target == nil {
			imp.Module = -1
			continue
		}
		imp.Module = target.Index
	}
}

// collectExports records the exported top-level names of f.
func (f *File) collectExports() {
	if f.Prog == nil {
		return
	}
	for _, stmt := range f.Prog.Stmts {
		switch s := stmt.(type) {
		case *ast.LetStmt:
			if s.Exported {
				f.Exports = append(f.Exports, s.Name.Name)
			}
		case *ast.FnStmt:
			if s.Exported {
				f.Exports = append(f.Exports, s.Name.Name)
			}
		}
	}
}

// isOnStack reports whether key appears in the current load stack.
func (f *File) isOnStack(stack []string) bool {
	for _, s := range stack {
		if s == f.Key {
			return true
		}
	}
	return false
}

// currentFile returns the source file of importer, or the entry file when
// importer is empty. It is used to attach loader diagnostics to a source.
func currentFile(importer string, files []*File) *source.File {
	if importer == "" {
		if len(files) > 0 {
			return files[0].Source
		}
		return nil
	}
	for _, f := range files {
		if f.Path == importer {
			return f.Source
		}
	}
	return nil
}

// resolvePath resolves an import path against the directory of the importing
// file. A missing ".spr" extension is appended when the exact name does not
// exist. The entry file (importer empty) is used as written.
func resolvePath(importer, path string) string {
	if importer == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	dir := filepath.Dir(importer)
	joined := filepath.Join(dir, path)
	if _, err := os.Stat(joined); err == nil {
		return joined
	}
	if filepath.Ext(joined) == "" {
		withExt := joined + ".spr"
		if _, err := os.Stat(withExt); err == nil {
			return withExt
		}
	}
	return joined
}

// cleanKey returns a canonical identity for a file path.
func cleanKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(abs)
}

// cycleMessage renders the import chain that closes on path.
func cycleMessage(stack []string, path string) string {
	msg := ""
	for _, s := range stack {
		msg += filepath.Base(s) + " -> "
	}
	return msg + filepath.Base(path)
}
