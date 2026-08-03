package compiler

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

func compileSrc(t *testing.T, src string) *code.Program {
	t.Helper()
	file := source.NewFile("test.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		t.Fatalf("parse errors: %v", diags)
	}
	p, err := Compile(file, prog)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return p
}

// containsOpcodes asserts that the disassembly contains each opcode in order.
func containsOpcodes(t *testing.T, fn *code.Function, want ...string) {
	t.Helper()
	out := code.Disassemble(fn)
	pos := 0
	for _, w := range want {
		i := strings.Index(out[pos:], w)
		if i < 0 {
			t.Errorf("missing %q in:\n%s", w, out)
			return
		}
		pos += i + len(w)
	}
}

func TestCompileLiterals(t *testing.T) {
	p := compileSrc(t, "print(1)\n")
	containsOpcodes(t, p.Main, "BUILTIN", "CALL", "POP", "RETURN")
}

func TestCompileArithmetic(t *testing.T) {
	p := compileSrc(t, "print(2 + 3 * 4)\n")
	containsOpcodes(t, p.Main, "PUSH_CONST", "PUSH_CONST", "PUSH_CONST", "MUL", "ADD")
}

func TestCompileAssignment(t *testing.T) {
	p := compileSrc(t, "let x = 1\nx = 2\nprint(x)\n")
	containsOpcodes(t, p.Main,
		"PUSH_CONST", "SET_LOCAL", "PUSH_CONST", "DUP", "SET_LOCAL", "GET_LOCAL")
}

func TestCompileClosure(t *testing.T) {
	p := compileSrc(t, `
fn make() {
	let n = 0
	return fn() {
		n = n + 1
		return n
	}
}
`)
	containsOpcodes(t, p.Main, "CLOSURE", "SET_LOCAL")
}

func TestCompileControlFlow(t *testing.T) {
	p := compileSrc(t, `
let total = 0
for i in range(0, 5) {
	if i == 3 {
		continue
	}
	total = total + i
}
print(total)
`)
	containsOpcodes(t, p.Main,
		"MAKE_ITER", "ITER_NEXT", "NEW_ENV", "SET_LOCAL",
		"EQ", "JUMP_IF_FALSE", "JUMP",
		"END_ENV", "JUMP", "POP", "BUILTIN", "CALL")
}

func TestCompileWhileBreak(t *testing.T) {
	p := compileSrc(t, `
let i = 0
while i < 10 {
	if i > 5 {
		break
	}
	i = i + 1
}
`)
	containsOpcodes(t, p.Main, "JUMP_IF_FALSE", "JUMP", "END_ENV", "JUMP")
}

func TestCompileUnresolvedNameFails(t *testing.T) {
	file := source.NewFile("t.spr", "print(missing)\n")
	prog, _ := parser.Parse(file)
	// The compiler assumes a checked program, so an unresolved name is an
	// internal error rather than a user diagnostic.
	if _, err := Compile(file, prog); err == nil {
		t.Fatal("expected compile error for unresolved name")
	}
}

func TestCompileConstantPool(t *testing.T) {
	p := compileSrc(t, "print(\"hello\")\n")
	if len(p.Main.Consts) == 0 {
		t.Fatal("expected constants in pool")
	}
	if p.Main.Consts[0].Type().String() != "string" {
		t.Errorf("first constant is %s", p.Main.Consts[0].Type())
	}
}

func TestCompileFunctionArity(t *testing.T) {
	p := compileSrc(t, "fn add(a, b) { return a + b }\n")
	fn := findFunction(t, p.Main, "add")
	if fn.NumSlots != 2 {
		t.Errorf("add NumSlots = %d, want 2", fn.NumSlots)
	}
	if len(fn.ParamNames) != 2 || fn.ParamNames[0] != "a" {
		t.Errorf("ParamNames = %v", fn.ParamNames)
	}
}

func findFunction(t *testing.T, root *code.Function, name string) *code.Function {
	t.Helper()
	var found *code.Function
	var walk func(fn *code.Function)
	walk = func(fn *code.Function) {
		if fn.Name == name {
			found = fn
			return
		}
		for _, c := range fn.Consts {
			if f, ok := c.(*code.Function); ok {
				walk(f)
			}
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("function %q not found in constants", name)
	}
	return found
}

var _ = ast.Program{}
