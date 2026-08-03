// Package vm is the stack-based bytecode virtual machine for Sprout.
//
// The VM executes the bytecode produced by the compiler. It shares value
// semantics and the standard library with the tree-walking interpreter
// through the runtime package, so both engines behave the same way.
//
// The VM uses an operand stack and a stack of call frames. Environments are
// heap objects that mirror the compiler's scopes. Closures capture their
// defining environment, which gives Sprout the standard shared-environment
// semantics.
package vm

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
)

// Closure is a compiled function with its captured environment.
type Closure struct {
	fn  *code.Function
	env *Env
}

// Type reports the runtime type of a closure.
func (c *Closure) Type() object.Type { return object.TypeFunction }

// String renders a closure for display.
func (c *Closure) String() string {
	params := strings.Join(c.fn.ParamNames, ", ")
	if c.fn.Name != "" {
		return fmt.Sprintf("<fn %s (%s)>", c.fn.Name, params)
	}
	return fmt.Sprintf("<fn (%s)>", params)
}

// Env is a lexical environment. It mirrors one compiler scope.
type Env struct {
	parent *Env
	slots  []object.Object
}

// iterator holds a materialized sequence and its position.
//
// It is a stack value that only the VM sees. User code never observes it.
type iterator struct {
	items []object.Object
	index int
}

func (i *iterator) Type() object.Type { return object.TypeRange }
func (i *iterator) String() string    { return "<iterator>" }

// frame is one active function call.
type frame struct {
	cl      *Closure
	fn      *code.Function
	env     *Env
	ip      int
	base    int
	callPos source.Pos
}

// pos returns the source position of the current instruction.
func (f *frame) pos() source.Pos {
	if f.ip >= 0 && f.ip < len(f.fn.Positions) {
		return f.fn.Positions[f.ip]
	}
	return source.Pos{}
}

// RunError is a runtime error with its source position and call stack.
type RunError struct {
	Message string
	File    *source.File
	Pos     source.Pos
	Frames  []diag.Frame
}

func (e *RunError) Error() string { return e.Message }

// vmErr is the panic signal for runtime errors.
type vmErr struct {
	msg    string
	pos    source.Pos
	frames []diag.Frame
}

// VM executes compiled Sprout programs.
type VM struct {
	ctx    runtime.Context
	stack  []object.Object
	frames []*frame
	loader *module.Loader
}

// New returns a VM wired to the process standard streams.
func New() *VM {
	return NewWithIO(os.Stdin, os.Stdout, os.Stderr)
}

// NewWithIO returns a VM with explicit streams.
func NewWithIO(stdin io.Reader, stdout, stderr io.Writer) *VM {
	vm := &VM{}
	vm.ctx = runtime.Context{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Call: func(fn object.Object, args []object.Object, pos source.Pos) object.Object {
			return vm.callValue(fn, args, pos)
		},
	}
	vm.loader = module.NewLoader(module.OS{})
	return vm
}

// SetLoader replaces the module loader used by the VM.
//
// Tests use this to point the VM at an in-memory filesystem.
func (vm *VM) SetLoader(l *module.Loader) { vm.loader = l }

// Run executes the compiled program.
//
// It returns the final value of the entry function (usually nil) and a
// *RunError when the program stops with a runtime error.
func (vm *VM) Run(file *source.File, prog *code.Program) (val object.Object, rerr *RunError) {
	prevStack, prevFrames := vm.stack, vm.frames
	vm.stack = vm.stack[:0]
	vm.frames = vm.frames[:0]
	defer func() {
		vm.stack = prevStack
		vm.frames = prevFrames
		if r := recover(); r != nil {
			if e, ok := r.(*vmErr); ok {
				val, rerr = nil, &RunError{Message: e.msg, File: file, Pos: e.pos, Frames: e.frames}
				return
			}
			panic(r)
		}
	}()

	main := prog.Main
	env := &Env{slots: make([]object.Object, main.NumSlots)}
	vm.frames = append(vm.frames, &frame{
		fn:   main,
		env:  env,
		ip:   0,
		base: 0,
	})
	val = vm.runFrames(0)
	return val, nil
}

// failAtPos aborts execution with a runtime error.
func (vm *VM) failAtPos(msg string, pos source.Pos) {
	panic(&vmErr{msg: msg, pos: pos, frames: vm.stackTrace()})
}

// stackTrace builds the call stack, skipping the entry frame.
func (vm *VM) stackTrace() []diag.Frame {
	frames := make([]diag.Frame, 0, len(vm.frames)-1)
	for i := 1; i < len(vm.frames); i++ {
		fr := vm.frames[i]
		frames = append(frames, diag.Frame{Name: fr.fn.Name, FileName: fr.fn.FileName, Pos: fr.callPos})
	}
	return frames
}

