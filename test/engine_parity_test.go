package test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// runInterpSrc runs src on the tree-walking interpreter.
func runInterpSrc(src string) (string, error) {
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	file := source.NewFile("test.spr", src)
	prog, _ := parser.Parse(file)
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

// runVMSrc runs src on the bytecode virtual machine.
func runVMSrc(src string) (string, error) {
	var stdout, stderr strings.Builder
	file := source.NewFile("test.spr", src)
	prog, _ := parser.Parse(file)
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return "", errors.New(diags[0].Message)
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", err
	}
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

// TestEnginesAgree runs the same programs on both engines and checks that
// their output matches.
//
// The VM shares its value semantics and standard library with the
// interpreter through the runtime package. This test guards that contract
// on programs that stress scoping, closures, loops, and containers.
func TestEnginesAgree(t *testing.T) {
	cases := []string{
		`let x = 1
if x == 0 {
	print("zero")
} elif x == 1 {
	print("one")
} else {
	print("other")
}`,
		`let x = 7
if x > 3 {
	print("big")
} elif x > 5 {
	print("huge")
} else {
	print("small")
}`,
		`fn make_counter() {
	let count = 0
	return fn() {
		count = count + 1
		return count
	}
}
let a = make_counter()
let b = make_counter()
print(a(), a(), b(), a())`,
		`let m = {"a": 1, "b": 2}
m["c"] = 3
print(keys(m))
print(values(m))
print(len(m))`,
		`let total = 0
for i in range(0, 10) {
	if i % 2 == 0 {
		continue
	}
	if i > 5 {
		break
	}
	total = total + i
}
print(total)`,
		`print(map([1, 2, 3], fn(x) { return x * x }))
print(filter([1, 2, 3, 4], fn(x) { return x % 2 == 0 }))
print(fold([1, 2, 3], 0, fn(acc, x) { return acc + x }))`,
		`let s = 0
while s < 3 {
	s = s + 1
}
print(s)`,
		`const pi = 3
print(pi)
let f = fn(x) { return x * pi }
print(f(2))`,
		`let l = [1, 2, 3]
l[1] = 9
print(l)
print([1, 2] == [1, 2])
print(1 == 1.0)`,
		`print(2 ^ 3 ^ 2)
print(-2 ^ 2)
print(7.0 / 2)
print("a" < "b")`,
		`fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
print(fib(12))`,
		`let fs = []
for i in range(0, 3) {
	push(fs, fn() {
		return i
	})
}
print(fs[0](), fs[1](), fs[2]())`,
		`let a = 10
fn set_a() {
	a = a + 5
}
set_a()
print(a)`,
		`print(type(1))
print(type("s"))
print(type([1]))
print(str(42))
print(int("42"))
print(round(2.5))`,
	}

	for _, src := range cases {
		ivOut, ivErr := runInterpSrc(src)
		vmOut, vmErr := runVMSrc(src)
		if ivErr != nil {
			t.Errorf("interpreter error for:\n%s\n  %v", src, ivErr)
			continue
		}
		if vmErr != nil {
			t.Errorf("vm error for:\n%s\n  %v", src, vmErr)
			continue
		}
		if ivOut != vmOut {
			t.Errorf("engines differ for:\n%s\ninterp: %q\n    vm: %q", src, ivOut, vmOut)
		}
	}
}

// TestEnginesAgreeOnErrors checks that both engines report the same message
// for the same failing program.
func TestEnginesAgreeOnErrors(t *testing.T) {
	cases := []string{
		"print(1 / 0)",
		`print("a" + 1)`,
		"print(len(5))",
		"[1, 2][9]",
		`fn f(a) { return a } f()`,
		"print(1())",
	}
	for _, src := range cases {
		ivOut, ivErr := runInterpSrc(src)
		vmOut, vmErr := runVMSrc(src)
		if ivErr == nil || vmErr == nil {
			t.Errorf("expected both engines to fail for:\n%s\ninterp err: %v\nvm err: %v", src, ivErr, vmErr)
			continue
		}
		if ivOut != vmOut {
			t.Errorf("stdout differs for failing program:\n%s\ninterp: %q\n    vm: %q", src, ivOut, vmOut)
		}
		if ivErr.Error() != vmErr.Error() {
			t.Errorf("error messages differ for:\n%s\ninterp: %q\n    vm: %q", src, ivErr.Error(), vmErr.Error())
		}
	}
}

// TestEnginesAgreeOnModules runs a program that imports modules on both
// engines. The loader contract must hold for the interpreter and the VM.
func TestEnginesAgreeOnModules(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"main.spr": `let math = import "lib/math.spr"
let text = import "lib/text.spr"
print(math.fib(8))
print(math.answer)
print(text.shout("hey"))
print(math.answer == math["answer"])`,
		"lib/math.spr": `let answer = 42
fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}`,
		"lib/text.spr": `fn shout(s) {
	return upper(s) + "!"
}`,
	}
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mainPath := filepath.Join(dir, "main.spr")
	ivOut, ivErr := runInterpFile(mainPath)
	vmOut, vmErr := runVMFile(mainPath)
	if ivErr != nil {
		t.Fatalf("interpreter error: %v", ivErr)
	}
	if vmErr != nil {
		t.Fatalf("vm error: %v", vmErr)
	}
	if ivOut != vmOut {
		t.Errorf("engines differ on modules:\ninterp: %q\n    vm: %q", ivOut, vmOut)
	}
}

// runInterpFile runs a source file on the interpreter with a module loader.
func runInterpFile(path string) (string, *interp.RunError) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", &interp.RunError{Message: err.Error()}
	}
	file := source.NewFile(path, string(text))
	prog, _ := parser.Parse(file)
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	iv.SetModuleLoader(module.NewInterpLoader(strings.NewReader(""), &stdout, &stderr))
	_, rerr := iv.Exec(file, prog)
	return stdout.String(), rerr
}

// runVMFile runs a source file on the bytecode VM with a module loader.
func runVMFile(path string) (string, *vm.RunError) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	file := source.NewFile(path, string(text))
	prog, _ := parser.Parse(file)
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	machine.SetModuleLoader(module.NewVMLoader(strings.NewReader(""), &stdout, &stderr))
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}
