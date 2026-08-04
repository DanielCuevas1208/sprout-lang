package checker

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

func checkMsgs(t *testing.T, src string) []string {
	t.Helper()
	file := source.NewFile("test.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, "parse: "+d.Message)
		}
		return msgs
	}
	var msgs []string
	for _, d := range Check(file, prog) {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func expectClean(t *testing.T, src string) {
	t.Helper()
	if msgs := checkMsgs(t, src); len(msgs) > 0 {
		t.Errorf("check %q: unexpected errors: %v", src, msgs)
	}
}

func expectError(t *testing.T, src, want string) {
	t.Helper()
	msgs := checkMsgs(t, src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, want) {
			found = true
		}
	}
	if !found {
		t.Errorf("check %q: expected error containing %q, got %v", src, want, msgs)
	}
}

func TestCleanPrograms(t *testing.T) {
	cases := []string{
		"let x = 5\nprint(x)",
		"const pi = 3.14\nprint(pi)",
		"fn add(a, b) { return a + b }\nprint(add(1, 2))",
		"fn later() { return later() }\nlater()",
		"for i in range(0, 3) { print(i) }",
		"while false { break }\nwhile true { continue }",
		"let f = fn() { return 1 }\nprint(f())",
		"let x: int = 5\nlet y: float = 5\nlet z: string = \"hi\"",
		"let x = 1\nif x { print(1) } else { print(2) }",
	}
	for _, src := range cases {
		expectClean(t, src)
	}
}

func TestCheckerErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"print(y)", "undefined name 'y'"},
		{"let x = 1\nx = 2", "no error"},
		{"const pi = 3\npi = 4", "cannot assign to constant"},
		{"const pi = 3\npi = pi + 1", "cannot assign to constant"},
		{"return 1", "inside a function"},
		{"break", "inside a loop"},
		{"continue", "inside a loop"},
		{"let x = 1\nlet x = 2", "duplicate declaration"},
		{"fn f(a, a) { }", "duplicate parameter"},
		{"let x: int = \"hi\"", "cannot initialize"},
		{"let x: int = 1.5", "cannot initialize"},
		{"let x: float = 5.5", "no error"},
		{"let x: bogus = 1", "unknown type"},
		{"x = 5", "undefined name 'x'"},
		{"let x = undefined_func()", "undefined name 'undefined_func'"},
		{"1 = 2", "cannot assign to this expression"},
	}
	for _, c := range cases {
		if c.want == "no error" {
			expectClean(t, c.src)
		} else {
			expectError(t, c.src, c.want)
		}
	}
}

func TestClosureCapture(t *testing.T) {
	expectClean(t, "let count = 0\nfn bump() { count = count + 1 }\nbump()")
	expectError(t, "fn outer() { fn inner() { return secret } }", "undefined name 'secret'")
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

// checkBundleMsgs loads the bundle rooted at dir/main.spr and returns the
// checker messages for the whole graph.
func checkBundleMsgs(t *testing.T, dir string) []string {
	t.Helper()
	bundle, diags := module.Load(filepath.Join(dir, "main.spr"))
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return []string{"load: " + d.Message}
		}
	}
	var msgs []string
	for _, d := range CheckBundle(bundle) {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func expectBundleClean(t *testing.T, dir string) {
	t.Helper()
	if msgs := checkBundleMsgs(t, dir); len(msgs) > 0 {
		t.Errorf("check bundle: unexpected errors: %v", msgs)
	}
}

func expectBundleError(t *testing.T, dir, want string) {
	t.Helper()
	msgs := checkBundleMsgs(t, dir)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, want) {
			found = true
		}
	}
	if !found {
		t.Errorf("check bundle: expected error containing %q, got %v", want, msgs)
	}
}

func TestCheckBundleImports(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "lib/greetings.spr",
		"export fn greet(name) { return name }\nexport const punc = \"!\"\n")
	writeModule(t, dir, "main.spr",
		"import \"lib/greetings.spr\"\nprint(greet(\"x\") + punc)\n")
	expectBundleClean(t, dir)
}

func TestCheckBundleAlias(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "calc.spr",
		"export fn add(a, b) { return a + b }\n")
	writeModule(t, dir, "main.spr",
		"import calc from \"calc.spr\"\nprint(calc[\"add\"](1, 2))\n")
	expectBundleClean(t, dir)
}

func TestCheckBundleTransitive(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "lib/a.spr",
		"export fn base() { return 1 }\n")
	writeModule(t, dir, "lib/b.spr",
		"import \"a.spr\"\nexport fn double() { return base() * 2 }\n")
	writeModule(t, dir, "main.spr",
		"import \"lib/b.spr\"\nprint(double())\n")
	expectBundleClean(t, dir)
}

func TestCheckBundleMissingImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "main.spr", "import \"nope.spr\"\n")
	expectBundleError(t, dir, "cannot find module")
}

func TestCheckBundleDuplicateImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 1\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\nimport \"m.spr\"\nprint(x)\n")
	expectBundleError(t, dir, "duplicate import")
}

func TestCheckBundleCycle(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "a.spr", "import \"b.spr\"\n")
	writeModule(t, dir, "b.spr", "import \"a.spr\"\n")
	writeModule(t, dir, "main.spr", "import \"a.spr\"\n")
	expectBundleError(t, dir, "import cycle")
}

func TestCheckBundleNameConflicts(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let count = 1\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\nlet count = 2\nprint(count)\n")
	expectBundleError(t, dir, "duplicate declaration")
}

func TestCheckBundleImportedNameIsConst(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let count = 1\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\ncount = 2\n")
	expectBundleError(t, dir, "cannot assign to constant")
}

func TestCheckBundleUseBeforeImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 1\n")
	// Imports are hoisted, so using x before the import line is legal.
	writeModule(t, dir, "main.spr", "print(x)\nimport \"m.spr\"\n")
	expectBundleClean(t, dir)
}
