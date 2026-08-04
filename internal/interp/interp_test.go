package interp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

type testIO struct {
	out strings.Builder
	err strings.Builder
}

// run executes src and returns its stdout and any runtime error.
func run(src string, stdin string) (string, *RunError) {
	io := &testIO{}
	iv := NewWithIO(strings.NewReader(stdin), &io.out, &io.err)
	file := source.NewFile("test.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		t := &testing.T{}
		t.Logf("parse errors: %v", diags)
	}
	_, rerr := iv.Exec(file, prog)
	return io.out.String(), rerr
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

func TestArithmetic(t *testing.T) {
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
		{"print(5 % 3)", "2\n"},
		{"print(1_000 + 1)", "1001\n"},
		{"print(1e2 + 0)", "100.0\n"},
	}
	for _, c := range cases {
		expectOutput(t, c.src, c.want)
	}
}

func TestStrings(t *testing.T) {
	cases := []struct{ src, want string }{
		{`print("a" + "b")`, "ab\n"},
		{`print("length: " + str(len("hello")))`, "length: 5\n"},
		{`print("héllo")`, "héllo\n"},
		{`print(len("héllo"))`, "5\n"},
		{`print("" == "")`, "true\n"},
		{`print("a" < "b")`, "true\n"},
		{`print("abc" < "abd")`, "true\n"},
	}
	for _, c := range cases {
		expectOutput(t, c.src, c.want)
	}
}

func TestBooleans(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print(1 == 1)", "true\n"},
		{"print(1 != 2)", "true\n"},
		{"print(1 < 2)", "true\n"},
		{"print(2 <= 2)", "true\n"},
		{"print(3 > 2)", "true\n"},
		{"print(3 >= 4)", "false\n"},
		{"print(1 == 1.0)", "true\n"},
		{"print(0 and 1)", "1\n"},
		{"print(1 and 2)", "2\n"},
		{"print(0 or 5)", "0\n"},
		{"print(1 or 5)", "1\n"},
		{"print(not false)", "true\n"},
		{"print(not 0)", "false\n"},
		{"print(nil or 7)", "7\n"},
		{"print(nil and 7)", "nil\n"},
	}
	for _, c := range cases {
		expectOutput(t, c.src, c.want)
	}
}

func TestControlFlow(t *testing.T) {
	expectOutput(t, `
let x = 5
if x > 3 {
	print("big")
} else {
	print("small")
}
`, "big\n")

	expectOutput(t, `
let x = 1
if x == 0 {
	print("zero")
} elif x == 1 {
	print("one")
} else {
	print("other")
}
`, "one\n")

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

	expectOutput(t, `
for ch in "ab" {
	print(ch)
}
`, "a\nb\n")
}

func TestFunctions(t *testing.T) {
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

	expectOutput(t, `
fn add(a, b) {
	return a + b
}
let g = add
print(g(1, 2))
`, "3\n")
}

