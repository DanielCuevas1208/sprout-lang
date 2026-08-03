// Package interp is the tree-walking interpreter for Sprout.
package interp

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/object"
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
	input   *bufio.Reader
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
		if truthy(iv.evalExpr(n.Cond, env)) {
			return iv.evalBlock(n.Then, env)
		}
		for _, b := range n.Elifs {
			if truthy(iv.evalExpr(b.Cond, env)) {
				return iv.evalBlock(b.Body, env)
			}
		}
		if n.Else != nil {
			return iv.evalBlock(n.Else, env)
		}
		return object.NilValue

	case *ast.WhileStmt:
		for {
			if !truthy(iv.evalExpr(n.Cond, env)) {
				return object.NilValue
			}
			if iv.runLoopBody(n.Body, env) {
				return object.NilValue
			}
		}

	case *ast.ForInStmt:
		iterable := iv.evalExpr(n.Iterable, env)
		items, err := iv.toSeq(iterable)
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

// toSeq materializes an iterable value as a slice.
func (iv *Interpreter) toSeq(o object.Object) ([]object.Object, error) {
	switch v := o.(type) {
	case *object.List:
		return v.Elems, nil
	case object.Str:
		runes := []rune(v.Value)
		out := make([]object.Object, len(runes))
		for i, r := range runes {
			out[i] = object.Str{Value: string(r)}
		}
		return out, nil
	case object.Range:
		var out []object.Object
		step := v.Step
		if step == 0 {
			step = 1
		}
		for i := v.Start; i < v.End; i += step {
			out = append(out, object.Int{Value: i})
		}
		return out, nil
	}
	return nil, fmtErr("cannot iterate a %s", o.Type())
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
		return object.Bool{Value: !truthy(iv.evalExpr(n.X, env))}
	}
	iv.raise("unsupported unary operator", n.OpPos)
	return nil
}

func (iv *Interpreter) evalBinary(n *ast.BinaryExpr, env *Env) object.Object {
	left := iv.evalExpr(n.Left, env)

	switch n.Op {
	case token.AND:
		if !truthy(left) {
			return left
		}
		return iv.evalExpr(n.Right, env)
	case token.OR:
		if truthy(left) {
			return left
		}
		return iv.evalExpr(n.Right, env)
	}

	right := iv.evalExpr(n.Right, env)

	switch n.Op {
	case token.PLUS:
		return iv.add(left, right, n.OpPos)
	case token.MINUS:
		return iv.sub(left, right, n.OpPos)
	case token.STAR:
		return iv.mul(left, right, n.OpPos)
	case token.SLASH:
		return iv.div(left, right, n.OpPos)
	case token.PERCENT:
		return iv.mod(left, right, n.OpPos)
	case token.CARET:
		return iv.pow(left, right, n.OpPos)
	case token.EQ:
		return object.Bool{Value: objectEqual(left, right)}
	case token.NEQ:
		return object.Bool{Value: !objectEqual(left, right)}
	case token.LT, token.LE, token.GT, token.GE:
		return iv.compare(left, right, n.Op, n.OpPos)
	}
	iv.raise("unsupported binary operator", n.OpPos)
	return nil
}

func (iv *Interpreter) add(a, b object.Object, pos source.Pos) object.Object {
	switch x := a.(type) {
	case object.Int:
		switch y := b.(type) {
		case object.Int:
			return object.Int{Value: x.Value + y.Value}
		case object.Float:
			return object.Float{Value: float64(x.Value) + y.Value}
		}
	case object.Float:
		switch y := b.(type) {
		case object.Int:
			return object.Float{Value: x.Value + float64(y.Value)}
		case object.Float:
			return object.Float{Value: x.Value + y.Value}
		}
	case object.Str:
		if y, ok := b.(object.Str); ok {
			return object.Str{Value: x.Value + y.Value}
		}
	}
	iv.raise(fmt.Sprintf("cannot add %s and %s", a.Type(), b.Type()), pos)
	return nil
}

func (iv *Interpreter) sub(a, b object.Object, pos source.Pos) object.Object {
	if an, af, aNum := asNumber(a); aNum {
		if bn, bf, bNum := asNumber(b); bNum {
			if _, aF := a.(object.Float); aF {
				return object.Float{Value: af - bf}
			}
			if _, bF := b.(object.Float); bF {
				return object.Float{Value: af - bf}
			}
			return object.Int{Value: an - bn}
		}
	}
	iv.raise(fmt.Sprintf("cannot subtract %s from %s", b.Type(), a.Type()), pos)
	return nil
}

