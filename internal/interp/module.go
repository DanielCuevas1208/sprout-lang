package interp

import (
	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// RunModule executes a module file in an isolated scope and returns its
// exported values.
//
// The module runs on a fresh interpreter that shares the streams and the
// loader of the importer. Sharing the loader keeps the module cache and the
// circular-import guard across the whole import graph.
func (iv *Interpreter) RunModule(path string, file *source.File, prog *ast.Program) (map[string]object.Object, error) {
	sub := NewWithIO(iv.ctx.Stdin, iv.ctx.Stdout, iv.ctx.Stderr)
	sub.loader = iv.loader
	if _, rerr := sub.Exec(file, prog); rerr != nil {
		return nil, rerr
	}
	return collectExports(prog, sub.globals), nil
}

// collectExports reads the exported names of prog from env.
//
// Exports are collected from the syntax tree, not from an execution flag.
// This keeps the module scope free of bookkeeping and matches the bytecode
// virtual machine, which records exports with an explicit opcode.
func collectExports(prog *ast.Program, env *Env) map[string]object.Object {
	exports := make(map[string]object.Object)
	for _, s := range prog.Stmts {
		es, ok := s.(*ast.ExportStmt)
		if !ok {
			continue
		}
		name := exportedName(es.Decl)
		if name == "" {
			continue
		}
		if v, err := env.Get(name); err == nil {
			exports[name] = v
		}
	}
	return exports
}

// exportedName returns the declared name of an exported declaration.
func exportedName(decl ast.Stmt) string {
	switch d := decl.(type) {
	case *ast.LetStmt:
		if d.Name != nil {
			return d.Name.Name
		}
	case *ast.FnStmt:
		return d.Name.Name
	}
	return ""
}
