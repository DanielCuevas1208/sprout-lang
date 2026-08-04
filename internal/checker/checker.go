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
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

type symKind int

const (
	symVar symKind = iota
	symConst
	symFunc
	symParam
	symModule
	symType
)

type symbol struct {
	kind    symKind
	pos     source.Pos
	typeAnn string
	// exports lists the members of an imported module symbol.
	exports map[string]source.Pos
}

// structInfo is the static shape of a struct declaration.
type structInfo struct {
	name     string
	pos      source.Pos
	fields   []string
	fieldSet map[string]bool
	// methods maps a method name to its arity.
	methods map[string]int
}

func (s *structInfo) hasField(name string) bool { return s.fieldSet[name] }
func (s *structInfo) hasMethod(name string) bool {
	_, ok := s.methods[name]
	return ok
}
func (s *structInfo) hasMember(name string) bool {
	return s.hasField(name) || s.hasMethod(name)
}

// ifaceInfo is the static contract of an interface declaration.
type ifaceInfo struct {
	name string
	pos  source.Pos
	// methods maps a method name to its required arity.
	methods map[string]int
}

func (i *ifaceInfo) hasMethod(name string) bool {
	_, ok := i.methods[name]
	return ok
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
	return CheckContext(Context{}, file, prog)
}

// Context carries module information that affects how a file is checked.
type Context struct {
	// IsModule reports whether the file is loaded as a module. Exports are
	// only allowed at the top level of a module file.
	IsModule bool
	// ModuleExports resolves an import specifier to its exported names. It
	// returns ok=false when the specifier cannot be resolved.
	ModuleExports func(spec string) (map[string]source.Pos, bool)
}

// CheckContext checks a program with module context.
func CheckContext(ctx Context, file *source.File, prog *ast.Program) []diag.Diagnostic {
	c := &checker{
		file:          file,
		isModule:      ctx.IsModule,
		moduleExports: ctx.ModuleExports,
		types:         make(map[string]*structInfo),
		interfaces:    make(map[string]*ifaceInfo),
	}
	c.checkStmts(prog.Stmts, newScope(nil))
	return c.diags
}

// CheckGraph checks every file in a module graph.
//
// Each module file is checked with its export table available, so member
// access on imported modules is validated statically.
func CheckGraph(g *module.Graph) []diag.Diagnostic {
	var diags []diag.Diagnostic
	for _, f := range g.Files() {
		ctx := Context{
			IsModule: f.IsModule,
			ModuleExports: func(spec string) (map[string]source.Pos, bool) {
				target, ok := g.Resolve(spec, f.Path)
				if !ok {
					return nil, false
				}
				return target.Exports, true
			},
		}
		diags = append(diags, CheckContext(ctx, f.Source, f.Prog)...)
	}
	return diags
}

