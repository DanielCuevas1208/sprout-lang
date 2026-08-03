package code

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

func pos(line int) source.Pos { return source.Pos{Line: line, Column: 1} }

func TestBuilderEmitsInstructions(t *testing.T) {
	b := NewBuilder("test", "t.spr", nil)
	p := pos(1)
	start := b.AddU16(OpPushConst, 0, p)
	idx := b.Const(object.Int{Value: 42})
	b.PatchU16(start, idx)
	b.Add(OpReturn, p)
	b.SetNumSlots(0)

	fn := b.Finish()
	if fn.Name != "test" {
		t.Errorf("Name = %q", fn.Name)
	}
	if len(fn.Code) != 4 { // PUSH_CONST(3) + RETURN(1)
		t.Errorf("Code length = %d, want 4: % x", len(fn.Code), fn.Code)
	}
	if got := U16(fn.Code, 1); got != idx {
		t.Errorf("constant operand = %d, want %d", got, idx)
	}
	if len(fn.Consts) != 1 {
		t.Errorf("Consts = %v", fn.Consts)
	}
	if !fn.Positions[0].IsValid() {
		t.Errorf("position not recorded at offset 0")
	}
}

func TestOpcodeNames(t *testing.T) {
	want := []struct {
		op   Opcode
		name string
	}{
		{OpPushConst, "PUSH_CONST"},
		{OpGetUp, "GET_UP"},
		{OpIterNext, "ITER_NEXT"},
		{OpReturnValue, "RETURN_VALUE"},
	}
	for _, c := range want {
		if c.op.String() != c.name {
			t.Errorf("%d.String() = %q, want %q", c.op, c.op.String(), c.name)
		}
	}
}

func TestDisassemble(t *testing.T) {
	b := NewBuilder("demo", "demo.spr", []string{"a"})
	p := pos(2)
	b.AddU16(OpGetLocal, 0, p)
	idx := b.Const(object.Str{Value: "hi"})
	b.AddU16(OpPushConst, idx, p)
	b.Add(OpAdd, p)
	b.Add(OpReturnValue, p)
	b.SetNumSlots(1)

	out := Disassemble(b.Finish())
	if !strings.Contains(out, "GET_LOCAL") {
		t.Errorf("missing GET_LOCAL in:\n%s", out)
	}
	if !strings.Contains(out, "PUSH_CONST") {
		t.Errorf("missing PUSH_CONST in:\n%s", out)
	}
	if !strings.Contains(out, "demo.spr:2:1") {
		t.Errorf("missing position annotation in:\n%s", out)
	}
	if !strings.Contains(out, "RETURN_VALUE") {
		t.Errorf("missing RETURN_VALUE in:\n%s", out)
	}
}

func TestDisassembleClosureAndBuiltin(t *testing.T) {
	b := NewBuilder("outer", "o.spr", nil)
	inner := NewBuilder("inner", "o.spr", nil)
	inner.Add(OpReturn, pos(1))
	inner.SetNumSlots(0)
	fnIdx := b.Const(inner.Finish())
	b.AddU16(OpClosure, fnIdx, pos(2))
	b.AddU16(OpBuiltin, 0, pos(3)) // index 0 is "print"
	b.AddByte(OpCall, 0, pos(4))
	b.Add(OpReturn, pos(5))
	b.SetNumSlots(0)

	out := Disassemble(b.Finish())
	if !strings.Contains(out, "CLOSURE") || !strings.Contains(out, "(inner)") {
		t.Errorf("closure not annotated in:\n%s", out)
	}
	if !strings.Contains(out, "(print)") {
		t.Errorf("builtin not annotated in:\n%s", out)
	}
	if !strings.Contains(out, "CALL") || !strings.Contains(out, "0\n") {
		t.Errorf("call not annotated in:\n%s", out)
	}
}
