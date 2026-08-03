// Package compiler translates a Sprout syntax tree into bytecode.
//
// The compiler is a recursive-descent code generator. It walks the AST the
// same way the interpreter does, but it emits instructions for the bytecode
// virtual machine instead of evaluating nodes.
//
// Scoping mirrors the interpreter. Every block becomes an environment in the
// VM. Each identifier resolves to a depth and a slot. Depth counts the
// environments between the use and the definition. Slot is the variable's
// index in its environment.
package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/modules"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// maxCallArgs is the largest argument count that fits in a CALL byte.
const maxCallArgs = 255

// symbol is one declared name in a scope.
type symbol struct {
	slot     int
	isConst  bool
	isImport bool
}

// scope is one lexical scope. It mirrors one runtime environment.
type scope struct {
	parent   *scope
	depth    int
	nextSlot int
	names    map[string]*symbol
}

func newScope(parent *scope) *scope {
	sc := &scope{parent: parent, names: make(map[string]*symbol)}
	if parent != nil {
		sc.depth = parent.depth + 1
	}
	return sc
}

// loopInfo describes the loop that break and continue target.
type loopInfo struct {
	continueTarget int
	envDepth       int
	hasIter        bool
	breaks         []int
}

// fnState is the compilation state of one function.
type fnState struct {
	b   *code.Builder
	cur *scope
}

// Compiler turns a program into bytecode.
type Compiler struct {
	file *source.File
	// fileIdx is the index of file in the program's file list.
	fileIdx int
	// imports maps an import specifier to its module index for the unit
	// currently being compiled.
	imports map[string]int
	// fns is the stack of functions being compiled. The top is current.
	fns []fnState
	// loops is the stack of enclosing loops in the current function.
	loops []loopInfo
	// builtins maps a standard library name to its index.
	builtins map[string]uint16
}

// Compile translates prog into a runnable program.
func Compile(file *source.File, prog *ast.Program) (p *code.Program, err error) {
	c := &Compiler{file: file, builtins: builtinIndex()}
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*compileError); ok {
				p, err = nil, ce
				return
			}
			panic(r)
		}
	}()
	c.pushFn(code.NewBuilder("<main>", file.Name, nil), newScope(nil))
	c.compileStmts(prog.Stmts)
	c.b().Add(code.OpReturn, endPos(prog.Stmts))
	main := c.finishFn()
	return &code.Program{Main: main, Files: []*source.File{file}}, nil
}

// CompileGraph translates a module graph into a runnable program.
//
// Each module becomes an initializer function in Program.Modules, in
// dependency order. The entry unit becomes the main function. Import
// statements inside a unit compile to a PUSH_MODULE of the referenced
// module's index followed by a SET_LOCAL of the alias.
func CompileGraph(g *modules.Graph) (p *code.Program, err error) {
	c := &Compiler{builtins: builtinIndex()}
	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*compileError); ok {
				p, err = nil, ce
				return
			}
			panic(r)
		}
	}()

	prog := &code.Program{Files: make([]*source.File, len(g.Units))}
	for i, u := range g.Units {
		prog.Files[i] = u.File
	}

	for i, u := range g.Units {
		if i == g.Entry {
			continue
		}
		c.file = u.File
		c.fileIdx = i
		c.imports = importIndex(g, i)

		sc := newScope(nil)
		c.pushFn(code.NewBuilder("module:"+filepath.Base(u.Path), u.File.Name, nil), sc)
		c.compileStmts(u.Prog.Stmts)
		c.b().Add(code.OpReturn, endPos(u.Prog.Stmts))
		fn := c.finishFn()
		prog.Modules = append(prog.Modules, &code.Module{
			Path:    u.Path,
			Exports: moduleExports(sc, fn.NumSlots),
			Init:    fn,
		})
		c.popFn()
	}

	entry := g.Units[g.Entry]
	c.file = entry.File
	c.fileIdx = g.Entry
	c.imports = importIndex(g, g.Entry)

	c.pushFn(code.NewBuilder("<main>", entry.File.Name, nil), newScope(nil))
	c.compileStmts(entry.Prog.Stmts)
	c.b().Add(code.OpReturn, endPos(entry.Prog.Stmts))
	prog.Main = c.finishFn()
	return prog, nil
}