type checker struct {
	file          *source.File
	isModule      bool
	moduleExports func(spec string) (map[string]source.Pos, bool)
	diags         []diag.Diagnostic
	funcDepth     int
	loopDepth     int
	// types holds the struct declarations seen so far, by type name.
	types map[string]*structInfo
	// interfaces holds the interface declarations, by type name.
	interfaces map[string]*ifaceInfo
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
	// Methods bind to a struct type instead of a name, so they are skipped.
	for _, s := range stmts {
		if fn, ok := s.(*ast.FnStmt); ok && fn.Receiver == nil {
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
	case *ast.ImportStmt:
		c.checkImport(n, sc)
	case *ast.FnStmt:
		if n.Receiver != nil {
			c.checkMethod(n, sc)
			break
		}
		if n.Export {
			c.checkExportPos(n.Name.Position)
		}
		funcScope := newScope(sc)
		for _, p := range n.Params {
			c.checkParam(funcScope, p)
		}
		c.funcDepth++
		c.checkStmts(n.Body.Stmts, funcScope)
		c.funcDepth--
	case *ast.StructStmt:
		c.checkStruct(n, sc)
	case *ast.InterfaceStmt:
		c.checkInterface(n, sc)
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
	var typeAnn string
	if p.Type != nil {
		c.checkTypeAnn(p.Type)
		typeAnn = p.Type.Name
	}
	if _, dup := sc.names[p.Name.Name]; dup {
		c.errorf(p.Name.Position, "duplicate parameter '%s'", p.Name.Name)
	} else {
		sc.names[p.Name.Name] = symbol{kind: symParam, pos: p.Name.Position, typeAnn: typeAnn}
	}
}

// checkImport resolves an import and declares its module name.
//
// Imports bind in the file's top-level scope, so a nested import is an
// error. The module loader only looks for imports at the top level.
func (c *checker) checkImport(n *ast.ImportStmt, sc *scope) {
	if c.funcDepth > 0 || c.loopDepth > 0 {
		c.errorf(n.ImportPos, "import can only appear at the top level of a file")
		return
	}
	if c.moduleExports == nil {
		c.errorf(n.PathPos, "cannot find module '%s'", n.Path)
		return
	}
	exports, ok := c.moduleExports(n.Path)
	if !ok {
		c.errorf(n.PathPos, "cannot find module '%s'", n.Path)
		return
	}
	if _, dup := sc.names[n.Name.Name]; dup {
		c.errorf(n.Name.Position, "duplicate declaration of '%s'", n.Name.Name)
		return
	}
	sc.names[n.Name.Name] = symbol{kind: symModule, pos: n.Name.Position, exports: exports}
}

// checkExportPos rejects exports that a module cannot honor.
func (c *checker) checkExportPos(pos source.Pos) {
	if !c.isModule {
		c.errorf(pos, "export can only appear in a module file")
		return
	}
	if c.funcDepth > 0 || c.loopDepth > 0 {
		c.errorf(pos, "export can only appear at the top level of a module")
	}
}

func (c *checker) checkLet(n *ast.LetStmt, sc *scope) {
	if n.Export {
		c.checkExportPos(n.Name.Position)
	}
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
		if typeAnn == "" && n.Value != nil {
			// Infer the type of a value that has one, so member access on
			// struct literals can be checked statically.
			typeAnn = c.staticType(sc, n.Value)
		}
		sc.names[n.Name.Name] = symbol{kind: kind, pos: n.Name.Position, typeAnn: typeAnn}
	}
	if n.Value != nil {
		if typeAnn != "" && !c.annotCompatible(sc, typeAnn, n.Value) {
			c.errorf(n.Value.Pos(), "cannot initialize a value of type '%s' with a value of type '%s'", typeAnn, c.typeOf(sc, n.Value))
		}
		c.checkExpr(n.Value, sc)
	}
}

// annotCompatible reports whether the value of e can initialize an
// annotation of type to. Unknown static types are accepted; the runtime is
// the final arbiter.
func (c *checker) annotCompatible(sc *scope, to string, e ast.Expr) bool {
	from := c.staticType(sc, e)
	if from == "" {
		return true
	}
	if from == to {
		return true
	}
	// An integer literal fits a float annotation.
	if from == "int" && to == "float" {
		return true
	}
	// An interface accepts a struct or interface that satisfies it.
	if iface, ok := c.interfaces[to]; ok {
		return c.satisfies(from, iface)
	}
	return false
}

// satisfies reports whether typeName provides every method of iface.
//
// typeName may name a struct or another interface. An interface satisfies
// another when it declares a superset of its methods.
func (c *checker) satisfies(typeName string, iface *ifaceInfo) bool {
	info, ok := c.types[typeName]
	if ok {
		for name, arity := range iface.methods {
			if a, ok := info.methods[name]; !ok || a != arity {
				return false
			}
		}
		return true
	}
	if other, ok := c.interfaces[typeName]; ok {
		for name, arity := range iface.methods {
			if a, ok := other.methods[name]; !ok || a != arity {
				return false
			}
		}
		return true
	}
	return false
}

// staticType returns the statically known type of e, or "" when unknown.
//
// The type is a struct or interface name for those values, or a builtin
// name for a literal. An expression whose type cannot be resolved returns "".
func (c *checker) staticType(sc *scope, e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		if sym, ok := c.lookup(sc, v.Name); ok {
			return sym.typeAnn
		}
	case *ast.CallExpr:
		if id, ok := v.Callee.(*ast.Ident); ok {
			if info, isStruct := c.types[id.Name]; isStruct {
				return info.name
			}
		}
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
	case *ast.ListLit:
		return "list"
	case *ast.MapLit:
		return "map"
	case *ast.UnaryExpr:
		if v.Op == token.MINUS {
			return c.staticType(sc, v.X)
		}
	}
	return ""
}

// typeOf returns the static type of e for use in diagnostics.
func (c *checker) typeOf(sc *scope, e ast.Expr) string {
	if t := c.staticType(sc, e); t != "" {
		return t
	}
	return "unknown"
}

// checkStruct validates a struct declaration and records its shape.
func (c *checker) checkStruct(n *ast.StructStmt, sc *scope) {
	if n.Export {
		c.checkExportPos(n.Name.Position)
	}
	if _, dup := sc.names[n.Name.Name]; dup {
		c.errorf(n.Name.Position, "duplicate declaration of '%s'", n.Name.Name)
		return
	}
	info := &structInfo{
		name:     n.Name.Name,
		pos:      n.Name.Position,
		fieldSet: make(map[string]bool),
		methods:  make(map[string]int),
	}
	for _, f := range n.Fields {
		if f.Name == "self" {
			c.errorf(f.Position, "field name 'self' is reserved for methods")
			continue
		}
		if info.fieldSet[f.Name] {
			c.errorf(f.Position, "duplicate field '%s' in struct '%s'", f.Name, n.Name.Name)
			continue
		}
		info.fieldSet[f.Name] = true
		info.fields = append(info.fields, f.Name)
	}
	sc.names[n.Name.Name] = symbol{kind: symType, pos: n.Name.Position}
	c.types[n.Name.Name] = info
}