func TestClosures(t *testing.T) {
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
let fs = []
for i in range(0, 3) {
	push(fs, fn() {
		return i
	})
}
print(fs[0](), fs[1](), fs[2]())
`, "0 1 2\n")
}

func TestLists(t *testing.T) {
	expectOutput(t, `
let l = [1, 2, 3]
print(l[0] + l[2])
l[1] = 20
print(l)
print(len(l))
push(l, 4)
print(l)
pop(l)
print(l)
print([1, 2] == [1, 2])
`, "4\n[1, 20, 3]\n3\n[1, 20, 3, 4]\n[1, 20, 3]\ntrue\n")
}

func TestMaps(t *testing.T) {
	expectOutput(t, `
let m = {"a": 1, "b": 2}
print(m["a"])
m["c"] = 3
print(m)
print(len(m))
print(m["missing"])
print(has(m, "b"))
print(keys(m))
print(values(m))
`, "1\n{a: 1, b: 2, c: 3}\n3\nnil\ntrue\n[a, b, c]\n[1, 2, 3]\n")
}

func TestHigherOrder(t *testing.T) {
	expectOutput(t, `
let nums = [1, 2, 3, 4]
print(map(nums, fn(x) { return x * 2 }))
print(filter(nums, fn(x) { return x % 2 == 0 }))
print(fold(nums, 0, fn(acc, x) { return acc + x }))
`, "[2, 4, 6, 8]\n[2, 4]\n10\n")
}

func TestBuiltins(t *testing.T) {
	expectOutput(t, `
print(str(42))
print(int("42"))
print(float("2.5"))
print(int(2.9))
print(bool(0))
print(bool(1))
print(bool(nil))
print(type(1))
print(type("s"))
print(type([1]))
print(type({"a": 1}))
	print(type(fn() { }))
`, "42\n42\n2.5\n2\ntrue\ntrue\nfalse\nint\nstring\nlist\nmap\nfunction\n")

	expectOutput(t, `
print(abs(-3))
print(min(3, 1, 2))
print(max(3, 1, 2))
print(floor(2.9))
print(ceil(2.1))
print(round(2.5))
print(sqrt(9))
`, "3\n1\n3\n2\n3\n3\n3.0\n")

	expectOutput(t, `
print(upper("hi"))
print(lower("HI"))
print(trim("  x  "))
print(starts_with("hello", "he"))
print(ends_with("hello", "lo"))
print(contains("hello", "ell"))
print(split("a,b", ","))
print(join("-", ["a", "b"]))
print(repeat("ab", 2))
`, "HI\nhi\nx\ntrue\ntrue\ntrue\n[a, b]\na-b\nabab\n")

	expectOutput(t, `
assert(1 + 1 == 2)
assert(true, "should pass")
print("assertions passed")
`, "assertions passed\n")
}

func TestInput(t *testing.T) {
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

func TestRuntimeErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"print(1 / 0)", "cannot divide by zero"},
		{"print(1 % 0)", "remainder by zero"},
		{`print(1 + "a")`, "cannot add int and string"},
		{"let x = y", "undefined name 'y'"},
		{"const c = 1\nc = 2", "cannot assign to constant"},
		{"print(len(5))", "len() expects"},
		{"print(len(5))", "got int"},
		{"[1, 2][5]", "out of range"},
		{`"abc"[9]`, "out of range"},
		{"print(1[0])", "cannot index a int"},
		{`fn f() { } f(1)`, "expects 0 arguments, got 1"},
		{`fn f(a) { } f()`, "expects 1 argument"},
		{"print(1())", "cannot call a int"},
		{"nil + 1", "cannot add"},
		{"print([][0])", "out of range"},
	}
	for _, c := range cases {
		expectError(t, c.src, c.want)
	}
}

func TestStackTrace(t *testing.T) {
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

func TestObjectString(t *testing.T) {
	expectOutput(t, `
print([1, "a", true, nil, 2.5])
print({"k": [1, 2]})
print(range(0, 3))
`, "[1, a, true, nil, 2.5]\n{k: [1, 2]}\nrange(0, 3)\n")
}

// writeModule writes src to a named file inside dir.
func writeModule(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runBundle loads the bundle rooted at dir/main.spr and runs it.
func runBundle(t *testing.T, dir string) (string, *RunError) {
	t.Helper()
	bundle, diags := module.Load(filepath.Join(dir, "main.spr"))
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("load error: %s", d.Message)
		}
	}
	io := &testIO{}
	iv := NewWithIO(strings.NewReader(""), &io.out, &io.err)
	_, rerr := iv.ExecBundle(bundle)
	return io.out.String(), rerr
}

func TestBundleImports(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "lib/g.spr",
		"export fn greet(name) { return \"hello, \" + name }\nexport const punc = \"!\"\n")
	writeModule(t, dir, "main.spr",
		"import \"lib/g.spr\"\nprint(greet(\"world\") + punc)\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != "hello, world!\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleAliasImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "calc.spr",
		"export fn add(a, b) { return a + b }\nexport fn mul(a, b) { return a * b }\n")
	writeModule(t, dir, "main.spr",
		"import calc from \"calc.spr\"\nprint(calc[\"add\"](2, 3))\nprint(calc[\"mul\"](3, 4))\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != "5\n12\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleModuleRunsOnce(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "lib/a.spr", "print(\"init a\")\nexport let x = 1\n")
	writeModule(t, dir, "lib/b.spr", "import \"a.spr\"\nexport let y = x + 1\n")
	writeModule(t, dir, "lib/c.spr", "import \"a.spr\"\nexport let z = x + 2\n")
	writeModule(t, dir, "main.spr",
		"import \"lib/b.spr\"\nimport \"lib/c.spr\"\nprint(y, z)\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	// Module a is shared by b and c, so its top level runs once.
	if got != "init a\n2 3\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleModulePrivacy(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr",
		"export let visible = 1\nlet secret = 2\n")
	writeModule(t, dir, "main.spr",
		"import m from \"m.spr\"\nprint(m[\"visible\"])\nprint(m[\"secret\"])\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	// A module map only exposes exported names.
	if got != "1\nnil\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleModuleClosures(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr",
		"let count = 0\n"+
			"export fn next() {\n"+
			"	count = count + 1\n"+
			"	return count\n"+
			"}\n")
	writeModule(t, dir, "main.spr",
		"import \"m.spr\"\nprint(next(), next(), next())\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != "1 2 3\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleImportHoisted(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 7\n")
	// The use of x comes before the import statement, yet still works.
	writeModule(t, dir, "main.spr", "print(x)\nimport \"m.spr\"\n")
	got, rerr := runBundle(t, dir)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != "7\n" {
		t.Errorf("got %q", got)
	}
}

func TestBundleErrorPointsAtModule(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export fn kaboom() {\n\treturn 1 / 0\n}\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\nprint(kaboom())\n")
	_, rerr := runBundle(t, dir)
	if rerr == nil {
		t.Fatal("expected a runtime error")
	}
	if rerr.File == nil || !strings.HasSuffix(rerr.File.Name, "m.spr") {
		t.Errorf("error file: %v", rerr.File)
	}
}