// importIndex maps each import specifier of unit i to its module index.
//
// Module indexes equal unit indexes because every import target precedes the
// entry unit, and the entry unit is not a module.
func importIndex(g *modules.Graph, i int) map[string]int {
	idx := make(map[string]int)
	for _, imp := range g.Units[i].Imports {
		if imp.Target >= 0 {
			idx[imp.Spec] = imp.Target
		}
	}
	return idx
}

// moduleExports lists the exported name of each init environment slot.
//
// An empty name marks an import alias, which stays private to the module.
func moduleExports(sc *scope, slots int) []string {
	exports := make([]string, slots)
	for name, sym := range sc.names {
		if sym.isImport {
			continue
		}
		exports[sym.slot] = name
	}
	return exports
}

// compileError is the panic signal for internal compiler failures.
type compileError struct {
	pos source.Pos
	msg string
}

func (e *compileError) Error() string { return e.msg }

// failf aborts compilation with a formatted error.
func (c *Compiler) failf(pos source.Pos, format string, args ...any) {
	panic(&compileError{pos: pos, msg: fmt.Sprintf(format, args...)})
}

// builtinIndex builds the name-to-index table for the standard library.
func builtinIndex() map[string]uint16 {
	idx := make(map[string]uint16, len(runtime.Builtins))
	for i, b := range runtime.Builtins {
		idx[b.Name] = uint16(i)
	}
	return idx
}

// endPos returns a position that marks the end of a statement list.
func endPos(stmts []ast.Stmt) source.Pos {
	if len(stmts) > 0 {
		return stmts[len(stmts)-1].End()
	}
	return source.Pos{Line: 1, Column: 1}
}

func (c *Compiler) b() *code.Builder { return c.fns[len(c.fns)-1].b }

func (c *Compiler) cur() *scope { return c.fns[len(c.fns)-1].cur }

func (c *Compiler) pushFn(b *code.Builder, cur *scope) {
	c.fns = append(c.fns, fnState{b: b, cur: cur})
}

func (c *Compiler) popFn() { c.fns = c.fns[:len(c.fns)-1] }

// finishFn finalizes the current function and returns it.
func (c *Compiler) finishFn() *code.Function {
	st := &c.fns[len(c.fns)-1]
	st.b.SetNumSlots(st.cur.nextSlot)
	fn := st.b.Finish()
	fn.FileIdx = c.fileIdx
	return fn
}

// enterFn compiles a function body and returns its constant index.
//
// The compiled function is added to the enclosing function's constant pool.
// Loops do not cross function boundaries, so a fresh loop stack starts here.
func (c *Compiler) enterFn(name string, params []*ast.Param, body *ast.Block) uint16 {
	paramNames := make([]string, len(params))
	for i, p := range params {
		paramNames[i] = p.Name.Name
	}
	sc := newScope(c.cur())
	sc.nextSlot = len(params)
	for i, p := range params {
		sc.names[p.Name.Name] = &symbol{slot: i}
	}
	prevLoops := c.loops
	c.loops = nil
	c.pushFn(code.NewBuilder(name, c.file.Name, paramNames), sc)
	c.compileStmts(body.Stmts)
	c.b().Add(code.OpReturn, endPos(body.Stmts))
	fn := c.finishFn()
	c.popFn()
	c.loops = prevLoops
	return c.b().Const(fn)
}

// declare binds name to the next slot in the current scope.
func (c *Compiler) declare(name string, isConst bool, pos source.Pos) {
	sc := c.cur()
	if _, dup := sc.names[name]; dup {
		c.failf(pos, "duplicate declaration of '%s'", name)
	}
	sc.names[name] = &symbol{slot: sc.nextSlot, isConst: isConst}
	sc.nextSlot++
}