// checkInterface validates an interface declaration and records its contract.
func (c *checker) checkInterface(n *ast.InterfaceStmt, sc *scope) {
	if _, dup := sc.names[n.Name.Name]; dup {
		c.errorf(n.Name.Position, "duplicate declaration of '%s'", n.Name.Name)
		return
	}
	info := &ifaceInfo{name: n.Name.Name, pos: n.Name.Position, methods: make(map[string]int)}
	for _, m := range n.Methods {
		if info.hasMethod(m.Name.Name) {
			c.errorf(m.Name.Position, "duplicate method '%s' in interface '%s'", m.Name.Name, n.Name.Name)
			continue
		}
		info.methods[m.Name.Name] = len(m.Params)
	}
	sc.names[n.Name.Name] = symbol{kind: symType, pos: n.Name.Position}
	c.interfaces[n.Name.Name] = info
}

// checkMethod validates a method declaration and records it on its struct.
//
// Methods may appear in any scope. Inside a function they bind to the
// struct type visible in that scope. This keeps bundled modules valid: a
// bundle runs each module body inside a loader closure.
func (c *checker) checkMethod(n *ast.FnStmt, sc *scope) {
	if n.Export {
		c.checkExportPos(n.Name.Position)
	}
	info, ok := c.types[n.Receiver.Name]
	if !ok {
		if _, isIface := c.interfaces[n.Receiver.Name]; isIface {
			c.errorf(n.Receiver.Position, "cannot add a method to interface '%s'", n.Receiver.Name)
		} else {
			c.errorf(n.Receiver.Position, "unknown struct type '%s'", n.Receiver.Name)
		}
		return
	}
	if info.hasMethod(n.Name.Name) {
		c.errorf(n.Name.Position, "duplicate method '%s' on struct '%s'", n.Name.Name, n.Receiver.Name)
		return
	}
	if info.hasField(n.Name.Name) {
		c.errorf(n.Name.Position, "method '%s' conflicts with a field of struct '%s'", n.Name.Name, n.Receiver.Name)
		return
	}
	info.methods[n.Name.Name] = len(n.Params)

	funcScope := newScope(sc)
	funcScope.names["self"] = symbol{kind: symParam, pos: n.Name.Position, typeAnn: info.name}
	for _, p := range n.Params {
		c.checkParam(funcScope, p)
	}
	c.funcDepth++
	c.checkStmts(n.Body.Stmts, funcScope)
	c.funcDepth--
}

var knownTypes = map[string]bool{
	"int": true, "float": true, "string": true, "bool": true,
	"nil": true, "list": true, "map": true, "function": true, "range": true,
	"struct": true, "struct type": true, "method": true,
}

func (c *checker) checkTypeAnn(id *ast.Ident) {
	if knownTypes[id.Name] {
		return
	}
	if _, ok := c.types[id.Name]; ok {
		return
	}
	if _, ok := c.interfaces[id.Name]; ok {
		return
	}
	c.errorf(id.Position, "unknown type '%s'", id.Name)
}

func (c *checker) checkExpr(e ast.Expr, sc *scope) {
	switch n := e.(type) {
	case *ast.Ident:
		if _, isIface := c.interfaces[n.Name]; isIface {
			c.errorf(n.Position, "cannot use interface '%s' as a value", n.Name)
			return
		}
		if !c.resolve(sc, n.Name) && !builtinNames[n.Name] {
			c.errorf(n.Position, "undefined name '%s'", n.Name)
		}
	case *ast.AssignExpr:
		c.checkAssign(n, sc)
	case *ast.CallExpr:
		c.checkCall(n, sc)
	case *ast.IndexExpr:
		c.checkExpr(n.X, sc)
		c.checkExpr(n.Index, sc)
	case *ast.MemberExpr:
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
		c.checkStmts(n.Body.Stmts, fnScope)
		c.funcDepth--
	}
}

// checkAssign validates an assignment target and its value.
func (c *checker) checkAssign(n *ast.AssignExpr, sc *scope) {
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
		c.checkExpr(t.X, sc)
		c.checkExpr(t.Index, sc)
	case *ast.MemberExpr:
		c.checkMemberAssign(t, sc)
	default:
		c.errorf(t.Pos(), "cannot assign to this expression")
	}
	c.checkExpr(n.Value, sc)
}

