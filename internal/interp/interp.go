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
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Interpreter evaluates an AST against an environment chain.
type Interpreter struct {
	globals *Env
	file    *source.File
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	ctx     runtime.Context
	frames  []diag.Frame
}

// New returns an interpreter wired to the process standard streams.
func New() *Interpreter {
	return NewWithIO(os.Stdin, os.Stdout, os.Stderr)
}

// NewWithIO returns an interpreter with explicit streams.
func NewWithIO(stdin io.Reader, stdout, stderr io.Writer) *Interpreter {
	iv := &Interpreter{
		globals: NewEnv(nil),
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
	}
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
}

type returnSignal struct {
	value object.Object
	pos   source.Pos
}

type breakSignal struct{ pos source.Pos }

type continueSignal struct{ pos source.Pos }

// Exec evaluates the whole program in a fresh call from the caller.
//
// It returns the final statement value (usually nil) and a *RunError if the
// program stopped with a runtime error.
func (iv *Interpreter) Exec(file *source.File, prog *ast.Program) (val object.Object, rerr *RunError) {
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
	val = iv.evalStmts(prog.Stmts, iv.globals)
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
		return &RunError{Message: s.message, File: iv.file, Pos: s.pos, Frames: s.frames}
	case *returnSignal:
		return &RunError{Message: "return used outside a function", File: iv.file, Pos: s.pos}
	case *breakSignal:
		return &RunError{Message: "break used outside a loop", File: iv.file, Pos: s.pos}
	case *continueSignal:
		return &RunError{Message: "continue used outside a loop", File: iv.file, Pos: s.pos}
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
	})
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
		fn := &Function{Name: n.Name.Name, Params: n.Params, Body: n.Body, Env: env}
		env.Define(n.Name.Name, fn, true)
		return object.NilValue

	case *ast.ExprStmt:
		return iv.evalExpr(n.X, env)

	case *ast.ReturnStmt:
		var value object.Object = object.NilValue
		if n.Value != nil {
			value = iv.evalExpr(n.Value, env)
		}
		panic(&returnSignal{value: value, pos: n.ReturnPos})

	case *ast.BreakStmt:
		panic(&breakSignal{pos: n.Position})

	case *ast.ContinueStmt:
		panic(&continueSignal{pos: n.Position})

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
		return iv.indexGet(container, idx, n.Lbracket)

	case *ast.FnExpr:
		return &Function{Params: n.Params, Body: n.Body, Env: env}
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
		return iv.arith(runtime.Add, left, right, n.OpPos)
	case token.MINUS:
		return iv.arith(runtime.Sub, left, right, n.OpPos)
	case token.STAR:
		return iv.arith(runtime.Mul, left, right, n.OpPos)
	case token.SLASH:
		return iv.arith(runtime.Div, left, right, n.OpPos)
	case token.PERCENT:
		return iv.arith(runtime.Mod, left, right, n.OpPos)
	case token.CARET:
		return iv.arith(runtime.Pow, left, right, n.OpPos)
	case token.EQ:
		return object.Bool{Value: runtime.Equal(left, right)}
	case token.NEQ:
		return object.Bool{Value: !runtime.Equal(left, right)}
	case token.LT, token.LE, token.GT, token.GE:
		return iv.compare(left, right, n.Op, n.OpPos)
	}
	iv.raise("unsupported binary operator", n.OpPos)
	return nil
}

// arith runs a runtime arithmetic operation or raises the error.
func (iv *Interpreter) arith(op func(a, b object.Object) (object.Object, error), a, b object.Object, pos source.Pos) object.Object {
	v, err := op(a, b)
	if err != nil {
		iv.raise(err.Error(), pos)
	}
	return v
}

func (iv *Interpreter) compare(a, b object.Object, op token.Kind, pos source.Pos) object.Object {
	cmp, ok := runtime.Compare(a, b)
	if !ok {
		iv.raise(fmt.Sprintf("cannot compare %s with %s", a.Type(), b.Type()), pos)
	}
	switch op {
	case token.LT:
		return object.Bool{Value: cmp < 0}
	case token.LE:
		return object.Bool{Value: cmp <= 0}
	case token.GT:
		return object.Bool{Value: cmp > 0}
	case token.GE:
		return object.Bool{Value: cmp >= 0}
	}
	return object.Bool{Value: false}
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
		if _, err := runtime.SetIndex(container, idx, value); err != nil {
			iv.raise(err.Error(), n.OpPos)
		}
		return value
	}
	iv.raise("invalid assignment target", n.OpPos)
	return nil
}

func (iv *Interpreter) indexGet(container, idx object.Object, pos source.Pos) object.Object {
	v, err := runtime.IndexGet(container, idx)
	if err != nil {
		iv.raise(err.Error(), pos)
	}
	return v
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

	case *Builtin:
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
