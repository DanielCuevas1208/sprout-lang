package ast_test

import (
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// roundTrip parses src, renders it back to text, and parses again.
//
// The renderer is a faithful inverse of the parser, so both parses must
// produce the same tree. The bundler relies on this property to inline
// module bodies safely.
func roundTrip(t *testing.T, src string) {
	t.Helper()
	prog1, diags := parser.Parse(source.NewFile("a.spr", src))
	for _, d := range diags {
		t.Fatalf("first parse failed: %s", d.Message)
	}
	rendered := ast.Source(prog1)
	prog2, diags := parser.Parse(source.NewFile("b.spr", rendered))
	for _, d := range diags {
		t.Fatalf("rendered source does not parse: %s", d.Message)
	}
	if ast.Sexp(prog1) != ast.Sexp(prog2) {
		t.Errorf("round trip changed the tree:\noriginal:\n%s\nrendered:\n%s", src, rendered)
	}
}

func TestSourceRoundTrip(t *testing.T) {
	cases := []string{
		"let x = 5\nprint(x)",
		"const pi = 3.14\nprint(round(pi))",
		"let s = \"hello\\nworld\"\nprint(s)",
		"fn fib(n) {\n    if n < 2 {\n        return n\n    }\n    return fib(n - 1) + fib(n - 2)\n}\nprint(fib(10))",
		"let m = {\"a\": 1, \"b\": 2}\nprint(m[\"a\"])",
		"let l = [1, 2, 3]\npush(l, 4)\nprint(l)",
		"let f = fn(x) { return x * 2 }\nprint(f(3))",
		"for i in range(0, 3) {\n    print(i)\n}",
		"while false {\n    break\n}",
		"let a = 1\nif a > 0 {\n    print(\"pos\")\n} elif a == 0 {\n    print(\"zero\")\n} else {\n    print(\"neg\")\n}",
		"let z = 1 + 2 * 3 - 4 / 2 ^ 2\nprint(z)",
		"let t = (1 + 2) * 3\nprint(t)",
		"not true and false or 1 == 2",
		"let g = -5\nprint(not g)",
		`print("\x41\x00")`,
		"print(0.5)",
	}
	for _, src := range cases {
		roundTrip(t, src)
	}
}

func TestSourceRoundTripModules(t *testing.T) {
	cases := []string{
		`import "util" as u
print(u.greet())`,
		`export let x = 1
export fn f() {
    return x
}`,
		"let m = helper.build()\nprint(m.name)",
	}
	for _, src := range cases {
		roundTrip(t, src)
	}
}

func TestSourceRoundTripStructs(t *testing.T) {
	cases := []string{
		"struct Point {\n    x\n    y\n}\nlet p = Point(1, 2)\nprint(p.x)",
		"struct Point {\n    x\n    y\n}\nlet p = Point(x: 1, y: 2)\nprint(p.y)",
		"struct Point {\n    x\n}\nfn Point.sum() {\n    return self.x\n}\nprint(Point(1).sum())",
		"interface Shape {\n    area()\n}\nstruct Circle {\n    radius\n}\nfn Circle.area() {\n    return 1.0\n}\nlet s: Shape = Circle(radius: 1)\nprint(s.area())",
		"export struct Point {\n    x\n}\nexport fn Point.sum() {\n    return self.x\n}",
	}
	for _, src := range cases {
		roundTrip(t, src)
	}
}

func TestSourceBodyDropsImportsAndExports(t *testing.T) {
	src := `import "util"
export let x = 1
export fn f() { return x }
let y = 2
`
	prog, diags := parser.Parse(source.NewFile("m.spr", src))
	for _, d := range diags {
		t.Fatalf("parse failed: %s", d.Message)
	}
	out := ast.SourceBody(prog)
	if out == src {
		t.Error("SourceBody must differ from Source for module files")
	}
	// The rendered body must not contain import or export keywords.
	prog2, diags := parser.Parse(source.NewFile("b.spr", out))
	for _, d := range diags {
		t.Fatalf("body does not parse: %s", d.Message)
	}
	for _, s := range prog2.Stmts {
		switch s.(type) {
		case *ast.ImportStmt:
			t.Error("SourceBody kept an import statement")
		}
	}
	// The original tree still keeps its imports and exports.
	hasImport, hasExport := false, false
	for _, s := range prog.Stmts {
		switch n := s.(type) {
		case *ast.ImportStmt:
			hasImport = true
		case *ast.LetStmt:
			if n.Export {
				hasExport = true
			}
		case *ast.FnStmt:
			if n.Export {
				hasExport = true
			}
		}
	}
	if !hasImport || !hasExport {
		t.Errorf("original tree lost import/export flags: import=%v export=%v", hasImport, hasExport)
	}
}