// declareImport binds an import alias to the next slot.
//
// The alias occupies a slot so module members can resolve, but it is not an
// export of the module.
func (c *Compiler) declareImport(name string, pos source.Pos) {
	sc := c.cur()
	if _, dup := sc.names[name]; dup {
		c.failf(pos, "duplicate declaration of '%s'", name)
	}
	sc.names[name] = &symbol{slot: sc.nextSlot, isImport: true}
	sc.nextSlot++
}

// lookup finds name in the scope chain.
//
// It returns the symbol and the environment depth of the binding. Depth 0
// means the current environment.
func (c *Compiler) lookup(name string) (sym *symbol, depth int, ok bool) {
	for sc := c.cur(); sc != nil; sc = sc.parent {
		if s, found := sc.names[name]; found {
			return s, c.cur().depth - sc.depth, true
		}
	}
	return nil, 0, false
}

// resolve finds name in the scope chain.
//
// It returns the environment depth and slot of the binding.
func (c *Compiler) resolve(name string) (depth, slot int, ok bool) {
	sym, depth, ok := c.lookup(name)
	if !ok {
		return 0, 0, false
	}
	return depth, sym.slot, true
}

// pushScope enters a new block scope and emits OpNewEnv.
func (c *Compiler) pushScope(pos source.Pos) int {
	sc := newScope(c.cur())
	c.pushFn(c.b(), sc)
	return c.b().AddU16(code.OpNewEnv, 0, pos)
}

// popScope leaves the current block scope and emits OpEndEnv.
func (c *Compiler) popScope(envOff int, pos source.Pos) {
	sc := c.cur()
	c.b().PatchU16(envOff, uint16(sc.nextSlot))
	c.b().Add(code.OpEndEnv, pos)
	c.popFn()
}

// compileStmts compiles a statement list.
//
// Function names are pre-declared so later statements and recursion can see
// them. This mirrors the checker and the interpreter's forward references.
func (c *Compiler) compileStmts(stmts []ast.Stmt) {
	for _, s := range stmts {
		if fn, ok := s.(*ast.FnStmt); ok {
			c.declare(fn.Name.Name, true, fn.Name.Position)
		}
	}
	for _, s := range stmts {
		c.compileStmt(s)
	}
}

func (c *Compiler) compileStmt(s ast.Stmt) {
	switch n := s.(type) {
	case *ast.ImportStmt:
		idx, ok := c.imports[n.Path]
		if !ok {
			c.failf(n.KwPos, "cannot resolve import %q", n.Path)
		}
		c.declareImport(n.Alias.Name, n.KwPos)
		c.b().AddU16(code.OpPushModule, uint16(idx), n.KwPos)
		c.emitSet(n.Alias.Name, n.KwPos)

	case *ast.LetStmt:
		c.declare(n.Name.Name, n.IsConst, n.Name.Position)
		if n.Value != nil {
			c.compileExpr(n.Value)
		} else {
			c.b().Add(code.OpNil, n.KwPos)
		}
		c.emitSet(n.Name.Name, n.KwPos)

	case *ast.FnStmt:
		idx := c.enterFn(n.Name.Name, n.Params, n.Body)
		c.b().AddU16(code.OpClosure, idx, n.FnPos)
		c.emitSet(n.Name.Name, n.Name.Position)

	case *ast.ExprStmt:
		c.compileExpr(n.X)
		c.b().Add(code.OpPop, n.Pos())

	case *ast.ReturnStmt:
		if n.Value != nil {
			c.compileExpr(n.Value)
			c.b().Add(code.OpReturnValue, n.ReturnPos)
		} else {
			c.b().Add(code.OpReturn, n.ReturnPos)
		}

	case *ast.BreakStmt:
		c.emitBreak(n.Position)

	case *ast.ContinueStmt:
		c.emitContinue(n.Position)

	case *ast.IfStmt:
		c.compileIf(n)

	case *ast.WhileStmt:
		c.compileWhile(n)

	case *ast.ForInStmt:
		c.compileForIn(n)

	default:
		c.failf(s.Pos(), "internal error: unsupported statement")
	}
}