func (iv *Interpreter) mul(a, b object.Object, pos source.Pos) object.Object {
	if an, af, aNum := asNumber(a); aNum {
		if bn, bf, bNum := asNumber(b); bNum {
			if _, aF := a.(object.Float); aF {
				return object.Float{Value: af * bf}
			}
			if _, bF := b.(object.Float); bF {
				return object.Float{Value: af * bf}
			}
			return object.Int{Value: an * bn}
		}
	}
	iv.raise(fmt.Sprintf("cannot multiply %s and %s", a.Type(), b.Type()), pos)
	return nil
}

func (iv *Interpreter) div(a, b object.Object, pos source.Pos) object.Object {
	if _, _, aNum := asNumber(a); !aNum {
		iv.raise(fmt.Sprintf("cannot divide a %s", a.Type()), pos)
	}
	bn, bf, bNum := asNumber(b)
	if !bNum {
		iv.raise(fmt.Sprintf("cannot divide by a %s", b.Type()), pos)
	}
	if (bn == 0 && !isFloat(b)) || (bNum && isFloat(b) && bf == 0) {
		iv.raise("cannot divide by zero", pos)
	}
	if _, aF := a.(object.Float); aF {
		af := asFloat(a)
		return object.Float{Value: af / bf}
	}
	if _, bF := b.(object.Float); bF {
		return object.Float{Value: asFloat(a) / bf}
	}
	return object.Int{Value: asInt(a) / bn}
}

func (iv *Interpreter) mod(a, b object.Object, pos source.Pos) object.Object {
	if _, _, aNum := asNumber(a); !aNum {
		iv.raise(fmt.Sprintf("cannot take the remainder of a %s", a.Type()), pos)
	}
	if _, _, bNum := asNumber(b); !bNum {
		iv.raise(fmt.Sprintf("cannot take the remainder by a %s", b.Type()), pos)
	}
	if _, aF := a.(object.Float); aF {
		bf := asFloat(b)
		if bf == 0 {
			iv.raise("cannot take the remainder by zero", pos)
		}
		return object.Float{Value: floatMod(asFloat(a), bf)}
	}
	if _, bF := b.(object.Float); bF {
		if bf := asFloat(b); bf == 0 {
			iv.raise("cannot take the remainder by zero", pos)
		}
		return object.Float{Value: floatMod(asFloat(a), asFloat(b))}
	}
	bn := asInt(b)
	if bn == 0 {
		iv.raise("cannot take the remainder by zero", pos)
	}
	return object.Int{Value: asInt(a) % bn}
}

func (iv *Interpreter) pow(a, b object.Object, pos source.Pos) object.Object {
	if _, _, aNum := asNumber(a); !aNum {
		iv.raise(fmt.Sprintf("cannot raise a %s to a power", a.Type()), pos)
	}
	if _, _, bNum := asNumber(b); !bNum {
		iv.raise(fmt.Sprintf("cannot raise to a %s power", b.Type()), pos)
	}
	if _, aF := a.(object.Float); aF {
		return object.Float{Value: floatPow(asFloat(a), asFloat(b))}
	}
	if _, bF := b.(object.Float); bF {
		return object.Float{Value: floatPow(asFloat(a), asFloat(b))}
	}
	exponent := asInt(b)
	if exponent < 0 {
		return object.Float{Value: floatPow(asFloat(a), asFloat(b))}
	}
	return object.Int{Value: intPow(asInt(a), exponent)}
}

func (iv *Interpreter) compare(a, b object.Object, op token.Kind, pos source.Pos) object.Object {
	cmp, ok := compareValues(a, b)
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

// compareValues compares a and b, reporting whether they are comparable.
func compareValues(a, b object.Object) (int, bool) {
	if _, af, aNum := asNumber(a); aNum {
		if _, bf, bNum := asNumber(b); bNum {
			switch {
			case isFloat(a), isFloat(b):
				return compareFloats(af, bf), true
			default:
				return compareInts(asInt(a), asInt(b)), true
			}
		}
	}
	if as, aStr := a.(object.Str); aStr {
		if bs, bStr := b.(object.Str); bStr {
			return strings.Compare(as.Value, bs.Value), true
		}
	}
	return 0, false
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
		switch c := container.(type) {
		case *object.List:
			i, ok := asIntIndex(idx)
			if !ok {
				iv.raise("list index must be an integer", n.OpPos)
			}
			if i < 0 || i >= int64(len(c.Elems)) {
				iv.raise(fmt.Sprintf("list index %d out of range (length %d)", i, len(c.Elems)), n.OpPos)
			}
			c.Elems[i] = value
			return value
		case *object.Map:
			ks, ok := idx.(object.Str)
			if !ok {
				iv.raise("map key must be a string", n.OpPos)
			}
			c.Set(ks.Value, value)
			return value
		default:
			iv.raise(fmt.Sprintf("cannot assign to an index of a %s", container.Type()), n.OpPos)
		}
	}
	iv.raise("invalid assignment target", n.OpPos)
	return nil
}

