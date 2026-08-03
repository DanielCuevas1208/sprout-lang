package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/modules"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// run executes src on the VM and returns its stdout and any runtime error.
func run(src string, stdin string) (string, *RunError) {
	file := source.NewFile("test.spr", src)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", &RunError{Message: "parse error: " + d.Message}
		}
	}
	if cdiags := checker.Check(file, prog); len(cdiags) > 0 {
		return "", &RunError{Message: "check error: " + cdiags[0].Message}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &RunError{Message: "compile error: " + err.Error()}
	}
	var stdout, stderr strings.Builder
	machine := NewWithIO(strings.NewReader(stdin), &stdout, &stderr)
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}

func expectOutput(t *testing.T, src, want string) {
	t.Helper()
	got, rerr := run(src, "")
	if rerr != nil {
		t.Fatalf("run %q: runtime error: %s", src, rerr.Message)
	}
	if got != want {
		t.Errorf("run %q:\n got: %q\nwant: %q", src, got, want)
	}
}

func expectError(t *testing.T, src, want string) {
	t.Helper()
	_, rerr := run(src, "")
	if rerr == nil {
		t.Fatalf("run %q: expected error containing %q, got none", src, want)
	}
	if !strings.Contains(rerr.Message, want) {
		t.Errorf("run %q: error %q does not contain %q", src, rerr.Message, want)
	}
}

func TestVMArithmetic(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print(2 + 3)", "5\n"},
		{"print(7 - 10)", "-3\n"},
		{"print(6 * 7)", "42\n"},
		{"print(7 / 2)", "3\n"},
		{"print(7 % 3)", "1\n"},
		{"print(2 ^ 10)", "1024\n"},
		{"print(2 ^ 3 ^ 2)", "512\n"},
		{"print(-2 ^ 2)", "-4\n"},
		{"print(1 + 2.5)", "3.5\n"},
		{"print(2.0 * 3)", "6.0\n"},
		{"print(7.0 / 2)", "3.5\n"},
	}
	for _, c := range cases {
		expectOutput(t, c.src, c.want)
	}
}

func TestVMBooleans(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print(1 == 1)", "true\n"},
		{"print(1 != 2)", "true\n"},
		{"print(1 < 2)", "true\n"},
		{"print(3 >= 4)", "false\n"},
		{"print(0 and 1)", "1\n"},
		{"print(1 and 2)", "2\n"},
		{"print(1 or 5)", "1\n"},
		{"print(nil or 7)", "7\n"},
		{"print(not false)", "true\n"},
	}
	for _, c := range cases {
		expectOutput(t, c.src, c.want)
	}
}

func TestVMControlFlow(t *testing.T) {
	expectOutput(t, `
let x = 5
if x > 3 {
	print("big")
} else {
	print("small")
}
`, "big\n")

	expectOutput(t, `
let x = 2
if x == 0 {
	print("zero")
} elif x == 1 {
	print("one")
} else {
	print("other")
}
`, "other\n")

	expectOutput(t, `
let i = 0
let total = 0
while i < 5 {
	total = total + i
	i = i + 1
}
print(total)
`, "10\n")

	expectOutput(t, `
for i in range(0, 5) {
	print(i)
}
`, "0\n1\n2\n3\n4\n")

	expectOutput(t, `
for i in range(0, 10) {
	if i % 2 == 0 {
		continue
	}
	if i > 5 {
		break
	}
	print(i)
}
`, "1\n3\n5\n")
}

func TestVMFunctions(t *testing.T) {
	expectOutput(t, `
fn add(a, b) {
	return a + b
}
print(add(2, 3))
print(add(add(1, 1), add(2, 2)))
`, "5\n6\n")

	expectOutput(t, `
fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
print(fib(10))
`, "55\n")
}

func TestVMClosures(t *testing.T) {
	expectOutput(t, `
fn make_adder(n) {
	return fn(x) {
		return x + n
	}
}
let add5 = make_adder(5)
print(add5(10))
`, "15\n")

	expectOutput(t, `
fn make_counter() {
	let count = 0
	return fn() {
		count = count + 1
		return count
	}
}
let a = make_counter()
let b = make_counter()
print(a(), a(), b(), a())
`, "1 2 1 3\n")

	expectOutput(t, `
let fs = []
for i in range(0, 3) {
	push(fs, fn() {
		return i
	})
}
print(fs[0](), fs[1](), fs[2]())
`, "0 1 2\n")
}

func TestVMListsAndMaps(t *testing.T) {
	expectOutput(t, `
let l = [1, 2, 3]
print(l[0] + l[2])
l[1] = 20
print(l)
push(l, 4)
pop(l)
print(l)
`, "4\n[1, 20, 3]\n[1, 20, 3]\n")

	expectOutput(t, `
let m = {"a": 1, "b": 2}
m["c"] = 3
print(keys(m))
print(values(m))
print(m["missing"])
print(has(m, "b"))
`, "[a, b, c]\n[1, 2, 3]\nnil\ntrue\n")
}

func TestVMHigherOrder(t *testing.T) {
	expectOutput(t, `
let nums = [1, 2, 3, 4]
print(map(nums, fn(x) { return x * 2 }))
print(filter(nums, fn(x) { return x % 2 == 0 }))
print(fold(nums, 0, fn(acc, x) { return acc + x }))
`, "[2, 4, 6, 8]\n[2, 4]\n10\n")
}

