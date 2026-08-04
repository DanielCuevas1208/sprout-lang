// Package code defines the bytecode of the Sprout virtual machine.
//
// A compiled function is a stream of instructions. Each instruction starts
// with a one-byte opcode. Operands follow. All integer operands use
// big-endian encoding so the format is stable across platforms.
//
// The bytecode is a stack machine. Every instruction operates on a shared
// operand stack. Loops and conditionals use absolute jump targets.
package code

import (
	"fmt"
	"strings"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
)

// Opcode identifies one instruction.
type Opcode byte

// The instruction set of the Sprout virtual machine.
const (
	OpPushConst Opcode = iota // uint16 index into Consts
	OpNil
	OpTrue
	OpFalse
	OpPop         // pop and discard
	OpDup         // duplicate the top of the stack
	OpNewEnv      // uint16 slots: enter a block scope with n empty slots
	OpEndEnv      // leave the current block scope
	OpGetLocal    // uint16 slot: push slots[slot]
	OpSetLocal    // uint16 slot: store the top into slots[slot]
	OpGetUp       // uint16 depth, uint16 slot: push from an enclosing env
	OpSetUp       // uint16 depth, uint16 slot: store into an enclosing env
	OpBuiltin     // uint16 index into the standard library
	OpClosure     // uint16 index into Consts: capture the current env
	OpCall        // byte count: call the top value with count arguments
	OpReturn      // return nil
	OpReturnValue // return the top of the stack
	OpJump        // uint16 absolute target
	OpJumpIfFalse // uint16 target: pop, jump when falsy
	OpJumpIfTrue  // uint16 target: pop, jump when truthy
	OpGetIndex    // pop index and container, push element
	OpSetIndex    // pop value, index, and container, push value
	OpBuildList   // uint16 count: collect count values into a list
	OpBuildMap    // uint16 count: collect count key/value pairs into a map
	OpMakeIter    // pop an iterable, push an iterator
	OpIterNext    // uint16 target: advance; jump when the iteration ends
	OpNeg
	OpNot
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
	OpEq
	OpNeq
	OpLt
	OpLe
	OpGt
	OpGe
	OpImport     // uint16 index into Program.Modules
	OpMakeModule // uint16 index of the module name constant
)

// opNames maps an opcode to its disassembly name.
var opNames = map[Opcode]string{
	OpPushConst:   "PUSH_CONST",
	OpNil:         "PUSH_NIL",
	OpTrue:        "PUSH_TRUE",
	OpFalse:       "PUSH_FALSE",
	OpPop:         "POP",
	OpDup:         "DUP",
	OpNewEnv:      "NEW_ENV",
	OpEndEnv:      "END_ENV",
	OpGetLocal:    "GET_LOCAL",
	OpSetLocal:    "SET_LOCAL",
	OpGetUp:       "GET_UP",
	OpSetUp:       "SET_UP",
	OpBuiltin:     "BUILTIN",
	OpClosure:     "CLOSURE",
	OpCall:        "CALL",
	OpReturn:      "RETURN",
	OpReturnValue: "RETURN_VALUE",
	OpJump:        "JUMP",
	OpJumpIfFalse: "JUMP_IF_FALSE",
	OpJumpIfTrue:  "JUMP_IF_TRUE",
	OpGetIndex:    "GET_INDEX",
	OpSetIndex:    "SET_INDEX",
	OpBuildList:   "BUILD_LIST",
	OpBuildMap:    "BUILD_MAP",
	OpMakeIter:    "MAKE_ITER",
	OpIterNext:    "ITER_NEXT",
	OpNeg:         "NEG",
	OpNot:         "NOT",
	OpAdd:         "ADD",
	OpSub:         "SUB",
	OpMul:         "MUL",
	OpDiv:         "DIV",
	OpMod:         "MOD",
	OpPow:         "POW",
	OpEq:          "EQ",
	OpNeq:         "NEQ",
	OpLt:          "LT",
	OpLe:          "LE",
	OpGt:          "GT",
	OpGe:          "GE",
	OpImport:      "IMPORT",
	OpMakeModule:  "MAKE_MODULE",
}

