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

// ModuleResolver answers an import specifier with its loaded file.
type ModuleResolver func(spec, fromPath string) (*module.File, bool)

// Interpreter evaluates an AST against an environment chain.
type Interpreter struct {
	globals *Env
	file    *source.File
	ctx     runtime.Context
	frames  []diag.Frame

	// Module state. The cache keeps load-once semantics across runs of the
	// same interpreter. The stack guards against import cycles.
	resolver      ModuleResolver
	moduleCache   map[string]*object.Module
	moduleStack   []string
	moduleExports map[string]object.Object
}

// New returns an interpreter wired to the process standard streams.
func New() *Interpreter {
	return NewWithIO(os.Stdin, os.Stdout, os.Stderr)
}

// NewWithIO returns an interpreter with explicit streams.
func NewWithIO(stdin io.Reader, stdout, stderr io.Writer) *Interpreter {
	iv := &Interpreter{globals: NewEnv(nil)}
	iv.ctx = runtime.Context{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Call: func(fn object.Object, args []object.Object, pos source.Pos) object.Object {
			return iv.call(fn, args, pos)
		},
		Spawn: iv.spawn,
	}
	RegisterBuiltins(iv)
	return iv
}

// Globals returns the top-level environment.
func (iv *Interpreter) Globals() *Env { return iv.globals }

// SetModuleResolver installs the hook that resolves import statements.
//
// Without a resolver, an import fails with a runtime error. The CLI installs
// a resolver backed by the project's module graph.
func (iv *Interpreter) SetModuleResolver(r ModuleResolver) { iv.resolver = r }

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
	prevStack := iv.moduleStack
	prevExports := iv.moduleExports
	if iv.moduleCache == nil {
		iv.moduleCache = make(map[string]*object.Module)
	}
	iv.file = file
	iv.frames = nil
	iv.moduleStack = nil
	iv.moduleExports = nil
	defer func() {
		iv.file = prevFile
		iv.frames = prevFrames
		iv.moduleStack = prevStack
		iv.moduleExports = prevExports
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
		if n.Export && iv.moduleExports != nil {
			iv.moduleExports[n.Name.Name] = value
		}
		return object.NilValue

	case *ast.ImportStmt:
		m, err := iv.loadModule(n.Path, iv.currentFileName())
		if err != nil {
			iv.raise(err.Error(), n.ImportPos)
		}
		env.Define(n.Name.Name, m, true)
		return object.NilValue

	case *ast.FnStmt:
		if n.Receiver != nil {
			return iv.evalMethod(n, env)
		}
		fn := &Function{Name: n.Name.Name, Params: n.Params, Body: n.Body, Env: env}
		env.Define(n.Name.Name, fn, true)
		if n.Export && iv.moduleExports != nil {
			iv.moduleExports[n.Name.Name] = fn
		}
		return object.NilValue

	case *ast.StructStmt:
		st := &object.StructType{
			Name:    n.Name.Name,
			Fields:  fieldNames(n.Fields),
			Methods: make(map[string]object.Object),
		}
		env.Define(n.Name.Name, st, true)
		if n.Export && iv.moduleExports != nil {
			iv.moduleExports[n.Name.Name] = st
		}
		return object.NilValue

	case *ast.InterfaceStmt:
		// Interfaces are a static contract; they carry no runtime value.
		env.Define(n.Name.Name, object.NilValue, true)
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

// evalMethod registers a method on its struct type.
//
// The method captures the environment where it was declared, so a method
// body can close over module names and imported bindings. The receiver type
// must already be bound in env.
func (iv *Interpreter) evalMethod(n *ast.FnStmt, env *Env) object.Object {
	v, err := env.Get(n.Receiver.Name)
	if err != nil {
		iv.raise(fmt.Sprintf("unknown struct type '%s'", n.Receiver.Name), n.Receiver.Position)
	}
	st, ok := v.(*object.StructType)
	if !ok {
		iv.raise(fmt.Sprintf("cannot add a method to a %s", v.Type()), n.Receiver.Position)
	}
	if st.HasField(n.Name.Name) {
		iv.raise(fmt.Sprintf("method '%s' conflicts with a field of struct '%s'", n.Name.Name, n.Receiver.Name), n.Name.Position)
	}
	fn := &Function{Name: n.Receiver.Name + "." + n.Name.Name, Params: n.Params, Body: n.Body, Env: env}
	// Re-registration overrides the previous method, so a declaration that
	// runs more than once stays valid.
	st.Methods[n.Name.Name] = fn
	if n.Export && iv.moduleExports != nil {
		iv.moduleExports[n.Receiver.Name] = st
	}
	return object.NilValue
}

// fieldNames returns the declared field names of a struct body.
func fieldNames(fields []*ast.Ident) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
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
		if len(n.Named) > 0 {
			return iv.constructStruct(callee, n, env)
		}
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

	case *ast.MemberExpr:
		x := iv.evalExpr(n.X, env)
		v, err := runtime.GetMember(x, n.Name.Name)
		if err != nil {
			iv.raise(err.Error(), n.DotPos)
		}
		return v

	case *ast.FnExpr:
		return &Function{Params: n.Params, Body: n.Body, Env: env}

	case *ast.MatchExpr:
		return iv.evalMatch(n, env)
	}
	iv.raise("unsupported expression", e.Pos())
	return nil
}