// checkCall validates a call, or a struct literal when the callee is a
// struct type name.
func (c *checker) checkCall(n *ast.CallExpr, sc *scope) {
	if id, ok := n.Callee.(*ast.Ident); ok {
		if info, isStruct := c.types[id.Name]; isStruct {
			c.checkStructCall(info, n, sc)
			return
		}
	}
	c.checkExpr(n.Callee, sc)
	for _, a := range n.Args {
		c.checkExpr(a, sc)
	}
	for _, na := range n.Named {
		// A named call on a plain name cannot be a struct literal unless the
		// name is a struct type, which is handled above. A module member may
		// still be a struct type, so the runtime decides.
		if _, ok := n.Callee.(*ast.Ident); ok {
			c.errorf(na.Name.Position, "function '%s' does not accept named arguments", exprName(n.Callee))
		}
		c.checkExpr(na.Value, sc)
	}
}

// checkStructCall validates a struct literal against its type.
//
// Named arguments must name existing fields. Positional arguments must not
// exceed the field count. A struct literal is a value of its type, so a
// named literal is not re-checked as a call.
func (c *checker) checkStructCall(info *structInfo, n *ast.CallExpr, sc *scope) {
	c.checkExpr(n.Callee, sc)
	if len(n.Named) > 0 {
		seen := make(map[string]bool)
		for _, na := range n.Named {
			if !info.hasField(na.Name.Name) {
				c.errorf(na.Name.Position, "struct '%s' has no field '%s'", info.name, na.Name.Name)
			} else if seen[na.Name.Name] {
				c.errorf(na.Name.Position, "duplicate field '%s' in struct literal", na.Name.Name)
			}
			seen[na.Name.Name] = true
			c.checkExpr(na.Value, sc)
		}
		return
	}
	if len(n.Args) > len(info.fields) {
		c.errorf(n.Pos(), "struct '%s' has %d fields, got %d", info.name, len(info.fields), len(n.Args))
	}
	for _, a := range n.Args {
		c.checkExpr(a, sc)
	}
}

// checkMemberAssign validates a struct field assignment target.
func (c *checker) checkMemberAssign(t *ast.MemberExpr, sc *scope) {
	c.checkExpr(t.X, sc)
	if id, ok := t.X.(*ast.Ident); ok {
		if sym, found := c.lookup(sc, id.Name); found && sym.kind == symModule {
			c.errorf(t.Name.Position, "cannot assign to a member of module '%s'", id.Name)
			return
		}
	}
	if typ := c.staticType(sc, t.X); typ != "" {
		if info, ok := c.types[typ]; ok {
			if info.hasMethod(t.Name.Name) {
				c.errorf(t.Name.Position, "cannot assign to method '%s' of type '%s'", t.Name.Name, typ)
			} else if !info.hasField(t.Name.Name) {
				c.errorf(t.Name.Position, "type '%s' has no field '%s'", typ, t.Name.Name)
			}
		} else if _, isIface := c.interfaces[typ]; isIface {
			c.errorf(t.Name.Position, "cannot assign a field through interface '%s'", typ)
		}
	}
}

func (c *checker) resolve(sc *scope, name string) bool {
	_, ok := c.lookup(sc, name)
	return ok
}

// checkMember validates a member access on a statically known value.
//
// An imported module name gets a static check against the module's export
// table. A struct-typed base gets a check against its fields and methods. A
// base whose type is unknown is left to the runtime; a module value built by
// module() or by a bundle cannot be resolved statically.
func (c *checker) checkMember(n *ast.MemberExpr, sc *scope) {
	if id, ok := n.X.(*ast.Ident); ok {
		if sym, found := c.lookup(sc, id.Name); found && sym.kind == symModule {
			if _, exported := sym.exports[n.Name.Name]; !exported {
				c.errorf(n.Name.Position, "module '%s' has no exported member '%s'", id.Name, n.Name.Name)
			}
		}
	}
	if typ := c.staticType(sc, n.X); typ != "" {
		if info, ok := c.types[typ]; ok {
			if !info.hasMember(n.Name.Name) {
				c.errorf(n.Name.Position, "type '%s' has no field or method '%s'", typ, n.Name.Name)
			}
		} else if iface, ok := c.interfaces[typ]; ok {
			if !iface.hasMethod(n.Name.Name) {
				c.errorf(n.Name.Position, "interface '%s' has no method '%s'", typ, n.Name.Name)
			}
		}
	}
	c.checkExpr(n.X, sc)
}

// exprName renders an expression for use in diagnostics.
func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.MemberExpr:
		if id, ok := v.X.(*ast.Ident); ok {
			return id.Name + "." + v.Name.Name
		}
		return "member access"
	}
	return "this expression"
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
