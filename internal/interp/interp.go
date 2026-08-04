// Package interp is the tree-walking interpreter for Sprout.
//
// The interpreter shares its value semantics and standard library with the
// bytecode virtual machine through the runtime package. It evaluates the AST
// directly and keeps a single source of truth for arithmetic, comparison,
// indexing, and iteration behavior.
package interp

import (
	"fmt"
	"io"
	"os"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Interpreter evaluates an AST against an environment chain.
type Interpreter struct {
	globals  *Env
	builtins *Env
	file     *source.File
	ctx      runtime.Context
	frames   []diag.Frame
	// bundle and modCache support the module system. bundle is set for the
	// duration of an ExecBundle call. modCache holds each module's export
	// map so a module runs its top-level code exactly once.
	bundle   *module.Bundle
	modCache map[int]object.Object
}

// New returns an interpreter wired to the process standard streams.
func New() *Interpreter {
	return NewWithIO(os.Stdin, os.Stdout, os.Stderr)
}

// NewWithIO returns an interpreter with explicit streams.
func NewWithIO(stdin io.Reader, stdout, stderr io.Writer) *Interpreter {
	iv := &Interpreter{}
	iv.builtins = NewEnv(nil)
	iv.globals = NewEnv(iv.builtins)
	iv.ctx = runtime.Context{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Call: func(fn object.Object, args []object.Object, pos source.Pos) object.Object {
			return iv.call(fn, args, pos)
		},
	}
	RegisterBuiltins(iv)
	return iv
}

// Globals returns the top-level environment.
func (iv *Interpreter) Globals() *Env { return iv.globals }

// RunError is a runtime error with its source position and call stack.
type RunError struct {
	Message string
	File    *source.File
	Pos     source.Pos
	Frames  []diag.Frame
}

func (e *RunError) Error() string { return e.Message }

// runErr is the panic signal for runtime errors.
type runErr struct {
	message string
	pos     source.Pos
	frames  []diag.Frame
	file    *source.File
}

type returnSignal struct {
	value object.Object
	pos   source.Pos
	file  *source.File
}

type breakSignal struct {
	pos  source.Pos
	file *source.File
}

type continueSignal struct {
	pos  source.Pos
	file *source.File
}

// Exec evaluates the whole program in a fresh call from the caller.
//
// It returns the final statement value (usually nil) and a *RunError if the
// program stopped with a runtime error.
func (iv *Interpreter) Exec(file *source.File, prog *ast.Program) (val object.Object, rerr *RunError) {
	prevBundle, prevCache := iv.bundle, iv.modCache
	iv.bundle = nil
	iv.modCache = nil
	defer func() {
		iv.bundle = prevBundle
		iv.modCache = prevCache
	}()
	return iv.exec(file, prog, false)
}

// ExecBundle evaluates a whole module bundle, starting at its entry file.
//
// Import statements load their modules on first use. Each module runs its
// top-level code once and exposes its exports as a map value.
func (iv *Interpreter) ExecBundle(bundle *module.Bundle) (val object.Object, rerr *RunError) {
	prevBundle, prevCache := iv.bundle, iv.modCache
	iv.bundle = bundle
	iv.modCache = make(map[int]object.Object)
	defer func() {
		iv.bundle = prevBundle
		iv.modCache = prevCache
	}()
	return iv.exec(bundle.Files[0].Source, bundle.Files[0].Prog, true)
}

// exec runs prog against the global scope. When hoist is true, import
// statements run before any other statement of the file.
func (iv *Interpreter) exec(file *source.File, prog *ast.Program, hoist bool) (val object.Object, rerr *RunError) {
	prevFile := iv.file
	prevFrames := iv.frames
	iv.file = file
	iv.frames = nil
	defer func() {
		iv.file = prevFile
		iv.frames = prevFrames
		if r := recover(); r != nil {
			val = nil
			rerr = iv.asRunError(r)
		}
	}()
	if hoist {
		val = iv.evalTopLevel(prog.Stmts, iv.globals)
	} else {
		val = iv.evalStmts(prog.Stmts, iv.globals)
	}
	return val, nil
}

// Eval evaluates a single expression in the globals.
//
// It is used by the REPL and by tests.
func (iv *Interpreter) Eval(file *source.File, e ast.Expr) (val object.Object, rerr *RunError) {
	prevFile := iv.file
	prevFrames := iv.frames
	iv.file = file
	iv.frames = nil
	defer func() {
		iv.file = prevFile
		iv.frames = prevFrames
		if r := recover(); r != nil {
			val = nil
			rerr = iv.asRunError(r)
		}
	}()
	val = iv.evalExpr(e, iv.globals)
	return val, nil
}

func (iv *Interpreter) asRunError(r any) *RunError {
	switch s := r.(type) {
	case *runErr:
		return &RunError{Message: s.message, File: s.file, Pos: s.pos, Frames: s.frames}
	case *returnSignal:
		return &RunError{Message: "return used outside a function", File: s.file, Pos: s.pos}
	case *breakSignal:
		return &RunError{Message: "break used outside a loop", File: s.file, Pos: s.pos}
	case *continueSignal:
		return &RunError{Message: "continue used outside a loop", File: s.file, Pos: s.pos}
	default:
		panic(r)
	}
}

// raise aborts evaluation with a runtime error.
func (iv *Interpreter) raise(message string, pos source.Pos) {
	panic(&runErr{
		message: message,
		pos:     pos,
		frames:  append([]diag.Frame(nil), iv.frames...),
		file:    iv.file,
	})
}

// evalTopLevel evaluates a statement list with its import statements hoisted
// to the front, so a module loads before any statement that uses its names.
func (iv *Interpreter) evalTopLevel(stmts []ast.Stmt, env *Env) object.Object {
	var val object.Object = object.NilValue
	for _, s := range stmts {
		if _, ok := s.(*ast.ImportStmt); ok {
			val = iv.evalStmt(s, env)
		}
	}
	for _, s := range stmts {
		if _, ok := s.(*ast.ImportStmt); ok {
			continue
		}
		val = iv.evalStmt(s, env)
	}
	return val
}

func (iv *Interpreter) evalStmts(stmts []ast.Stmt, env *Env) object.Object {
	var val object.Object = object.NilValue
	for _, s := range stmts {
		val = iv.evalStmt(s, env)
	}
	return val
}

func (iv *Interpreter) evalBlock(block *ast.Block, env *Env) object.Object {
	child := NewEnv(env)
	var val object.Object = object.NilValue
	for _, s := range block.Stmts {
		val = iv.evalStmt(s, child)
	}
	return val
}

func (iv *Interpreter) evalStmt(s ast.Stmt, env *Env) object.Object {
	switch n := s.(type) {
	case *ast.LetStmt:
		var value object.Object = object.NilValue
		if n.Value != nil {
			value = iv.evalExpr(n.Value, env)
		}
		env.Define(n.Name.Name, value, n.IsConst)
		return object.NilValue

	case *ast.FnStmt:
		fn := &Function{Name: n.Name.Name, Params: n.Params, Body: n.Body, Env: env, File: iv.file}
		env.Define(n.Name.Name, fn, true)
		return object.NilValue

	case *ast.ExprStmt:
		return iv.evalExpr(n.X, env)

	case *ast.ReturnStmt:
		var value object.Object = object.NilValue
		if n.Value != nil {
			value = iv.evalExpr(n.Value, env)
		}
		panic(&returnSignal{value: value, pos: n.ReturnPos, file: iv.file})

	case *ast.BreakStmt:
		panic(&breakSignal{pos: n.Position, file: iv.file})

	case *ast.ContinueStmt:
		panic(&continueSignal{pos: n.Position, file: iv.file})

	case *ast.ImportStmt:
		return iv.evalImport(n, env)

	case *ast.IfStmt:
		if runtime.Truthy(iv.evalExpr(n.Cond, env)) {
			return iv.evalBlock(n.Then, env)
		}
		for _, b := range n.Elifs {
			if runtime.Truthy(iv.evalExpr(b.Cond, env)) {
				return iv.evalBlock(b.Body, env)
			}
		}
		if n.Else != nil {
			return iv.evalBlock(n.Else, env)
		}
		return object.NilValue

	case *ast.WhileStmt:
		for {
			if !runtime.Truthy(iv.evalExpr(n.Cond, env)) {
				return object.NilValue
			}
			if iv.runLoopBody(n.Body, env) {
				return object.NilValue
			}
		}

	case *ast.ForInStmt:
		iterable := iv.evalExpr(n.Iterable, env)
		items, err := runtime.Sequence(iterable)
		if err != nil {
			iv.raise(err.Error(), n.Iterable.Pos())
		}
		for _, item := range items {
			iterEnv := NewEnv(env)
			iterEnv.Define(n.Var.Name, item, false)
			if iv.runLoopBody(n.Body, iterEnv) {
				break
			}
		}
		return object.NilValue
	}
	iv.raise("unsupported statement", s.Pos())
	return nil
}

// evalImport loads a module and binds its exports into env.
//
// The alias form binds the module as one map value. The plain form binds
// every exported name directly, mirroring the compiler's behavior.
func (iv *Interpreter) evalImport(n *ast.ImportStmt, env *Env) object.Object {
	if iv.bundle == nil {
		iv.raise("'import' is not available in this context", n.Pos())
	}
	if n.Module < 0 || n.Module >= len(iv.bundle.Files) {
		iv.raise("internal error: unresolved import '"+n.Path+"'", n.Pos())
	}
	mod := iv.bundle.Files[n.Module]
	m, err := iv.loadModule(mod)
	if err != nil {
		iv.raise(err.Error(), n.Pos())
	}
	if n.Alias != nil {
		env.Define(n.Alias.Name, m, true)
		return object.NilValue
	}
	for _, name := range mod.Exports {
		env.Define(name, m.Vals[name], true)
	}
	return object.NilValue
}

// loadModule evaluates a module's top-level code and returns its exports.
//
// The result is cached, so a module is initialized exactly once even when
// several files import it. A module runs in an environment whose parent is
// the builtin scope, so it never sees another module's globals.
func (iv *Interpreter) loadModule(mod *module.File) (*object.Map, error) {
	if m, ok := iv.modCache[mod.Index]; ok {
		return m.(*object.Map), nil
	}
	prevFile := iv.file
	prevFrames := iv.frames
	iv.file = mod.Source
	iv.frames = nil
	defer func() {
		iv.file = prevFile
		iv.frames = prevFrames
	}()
	moduleEnv := NewEnv(iv.builtins)
	iv.evalTopLevel(mod.Prog.Stmts, moduleEnv)
	m := &object.Map{Vals: make(map[string]object.Object)}
	for _, name := range mod.Exports {
		v, err := moduleEnv.Get(name)
		if err != nil {
			return nil, err
		}
		m.Set(name, v)
	}
	iv.modCache[mod.Index] = m
	return m, nil
}

// runLoopBody executes a loop body, turning break and continue signals into
// booleans. It returns true when the loop must stop.
func (iv *Interpreter) runLoopBody(body *ast.Block, env *Env) (broken bool) {
	defer func() {
		switch r := recover().(type) {
		case nil:
		case *breakSignal:
			broken = true
		case *continueSignal:
			broken = false
		default:
			panic(r)
		}
	}()
	iv.evalBlock(body, env)
	return false
}

func (iv *Interpreter) evalExpr(e ast.Expr, env *Env) object.Object {
	switch n := e.(type) {
	case *ast.Ident:
		v, err := env.Get(n.Name)
		if err != nil {
			iv.raise(err.Error(), n.Position)
		}
		return v

	case *ast.IntLit:
		return object.Int{Value: n.Value}
	case *ast.FloatLit:
		return object.Float{Value: n.Value}
	case *ast.StrLit:
		return object.Str{Value: n.Value}
	case *ast.BoolLit:
		return object.Bool{Value: n.Value}
	case *ast.NilLit:
		return object.NilValue

	case *ast.ListLit:
		elems := make([]object.Object, len(n.Elems))
		for i, el := range n.Elems {
			elems[i] = iv.evalExpr(el, env)
		}
		return &object.List{Elems: elems}

	case *ast.MapLit:
		m := &object.Map{Vals: make(map[string]object.Object)}
		for _, en := range n.Entries {
			kv := iv.evalExpr(en.Key, env)
			ks, ok := kv.(object.Str)
			if !ok {
				iv.raise("map keys must be strings", en.Key.Pos())
			}
			m.Set(ks.Value, iv.evalExpr(en.Value, env))
		}
		return m

	case *ast.UnaryExpr:
		return iv.evalUnary(n, env)
	case *ast.BinaryExpr:
		return iv.evalBinary(n, env)
	case *ast.AssignExpr:
		return iv.evalAssign(n, env)

	case *ast.CallExpr:
		callee := iv.evalExpr(n.Callee, env)
		args := make([]object.Object, len(n.Args))
		for i, a := range n.Args {
			args[i] = iv.evalExpr(a, env)
		}
		return iv.call(callee, args, n.Pos())

	case *ast.IndexExpr:
		container := iv.evalExpr(n.X, env)
		idx := iv.evalExpr(n.Index, env)
		v, err := runtime.IndexGet(container, idx)
		if err != nil {
			iv.raise(err.Error(), n.Lbracket)
		}
		return v

	case *ast.FnExpr:
		return &Function{Params: n.Params, Body: n.Body, Env: env, File: iv.file}
	}
	iv.raise("unsupported expression", e.Pos())
	return nil
}

func (iv *Interpreter) evalUnary(n *ast.UnaryExpr, env *Env) object.Object {
	switch n.Op {
	case token.MINUS:
		v := iv.evalExpr(n.X, env)
		switch x := v.(type) {
		case object.Int:
			return object.Int{Value: -x.Value}
		case object.Float:
			return object.Float{Value: -x.Value}
		default:
			iv.raise(fmt.Sprintf("cannot negate a %s", v.Type()), n.OpPos)
		}
	case token.NOT:
		return object.Bool{Value: !runtime.Truthy(iv.evalExpr(n.X, env))}
	}
	iv.raise("unsupported unary operator", n.OpPos)
	return nil
}

func (iv *Interpreter) evalBinary(n *ast.BinaryExpr, env *Env) object.Object {
	left := iv.evalExpr(n.Left, env)

	switch n.Op {
	case token.AND:
		if !runtime.Truthy(left) {
			return left
		}
		return iv.evalExpr(n.Right, env)
	case token.OR:
		if runtime.Truthy(left) {
			return left
		}
		return iv.evalExpr(n.Right, env)
	}

	right := iv.evalExpr(n.Right, env)

	switch n.Op {
	case token.PLUS:
		return iv.arith(left, right, n.OpPos, runtime.Add)
	case token.MINUS:
		return iv.arith(left, right, n.OpPos, runtime.Sub)
	case token.STAR:
		return iv.arith(left, right, n.OpPos, runtime.Mul)
	case token.SLASH:
		return iv.arith(left, right, n.OpPos, runtime.Div)
	case token.PERCENT:
		return iv.arith(left, right, n.OpPos, runtime.Mod)
	case token.CARET:
		return iv.arith(left, right, n.OpPos, runtime.Pow)
	case token.EQ:
		return object.Bool{Value: runtime.Equal(left, right)}
	case token.NEQ:
		return object.Bool{Value: !runtime.Equal(left, right)}
	case token.LT, token.LE, token.GT, token.GE:
		cmp, ok := runtime.Compare(left, right)
		if !ok {
			iv.raise(fmt.Sprintf("cannot compare %s with %s", left.Type(), right.Type()), n.OpPos)
		}
		return object.Bool{Value: cmpResult(cmp, n.Op)}
	}
	iv.raise("unsupported binary operator", n.OpPos)
	return nil
}

// arith runs an arithmetic operation and raises a runtime error on failure.
func (iv *Interpreter) arith(a, b object.Object, pos source.Pos, op func(a, b object.Object) (object.Object, error)) object.Object {
	v, err := op(a, b)
	if err != nil {
		iv.raise(err.Error(), pos)
	}
	return v
}

// cmpResult turns a comparison result into a boolean.
func cmpResult(cmp int, op token.Kind) bool {
	switch op {
	case token.LT:
		return cmp < 0
	case token.LE:
		return cmp <= 0
	case token.GT:
		return cmp > 0
	case token.GE:
		return cmp >= 0
	}
	return false
}

func (iv *Interpreter) evalAssign(n *ast.AssignExpr, env *Env) object.Object {
	value := iv.evalExpr(n.Value, env)

	switch t := n.Target.(type) {
	case *ast.Ident:
		if err := env.Assign(t.Name, value); err != nil {
			iv.raise(err.Error(), n.OpPos)
		}
		return value

	case *ast.IndexExpr:
		container := iv.evalExpr(t.X, env)
		idx := iv.evalExpr(t.Index, env)
		v, err := runtime.SetIndex(container, idx, value)
		if err != nil {
			iv.raise(err.Error(), n.OpPos)
		}
		return v
	}
	iv.raise("invalid assignment target", n.OpPos)
	return nil
}

// call invokes a function or builtin with the given arguments.
func (iv *Interpreter) call(callee object.Object, args []object.Object, pos source.Pos) (val object.Object) {
	switch fn := callee.(type) {
	case *Function:
		if len(args) != len(fn.Params) {
			iv.raise(fmt.Sprintf("function '%s' expects %d arguments, got %d", fn.Name, len(fn.Params), len(args)), pos)
		}
		callEnv := NewEnv(fn.Env)
		for i, param := range fn.Params {
			callEnv.Define(param.Name.Name, args[i], false)
		}
		iv.pushFrame(fn.Name, pos)
		defer iv.popFrame()
		prevFile := iv.file
		iv.file = fn.File
		defer func() { iv.file = prevFile }()
		defer func() {
			if r := recover(); r != nil {
				if rs, ok := r.(*returnSignal); ok {
					val = rs.value
					return
				}
				panic(r)
			}
		}()
		return iv.evalBlock(fn.Body, callEnv)

	case *runtime.Builtin:
		if err := fn.CheckArgs(args, fn.Name); err != nil {
			iv.raise(err.Error(), pos)
		}
		v, err := fn.Fn(&iv.ctx, args, pos)
		if err != nil {
			iv.raise(err.Error(), pos)
		}
		return v
	}
	iv.raise(fmt.Sprintf("cannot call a %s", callee.Type()), pos)
	return nil
}

func (iv *Interpreter) pushFrame(name string, pos source.Pos) {
	fileName := ""
	if iv.file != nil {
		fileName = iv.file.Name
	}
	iv.frames = append(iv.frames, diag.Frame{Name: name, FileName: fileName, Pos: pos})
}

func (iv *Interpreter) popFrame() {
	iv.frames = iv.frames[:len(iv.frames)-1]
}
