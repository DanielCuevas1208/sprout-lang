package checker

import (
	"strings"
	"testing"

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

func TestMatchChecking(t *testing.T) {
	expectClean(t, `let r = ok(1)
print(match r {
    ok(v) => { v },
    err(e) => { 0 },
    _ => { -1 },
})`)

	expectClean(t, `let n = 3
let x = match n {
    1 => { "one" },
    v => { "other" },
}
print(x)`)

	// The last arm must be a catch-all so every subject is covered.
	expectError(t, `let r = ok(1)
match r {
    ok(v) => { v },
    err(e) => { 0 },
}`, "catch-all")

	expectError(t, `let n = 1
match n {
    1 => { "one" },
    _ => { "other" },
    2 => { "two" },
}`, "catch-all")

	// Pattern variables bind in the arm body only.
	expectClean(t, `let r = ok(5)
print(match r {
    ok(v) => { v + 1 },
    _ => { 0 },
})`)
	expectError(t, `let r = ok(5)
match r {
    ok(v) => { v },
    _ => { v },
}`, "undefined name 'v'")

	// A result type annotation accepts ok() and err() values.
	expectClean(t, "let r: result = ok(1)\nlet e: result = err(\"no\")\nprint(r, e)")
}
