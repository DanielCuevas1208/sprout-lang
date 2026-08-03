package compiler

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// compileSrc parses and compiles src into bytecode.
func compileSrc(t *testing.T, src string) (*code.Program, error) {
	t.Helper()
	file := source.NewFile("test.spr", src)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	return Compile(file, prog)
}

// opCount counts the occurrences of op in the given function, ignoring
// nested functions.
func opCount(t *testing.T, fn *code.Function, op code.Opcode) int {
	t.Helper()
	n := 0
	ip := 0
	for ip < len(fn.Code) {
		if code.Opcode(fn.Code[ip]) == op {
			n++
		}
		ip = nextOffset(fn.Code, ip)
	}
	return n
}

// nextOffset returns the offset of the instruction after ip.
func nextOffset(b []byte, ip int) int {
	switch code.Opcode(b[ip]) {
	case code.OpGetUp, code.OpSetUp:
		return ip + 5
	case code.OpCall:
		return ip + 2
	case code.OpPushConst, code.OpNewEnv, code.OpGetLocal, code.OpSetLocal,
		code.OpBuiltin, code.OpClosure, code.OpImport, code.OpGetMember,
		code.OpJump, code.OpJumpIfFalse, code.OpJumpIfTrue, code.OpBuildList,
		code.OpBuildMap, code.OpIterNext:
		return ip + 3
	default:
		return ip + 1
	}
}

// constValue returns the value of the i-th constant of fn.
func constValue(t *testing.T, fn *code.Function, i int) string {
	t.Helper()
	if i >= len(fn.Consts) {
		t.Fatalf("constant index %d out of range (len %d)", i, len(fn.Consts))
	}
	return fn.Consts[i].String()
}

func TestCompileLiterals(t *testing.T) {
	p, err := compileSrc(t, "print(1 + 2)\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if main.Name != "<main>" {
		t.Errorf("entry name: %q", main.Name)
	}
	if main.FileName != "test.spr" {
		t.Errorf("file name: %q", main.FileName)
	}
	if len(main.Consts) < 2 {
		t.Fatalf("expected int constants, got %d", len(main.Consts))
	}
	if constValue(t, main, 0) != "1" || constValue(t, main, 1) != "2" {
		t.Errorf("constants: %v", main.Consts)
	}
	if got := opCount(t, main, code.OpAdd); got != 1 {
		t.Errorf("add count: %d", got)
	}
	if got := opCount(t, main, code.OpPop); got != 1 {
		t.Errorf("pop count: %d", got)
	}
	if got := opCount(t, main, code.OpCall); got != 1 {
		t.Errorf("call count: %d", got)
	}
	if got := opCount(t, main, code.OpReturn); got != 1 {
		t.Errorf("return count: %d", got)
	}
}

func TestCompileControlFlow(t *testing.T) {
	p, err := compileSrc(t, "if 1 { print(2) } else { print(3) }\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpJumpIfFalse); got != 1 {
		t.Errorf("jump if false count: %d", got)
	}
	if got := opCount(t, main, code.OpJump); got != 1 {
		t.Errorf("jump count: %d", got)
	}
}

func TestCompileLoop(t *testing.T) {
	p, err := compileSrc(t, "while true { break }\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpJumpIfFalse); got != 1 {
		t.Errorf("jump if false count: %d", got)
	}
	// One jump closes the loop back to the condition. The break adds one
	// more jump that skips past the loop end.
	if got := opCount(t, main, code.OpJump); got != 2 {
		t.Errorf("jump count: %d", got)
	}
	// One EndEnv closes the block scope normally. The break emits a second
	// one to unwind the same scope early.
	if got := opCount(t, main, code.OpEndEnv); got != 2 {
		t.Errorf("end env count: %d", got)
	}
}

func TestCompileForIn(t *testing.T) {
	p, err := compileSrc(t, "for i in range(0, 3) { print(i) }\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpMakeIter); got != 1 {
		t.Errorf("make iter count: %d", got)
	}
	if got := opCount(t, main, code.OpIterNext); got != 1 {
		t.Errorf("iter next count: %d", got)
	}
	if got := opCount(t, main, code.OpNewEnv); got != 1 {
		t.Errorf("new env count: %d", got)
	}
}

func TestCompileClosure(t *testing.T) {
	p, err := compileSrc(t, `
fn counter() {
	let n = 0
	return fn() { return n }
}
counter()
`)
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpClosure); got != 1 {
		t.Errorf("closure count in main: %d", got)
	}
	// The nested function must live in the main constant pool.
	nested := 0
	for _, c := range main.Consts {
		if _, ok := c.(*code.Function); ok {
			nested++
		}
	}
	if nested != 1 {
		t.Errorf("nested functions in main pool: %d", nested)
	}
}

