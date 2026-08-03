// Package checker performs a lightweight static analysis pass over a program.
//
// It reports problems that would otherwise surface at runtime: undefined
// names, assignment to constants, duplicate declarations, and control-flow
// misuse. It also sanity-checks optional type annotations on literals.
//
// A program may import modules. When the caller provides a ModuleResolver,
// the checker validates the imports and the members read from them. Without a
// resolver, imports are bound but their contents are not inspected.
package checker

import (
	"fmt"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// ModuleResolver loads the exported names of a module for static checks.
//
// The checker calls ModuleExports once per import. The implementation is
// responsible for resolving the path and for reporting each module's
// diagnostics only once.
type ModuleResolver interface {
	// ModuleExports returns the exported names of the module at path,
	// relative to fromFile. It also returns the diagnostics found while
	// loading the module and an error when the module cannot be loaded.
	ModuleExports(path, fromFile string) (exports map[string]bool, diags []diag.Diagnostic, err error)
}

type symKind int

const (
	symVar symKind = iota
	symConst
	symFunc
	symParam
	symImport
)

type symbol struct {
	kind    symKind
	pos     source.Pos
	typeAnn string
}

type scope struct {
	parent *scope
	names  map[string]symbol
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, names: make(map[string]symbol)}
}

// Check reports the diagnostics found in prog.
func Check(file *source.File, prog *ast.Program) []diag.Diagnostic {
	return CheckWith(file, prog, nil)
}

// CheckWith reports the diagnostics found in prog, validating module imports
// with resolver when it is not nil.
func CheckWith(file *source.File, prog *ast.Program, resolver ModuleResolver) []diag.Diagnostic {
	c := &checker{file: file, resolver: resolver, modules: make(map[string]map[string]bool)}
	c.checkStmts(prog.Stmts, newScope(nil), true)
	return c.diags
}

type checker struct {
	file     *source.File
	resolver ModuleResolver
	diags    []diag.Diagnostic
	// modules maps a bound import name to its exported names. A nil value
	// means the module was imported without a resolver.
	modules map[string]map[string]bool
	// funcDepth is the number of open function bodies.
	funcDepth int
	loopDepth int
}

// builtinNames is the set of predeclared functions.
var builtinNames = buildBuiltinSet()

func buildBuiltinSet() map[string]bool {
	set := make(map[string]bool, len(runtime.Names))
	for _, n := range runtime.Names {
		set[n] = true
	}
	return set
}

func (c *checker) checkStmts(stmts []ast.Stmt, sc *scope, atTop bool) {
	// Pre-declare function names so functions can call later functions.
	for _, s := range stmts {
		if fn, ok := s.(*ast.FnStmt); ok {
			c.declare(sc, fn.Name.Name, symFunc, fn.Name.Position, "")
		}
	}
	for _, s := range stmts {
		c.checkStmt(s, sc, atTop)
	}
}

func (c *checker) declare(sc *scope, name string, kind symKind, pos source.Pos, typeAnn string) {
	if _, dup := sc.names[name]; dup {
		c.errorf(pos, "duplicate declaration of '%s'", name)
		return
	}
	sc.names[name] = symbol{kind: kind, pos: pos, typeAnn: typeAnn}
}

func (c *checker) checkStmt(s ast.Stmt, sc *scope, atTop bool) {
	switch n := s.(type) {
	case *ast.LetStmt:
		c.checkLet(n, sc, atTop)
	case *ast.FnStmt:
		if n.Public && !atTop {
			c.errorf(n.FnPos, "'export' can only appear at the top level of a file")
		}
		funcScope := newScope(sc)
		for _, p := range n.Params {
			c.checkParam(funcScope, p)
		}
		c.funcDepth++
		c.checkStmts(n.Body.Stmts, funcScope, false)
		c.funcDepth--
	case *ast.ImportStmt:
		c.checkImport(n, sc)
	case *ast.IfStmt:
		c.checkExpr(n.Cond, sc)
		c.checkStmts(n.Then.Stmts, newScope(sc), false)
		for _, b := range n.Elifs {
			c.checkExpr(b.Cond, sc)
			c.checkStmts(b.Body.Stmts, newScope(sc), false)
		}
		if n.Else != nil {
			c.checkStmts(n.Else.Stmts, newScope(sc), false)
		}
	case *ast.WhileStmt:
		c.checkExpr(n.Cond, sc)
		c.loopDepth++
		c.checkStmts(n.Body.Stmts, newScope(sc), false)
		c.loopDepth--
	case *ast.ForInStmt:
		c.checkExpr(n.Iterable, sc)
		bodyScope := newScope(sc)
		if _, dup := bodyScope.names[n.Var.Name]; dup {
			c.errorf(n.Var.Position, "duplicate declaration of '%s'", n.Var.Name)
		} else {
			bodyScope.names[n.Var.Name] = symbol{kind: symVar, pos: n.Var.Position}
		}
		c.loopDepth++
		c.checkStmts(n.Body.Stmts, bodyScope, false)
		c.loopDepth--
	case *ast.ReturnStmt:
		if c.funcDepth == 0 {
			c.errorf(n.ReturnPos, "'return' can only appear inside a function")
		}
		if n.Value != nil {
			c.checkExpr(n.Value, sc)
		}
	case *ast.BreakStmt:
		if c.loopDepth == 0 {
			c.errorf(n.Position, "'break' can only appear inside a loop")
		}
	case *ast.ContinueStmt:
		if c.loopDepth == 0 {
			c.errorf(n.Position, "'continue' can only appear inside a loop")
		}
	case *ast.ExprStmt:
		c.checkExpr(n.X, sc)
	}
}