func (vm *VM) push(v object.Object) { vm.stack = append(vm.stack, v) }

func (vm *VM) pop() object.Object {
	v := vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return v
}

// envAt returns the environment depth levels up from fr.env.
func (vm *VM) envAt(fr *frame, depth int) *Env {
	e := fr.env
	for i := 0; i < depth; i++ {
		e = e.parent
	}
	return e
}

// callValue invokes a closure or builtin with the given arguments.
func (vm *VM) callValue(callee object.Object, args []object.Object, pos source.Pos) object.Object {
	switch c := callee.(type) {
	case *Closure:
		if len(args) != len(c.fn.ParamNames) {
			vm.failAtPos(fmt.Sprintf("function '%s' expects %d arguments, got %d",
				c.fn.Name, len(c.fn.ParamNames), len(args)), pos)
		}
		env := &Env{parent: c.env, slots: make([]object.Object, c.fn.NumSlots)}
		for i, a := range args {
			env.slots[i] = a
		}
		before := len(vm.frames)
		vm.frames = append(vm.frames, &frame{
			cl:      c,
			fn:      c.fn,
			env:     env,
			ip:      0,
			base:    len(vm.stack),
			callPos: pos,
		})
		return vm.runFrames(before)

	case *runtime.Builtin:
		if err := c.CheckArgs(args, c.Name); err != nil {
			vm.failAtPos(err.Error(), pos)
		}
		v, err := c.Fn(&vm.ctx, args, pos)
		if err != nil {
			vm.failAtPos(err.Error(), pos)
		}
		return v
	}
	vm.failAtPos(fmt.Sprintf("cannot call a %s", callee.Type()), pos)
	return nil
}

// loadModule resolves, runs, and caches the module named by spec.
//
// The importer is the path of the file that holds the import statement.
// On failure it returns a vmErr ready to raise at the import site.
func (vm *VM) loadModule(importer, spec string, pos source.Pos) (m *object.Module, verr *vmErr) {
	path, ok := vm.loader.Resolve(importer, spec)
	if !ok {
		return nil, &vmErr{msg: fmt.Sprintf("cannot find module '%s'", spec), pos: pos}
	}
	if cached, ok := vm.loader.Cached(path); ok {
		return cached, nil
	}
	if !vm.loader.MarkLoading(path) {
		return nil, &vmErr{msg: fmt.Sprintf("circular import of module '%s'", spec), pos: pos}
	}
	defer vm.loader.DoneLoading(path)

	file, err := vm.loader.Source(path)
	if err != nil {
		return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': %v", spec, err), pos: pos}
	}
	prog, diags := parser.Parse(file)
	if msg := firstDiagMessage(diags); msg != "" {
		return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': %s (at %s)", spec, msg, path), pos: pos}
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		d := diags[0]
		return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': %s (at %s:%d:%d)",
			spec, d.Message, path, d.Pos.Line, d.Pos.Column), pos: pos}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': %s", spec, err.Error()), pos: pos}
	}

	env, _, merr := vm.runModule(file, compiled.Main)
	if merr != nil {
		return nil, &vmErr{
			msg:    fmt.Sprintf("cannot load module '%s': %s", spec, moduleAt(path, merr.pos, merr.msg)),
			pos:    pos,
			frames: merr.frames,
		}
	}

	name, ok := vm.loader.Name(path)
	if !ok {
		return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': invalid module name", spec), pos: pos}
	}
	mod := object.NewModule(name, path)
	for _, ename := range module.Exports(prog) {
		slot, ok := compiled.Main.Exports[ename]
		if !ok {
			return nil, &vmErr{msg: fmt.Sprintf("cannot load module '%s': internal error: missing export '%s'", spec, ename), pos: pos}
		}
		mod.Set(ename, env.slots[slot])
	}
	vm.loader.Cache(path, mod)
	return mod, nil
}

// runModule executes a module's entry function on the shared stack.
//
// The module runs like a nested call. The returned environment holds the
// module's top-level slots so the caller can collect its exports. On a
// runtime failure the returned error keeps only the frames that were pushed
// inside the module, matching the interpreter's module error traces.
func (vm *VM) runModule(file *source.File, main *code.Function) (env *Env, val object.Object, verr *vmErr) {
	env = &Env{slots: make([]object.Object, main.NumSlots)}
	before := len(vm.frames)
	vm.frames = append(vm.frames, &frame{fn: main, env: env, ip: 0, base: len(vm.stack)})
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*vmErr); ok {
				if len(e.frames) >= before {
					e.frames = e.frames[before:]
				}
				verr = e
				return
			}
			panic(r)
		}
	}()
	val = vm.runFrames(before)
	return env, val, nil
}

// firstDiagMessage returns the message of the first error diagnostic.
func firstDiagMessage(diags []diag.Diagnostic) string {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return d.Message
		}
	}
	return ""
}

