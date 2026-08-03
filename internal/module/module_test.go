package module

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// write creates a file under dir, creating parent directories as needed.
func write(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runInterp executes mainPath on the interpreter with loader.
func runInterp(l *Loader, mainPath string, out *strings.Builder, errb *strings.Builder) error {
	text, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}
	file := source.NewFile(mainPath, string(text))
	prog, diags := parser.Parse(file)
	if d := firstError(diags); d != nil {
		return fmt.Errorf("%s", d.Message)
	}
	iv := interp.NewWithIO(strings.NewReader(""), out, errb)
	iv.SetModuleLoader(l)
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		return rerr
	}
	return nil
}

// runVM executes mainPath on the bytecode virtual machine with loader.
func runVM(l *Loader, mainPath string, out *strings.Builder, errb *strings.Builder) error {
	text, err := os.ReadFile(mainPath)
	if err != nil {
		return err
	}
	file := source.NewFile(mainPath, string(text))
	prog, diags := parser.Parse(file)
	if d := firstError(diags); d != nil {
		return fmt.Errorf("%s", d.Message)
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return fmt.Errorf("%s", diags[0].Message)
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return err
	}
	machine := vm.NewWithIO(strings.NewReader(""), out, errb)
	machine.SetModuleLoader(l)
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		return rerr
	}
	return nil
}

func TestLoaderRunsModules(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib/math.spr", `
let answer = 42
fn double(x) {
	return x * 2
}
`)
	main := write(t, dir, "main.spr", `
let math = import "lib/math.spr"
print(math.answer)
print(math.double(21))
print(math["answer"])
`)

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	if err := runInterp(loader, main, &out, &errb); err != nil {
		t.Fatalf("interp: %v", err)
	}
	if out.String() != "42\n42\n42\n" {
		t.Errorf("interp output: %q", out.String())
	}
	if len(loader.Files()) != 1 {
		t.Errorf("loaded files: %v", loader.Files())
	}
}

func TestLoaderNestedImports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib/base.spr", "fn twice(x) {\n\treturn x * 2\n}\n")
	write(t, dir, "lib/calc.spr", `
let base = import "base.spr"
fn quad(x) {
	return base.twice(base.twice(x))
}
`)
	main := write(t, dir, "main.spr", `
let calc = import "lib/calc.spr"
print(calc.quad(3))
`)

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	if err := runInterp(loader, main, &out, &errb); err != nil {
		t.Fatalf("interp: %v", err)
	}
	if out.String() != "12\n" {
		t.Errorf("interp output: %q", out.String())
	}
	if len(loader.Files()) != 2 {
		t.Errorf("loaded files: %v", loader.Files())
	}
}

func TestLoaderCachesModules(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "m.spr", "print(\"module ran\")\nlet x = 7\n")
	main := write(t, dir, "main.spr", `
let a = import "m.spr"
let b = import "m.spr"
print(a.x)
print(a.x == b.x)
`)

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	if err := runInterp(loader, main, &out, &errb); err != nil {
		t.Fatalf("interp: %v", err)
	}
	// The module must run once even though it is imported twice.
	if out.String() != "module ran\n7\ntrue\n" {
		t.Errorf("interp output: %q", out.String())
	}
}

func TestLoaderDetectsCircularImport(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.spr", "let b = import \"b.spr\"\nlet name = \"a\"\n")
	write(t, dir, "b.spr", "let a = import \"a.spr\"\nlet name = \"b\"\n")
	main := write(t, dir, "main.spr", "let a = import \"a.spr\"\nprint(a.name)\n")

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	err := runInterp(loader, main, &out, &errb)
	if err == nil || !strings.Contains(err.Error(), "circular import") {
		t.Fatalf("expected circular import error, got %v", err)
	}
}

func TestLoaderMissingModule(t *testing.T) {
	dir := t.TempDir()
	main := write(t, dir, "main.spr", "let m = import \"nope.spr\"\nprint(m.x)\n")

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	err := runInterp(loader, main, &out, &errb)
	if err == nil || !strings.Contains(err.Error(), "cannot read module") {
		t.Fatalf("expected a read error, got %v", err)
	}
}

func TestLoaderParseErrorInModule(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "bad.spr", "let x =\n")
	main := write(t, dir, "main.spr", "let b = import \"bad.spr\"\nprint(b.x)\n")

	var out, errb strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &out, &errb)
	err := runInterp(loader, main, &out, &errb)
	if err == nil || !strings.Contains(err.Error(), "parse error in module") {
		t.Fatalf("expected a parse error, got %v", err)
	}
}

func TestEnginesAgreeOnModules(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib/seq.spr", `
fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
fn even(n) {
	return n % 2 == 0
}
`)
	write(t, dir, "lib/text.spr", `
let seq = import "seq.spr"
fn shout(s) {
	return upper(s) + "!"
}
fn count_words(s) {
	return len(split(s, " "))
}
fn even_fib(n) {
	return seq.even(seq.fib(n))
}
`)
	main := write(t, dir, "main.spr", `
let text = import "lib/text.spr"
print(text.shout("hi"))
print(text.count_words("one two three"))
print(text.even_fib(6))
print(text.even_fib(5))
`)

	var outI, errI strings.Builder
	loaderI := NewInterpLoader(strings.NewReader(""), &outI, &errI)
	if err := runInterp(loaderI, main, &outI, &errI); err != nil {
		t.Fatalf("interp: %v", err)
	}
	var outV, errV strings.Builder
	loaderV := NewVMLoader(strings.NewReader(""), &outV, &errV)
	if err := runVM(loaderV, main, &outV, &errV); err != nil {
		t.Fatalf("vm: %v", err)
	}
	if outI.String() != outV.String() {
		t.Errorf("engines differ:\ninterp: %q\n    vm: %q", outI.String(), outV.String())
	}
	if outI.String() != "HI!\n3\ntrue\nfalse\n" {
		t.Errorf("unexpected output: %q", outI.String())
	}
}

func TestGraphResolvesImports(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lib/base.spr", "let x = 1\n")
	write(t, dir, "lib/calc.spr", "let base = import \"base.spr\"\nlet y = 2\n")
	main := write(t, dir, "main.spr", "let calc = import \"lib/calc.spr\"\nprint(calc.y)\n")

	files, err := Graph(main)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("graph files: got %v", files)
	}
	got := make(map[string]bool)
	for _, f := range files {
		got[filepath.Base(f)] = true
	}
	for _, want := range []string{"main.spr", "calc.spr", "base.spr"} {
		if !got[want] {
			t.Errorf("graph missing %q in %v", want, files)
		}
	}
}

func TestTopLevelNames(t *testing.T) {
	file := source.NewFile("test.spr", `
let a = 1
const b = 2
fn f() {
	let c = 3
	return c
}
let d = 4
`)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		t.Fatalf("parse: %v", diags)
	}
	names := TopLevelNames(prog)
	want := "a b d f"
	if strings.Join(names, " ") != want {
		t.Errorf("names: got %v, want %q", names, want)
	}
}
