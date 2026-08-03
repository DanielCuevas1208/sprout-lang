package module

import (
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Loader resolves import specs and caches loaded modules.
//
// A Loader is not safe for concurrent use. Engines own one loader per run.
type Loader struct {
	fs      FS
	cache   map[string]*object.Module
	loading map[string]bool
}

// NewLoader returns a loader that reads from fs.
func NewLoader(fs FS) *Loader {
	return &Loader{
		fs:      fs,
		cache:   make(map[string]*object.Module),
		loading: make(map[string]bool),
	}
}

// Resolve maps an import spec to a canonical module path.
//
// A spec names a .spr file relative to the importing file. The call appends
// the .spr suffix when the spec omits it. It reports ok=false when no file
// exists at the resolved path.
func (l *Loader) Resolve(importer, spec string) (string, bool) {
	name := spec
	if !strings.HasSuffix(name, ".spr") {
		name += ".spr"
	}
	if !l.fs.IsAbs(name) {
		name = l.fs.Join(l.fs.Dir(importer), name)
	}
	name = l.fs.Clean(name)
	if !l.fs.Exists(name) {
		return "", false
	}
	return name, true
}

// Name returns the binding name for a resolved module path.
//
// The name is the file base without the .spr suffix. It must be a valid
// Sprout identifier. The second result reports whether the name is valid.
func (l *Loader) Name(path string) (string, bool) {
	name := strings.TrimSuffix(l.fs.Base(path), ".spr")
	if !ValidName(name) {
		return "", false
	}
	return name, true
}

// Source reads a resolved module path as a source file.
func (l *Loader) Source(path string) (*source.File, error) {
	text, err := l.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return source.NewFile(path, string(text)), nil
}

// Cached returns the cached module for a path, if any.
func (l *Loader) Cached(path string) (*object.Module, bool) {
	m, ok := l.cache[path]
	return m, ok
}

// Cache stores the loaded module for a path.
func (l *Loader) Cache(path string, m *object.Module) {
	l.cache[path] = m
}

// MarkLoading records that path is being loaded.
//
// It reports false when the path is already loading, which means a circular
// import. Call DoneLoading when the load finishes, whether it fails or not.
func (l *Loader) MarkLoading(path string) bool {
	if l.loading[path] {
		return false
	}
	l.loading[path] = true
	return true
}

// DoneLoading records that path is no longer loading.
func (l *Loader) DoneLoading(path string) {
	delete(l.loading, path)
}

// ValidName reports whether name is a usable Sprout binding name.
//
// The rules match the lexer: a letter or underscore, then letters, digits,
// and underscores. A keyword is not a valid binding name.
func ValidName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return token.Lookup(name) == token.IDENT
}

// Exports returns the top-level declaration names of prog in source order.
//
// A module exports its top-level let, const, and fn declarations. Import
// bindings and expression statements are not exported.
func Exports(prog *ast.Program) []string {
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
