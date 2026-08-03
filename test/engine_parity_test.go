package test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
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

// moduleFS builds an in-memory filesystem for module parity tests.
func moduleFS(files map[string]string) *module.MemFS {
	fs := module.NewMemFS()
	for name, text := range files {
		fs.Add(name, text)
	}
	return fs
}

// runInterpModules runs src with an in-memory module filesystem.
func runInterpModules(src string, fs *module.MemFS) (string, error) {
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	iv.SetLoader(module.NewLoader(fs))
	file := source.NewFile("main.spr", src)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", errors.New("parse error: " + d.Message)
		}
	}
	if cdiags := checker.Check(file, prog); len(cdiags) > 0 {
		return "", errors.New("check error: " + cdiags[0].Message)
	}
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

// runVMModules runs src on the VM with an in-memory module filesystem.
func runVMModules(src string, fs *module.MemFS) (string, error) {
	var stdout, stderr strings.Builder
	file := source.NewFile("main.spr", src)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", errors.New("parse error: " + d.Message)
		}
	}
	if cdiags := checker.Check(file, prog); len(cdiags) > 0 {
		return "", errors.New("check error: " + cdiags[0].Message)
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", err
	}
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	machine.SetLoader(module.NewLoader(fs))
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

// TestEnginesAgreeOnModules checks that module programs behave identically
// on both engines, including failure messages.
func TestEnginesAgreeOnModules(t *testing.T) {
	greeting := map[string]string{
		"lib/greeting.spr": "let pi = 3.14\nfn hi(name) { return \"hello, \" + name }\n",
	}
	cases := []struct {
		src   string
		files map[string]string
	}{
		{
			`import "lib/greeting"
print(greeting.hi("world"))
print(greeting.pi)
print(type(greeting))`,
			greeting,
		},
		{
			`import "lib/greeting" as g
print(g.hi("sprout"))`,
			greeting,
		},
		{
			`import "lib/outer"
print(outer.value)`,
			map[string]string{
				"lib/outer.spr": "import \"inner\"\nlet value = inner.n + 1\n",
				"lib/inner.spr": "let n = 41\n",
			},
		},
		{
			`import "counter"
let a = counter.next()
let b = counter.next()
print(a, b)`,
			map[string]string{
				"counter.spr": "let count = 0\nfn next() {\n\tcount = count + 1\n\treturn count\n}\n",
			},
		},
		{
			`import "mathlib"
print(mathlib.double(21))`,
			map[string]string{
				"mathlib.spr": "fn double(x) { return x * 2 }\n",
			},
		},
		{
			`import "greeting"
print(greeting.missing)`,
			greeting,
		},
		{
			`import "missing"`,
			greeting,
		},
		{
			`import "boom"
print("after")`,
			map[string]string{
				"boom.spr": "let x = 1 / 0\n",
			},
		},
	}

	for _, c := range cases {
		fs := moduleFS(c.files)
		ivOut, ivErr := runInterpModules(c.src, fs)
		vmOut, vmErr := runVMModules(c.src, moduleFS(c.files))
		if ivErr == nil && vmErr == nil {
			if ivOut != vmOut {
				t.Errorf("engines differ for:\n%s\ninterp: %q\n    vm: %q", c.src, ivOut, vmOut)
			}
			continue
		}
		if (ivErr == nil) != (vmErr == nil) {
			t.Errorf("one engine failed for:\n%s\ninterp err: %v\nvm err: %v", c.src, ivErr, vmErr)
			continue
		}
		if ivErr.Error() != vmErr.Error() {
			t.Errorf("error messages differ for:\n%s\ninterp: %q\n    vm: %q", c.src, ivErr.Error(), vmErr.Error())
		}
	}
}