// String returns the disassembly name of an opcode.
func (o Opcode) String() string {
	if name, ok := opNames[o]; ok {
		return name
	}
	return fmt.Sprintf("OPCODE(%d)", byte(o))
}

// Function is one compiled Sprout function.
type Function struct {
	Name       string
	FileName   string
	ParamNames []string
	// NumSlots is the number of slots in the call environment.
	// It covers parameters and function-body locals.
	NumSlots int
	Code     []byte
	Consts   []object.Object
	// Positions tracks the source position of each instruction.
	// Positions[offset] is valid only where an instruction starts.
	Positions []source.Pos
}

// Type reports the runtime type of a compiled function.
//
// A compiled function is a value in the constant pool. The VM wraps it in a
// closure before a call. Type and String make the pool uniform.
func (f *Function) Type() object.Type { return object.TypeFunction }

// String renders a compiled function for display.
func (f *Function) String() string {
	if f.Name == "" {
		return "<fn>"
	}
	return "<fn " + f.Name + ">"
}

// ModuleRef names a compiled module and its entry function.
type ModuleRef struct {
	// Name is the module name used for caching and diagnostics.
	Name string
	// Fn runs the module body and returns an object.Module.
	Fn *Function
}

// Program is a compiled Sprout program. Main is the entry function.
type Program struct {
	Main *Function
	// Modules holds every module the program imports, in compile order.
	Modules []*ModuleRef
}

// Builder assembles one function's bytecode.
type Builder struct {
	fn *Function
}

// NewBuilder returns a builder for a named function.
func NewBuilder(name, fileName string, paramNames []string) *Builder {
	return &Builder{fn: &Function{Name: name, FileName: fileName, ParamNames: paramNames}}
}

// Add emits an opcode with no operands and returns its offset.
func (b *Builder) Add(op Opcode, pos source.Pos) int {
	off := len(b.fn.Code)
	b.fn.Code = append(b.fn.Code, byte(op))
	b.track(off, pos)
	return off
}

// AddByte emits an opcode followed by a one-byte operand.
func (b *Builder) AddByte(op Opcode, v byte, pos source.Pos) int {
	off := len(b.fn.Code)
	b.fn.Code = append(b.fn.Code, byte(op), v)
	b.track(off, pos)
	return off
}

// AddU16 emits an opcode followed by a two-byte operand.
func (b *Builder) AddU16(op Opcode, v uint16, pos source.Pos) int {
	off := len(b.fn.Code)
	b.fn.Code = append(b.fn.Code, byte(op), byte(v>>8), byte(v))
	b.track(off, pos)
	return off
}

// AddU16Pair emits an opcode followed by two two-byte operands.
func (b *Builder) AddU16Pair(op Opcode, a, b16 uint16, pos source.Pos) int {
	off := len(b.fn.Code)
	b.fn.Code = append(b.fn.Code, byte(op),
		byte(a>>8), byte(a), byte(b16>>8), byte(b16))
	b.track(off, pos)
	return off
}

// PatchU16 overwrites the two-byte operand that starts at off.
func (b *Builder) PatchU16(off int, v uint16) {
	b.fn.Code[off+1] = byte(v >> 8)
	b.fn.Code[off+2] = byte(v)
}

// Len returns the length of the instruction stream so far.
func (b *Builder) Len() int { return len(b.fn.Code) }

// Const adds v to the constant pool and returns its index.
func (b *Builder) Const(v object.Object) uint16 {
	idx := uint16(len(b.fn.Consts))
	b.fn.Consts = append(b.fn.Consts, v)
	return idx
}

// SetNumSlots records how many slots the call environment needs.
func (b *Builder) SetNumSlots(n int) { b.fn.NumSlots = n }

// track records a position for the instruction that starts at off.
func (b *Builder) track(off int, pos source.Pos) {
	if off >= len(b.fn.Positions) {
		b.fn.Positions = append(b.fn.Positions, make([]source.Pos, off-len(b.fn.Positions)+1)...)
	}
	b.fn.Positions[off] = pos
}