// evalMatch evaluates a match expression.
//
// The subject is evaluated once. Arms are tried in order; the first arm
// whose pattern matches runs its body. Pattern variables bind in the arm's
// block scope. The checker requires a catch-all arm, so this loop always
// returns through an arm.
func (iv *Interpreter) evalMatch(n *ast.MatchExpr, env *Env) object.Object {
	subject := iv.evalExpr(n.Subject, env)
	for _, arm := range n.Arms {
		binding, matched := iv.matchPattern(arm.Pattern, subject)
		if !matched {
			continue
		}
		armEnv := NewEnv(env)
		if binding.name != "" {
			armEnv.Define(binding.name, binding.value, false)
		}
		return iv.evalMatchArm(arm.Body, armEnv)
	}
	iv.raise(fmt.Sprintf("no arm of the match matched a %s", subject.Type()), n.MatchPos)
	return nil
}

// evalMatchArm runs an arm body and returns its value.
//
// The value is the last expression statement's value. A body that ends in
// any other statement, or that is empty, is worth nil. This mirrors the
// compiler, so both engines agree on the value of a match.
func (iv *Interpreter) evalMatchArm(body *ast.Block, env *Env) object.Object {
	n := len(body.Stmts)
	for i := 0; i < n; i++ {
		s := body.Stmts[i]
		if i == n-1 {
			if es, ok := s.(*ast.ExprStmt); ok {
				return iv.evalExpr(es.X, env)
			}
			iv.evalStmt(s, env)
			return object.NilValue
		}
		iv.evalStmt(s, env)
	}
	return object.NilValue
}

// patternBinding is the single variable a match pattern can bind.
type patternBinding struct {
	name  string
	value object.Object
}

// matchPattern tests value against p. It reports whether the pattern
// matched and, when the pattern binds a variable, the name and value.
func (iv *Interpreter) matchPattern(p ast.Pattern, value object.Object) (patternBinding, bool) {
	switch v := p.(type) {
	case *ast.WildcardPattern:
		return patternBinding{}, true
	case *ast.VarPattern:
		return patternBinding{name: v.Ident.Name, value: value}, true
	case *ast.LitPattern:
		lit := literalValue(v.Value)
		if lit == nil || !runtime.Equal(lit, value) {
			return patternBinding{}, false
		}
		return patternBinding{}, true
	case *ast.ResultPattern:
		r, ok := value.(object.Result)
		if !ok || r.Ok != v.IsOk {
			return patternBinding{}, false
		}
		if r.Ok {
			return iv.matchPattern(v.Inner, r.Value)
		}
		return iv.matchPattern(v.Inner, object.Str{Value: r.Message})
	}
	return patternBinding{}, false
}