func (c *Compiler) compileIf(n *ast.IfStmt) {
	c.compileExpr(n.Cond)
	elseTarget := c.b().AddU16(code.OpJumpIfFalse, 0, n.IfPos)
	c.compileBlock(n.Then)
	endTarget := c.b().AddU16(code.OpJump, 0, n.Then.Pos())
	c.b().PatchU16(elseTarget, uint16(c.b().Len()))
	// Each elif body ends with a jump that skips the branches below it.
	// Collect them so they can be patched to the end of the whole chain.
	var elifEnds []int
	for _, b := range n.Elifs {
		c.compileExpr(b.Cond)
		next := c.b().AddU16(code.OpJumpIfFalse, 0, b.Cond.Pos())
		c.compileBlock(b.Body)
		elifEnds = append(elifEnds, c.b().AddU16(code.OpJump, 0, b.Body.Pos()))
		c.b().PatchU16(next, uint16(c.b().Len()))
	}
	if n.Else != nil {
		c.compileBlock(n.Else)
	}
	end := c.b().Len()
	c.b().PatchU16(endTarget, uint16(end))
	for _, off := range elifEnds {
		c.b().PatchU16(off, uint16(end))
	}
}

// compileBlock compiles a braced block as a fresh scope.
func (c *Compiler) compileBlock(b *ast.Block) {
	envOff := c.pushScope(b.Lbrace)
	c.compileStmts(b.Stmts)
	c.popScope(envOff, b.Rbrace)
}

func (c *Compiler) compileWhile(n *ast.WhileStmt) {
	condStart := c.b().Len()
	c.compileExpr(n.Cond)
	endTarget := c.b().AddU16(code.OpJumpIfFalse, 0, n.WhilePos)
	c.loops = append(c.loops, loopInfo{continueTarget: condStart, envDepth: c.cur().depth})
	c.compileBlock(n.Body)
	c.b().AddU16(code.OpJump, uint16(condStart), n.Body.Pos())
	c.patchLoop(loopEndPos(n), endTarget)
}

func (c *Compiler) compileForIn(n *ast.ForInStmt) {
	c.compileExpr(n.Iterable)
	c.b().Add(code.OpMakeIter, n.ForPos)
	nextStart := c.b().Len()
	iterEnd := c.b().AddU16(code.OpIterNext, 0, n.ForPos)
	c.loops = append(c.loops, loopInfo{continueTarget: nextStart, envDepth: c.cur().depth, hasIter: true})

	envOff := c.b().AddU16(code.OpNewEnv, 0, n.ForPos)
	sc := newScope(c.cur())
	c.pushFn(c.b(), sc)
	c.declare(n.Var.Name, false, n.Var.Position)
	c.emitSet(n.Var.Name, n.Var.Position)

	c.compileStmts(n.Body.Stmts)
	c.popScope(envOff, n.Body.Pos())
	c.b().AddU16(code.OpJump, uint16(nextStart), n.Body.Pos())
	c.patchLoop(loopEndPos(n), iterEnd)
}

// loopEndPos returns a position to attach loop-finalization instructions to.
func loopEndPos(n ast.Stmt) source.Pos {
	switch v := n.(type) {
	case *ast.WhileStmt:
		return v.Body.Pos()
	case *ast.ForInStmt:
		return v.Body.Pos()
	}
	return source.Pos{}
}