// Finish finalizes the function and returns it.
func (b *Builder) Finish() *Function { return b.fn }

// U16 reads a big-endian two-byte operand at off.
func U16(b []byte, off int) uint16 {
	return uint16(b[off])<<8 | uint16(b[off+1])
}

// disassembler walks an instruction stream and renders each instruction.
type disassembler struct {
	fn  *Function
	prg *Program // optional; names OpImport operands
	b   strings.Builder
}

// Disassemble renders fn as a human-readable instruction listing.
func Disassemble(fn *Function) string {
	return DisassembleProgram(fn, nil)
}

// DisassembleProgram renders fn with program context for module operands.
func DisassembleProgram(fn *Function, prg *Program) string {
	d := &disassembler{fn: fn, prg: prg}
	ip := 0
	for ip < len(fn.Code) {
		next, ok := d.instruction(ip)
		if !ok {
			break
		}
		ip = next
	}
	return d.b.String()
}

// instruction decodes one instruction starting at ip. It returns the offset
// of the next instruction, or ok=false when the stream is malformed.
func (d *disassembler) instruction(ip int) (int, bool) {
	if ip >= len(d.fn.Code) {
		return ip, true
	}
	op := Opcode(d.fn.Code[ip])
	fmt.Fprintf(&d.b, "%04d  %s", ip, op)
	if ip < len(d.fn.Positions) {
		pos := d.fn.Positions[ip]
		if pos.IsValid() {
			fmt.Fprintf(&d.b, "  ; %s:%d:%d", d.fn.FileName, pos.Line, pos.Column)
		}
	}
	defer func() { d.b.WriteString("\n") }()
	switch op {
	case OpPushConst:
		idx := U16(d.fn.Code, ip+1)
		fmt.Fprintf(&d.b, "  %d  (%s)", idx, d.fn.Consts[idx])
		return ip + 3, true
	case OpNewEnv, OpGetLocal, OpSetLocal, OpBuildList, OpBuildMap:
		fmt.Fprintf(&d.b, "  %d", U16(d.fn.Code, ip+1))
		return ip + 3, true
	case OpGetUp, OpSetUp:
		fmt.Fprintf(&d.b, "  depth %d slot %d", U16(d.fn.Code, ip+1), U16(d.fn.Code, ip+3))
		return ip + 5, true
	case OpBuiltin:
		idx := U16(d.fn.Code, ip+1)
		name := "<unknown>"
		if int(idx) < len(runtime.Builtins) {
			name = runtime.Builtins[idx].Name
		}
		fmt.Fprintf(&d.b, "  %d  (%s)", idx, name)
		return ip + 3, true
	case OpClosure:
		idx := U16(d.fn.Code, ip+1)
		if c, ok := d.fn.Consts[idx].(*Function); ok {
			fmt.Fprintf(&d.b, "  %d  (%s)", idx, c.Name)
		} else {
			fmt.Fprintf(&d.b, "  %d", idx)
		}
		return ip + 3, true
	case OpCall:
		fmt.Fprintf(&d.b, "  %d", d.fn.Code[ip+1])
		return ip + 2, true
	case OpJump, OpJumpIfFalse, OpJumpIfTrue, OpIterNext:
		fmt.Fprintf(&d.b, "  -> %04d", U16(d.fn.Code, ip+1))
		return ip + 3, true
	case OpImport:
		idx := U16(d.fn.Code, ip+1)
		name := "<unknown>"
		if d.prg != nil && int(idx) < len(d.prg.Modules) {
			name = d.prg.Modules[idx].Name
		}
		fmt.Fprintf(&d.b, "  %d  (%s)", idx, name)
		return ip + 3, true
	case OpMakeModule:
		idx := U16(d.fn.Code, ip+1)
		name := "<unknown>"
		if int(idx) < len(d.fn.Consts) {
			if s, ok := d.fn.Consts[idx].(object.Str); ok {
				name = s.Value
			}
		}
		fmt.Fprintf(&d.b, "  %d  (%s)", idx, name)
		return ip + 3, true
	default:
		return ip + 1, true
	}
}