// literalValue returns the runtime value of a match literal, or nil when the
// node is not a literal the parser is allowed to produce here.
func literalValue(e ast.Expr) object.Object {
	switch v := e.(type) {
	case *ast.IntLit:
		return object.Int{Value: v.Value}
	case *ast.FloatLit:
		return object.Float{Value: v.Value}
	case *ast.StrLit:
		return object.Str{Value: v.Value}
	case *ast.BoolLit:
		return object.Bool{Value: v.Value}
	case *ast.NilLit:
		return object.NilValue
	}
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

	case *ast.MemberExpr:
		container := iv.evalExpr(t.X, env)
		v, err := runtime.SetMember(container, t.Name.Name, value)
		if err != nil {
			iv.raise(err.Error(), n.OpPos)
		}
		return v
	}
	iv.raise("invalid assignment target", n.OpPos)
	return nil
}

// constructStruct builds a struct instance from named arguments.
func (iv *Interpreter) constructStruct(callee object.Object, n *ast.CallExpr, env *Env) object.Object {
	st, ok := callee.(*object.StructType)
	if !ok {
		iv.raise(fmt.Sprintf("cannot call a %s with named arguments", callee.Type()), n.Pos())
	}
	names := make([]string, len(n.Named))
	values := make([]object.Object, len(n.Named))
	for i, na := range n.Named {
		names[i] = na.Name.Name
		values[i] = iv.evalExpr(na.Value, env)
	}
	s, err := st.ConstructNamed(names, values)
	if err != nil {
		iv.raise(err.Error(), n.Pos())
	}
	return s
}

// call invokes a function or builtin with the given arguments.
func (iv *Interpreter) call(callee object.Object, args []object.Object, pos source.Pos) (val object.Object) {
	switch fn := callee.(type) {
	case *Function:
		return iv.callFunction(fn, args, nil, pos)

	case *object.BoundMethod:
		f, ok := fn.Method.(*Function)
		if !ok {
			iv.raise(fmt.Sprintf("cannot call this method value"), pos)
		}
		return iv.callFunction(f, args, fn.Receiver, pos)

	case *object.StructType:
		s, err := fn.Construct(args)
		if err != nil {
			iv.raise(err.Error(), pos)
		}
		return s

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

// callFunction runs a user function, optionally with a bound receiver.
//
// A method passes its receiver as the implicit self parameter. Without a
// receiver the function behaves like a plain call.
func (iv *Interpreter) callFunction(fn *Function, args []object.Object, receiver object.Object, pos source.Pos) (val object.Object) {
	if len(args) != len(fn.Params) {
		iv.raise(fmt.Sprintf("function '%s' expects %d arguments, got %d", fn.Name, len(fn.Params), len(args)), pos)
	}
	callEnv := NewEnv(fn.Env)
	if receiver != nil {
		callEnv.Define("self", receiver, false)
	}
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
}

// currentFileName returns the name of the file being evaluated.
func (iv *Interpreter) currentFileName() string {
	if iv.file != nil {
		return iv.file.Name
	}
	return ""
}

// loadModule loads the module named by spec exactly once.
//
// The module body executes in a fresh environment. Its exported names are
// gathered into a module value that the importer can read.
func (iv *Interpreter) loadModule(spec, fromPath string) (*object.Module, error) {
	if iv.resolver == nil {
		return nil, fmt.Errorf("cannot find module '%s'", spec)
	}
	f, ok := iv.resolver(spec, fromPath)
	if !ok {
		return nil, fmt.Errorf("cannot find module '%s'", spec)
	}
	if m, ok := iv.moduleCache[f.Path]; ok {
		return m, nil
	}
	for _, p := range iv.moduleStack {
		if p == f.Path {
			return nil, fmt.Errorf("import cycle involving '%s'", module.ModuleName(f.Spec))
		}
	}

	iv.moduleStack = append(iv.moduleStack, f.Path)
	prevFile := iv.file
	prevExports := iv.moduleExports
	iv.file = f.Source
	exports := make(map[string]object.Object)
	iv.moduleExports = exports
	defer func() {
		iv.file = prevFile
		iv.moduleExports = prevExports
		iv.moduleStack = iv.moduleStack[:len(iv.moduleStack)-1]
	}()
	iv.evalStmts(f.Prog.Stmts, NewEnv(iv.globals))

	m := &object.Module{Name: module.ModuleName(f.Spec), Exports: exports}
	iv.moduleCache[f.Path] = m
	return m, nil
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