func TestVMInput(t *testing.T) {
	got, rerr := run(`
let name = input("name? ")
print("hi, " + name)
`, "sprout\n")
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != "name? hi, sprout\n" {
		t.Errorf("got %q", got)
	}
}

func TestVMRuntimeErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print(1 / 0)", "cannot divide by zero"},
		{`print(1 + "a")`, "cannot add int and string"},
		{"let x = y", "undefined name 'y'"},
		{"const c = 1\nc = 2", "cannot assign to constant"},
		{"[1, 2][5]", "out of range"},
		{`"abc"[9]`, "out of range"},
		{`fn f() { } f(1)`, "expects 0 arguments, got 1"},
		{"print(1())", "cannot call a int"},
		{"print(1[0])", "cannot index a int"},
	}
	for _, c := range cases {
		expectError(t, c.src, c.want)
	}
}

func TestVMStackTrace(t *testing.T) {
	_, rerr := run(`
fn inner() {
	return 1 / 0
}
fn outer() {
	return inner()
}
outer()
`, "")
	if rerr == nil {
		t.Fatal("expected a runtime error")
	}
	if len(rerr.Frames) < 2 {
		t.Fatalf("expected 2+ frames, got %d", len(rerr.Frames))
	}
	if rerr.Frames[0].Name != "outer" || rerr.Frames[1].Name != "inner" {
		t.Errorf("frames: %+v", rerr.Frames)
	}
	if rerr.Pos.Line != 3 {
		t.Errorf("error position line: got %d, want 3", rerr.Pos.Line)
	}
}

func TestVMStringOps(t *testing.T) {
	expectOutput(t, `
print(upper("hi"))
print(join("-", ["a", "b"]))
print(split("a,b", ","))
print(len("héllo"))
`, "HI\na-b\n[a, b]\n5\n")
}

func TestVMClosureMutation(t *testing.T) {
	expectOutput(t, `
let log = []
fn add_entry(x) {
	push(log, x)
}
add_entry("one")
add_entry("two")
print(log)
`, "[one, two]\n")
}

// loadGraph writes the given files to a temp directory and loads the graph
// rooted at entryName.
func loadGraph(t *testing.T, files map[string]string, entryName string) *modules.Graph {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g, err := (&modules.Loader{}).Load(filepath.Join(dir, entryName))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestVMModules(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"math.spr": "fn square(n) { return n * n }\nlet version = \"0.1\"\n",
		"app.spr":  "import \"math\" as m\nprint(m.square(7))\nprint(m.version)\n",
	}, "app.spr")
	if g.HasErrors() {
		t.Fatalf("load errors: %v", g.Diags())
	}
	compiled, err := compiler.CompileGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	machine := NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(g.Units[g.Entry].File, compiled)
	if rerr != nil {
		t.Fatalf("run: %s", rerr.Message)
	}
	if stdout.String() != "49\n0.1\n" {
		t.Errorf("output: %q", stdout.String())
	}
}

func TestVMModuleSharedState(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"count.spr": `let n = 0
fn next() {
	n = n + 1
	return n
}`,
		"app.spr": "import \"count\" as c\nprint(c.next(), c.next(), c.next())\n",
	}, "app.spr")
	if g.HasErrors() {
		t.Fatalf("load errors: %v", g.Diags())
	}
	compiled, err := compiler.CompileGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	machine := NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(g.Units[g.Entry].File, compiled)
	if rerr != nil {
		t.Fatalf("run: %s", rerr.Message)
	}
	if stdout.String() != "1 2 3\n" {
		t.Errorf("output: %q", stdout.String())
	}
}

func TestVMModuleErrorPointsToModule(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"boom.spr": "fn bad() { return 1 / 0 }\nbad()\n",
		"app.spr":  "import \"boom\" as b\nprint(b.bad())\n",
	}, "app.spr")
	if g.HasErrors() {
		t.Fatalf("load errors: %v", g.Diags())
	}
	compiled, err := compiler.CompileGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	machine := NewWithIO(strings.NewReader(""), &strings.Builder{}, &strings.Builder{})
	_, rerr := machine.Run(g.Units[g.Entry].File, compiled)
	if rerr == nil {
		t.Fatal("expected a runtime error")
	}
	if !strings.Contains(rerr.Message, "cannot divide by zero") {
		t.Errorf("message: %q", rerr.Message)
	}
	// The error must point at the module file, not the entry.
	entryName := g.Units[g.Entry].File.Name
	if rerr.File == nil || rerr.File.Name == entryName || !strings.Contains(rerr.File.Name, "boom") {
		t.Errorf("error file: %v (entry: %s)", rerr.File, entryName)
	}
}

func TestVMMemberErrors(t *testing.T) {
	expectError(t, "let x = 5\nprint(x.foo)\n", "cannot access a member of a int")
	expectError(t, "print(print.foo)\n", "cannot access a member of a function")

	// A missing member is caught statically, before the program runs.
	g := loadGraph(t, map[string]string{
		"math.spr": "fn square(n) { return n * n }\n",
		"app.spr":  "import \"math\"\nprint(math.missing)\n",
	}, "app.spr")
	if !g.HasErrors() {
		t.Fatalf("expected load errors, got none")
	}
	found := false
	for _, d := range g.Diags() {
		if strings.Contains(d.Message, "module 'math' has no member 'missing'") {
			found = true
		}
	}
	if !found {
		t.Errorf("load errors: %v", g.Diags())
	}
}