// patchLoop finalizes a loop's end target and its pending break jumps.
func (c *Compiler) patchLoop(pos source.Pos, endTarget int) {
	end := c.b().Len()
	c.b().PatchU16(endTarget, uint16(end))
	top := &c.loops[len(c.loops)-1]
	for _, off := range top.breaks {
		c.b().PatchU16(off, uint16(end))
	}
	c.loops = c.loops[:len(c.loops)-1]
}

func (c *Compiler) emitBreak(pos source.Pos) {
	if len(c.loops) == 0 {
		c.failf(pos, "break outside a loop")
	}
	top := &c.loops[len(c.loops)-1]
	c.emitEnvCleanup(top.envDepth, pos)
	if top.hasIter {
		c.b().Add(code.OpPop, pos)
	}
	off := c.b().AddU16(code.OpJump, 0, pos)
	top.breaks = append(top.breaks, off)
}

func (c *Compiler) emitContinue(pos source.Pos) {
	if len(c.loops) == 0 {
		c.failf(pos, "continue outside a loop")
	}
	top := c.loops[len(c.loops)-1]
	c.emitEnvCleanup(top.envDepth, pos)
	c.b().AddU16(code.OpJump, uint16(top.continueTarget), pos)
}

// emitEnvCleanup pops the block environments down to envDepth.
func (c *Compiler) emitEnvCleanup(envDepth int, pos source.Pos) {
	for d := c.cur().depth; d > envDepth; d-- {
		c.b().Add(code.OpEndEnv, pos)
	}
}

func (c *Compiler) compileExpr(e ast.Expr) {
	switch n := e.(type) {
	case *ast.IntLit:
		idx := c.b().Const(object.Int{Value: n.Value})
		c.b().AddU16(code.OpPushConst, idx, n.Position)
	case *ast.FloatLit:
		idx := c.b().Const(object.Float{Value: n.Value})
		c.b().AddU16(code.OpPushConst, idx, n.Position)
	case *ast.StrLit:
		idx := c.b().Const(object.Str{Value: n.Value})
		c.b().AddU16(code.OpPushConst, idx, n.Position)
	case *ast.BoolLit:
		if n.Value {
			c.b().Add(code.OpTrue, n.Position)
		} else {
			c.b().Add(code.OpFalse, n.Position)
		}
	case *ast.NilLit:
		c.b().Add(code.OpNil, n.Position)
	case *ast.Ident:
		c.emitGet(n.Name, n.Position)

	case *ast.ListLit:
		for _, el := range n.Elems {
			c.compileExpr(el)
		}
		c.b().AddU16(code.OpBuildList, uint16(len(n.Elems)), n.Lbracket)

	case *ast.MapLit:
		for _, en := range n.Entries {
			c.compileExpr(en.Key)
			c.compileExpr(en.Value)
		}
		c.b().AddU16(code.OpBuildMap, uint16(len(n.Entries)), n.Lbrace)

	case *ast.UnaryExpr:
		c.compileExpr(n.X)
		switch n.Op {
		case token.MINUS:
			c.b().Add(code.OpNeg, n.OpPos)
		case token.NOT:
			c.b().Add(code.OpNot, n.OpPos)
		default:
			c.failf(n.OpPos, "internal error: unsupported unary operator")
		}

	case *ast.BinaryExpr:
		c.compileBinary(n)

	case *ast.AssignExpr:
		c.compileAssign(n)

	case *ast.CallExpr:
		if len(n.Args) > maxCallArgs {
			c.failf(n.Lparen, "cannot call a function with more than %d arguments", maxCallArgs)
		}
		c.compileExpr(n.Callee)
		for _, a := range n.Args {
			c.compileExpr(a)
		}
		c.b().AddByte(code.OpCall, byte(len(n.Args)), n.Lparen)

	case *ast.IndexExpr:
		c.compileExpr(n.X)
		c.compileExpr(n.Index)
		c.b().Add(code.OpGetIndex, n.Lbracket)

	case *ast.MemberExpr:
		c.compileExpr(n.X)
		idx := c.b().Const(object.Str{Value: n.Name.Name})
		c.b().AddU16(code.OpGetMember, idx, n.Dot)

	case *ast.FnExpr:
		idx := c.enterFn("", n.Params, n.Body)
		c.b().AddU16(code.OpClosure, idx, n.FnPos)

	default:
		c.failf(e.Pos(), "internal error: unsupported expression")
	}
}

