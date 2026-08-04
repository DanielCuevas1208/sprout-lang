package code

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

func pos() source.Pos { return source.Pos{Line: 1, Column: 1, Offset: 0} }

func TestOpcodeNames(t *testing.T) {
	names := map[Opcode]string{
		OpPushConst:   "PUSH_CONST",
		OpReturnValue: "RETURN_VALUE",
		OpJumpIfFalse: "JUMP_IF_FALSE",
		OpGe:          "GE",
	}
	for op, want := range names {
		if op.String() != want {
			t.Errorf("opcode %d: got %q, want %q", byte(op), op.String(), want)
		}
	}
	if Opcode(255).String() != "OPCODE(255)" {
		t.Errorf("unknown opcode: got %q", Opcode(255).String())
	}
}

func TestBuilderEmitsBytes(t *testing.T) {
	b := NewBuilder("main", "test.spr", []string{"a", "b"})
	b.Add(OpNil, pos())
	b.AddByte(OpCall, 2, pos())
	b.AddU16(OpPushConst, 0x1234, pos())
	b.AddU16Pair(OpGetUp, 0x0102, 0x0304, pos())

	got := b.Finish()
	want := []byte{
		byte(OpNil),
		byte(OpCall), 2,
		byte(OpPushConst), 0x12, 0x34,
		byte(OpGetUp), 0x01, 0x02, 0x03, 0x04,
	}
	if len(got.Code) != len(want) {
		t.Fatalf("code length: got %d, want %d", len(got.Code), len(want))
	}
	for i := range want {
		if got.Code[i] != want[i] {
			t.Errorf("byte %d: got %#x, want %#x", i, got.Code[i], want[i])
		}
	}
	if got.Name != "main" || got.FileName != "test.spr" {
		t.Errorf("metadata: %+v", got)
	}
	if len(got.ParamNames) != 2 || got.ParamNames[1] != "b" {
		t.Errorf("params: %v", got.ParamNames)
	}
}

func TestPatchU16(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	off := b.AddU16(OpJump, 0, pos())
	b.Add(OpNil, pos())
	b.PatchU16(off, 42)
	got := b.Finish().Code
	if U16(got, off+1) != 42 {
		t.Errorf("patched operand: got %d, want 42", U16(got, off+1))
	}
}

func TestBuilderConst(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	i := b.Const(object.Int{Value: 7})
	s := b.Const(object.Str{Value: "hi"})
	fn := b.Finish()
	if i != 0 || s != 1 {
		t.Errorf("const indexes: %d %d", i, s)
	}
	if fn.Consts[0].(object.Int).Value != 7 {
		t.Errorf("const 0: %v", fn.Consts[0])
	}
}

func TestBuilderPositions(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	p := source.Pos{Line: 3, Column: 5, Offset: 20}
	b.Add(OpNil, p)
	b.AddU16(OpPushConst, 0, pos())
	fn := b.Finish()
	if !fn.Positions[0].IsValid() || fn.Positions[0].Line != 3 {
		t.Errorf("position of first instruction: %+v", fn.Positions[0])
	}
	if !fn.Positions[1].IsValid() {
		t.Errorf("position of second instruction should be set")
	}
}

func TestSetNumSlots(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	b.SetNumSlots(4)
	if b.Finish().NumSlots != 4 {
		t.Error("NumSlots not recorded")
	}
}

func TestDisassemble(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	p := source.Pos{Line: 2, Column: 1, Offset: 10}
	idx := b.Const(object.Int{Value: 42})
	b.AddU16(OpPushConst, idx, p)
	b.Add(OpPop, p)
	b.AddU16Pair(OpGetUp, 0x0102, 0x0304, pos())
	b.Add(OpReturn, p)
	out := Disassemble(b.Finish())

	for _, want := range []string{
		"PUSH_CONST",
		"POP",
		"GET_UP",
		"RETURN",
		"test.spr:2:1",
		"(42)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("disassembly missing %q:\n%s", want, out)
		}
	}
}

func TestStructOpcodeNames(t *testing.T) {
	for _, c := range []struct {
		op   Opcode
		want string
	}{
		{OpMakeStruct, "MAKE_STRUCT"},
		{OpAddMethod, "ADD_METHOD"},
		{OpGetMember, "GET_MEMBER"},
		{OpSetMember, "SET_MEMBER"},
		{OpBuildStruct, "BUILD_STRUCT"},
	} {
		if got := c.op.String(); got != c.want {
			t.Errorf("opcode: got %q, want %q", got, c.want)
		}
	}
}

func TestDisassembleStructOps(t *testing.T) {
	b := NewBuilder("main", "test.spr", nil)
	nameIdx := b.Const(object.Str{Value: "Point"})
	fieldIdx := b.Const(object.Str{Value: "x"})
	b.AddU16Pair(OpMakeStruct, nameIdx, 1, pos())
	b.AddU16(OpGetMember, fieldIdx, pos())
	b.AddU16(OpSetMember, fieldIdx, pos())
	b.AddU16(OpAddMethod, fieldIdx, pos())
	b.AddU16(OpBuildStruct, 1, pos())
	b.Add(OpReturn, pos())
	out := Disassemble(b.Finish())

	for _, want := range []string{
		"MAKE_STRUCT",
		"(Point)",
		"fields 1",
		"GET_MEMBER",
		"SET_MEMBER",
		"ADD_METHOD",
		"BUILD_STRUCT",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("disassembly missing %q:\n%s", want, out)
		}
	}
}

func TestFunctionString(t *testing.T) {
	fn := &Function{Name: "fib"}
	if fn.String() != "<fn fib>" {
		t.Errorf("named fn string: %q", fn.String())
	}
	if (&Function{}).String() != "<fn>" {
		t.Errorf("anonymous fn string: %q", (&Function{}).String())
	}
	if fn.Type().String() != "function" {
		t.Errorf("fn type: %q", fn.Type())
	}
}

func TestU16(t *testing.T) {
	b := []byte{0xAB, 0xCD}
	if got := U16(b, 0); got != 0xABCD {
		t.Errorf("U16: got %#x", got)
	}
}
