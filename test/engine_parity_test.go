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
		`struct Point { x y }
fn Point.sum() { return self.x + self.y }
let a = Point(1, 2)
let b = Point(x: 3, y: 4)
print(a)
print(a.sum(), b.sum())
a.x = 9
print(a)
print(a.sum())`,
		`struct Circle { radius }
struct Square { side }
interface Shape { area() }
fn Circle.area() { return 3.14 * self.radius * self.radius }
fn Square.area() { return self.side * self.side }
let shapes = [Circle(radius: 2), Square(side: 3)]
print(shapes[0].area(), shapes[1].area())
let total = 0.0
for s in shapes {
    total = total + s.area()
}
print(round(total * 100) / 100.0)`,
		`struct Tally { total }
fn Tally.add(n) {
    self.total = self.total + n
    return self.total
}
let t = Tally(0)
print(t.add(2), t.add(3))
print(t.total)`,
		`struct Box { v }
fn Box.get() { return self.v }
let b = Box(41)
let g = b.get
print(g() + 1)`,
		`struct Node { name }
fn Node.greet(prefix) {
    return prefix + " " + self.name
}
let n = Node("sprout")
print(n.greet("hello"))
print(type(n))
print(type(Node))
print(type(n.greet))`,
		`fn safe_div(a, b) {
    if b == 0 {
        return err("division by zero")
    }
    return ok(a / b)
}
print(match safe_div(10, 2) {
    ok(v) => { v },
    err(m) => { 0 },
    _ => { -1 },
})
print(match safe_div(1, 0) {
    ok(v) => { v },
    err(m) => { "error: " + m },
    _ => { "?" },
})`,
		`let total = 0
for i in range(0, 5) {
    total = total + match i {
        0 => { 10 },
        3 => { 30 },
        _ => { i },
    }
}
print(total)`,
		`let nested = ok(err("deep"))
print(match nested {
    ok(err(m)) => { "nested: " + m },
    ok(v) => { "shallow" },
    err(m) => { m },
    _ => { "?" },
})
print(match ok(ok(6)) {
    ok(ok(x)) => { x },
    _ => { 0 },
})`,
		`fn describe(n) {
    return match n {
        0 => { "zero" },
        1 => { "one" },
        v => { "other " + str(v) },
    }
}
print(describe(0), describe(1), describe(7))`,
		`let r = ok(ok("nested"))
print(match r {
    ok(ok(v)) => { v },
    _ => { "?" },
})
print(unwrap(r))`,
		`let score = 5
let label = match score {
    5 => { "five" },
    _ => { "other" },
}
print(label)
let r = err("nope")
print(unwrap_or(r, "fallback"))`,
		`let double = match ok(5) {
    ok(v) => {
        let get = fn() { return v }
        get() * 2
    },
    _ => { 0 },
}
print(double)`,
		`let jobs = channel()
let left = spawn(fn() {
    send(jobs, 20)
    return "left"
})
let right = spawn(fn() {
    send(jobs, 22)
    return "right"
})
print(unwrap(recv(jobs)) + unwrap(recv(jobs)))
print(unwrap(await(left)), unwrap(await(right)))`,
		`let gate = channel()
let worker = spawn(fn() {
    let signal = recv(gate)
    if is_cancelled() {
        return "stopped"
    }
    return signal
})
cancel(worker)
print(unwrap_or(await(worker), "cancelled"))`,
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
		`struct Point { x }
let p = Point(1)
print(p.missing)`,
		`struct Point { x }
let p = Point(1)
p.missing = 2`,
		"print(unwrap(err(\"boom\")))",
		`fn fail() {
    return err("kaboom")
}
print(match fail() {
    ok(v) => { v },
    err(m) => { 1 / 0 },
    _ => { 0 },
})`,
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

func TestEnginesAgreeOnSelect(t *testing.T) {
	src := `let first = channel()
let second = channel(1)
send(second, 7)
let event = unwrap(select([first, second]))
print(event["index"], event["value"])
close(first)
close(second)
print(unwrap_or(select([first, second]), "all closed"))`
	ivOut, ivErr := runInterpSrc(src)
	vmOut, vmErr := runVMSrc(src)
	if ivErr != nil || vmErr != nil {
		t.Fatalf("select errors: interpreter=%v vm=%v", ivErr, vmErr)
	}
	if ivOut != vmOut {
		t.Fatalf("select engines differ: interp=%q vm=%q", ivOut, vmOut)
	}
	want := "1 7\nall closed\n"
	if ivOut != want {
		t.Fatalf("select output = %q, want %q", ivOut, want)
	}
}

func TestEnginesAgreeOnSchedulerControls(t *testing.T) {
	src := `let events = channel(1)
let worker = spawn(fn() {
    send(events, "started")
    yield()
    sleep(1)
    send(events, "finished")
    return "done"
})
print(unwrap(recv(events)))
print(unwrap(recv(events)))
print(unwrap(await(worker)))
let canceled = spawn(fn() {
    sleep(1000)
    return "finished"
})
cancel(canceled)
print(unwrap_or(await(canceled), "cancelled"))
close(events)`
	ivOut, ivErr := runInterpSrc(src)
	vmOut, vmErr := runVMSrc(src)
	if ivErr != nil || vmErr != nil {
		t.Fatalf("scheduler errors: interpreter=%v vm=%v", ivErr, vmErr)
	}
	if ivOut != vmOut {
		t.Fatalf("scheduler engines differ: interp=%q vm=%q", ivOut, vmOut)
	}
	want := "started\nfinished\ndone\ncancelled\n"
	if ivOut != want {
		t.Fatalf("scheduler output = %q, want %q", ivOut, want)
	}
}
