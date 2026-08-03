package test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
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
