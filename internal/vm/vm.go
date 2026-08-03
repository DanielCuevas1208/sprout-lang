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

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/object"
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

// SetModuleLoader installs the loader that import expressions use.
func (vm *VM) SetModuleLoader(l runtime.ModuleLoader) { vm.ctx.Modules = l }

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
	// file is the source file of the running program. Import expressions
	// resolve their module paths against it.
	file *source.File
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
	return vm
}

// Run executes the compiled program.
//
// It returns the final value of the entry function (usually nil) and a
// *RunError when the program stops with a runtime error.
func (vm *VM) Run(file *source.File, prog *code.Program) (val object.Object, rerr *RunError) {
	_, val, rerr = vm.executeMain(file, prog.Main)
	return val, rerr
}

// RunModule compiles and executes a module program, then snapshots its
// top-level exports into a Module value.
//
// The module runs in a fresh environment. Its top-level names are read back
// through the exports map that the compiler records on the program.
func (vm *VM) RunModule(file *source.File, prog *ast.Program) (mod *object.Module, val object.Object, rerr *RunError) {
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return nil, nil, &RunError{Message: err.Error(), File: file}
	}
	env, val, rerr := vm.executeMain(file, compiled.Main)
	if rerr != nil {
		return nil, nil, rerr
	}
	exports := make(map[string]object.Object, len(compiled.Exports))
	for name, slot := range compiled.Exports {
		exports[name] = env.slots[slot]
	}
	return &object.Module{Name: file.Name, Exports: exports}, val, nil
}

// executeMain sets up the entry frame for main and runs it to completion.
//
// It returns the entry environment so callers can inspect top-level state.
func (vm *VM) executeMain(file *source.File, main *code.Function) (env *Env, val object.Object, rerr *RunError) {
	prevFile := vm.file
	vm.file = file
	prevStack, prevFrames := vm.stack, vm.frames
	vm.stack = vm.stack[:0]
	vm.frames = vm.frames[:0]
	defer func() {
		vm.file = prevFile
		vm.stack = prevStack
		vm.frames = prevFrames
		if r := recover(); r != nil {
			if e, ok := r.(*vmErr); ok {
				env, val, rerr = nil, nil, &RunError{Message: e.msg, File: file, Pos: e.pos, Frames: e.frames}
				return
			}
			panic(r)
		}
	}()

	env = &Env{slots: make([]object.Object, main.NumSlots)}
	vm.frames = append(vm.frames, &frame{
		fn:   main,
		env:  env,
		ip:   0,
		base: 0,
	})
	val = vm.runFrames(0)
	return env, val, nil
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
		case code.OpImport:
			pos := fr.pos()
			idx := code.U16(fn.Code, fr.ip+1)
			fr.ip += 3
			path := fn.Consts[idx].(object.Str).Value
			if vm.ctx.Modules == nil {
				vm.failAtPos("import is not available in this session", pos)
			}
			m, err := vm.ctx.Modules.LoadModule(vm.file, path)
			if err != nil {
				vm.failAtPos(err.Error(), pos)
			}
			vm.push(m)
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