// moduleAt renders a module error detail with its source position.
func moduleAt(path string, pos source.Pos, msg string) string {
	if pos.IsValid() {
		return fmt.Sprintf("%s (at %s:%d:%d)", msg, path, pos.Line, pos.Column)
	}
	return msg
}

// runFrames executes instructions until the frame stack reaches until.
//
// The entry call uses until 0, so execution ends when the entry frame
// returns. A nested call uses the frame count before its frame was pushed.
func (vm *VM) runFrames(until int) object.Object {
	var result object.Object
	for len(vm.frames) > until {
		fr := vm.frames[len(vm.frames)-1]
		fn := fr.fn
		if fr.ip >= len(fn.Code) {
			vm.failAtPos("internal error: ran past the end of a function", fr.pos())
		}
		op := code.Opcode(fn.Code[fr.ip])

		switch op {
		case code.OpPushConst:
			vm.push(fn.Consts[code.U16(fn.Code, fr.ip+1)])
			fr.ip += 3
		case code.OpNil:
			vm.push(object.NilValue)
			fr.ip++
		case code.OpTrue:
			vm.push(object.Bool{Value: true})
			fr.ip++
		case code.OpFalse:
			vm.push(object.Bool{Value: false})
			fr.ip++
		case code.OpPop:
			vm.pop()
			fr.ip++
		case code.OpDup:
			vm.push(vm.stack[len(vm.stack)-1])
			fr.ip++
		case code.OpNewEnv:
			n := int(code.U16(fn.Code, fr.ip+1))
			fr.env = &Env{parent: fr.env, slots: make([]object.Object, n)}
			fr.ip += 3
		case code.OpEndEnv:
			fr.env = fr.env.parent
			fr.ip++
		case code.OpGetLocal:
			slot := int(code.U16(fn.Code, fr.ip+1))
			vm.push(fr.env.slots[slot])
			fr.ip += 3
		case code.OpSetLocal:
			slot := int(code.U16(fn.Code, fr.ip+1))
			fr.env.slots[slot] = vm.pop()
			fr.ip += 3
		case code.OpGetUp:
			depth := int(code.U16(fn.Code, fr.ip+1))
			slot := int(code.U16(fn.Code, fr.ip+3))
			vm.push(vm.envAt(fr, depth).slots[slot])
			fr.ip += 5
		case code.OpSetUp:
			depth := int(code.U16(fn.Code, fr.ip+1))
			slot := int(code.U16(fn.Code, fr.ip+3))
			vm.envAt(fr, depth).slots[slot] = vm.pop()
			fr.ip += 5
		case code.OpBuiltin:
			vm.push(runtime.Builtins[code.U16(fn.Code, fr.ip+1)])
			fr.ip += 3
		case code.OpClosure:
			idx := code.U16(fn.Code, fr.ip+1)
			f := fn.Consts[idx].(*code.Function)
			vm.push(&Closure{fn: f, env: fr.env})
			fr.ip += 3
		case code.OpCall:
			n := int(fn.Code[fr.ip+1])
			pos := fr.pos()
			fr.ip += 2
			args := append([]object.Object(nil), vm.stack[len(vm.stack)-n:]...)
			callee := vm.stack[len(vm.stack)-n-1]
			vm.stack = vm.stack[:len(vm.stack)-n-1]
			vm.push(vm.callValue(callee, args, pos))
		case code.OpReturn, code.OpReturnValue:
			if op == code.OpReturnValue {
				result = vm.pop()
			} else {
				result = object.NilValue
			}
			vm.stack = vm.stack[:fr.base]
			vm.frames = vm.frames[:len(vm.frames)-1]
			if len(vm.frames) == until {
				return result
			}
			vm.push(result)
		case code.OpJump:
			fr.ip = int(code.U16(fn.Code, fr.ip+1))
		case code.OpJumpIfFalse:
			target := int(code.U16(fn.Code, fr.ip+1))
			fr.ip += 3
			if !runtime.Truthy(vm.pop()) {
				fr.ip = target
			}
		case code.OpJumpIfTrue:
			target := int(code.U16(fn.Code, fr.ip+1))
			fr.ip += 3
			if runtime.Truthy(vm.pop()) {
				fr.ip = target
			}
		case code.OpGetIndex:
			pos := fr.pos()
			fr.ip++
			idx := vm.pop()
			container := vm.pop()
			v, err := runtime.IndexGet(container, idx)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(v)
		case code.OpSetIndex:
			pos := fr.pos()
			fr.ip++
			value := vm.pop()
			idx := vm.pop()
			container := vm.pop()
			v, err := runtime.SetIndex(container, idx, value)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(v)
		case code.OpGetMember:
			pos := fr.pos()
			idx := code.U16(fn.Code, fr.ip+1)
			fr.ip += 3
			name := fn.Consts[idx].(object.Str).Value
			container := vm.pop()
			v, err := runtime.MemberGet(container, name)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(v)
		case code.OpImport:
			pos := fr.pos()
			idx := code.U16(fn.Code, fr.ip+1)
			fr.ip += 3
			spec := fn.Consts[idx].(object.Str).Value
			m, lerr := vm.loadModule(fn.FileName, spec, pos)
			if lerr != nil {
				panic(lerr)
			}
			vm.push(m)
		case code.OpBuildList:
			n := int(code.U16(fn.Code, fr.ip+1))
			elems := make([]object.Object, n)
			for i := n - 1; i >= 0; i-- {
				elems[i] = vm.pop()
			}
			vm.push(&object.List{Elems: elems})
			fr.ip += 3
		case code.OpBuildMap:
			n := int(code.U16(fn.Code, fr.ip+1))
			pos := fr.pos()
			// The pairs sit on the stack in source order. Pop them into
			// reversed slices so insertion order matches the source.
			keys := make([]string, n)
			vals := make([]object.Object, n)
			for i := n - 1; i >= 0; i-- {
				value := vm.pop()
				key := vm.pop()
				ks, ok := key.(object.Str)
				if !ok {
					vm.failAtPos("map keys must be strings", pos)
				}
				keys[i] = ks.Value
				vals[i] = value
			}
			m := &object.Map{Vals: make(map[string]object.Object)}
			for i := 0; i < n; i++ {
				m.Set(keys[i], vals[i])
			}
			vm.push(m)
			fr.ip += 3
		case code.OpMakeIter:
			pos := fr.pos()
			fr.ip++
			iterable := vm.pop()
			items, err := runtime.Sequence(iterable)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(&iterator{items: items})
		case code.OpIterNext:
			target := int(code.U16(fn.Code, fr.ip+1))
			iter := vm.stack[len(vm.stack)-1].(*iterator)
			if iter.index >= len(iter.items) {
				vm.stack = vm.stack[:len(vm.stack)-1]
				fr.ip = target
			} else {
				vm.push(iter.items[iter.index])
				iter.index++
				fr.ip += 3
			}
		case code.OpNeg:
			pos := fr.pos()
			fr.ip++
			v := vm.pop()
			switch x := v.(type) {
			case object.Int:
				vm.push(object.Int{Value: -x.Value})
			case object.Float:
				vm.push(object.Float{Value: -x.Value})
			default:
				vm.failAtPos(fmt.Sprintf("cannot negate a %s", v.Type()), pos)
			}
		case code.OpNot:
			fr.ip++
			vm.push(object.Bool{Value: !runtime.Truthy(vm.pop())})
		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv, code.OpMod, code.OpPow:
			pos := fr.pos()
			fr.ip++
			b := vm.pop()
			a := vm.pop()
			v, err := arithmetic(op, a, b)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(v)
		case code.OpEq, code.OpNeq:
			fr.ip++
			b := vm.pop()
			a := vm.pop()
			eq := runtime.Equal(a, b)
			if op == code.OpNeq {
				eq = !eq
			}
			vm.push(object.Bool{Value: eq})
		case code.OpLt, code.OpLe, code.OpGt, code.OpGe:
			pos := fr.pos()
			fr.ip++
			b := vm.pop()
			a := vm.pop()
			cmp, ok := runtime.Compare(a, b)
			if !ok {
				vm.failAtPos(fmt.Sprintf("cannot compare %s with %s", a.Type(), b.Type()), pos)
			}
			vm.push(object.Bool{Value: compareResult(cmp, op)})
		default:
			vm.failAtPos(fmt.Sprintf("internal error: unknown opcode %d", byte(op)), fr.pos())
		}
	}
	return result
}

// arithmetic runs a runtime arithmetic operation.
func arithmetic(op code.Opcode, a, b object.Object) (object.Object, error) {
	switch op {
	case code.OpAdd:
		return runtime.Add(a, b)
	case code.OpSub:
		return runtime.Sub(a, b)
	case code.OpMul:
		return runtime.Mul(a, b)
	case code.OpDiv:
		return runtime.Div(a, b)
	case code.OpMod:
		return runtime.Mod(a, b)
	case code.OpPow:
		return runtime.Pow(a, b)
	}
	return nil, fmt.Errorf("unknown arithmetic opcode")
}

// compareResult turns a comparison result into a boolean.
func compareResult(cmp int, op code.Opcode) bool {
	switch op {
	case code.OpLt:
		return cmp < 0
	case code.OpLe:
		return cmp <= 0
	case code.OpGt:
		return cmp > 0
	case code.OpGe:
		return cmp >= 0
	}
	return false
}