func TestCompileScopedAccess(t *testing.T) {
	p, err := compileSrc(t, `
fn outer() {
	let a = 1
	fn inner() {
		return a
	}
	return inner
}
outer()
`)
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	// Find the outer and inner functions.
	var outer, inner *code.Function
	for _, c := range main.Consts {
		if f, ok := c.(*code.Function); ok {
			if f.Name == "outer" {
				outer = f
			}
			for _, cc := range f.Consts {
				if ff, ok := cc.(*code.Function); ok {
					inner = ff
				}
			}
		}
	}
	if outer == nil || inner == nil {
		t.Fatalf("missing functions: outer=%v inner=%v", outer, inner)
	}
	if got := opCount(t, inner, code.OpGetUp); got != 1 {
		t.Errorf("get up count in inner: %d", got)
	}
	if got := opCount(t, outer, code.OpSetLocal); got < 1 {
		t.Errorf("set local count in outer: %d", got)
	}
}

func TestCompileShortCircuit(t *testing.T) {
	p, err := compileSrc(t, "print(0 and 1)\nprint(1 or 2)\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpDup); got != 2 {
		t.Errorf("dup count: %d", got)
	}
}

func TestCompileAssignment(t *testing.T) {
	p, err := compileSrc(t, "let x = 1\nx = 2\nprint(x)\n")
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpSetLocal); got != 2 {
		t.Errorf("set local count: %d", got)
	}
	if got := opCount(t, main, code.OpGetLocal); got != 1 {
		t.Errorf("get local count: %d", got)
	}
}

func TestCompileConstantAssignmentFails(t *testing.T) {
	_, err := compileSrc(t, "const c = 1\nc = 2\n")
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !strings.Contains(err.Error(), "constant") {
		t.Errorf("error: %v", err)
	}
}

func TestCompileUndefinedNameFails(t *testing.T) {
	_, err := compileSrc(t, "print(missing)\n")
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error: %v", err)
	}
}

func TestCompileDuplicateDeclarationFails(t *testing.T) {
	_, err := compileSrc(t, "let x = 1\nlet x = 2\n")
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error: %v", err)
	}
}

func TestCompileFunctionAssignmentFails(t *testing.T) {
	_, err := compileSrc(t, "fn f() { }\nf = 1\n")
	if err == nil {
		t.Fatal("expected a compile error for assigning to a function")
	}
	if !strings.Contains(err.Error(), "constant") {
		t.Errorf("error: %v", err)
	}
}

func TestCompileBreakOutsideLoopFails(t *testing.T) {
	_, err := compileSrc(t, "break\n")
	if err == nil {
		t.Fatal("expected a compile error")
	}
	if !strings.Contains(err.Error(), "break outside a loop") {
		t.Errorf("error: %v", err)
	}
}

func TestCompileRecursion(t *testing.T) {
	p, err := compileSrc(t, `
fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
print(fib(10))
`)
	if err != nil {
		t.Fatal(err)
	}
	// The fib body must reference fib through an upvalue.
	var fib *code.Function
	for _, c := range p.Main.Consts {
		if f, ok := c.(*code.Function); ok && f.Name == "fib" {
			fib = f
		}
	}
	if fib == nil {
		t.Fatal("fib function not found")
	}
	if got := opCount(t, fib, code.OpGetUp); got < 1 {
		t.Errorf("fib should read itself as an upvalue, got %d", got)
	}
}

func TestCompileASTPositionsTracked(t *testing.T) {
	p, err := compileSrc(t, "print(7)\n")
	if err != nil {
		t.Fatal(err)
	}
	// Every instruction start must carry a source position. Operand bytes
	// are skipped, so walk the stream with the decoder.
	main := p.Main
	for ip := 0; ip < len(main.Code); {
		if !main.Positions[ip].IsValid() {
			t.Errorf("missing position for instruction at offset %d", ip)
		}
		ip = nextOffset(main.Code, ip)
	}
}

func TestCompileImport(t *testing.T) {
	p, err := compileSrc(t, `import "./math"
print(math.square(5))
`)
	if err != nil {
		t.Fatal(err)
	}
	main := p.Main
	if got := opCount(t, main, code.OpImport); got != 1 {
		t.Errorf("import count: %d", got)
	}
	if got := opCount(t, main, code.OpGetMember); got != 1 {
		t.Errorf("member count: %d", got)
	}
	// The import binds a constant, so the name cannot be reassigned.
	if _, err := compileSrc(t, `import "./m"
m = 1`); err == nil {
		t.Fatal("expected a compile error for reassigning an import")
	}
}

func TestCompileMember(t *testing.T) {
	p, err := compileSrc(t, `import "./util"
print(util.shout("hi"))
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := opCount(t, p.Main, code.OpGetMember); got != 1 {
		t.Errorf("member count: %d", got)
	}
}

func TestCompileExports(t *testing.T) {
	p, err := compileSrc(t, `export fn f() { return 1 }
export let x = 2
let y = 3
`)
	if err != nil {
		t.Fatal(err)
	}
	exports := p.Main.Exports
	if len(exports) != 2 {
		t.Fatalf("export count: %d", len(exports))
	}
	// Function names are pre-declared, so f owns slot 0. The let binds the
	// next slot.
	if slot, ok := exports["f"]; !ok || slot != 0 {
		t.Errorf("f export slot: %d, %v", slot, ok)
	}
	if slot, ok := exports["x"]; !ok || slot != 1 {
		t.Errorf("x export slot: %d, %v", slot, ok)
	}
	if _, ok := exports["y"]; ok {
		t.Error("y must not be exported")
	}
}