func (iv *Interpreter) indexGet(container, idx object.Object, pos source.Pos) object.Object {
	switch c := container.(type) {
	case *object.List:
		i, ok := asIntIndex(idx)
		if !ok {
			iv.raise("list index must be an integer", pos)
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			iv.raise(fmt.Sprintf("list index %d out of range (length %d)", i, len(c.Elems)), pos)
		}
		return c.Elems[i]
	case object.Str:
		i, ok := asIntIndex(idx)
		if !ok {
			iv.raise("string index must be an integer", pos)
		}
		runes := []rune(c.Value)
		if i < 0 || i >= int64(len(runes)) {
			iv.raise(fmt.Sprintf("string index %d out of range (length %d)", i, len(runes)), pos)
		}
		return object.Str{Value: string(runes[i])}
	case *object.Map:
		ks, ok := idx.(object.Str)
		if !ok {
			iv.raise("map key must be a string", pos)
		}
		if v, exists := c.Get(ks.Value); exists {
			return v
		}
		return object.NilValue
	}
	iv.raise(fmt.Sprintf("cannot index a %s", container.Type()), pos)
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
		if len(args) < fn.MinArgs || (fn.MaxArgs >= 0 && len(args) > fn.MaxArgs) {
			iv.raise(fmt.Sprintf("function '%s' expects %d arguments, got %d", fn.Name, fn.MinArgs, len(args)), pos)
		}
		v, err := fn.Fn(args, pos)
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

// truthy reports whether o counts as true in a condition.
//
// Only nil and false are falsy.
func truthy(o object.Object) bool {
	switch v := o.(type) {
	case object.Nil:
		return false
	case object.Bool:
		return v.Value
	}
	return true
}

func objectEqual(a, b object.Object) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type() != b.Type() {
		an, af, aNum := asNumber(a)
		bn, bf, bNum := asNumber(b)
		if aNum && bNum {
			if isFloat(a) || isFloat(b) {
				return af == bf
			}
			return an == bn
		}
		return false
	}
	switch x := a.(type) {
	case object.Int:
		return x.Value == b.(object.Int).Value
	case object.Float:
		return x.Value == b.(object.Float).Value
	case object.Str:
		return x.Value == b.(object.Str).Value
	case object.Bool:
		return x.Value == b.(object.Bool).Value
	case object.Nil:
		return true
	case object.Range:
		y := b.(object.Range)
		return x == y
	case *object.List:
		y := b.(*object.List)
		if len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !objectEqual(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *object.Map:
		y := b.(*object.Map)
		if len(x.Keys) != len(y.Keys) {
			return false
		}
		for i, k := range x.Keys {
			if k != y.Keys[i] {
				return false
			}
			if !objectEqual(x.Vals[k], y.Vals[k]) {
				return false
			}
		}
		return true
	case *Function:
		return a == b
	case *Builtin:
		return a == b
	}
	return false
}

func asNumber(o object.Object) (int64, float64, bool) {
	switch v := o.(type) {
	case object.Int:
		return v.Value, float64(v.Value), true
	case object.Float:
		return 0, v.Value, true
	}
	return 0, 0, false
}

func asInt(o object.Object) int64 {
	if v, ok := o.(object.Int); ok {
		return v.Value
	}
	return 0
}

func asFloat(o object.Object) float64 {
	if v, ok := o.(object.Float); ok {
		return v.Value
	}
	if v, ok := o.(object.Int); ok {
		return float64(v.Value)
	}
	return 0
}

func isFloat(o object.Object) bool {
	_, ok := o.(object.Float)
	return ok
}

func asIntIndex(o object.Object) (int64, bool) {
	if v, ok := o.(object.Int); ok {
		return v.Value, true
	}
	return 0, false
}

func compareInts(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareFloats(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func floatMod(a, b float64) float64 {
	return math.Mod(a, b)
}

func floatPow(a, b float64) float64 {
	return math.Pow(a, b)
}

// intPow raises base to a non-negative integer exponent.
func intPow(base, exponent int64) int64 {
	result := int64(1)
	for exponent > 0 {
		if exponent&1 == 1 {
			result *= base
		}
		exponent >>= 1
		if exponent > 0 {
			base *= base
		}
	}
	return result
}