func (c *Compiler) compileBinary(n *ast.BinaryExpr) {
	switch n.Op {
	case token.AND:
		c.compileExpr(n.Left)
		c.b().Add(code.OpDup, n.OpPos)
		skip := c.b().AddU16(code.OpJumpIfFalse, 0, n.OpPos)
		c.b().Add(code.OpPop, n.OpPos)
		c.compileExpr(n.Right)
		c.b().PatchU16(skip, uint16(c.b().Len()))
	case token.OR:
		c.compileExpr(n.Left)
		c.b().Add(code.OpDup, n.OpPos)
		skip := c.b().AddU16(code.OpJumpIfTrue, 0, n.OpPos)
		c.b().Add(code.OpPop, n.OpPos)
		c.compileExpr(n.Right)
		c.b().PatchU16(skip, uint16(c.b().Len()))
	default:
		c.compileExpr(n.Left)
		c.compileExpr(n.Right)
		c.b().Add(binOp(n.Op), n.OpPos)
	}
}

// binOp maps a binary operator token to its opcode.
func binOp(k token.Kind) code.Opcode {
	switch k {
	case token.PLUS:
		return code.OpAdd
	case token.MINUS:
		return code.OpSub
	case token.STAR:
		return code.OpMul
	case token.SLASH:
		return code.OpDiv
	case token.PERCENT:
		return code.OpMod
	case token.CARET:
		return code.OpPow
	case token.EQ:
		return code.OpEq
	case token.NEQ:
		return code.OpNeq
	case token.LT:
		return code.OpLt
	case token.LE:
		return code.OpLe
	case token.GT:
		return code.OpGt
	case token.GE:
		return code.OpGe
	}
	panic("unreachable")
}

func (c *Compiler) compileAssign(n *ast.AssignExpr) {
	switch t := n.Target.(type) {
	case *ast.Ident:
		if sym, _, ok := c.lookup(t.Name); ok && sym.isConst {
			c.failf(n.OpPos, "cannot assign to constant '%s'", t.Name)
		}
		c.compileExpr(n.Value)
		c.b().Add(code.OpDup, n.OpPos)
		c.emitSet(t.Name, n.OpPos)
	case *ast.IndexExpr:
		c.compileExpr(t.X)
		c.compileExpr(t.Index)
		c.compileExpr(n.Value)
		c.b().Add(code.OpSetIndex, n.OpPos)
	default:
		c.failf(n.OpPos, "internal error: invalid assignment target")
	}
}

// emitGet loads a name onto the stack.
func (c *Compiler) emitGet(name string, pos source.Pos) {
	if depth, slot, ok := c.resolve(name); ok {
		if depth == 0 {
			c.b().AddU16(code.OpGetLocal, uint16(slot), pos)
		} else {
			c.b().AddU16Pair(code.OpGetUp, uint16(depth), uint16(slot), pos)
		}
		return
	}
	if idx, ok := c.builtins[name]; ok {
		c.b().AddU16(code.OpBuiltin, idx, pos)
		return
	}
	c.failf(pos, "undefined name '%s'", name)
}

// emitSet stores the top of the stack into a name.
func (c *Compiler) emitSet(name string, pos source.Pos) {
	depth, slot, ok := c.resolve(name)
	if !ok {
		c.failf(pos, "undefined name '%s'", name)
	}
	if depth == 0 {
		c.b().AddU16(code.OpSetLocal, uint16(slot), pos)
	} else {
		c.b().AddU16Pair(code.OpSetUp, uint16(depth), uint16(slot), pos)
	}
}
