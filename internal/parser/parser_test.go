package parser

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/source"
)

func parseSexp(src string) (string, []string) {
	file := source.NewFile("test.spr", src)
	prog, diags := Parse(file)
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}
	return ast.Sexp(prog), msgs
}

func expectParse(t *testing.T, src, wantSexp string) {
	t.Helper()
	got, msgs := parseSexp(src)
	if len(msgs) > 0 {
		t.Errorf("parse %q: unexpected errors: %v", src, msgs)
		return
	}
	got = normalize(got)
	want := normalize(wantSexp)
	if got != want {
		t.Errorf("parse %q:\n got:\n%s\nwant:\n%s", src, got, want)
	}
}

// normalize collapses whitespace so tests can write compact S-expressions.
func normalize(s string) string {
	var b strings.Builder
	inSpace := false
	for _, r := range s {
		if r == '\n' || r == ' ' || r == '\t' {
			if !inSpace {
				b.WriteByte(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	out := strings.ReplaceAll(b.String(), " )", ")")
	return strings.TrimSpace(out)
}

func expectErrors(t *testing.T, src string, want ...string) {
	t.Helper()
	_, msgs := parseSexp(src)
	if len(msgs) == 0 {
		t.Errorf("parse %q: expected errors, got none", src)
		return
	}
	for _, w := range want {
		found := false
		for _, m := range msgs {
			if strings.Contains(m, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("parse %q: expected error containing %q, got %v", src, w, msgs)
		}
	}
}

func TestPrecedence(t *testing.T) {
	cases := []struct{ src, want string }{
		{"1 + 2 * 3", "(program (binary + (int 1) (binary * (int 2) (int 3))))"},
		{"1 * 2 + 3", "(program (binary + (binary * (int 1) (int 2)) (int 3)))"},
		{"2 ^ 3 ^ 2", "(program (binary ^ (int 2) (binary ^ (int 3) (int 2))))"},
		{"-3 ^ 2", "(program (unary - (binary ^ (int 3) (int 2))))"},
		{"not a and b", "(program (binary and (unary not a) b))"},
		{"a == b or c", "(program (binary or (binary == a b) c))"},
		{"1 + 2 < 4", "(program (binary < (binary + (int 1) (int 2)) (int 4)))"},
		{"a = b = 1", "(program (assign a (assign b (int 1))))"},
	}
	for _, c := range cases {
		expectParse(t, c.src, c.want)
	}
}

func TestExpressions(t *testing.T) {
	cases := []struct{ src, want string }{
		{"f()", "(program (call f))"},
		{"f(1, 2)", "(program (call f (int 1) (int 2)))"},
		{"f(a)(b)", "(program (call (call f a) b))"},
		{"l[0]", "(program (index l (int 0)))"},
		{"l[i] = v", "(program (assign (index l i) v))"},
		{"[1, 2, 3]", "(program (list (int 1) (int 2) (int 3)))"},
		{`{"a": 1}`, `(program (map (entry (string "a") (int 1))))`},
		{`"hi"`, `(program (string "hi"))`},
		{"1.5", "(program (float 1.5))"},
		{"true", "(program true)"},
		{"nil", "(program nil)"},
		{"-x", "(program (unary - x))"},
		{"not x", "(program (unary not x))"},
		{"(1 + 2) * 3", "(program (binary * (binary + (int 1) (int 2)) (int 3)))"},
	}
	for _, c := range cases {
		expectParse(t, c.src, c.want)
	}
}

func TestStatements(t *testing.T) {
	cases := []struct{ src, want string }{
		{
			"let x = 5",
			"(program (let x value (int 5)))",
		},
		{
			"let x: int = 5",
			"(program (let x :type int value (int 5)))",
		},
		{
			"const pi = 3.14",
			"(program (const pi value (float 3.14)))",
		},
		{
			"fn fib(n) { return n }",
			"(program (fn fib (params n) (block (return value n))))",
		},
		{
			"if x { print(1) } else { print(2) }",
			"(program (if cond x (block (call print (int 1))) (else (block (call print (int 2))))))",
		},
		{
			"if a { 1 } elif b { 2 } else { 3 }",
			"(program (if cond a (block (int 1)) (elif cond b (block (int 2))) (else (block (int 3)))))",
		},
		{
			"while x < 3 { x = x + 1 }",
			"(program (while cond (binary < x (int 3)) (block (assign x (binary + x (int 1))))))",
		},
		{
			"for i in range(0, 3) { print(i) }",
			"(program (for var i in (call range (int 0) (int 3)) (block (call print i))))",
		},
		{
			"break\ncontinue\nreturn\nreturn 5",
			"(program (break) (continue) (return (nothing)) (return value (int 5)))",
		},
		{
			"let g = fn(x) { return x }",
			"(program (let g value (fn (params x) (block (return value x)))))",
		},
	}
	for _, c := range cases {
		expectParse(t, c.src, c.want)
	}
}

func TestModules(t *testing.T) {
	cases := []struct{ src, want string }{
		{
			`import "greet"`,
			`(program (import "greet" greet))`,
		},
		{
			`import "./util/format"`,
			`(program (import "./util/format" format))`,
		},
		{
			`import "./numbers" as math`,
			`(program (import "./numbers" math))`,
		},
		{
			`export fn hello(name) { return name }`,
			`(program (export fn hello (params name) (block (return value name))))`,
		},
		{
			`export const PI = 3`,
			`(program (export const PI value (int 3)))`,
		},
		{
			`export let count = 0`,
			`(program (export let count value (int 0)))`,
		},
		{
			`math.square(5)`,
			`(program (call (member math square) (int 5)))`,
		},
		{
			`a.b.c`,
			`(program (member (member a b) c))`,
		},
		{
			`mod.api().x`,
			`(program (member (call (member mod api)) x))`,
		},
	}
	for _, c := range cases {
		expectParse(t, c.src, c.want)
	}
}

func TestModuleErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`import 42`, "expected a module path string"},
		{`import "x" bar`, "expected 'as' or the end"},
		{`export 5`, "expected 'let', 'const', or a named 'fn'"},
		{`export print(1)`, "expected 'let', 'const', or a named 'fn'"},
		{`a.`, "expected a member name after '.'"},
	}
	for _, c := range cases {
		expectErrors(t, c.src, c.want)
	}
}

func TestMultiline(t *testing.T) {
	src := "let total = [\n  1,\n  2,\n]\nprint(\n  total\n)"
	_, msgs := parseSexp(src)
	if len(msgs) > 0 {
		t.Fatalf("multiline: unexpected errors: %v", msgs)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"let = 5", "expected a name after"},
		{"let x 5", "expected '='"},
		{"let x =", "expected a value after"},
		{"print(1", "expected ')'"},
		{"fn f(x { return x }", "expected ')'"},
		{"if x print(1)", "expected '{'"},
		{"1 +", "expected an expression"},
		{"x = ", "expected an expression"},
		{"}", "unexpected '}'"},
		{"for i range(0, 3) { }", "expected 'in'"},
		{"let x = 1 )", "expected a statement"},
	}
	for _, c := range cases {
		expectErrors(t, c.src, c.want)
	}
}
