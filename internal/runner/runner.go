// Package runner drives the Sprout pipeline over a module graph.
//
// A graph is loaded and checked once, then run on either execution engine or
// compiled into bytecode. The command line uses this package for the run,
// vm, check, dis, and build commands.
package runner

import (
	"io"
	"os"
	"path/filepath"

	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/modules"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/vm"
)

// Load loads and checks the module graph rooted at entry.
//
// The SPROUT_PATH environment variable supplies extra module search paths,
// separated by the platform path list separator.
func Load(entry string) (*modules.Graph, []diag.Diagnostic) {
	l := &modules.Loader{}
	if sp := os.Getenv("SPROUT_PATH"); sp != "" {
		l.SearchPaths = filepath.SplitList(sp)
	}
	g, err := l.Load(entry)
	if err != nil {
		return nil, []diag.Diagnostic{{Severity: diag.SeverityError, Message: err.Error()}}
	}
	return g, g.Diags()
}

// Compile compiles a checked module graph into bytecode.
func Compile(g *modules.Graph) (*code.Program, error) {
	return compiler.CompileGraph(g)
}

// RunInterp runs a checked module graph on the tree-walking interpreter.
//
// Every module runs once, in dependency order, into a namespace. The entry
// program runs last with its import aliases bound.
func RunInterp(g *modules.Graph, stdin io.Reader, stdout, stderr io.Writer) *interp.RunError {
	iv := interp.NewWithIO(stdin, stdout, stderr)
	nss := make([]*object.Namespace, len(g.Units))

	for i, u := range g.Units {
		if i == g.Entry {
			continue
		}
		ns, rerr := iv.RunModule(u.Path, u.File, u.Prog, namespacesFor(g, nss, i), modules.ExportedNames(u.Prog))
		if rerr != nil {
			return rerr
		}
		nss[i] = ns
	}

	entry := g.Units[g.Entry]
	_, rerr := iv.ExecWithImports(entry.File, entry.Prog, namespacesFor(g, nss, g.Entry))
	return rerr
}

// RunVM runs a checked module graph on the bytecode virtual machine.
func RunVM(g *modules.Graph, stdin io.Reader, stdout, stderr io.Writer) *vm.RunError {
	compiled, err := compiler.CompileGraph(g)
	if err != nil {
		// The checker runs before execution, so this is an internal error.
		return &vm.RunError{Message: err.Error()}
	}
	machine := vm.NewWithIO(stdin, stdout, stderr)
	_, rerr := machine.Run(g.Units[g.Entry].File, compiled)
	return rerr
}

// namespacesFor builds the alias-to-namespace map of unit i.
func namespacesFor(g *modules.Graph, nss []*object.Namespace, i int) map[string]*object.Namespace {
	imports := make(map[string]*object.Namespace)
	for _, imp := range g.Units[i].Imports {
		if imp.Target >= 0 && imp.Target < len(nss) && nss[imp.Target] != nil {
			imports[imp.Alias] = nss[imp.Target]
		}
	}
	return imports
}