// checkImport validates an import statement and binds its module name.
//
// Importing the same module path twice in one file is allowed and is a no-op.
// A second import under the same name but a different path is a duplicate.
func (c *checker) checkImport(n *ast.ImportStmt, sc *scope) {
	if sym, dup := sc.names[n.Name.Name]; dup {
		if !(sym.kind == symImport && sym.typeAnn == n.Path) {
			c.errorf(n.Name.Position, "duplicate declaration of '%s'", n.Name.Name)
		}
	} else {
		// The path rides in typeAnn so a re-import of the same path is
		// recognized as the same binding.
		sc.names[n.Name.Name] = symbol{kind: symImport, pos: n.KwPos, typeAnn: n.Path}
	}
	if c.resolver == nil {
		c.modules[n.Name.Name] = nil
		return
	}
	exports, diags, err := c.resolver.ModuleExports(n.Path, c.file.Name)
	c.diags = append(c.diags, diags...)
	if err != nil {
		c.errorf(n.KwPos, "cannot load module '%s': %v", n.Path, err)
	}
	c.modules[n.Name.Name] = exports
}

func (c *checker) checkParam(sc *scope, p *ast.Param) {
	if _, dup := sc.names[p.Name.Name]; dup {
		c.errorf(p.Name.Position, "duplicate parameter '%s'", p.Name.Name)
	} else {
		sc.names[p.Name.Name] = symbol{kind: symParam, pos: p.Name.Position}
	}
	if p.Type != nil {
		c.checkTypeAnn(p.Type)
	}
}

func (c *checker) checkLet(n *ast.LetStmt, sc *scope, atTop bool) {
	if n.Public && !atTop {
		c.errorf(n.KwPos, "'export' can only appear at the top level of a file")
	}
	var typeAnn string
	if n.Type != nil {
		c.checkTypeAnn(n.Type)
		typeAnn = n.Type.Name
	} else if n.Value != nil {
		// A literal initializer gives the name a known type. This lets the
		// checker reject a member read on a value that is clearly not a
		// module.
		typeAnn = literalType(n.Value)
	}
	if _, dup := sc.names[n.Name.Name]; dup {
		c.errorf(n.Name.Position, "duplicate declaration of '%s'", n.Name.Name)
	} else {
		kind := symVar
		if n.IsConst {
			kind = symConst
		}
		sc.names[n.Name.Name] = symbol{kind: kind, pos: n.Name.Position, typeAnn: typeAnn}
	}
	if n.Value != nil {
		if lit := literalType(n.Value); lit != "" && n.Type != nil && !assignable(lit, n.Type.Name) {
			c.errorf(n.Value.Pos(), "cannot initialize a value of type '%s' with a value of type '%s'", n.Type.Name, lit)
		}
		c.checkExpr(n.Value, sc)
	}
}

var knownTypes = map[string]bool{
	"int": true, "float": true, "string": true, "bool": true,
	"nil": true, "list": true, "map": true, "function": true, "range": true,
}

func (c *checker) checkTypeAnn(id *ast.Ident) {
	if !knownTypes[id.Name] {
		c.errorf(id.Position, "unknown type '%s'", id.Name)
	}
}

// literalType returns the type of e when e is a literal, or "".
func literalType(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.IntLit:
		return "int"
	case *ast.FloatLit:
		return "float"
	case *ast.StrLit:
		return "string"
	case *ast.BoolLit:
		return "bool"
	case *ast.NilLit:
		return "nil"
	case *ast.UnaryExpr:
		if v.Op == token.MINUS {
			return literalType(v.X)
		}
	case *ast.ListLit:
		return "list"
	case *ast.MapLit:
		return "map"
	}
	return ""
}

