// Package checker performs a lightweight static analysis pass over a program.
//
// It reports problems that would otherwise surface at runtime: undefined
// names, assignment to constants, duplicate declarations, and control-flow
// misuse. It also sanity-checks optional type annotations on literals.
package checker

import (
	"fmt"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

type symKind int

const (
	symVar symKind = iota
	symConst
	symFunc
	symParam
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
	c := &checker{file: file, modules: make(map[string]bool)}
	c.checkStmts(prog.Stmts, newScope(nil))
	return c.diags
}

type checker struct {
	file      *source.File
	diags     []diag.Diagnostic
	funcDepth int
	loopDepth int
	// modules tracks names bound to an import expression.
	modules map[string]bool
}

// builtinNames is the set of predeclared functions.
var builtinNames = buildBuiltinSet()

func buildBuiltinSet() map[string]bool {
	set := make(map[string]bool, len(interp.BuiltinNames))
	for _, n := range interp.BuiltinNames {
		set[n] = true
	}
	return set
}

func (c *checker) checkStmts(stmts []ast.Stmt, sc *scope) {
	// Pre-declare function names so functions can call later functions.
	for _, s := range stmts {
		if fn, ok := s.(*ast.FnStmt); ok {
			c.declare(sc, fn.Name.Name, symFunc, fn.Name.Position, "")
		}
	}
	for _, s := range stmts {
		c.checkStmt(s, sc)
	}
}

func (c *checker) declare(sc *scope, name string, kind symKind, pos source.Pos, typeAnn string) {
	if _, dup := sc.names[name]; dup {
		c.errorf(pos, "duplicate declaration of '%s'", name)
		return
	}
	sc.names[name] = symbol{kind: kind, pos: pos, typeAnn: typeAnn}
}

func (c *checker) checkStmt(s ast.Stmt, sc *scope) {
	switch n := s.(type) {
	case *ast.LetStmt:
		c.checkLet(n, sc)
	case *ast.FnStmt:
		funcScope := newScope(sc)
		for _, p := range n.Params {
			c.checkParam(funcScope, p)
		}
		c.funcDepth++
		c.checkStmts(n.Body.Stmts, funcScope)
		c.funcDepth--
	case *ast.IfStmt:
		c.checkExpr(n.Cond, sc)
		c.checkStmts(n.Then.Stmts, newScope(sc))
		for _, b := range n.Elifs {
			c.checkExpr(b.Cond, sc)
			c.checkStmts(b.Body.Stmts, newScope(sc))
		}
		if n.Else != nil {
			c.checkStmts(n.Else.Stmts, newScope(sc))
		}
	case *ast.WhileStmt:
		c.checkExpr(n.Cond, sc)
		c.loopDepth++
		c.checkStmts(n.Body.Stmts, newScope(sc))
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
		c.checkStmts(n.Body.Stmts, bodyScope)
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

func (c *checker) checkLet(n *ast.LetStmt, sc *scope) {
	var typeAnn string
	if n.Type != nil {
		c.checkTypeAnn(n.Type)
		typeAnn = n.Type.Name
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
		if lit := literalType(n.Value); lit != "" && typeAnn != "" && !assignable(lit, typeAnn) {
			c.errorf(n.Value.Pos(), "cannot initialize a value of type '%s' with a value of type '%s'", typeAnn, lit)
		}
		if _, isImport := n.Value.(*ast.ImportExpr); isImport {
			c.modules[n.Name.Name] = true
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
	case *ast.ImportExpr:
		return "module"
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
			}
		case *ast.IndexExpr:
			if id, ok := t.X.(*ast.Ident); ok && c.modules[id.Name] {
				c.errorf(t.Pos(), "cannot assign to a member of a module")
			}
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
		c.checkStmts(n.Body.Stmts, fnScope)
		c.funcDepth--
	case *ast.ImportExpr:
		// Loading happens at runtime, so there is nothing to check here.
	case *ast.MemberExpr:
		c.checkExpr(n.Object, sc)
	}
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