// assignable reports whether a literal of type from fits an annotation to.
func assignable(from, to string) bool {
	if from == to {
		return true
	}
	// An integer literal fits a float annotation.
	return from == "int" && to == "float"
}

func (c *checker) checkExpr(e ast.Expr, sc *scope) {
	switch n := e.(type) {
	case *ast.Ident:
		if !c.resolve(sc, n.Name) && !builtinNames[n.Name] {
			c.errorf(n.Position, "undefined name '%s'", n.Name)
		}
	case *ast.AssignExpr:
		switch t := n.Target.(type) {
		case *ast.Ident:
			sym, ok := c.lookup(sc, t.Name)
			if !ok {
				if !builtinNames[t.Name] {
					c.errorf(t.Position, "cannot assign to undefined name '%s'", t.Name)
				} else {
					c.errorf(t.Position, "cannot assign to builtin '%s'", t.Name)
				}
			} else if sym.kind == symConst {
				c.errorf(t.Position, "cannot assign to constant '%s'", t.Name)
			} else if sym.kind == symImport {
				c.errorf(t.Position, "cannot assign to an imported module '%s'", t.Name)
			}
		case *ast.IndexExpr:
			c.checkExpr(t.X, sc)
			c.checkExpr(t.Index, sc)
		default:
			c.errorf(t.Pos(), "cannot assign to this expression")
		}
		c.checkExpr(n.Value, sc)
	case *ast.CallExpr:
		c.checkExpr(n.Callee, sc)
		for _, a := range n.Args {
			c.checkExpr(a, sc)
		}
	case *ast.IndexExpr:
		c.checkExpr(n.X, sc)
		c.checkExpr(n.Index, sc)
	case *ast.MemberExpr:
		c.checkExpr(n.X, sc)
		c.checkMember(n, sc)
	case *ast.UnaryExpr:
		c.checkExpr(n.X, sc)
	case *ast.BinaryExpr:
		c.checkExpr(n.Left, sc)
		c.checkExpr(n.Right, sc)
	case *ast.ListLit:
		for _, el := range n.Elems {
			c.checkExpr(el, sc)
		}
	case *ast.MapLit:
		for _, en := range n.Entries {
			c.checkExpr(en.Key, sc)
			c.checkExpr(en.Value, sc)
		}
	case *ast.FnExpr:
		fnScope := newScope(sc)
		for _, p := range n.Params {
			c.checkParam(fnScope, p)
		}
		c.funcDepth++
		c.checkStmts(n.Body.Stmts, fnScope, false)
		c.funcDepth--
	}
}

// checkMember validates a member read.
//
// A module import carries a known export list, so a missing member is a
// static error. A value with a known literal type can never be a module.
// Other bases are left to the runtime.
func (c *checker) checkMember(n *ast.MemberExpr, sc *scope) {
	if id, ok := n.X.(*ast.Ident); ok {
		if exports, known := c.modules[id.Name]; known {
			if exports != nil && !exports[n.Name.Name] {
				c.errorf(n.Name.Position, "module '%s' does not export '%s'", id.Name, n.Name.Name)
			}
			return
		}
		if sym, ok := c.lookup(sc, id.Name); ok && sym.typeAnn != "" && sym.kind != symImport {
			c.errorf(n.DotPos, "cannot read a member of a %s", sym.typeAnn)
		}
		return
	}
	if lit, ok := memberBaseType(n.X); ok {
		c.errorf(n.DotPos, "cannot read a member of a %s", lit)
	}
}

// memberBaseType returns the type name of a literal member base.
func memberBaseType(e ast.Expr) (string, bool) {
	switch e.(type) {
	case *ast.IntLit:
		return "int", true
	case *ast.FloatLit:
		return "float", true
	case *ast.StrLit:
		return "string", true
	case *ast.BoolLit:
		return "bool", true
	case *ast.NilLit:
		return "nil", true
	case *ast.ListLit:
		return "list", true
	case *ast.MapLit:
		return "map", true
	}
	return "", false
}

func (c *checker) resolve(sc *scope, name string) bool {
	_, ok := c.lookup(sc, name)
	return ok
}

func (c *checker) lookup(sc *scope, name string) (symbol, bool) {
	for s := sc; s != nil; s = s.parent {
		if sym, ok := s.names[name]; ok {
			return sym, true
		}
	}
	return symbol{}, false
}

func (c *checker) errorf(pos source.Pos, format string, args ...any) {
	c.diags = append(c.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Message:  fmt.Sprintf(format, args...),
		File:     c.file,
		Pos:      pos,
	})
}
