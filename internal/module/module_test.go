package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// writeTree writes a set of source files into a fresh temp directory and
// returns its path. Keys use slash separators for portability.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func interpRun(dir, path string) (string, *interp.RunError) {
	full := filepath.Join(dir, filepath.FromSlash(path))
	text, err := os.ReadFile(full)
	if err != nil {
		return "", &interp.RunError{Message: err.Error()}
	}
	file := source.NewFile(full, string(text))
	prog, _ := parser.Parse(file)
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	iv.SetModuleLoader(NewInterpLoader(strings.NewReader(""), &stdout, &stderr))
	_, rerr := iv.Exec(file, prog)
	return stdout.String(), rerr
}

func vmRun(dir, path string) (string, *vm.RunError) {
	full := filepath.Join(dir, filepath.FromSlash(path))
	text, err := os.ReadFile(full)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	file := source.NewFile(full, string(text))
	prog, _ := parser.Parse(file)
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	machine.SetModuleLoader(NewVMLoader(strings.NewReader(""), &stdout, &stderr))
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}

func TestLoadModuleRunsOnce(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"
let b = import "lib/a.spr"
print(a == b)`,
		"lib/a.spr": `print("loaded a")
let answer = 42`,
	})
	got, ivErr := interpRun(dir, "main.spr")
	if ivErr != nil {
		t.Fatalf("interp: %v", ivErr)
	}
	if got != "loaded a\ntrue\n" {
		t.Errorf("interp output: %q", got)
	}
	got, vmErr := vmRun(dir, "main.spr")
	if vmErr != nil {
		t.Fatalf("vm: %v", vmErr)
	}
	if got != "loaded a\ntrue\n" {
		t.Errorf("vm output: %q", got)
	}
}

func TestLoadModuleExports(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let math = import "lib/math.spr"
print(math.double(4))
print(math.answer)
print(math["name"])`,
		"lib/math.spr": `let answer = 42
const name = "math"
fn double(x) {
	return x * 2
}`,
	})
	got, ivErr := interpRun(dir, "main.spr")
	if ivErr != nil {
		t.Fatalf("interp: %v", ivErr)
	}
	if got != "8\n42\nmath\n" {
		t.Errorf("interp output: %q", got)
	}
	got, vmErr := vmRun(dir, "main.spr")
	if vmErr != nil {
		t.Fatalf("vm: %v", vmErr)
	}
	if got != "8\n42\nmath\n" {
		t.Errorf("vm output: %q", got)
	}
}

func TestNestedImport(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let outer = import "lib/outer.spr"
print(outer.inner.message)
print(outer.greet("sprout"))`,
		"lib/outer.spr": `let inner = import "inner.spr"
fn greet(name) {
	return inner.shout(name)
}`,
		"lib/inner.spr": `let message = "hello from inner"
fn shout(text) {
	return upper(text) + "!"
}`,
	})
	got, ivErr := interpRun(dir, "main.spr")
	if ivErr != nil {
		t.Fatalf("interp: %v", ivErr)
	}
	if got != "hello from inner\nSPROUT!\n" {
		t.Errorf("interp output: %q", got)
	}
	got, vmErr := vmRun(dir, "main.spr")
	if vmErr != nil {
		t.Fatalf("vm: %v", vmErr)
	}
	if got != "hello from inner\nSPROUT!\n" {
		t.Errorf("vm output: %q", got)
	}
}

func TestCircularImportRejected(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"`,
		"lib/a.spr": `let b = import "b.spr"
let value = 1`,
		"lib/b.spr": `let a = import "a.spr"
let value = 2`,
	})
	_, ivErr := interpRun(dir, "main.spr")
	if ivErr == nil || !strings.Contains(ivErr.Error(), "circular import") {
		t.Errorf("interp: want circular import error, got %v", ivErr)
	}
	_, vmErr := vmRun(dir, "main.spr")
	if vmErr == nil || !strings.Contains(vmErr.Error(), "circular import") {
		t.Errorf("vm: want circular import error, got %v", vmErr)
	}
}

func TestMissingModuleRejected(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/ghost.spr"`,
	})
	_, ivErr := interpRun(dir, "main.spr")
	if ivErr == nil || !strings.Contains(ivErr.Error(), "cannot read module") {
		t.Errorf("interp: want read error, got %v", ivErr)
	}
	_, vmErr := vmRun(dir, "main.spr")
	if vmErr == nil || !strings.Contains(vmErr.Error(), "cannot read module") {
		t.Errorf("vm: want read error, got %v", vmErr)
	}
}

func TestModuleParseErrorRejected(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/broken.spr"`,
		"lib/broken.spr": `let x =
fn f( { }`,
	})
	_, ivErr := interpRun(dir, "main.spr")
	if ivErr == nil || !strings.Contains(ivErr.Error(), "parse error in module") {
		t.Errorf("interp: want parse error, got %v", ivErr)
	}
	_, vmErr := vmRun(dir, "main.spr")
	if vmErr == nil || !strings.Contains(vmErr.Error(), "parse error in module") {
		t.Errorf("vm: want parse error, got %v", vmErr)
	}
}

func TestModuleMemberMissing(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"
print(a.nope)`,
		"lib/a.spr": `let yes = 1`,
	})
	_, ivErr := interpRun(dir, "main.spr")
	if ivErr == nil || !strings.Contains(ivErr.Error(), "no exported member 'nope'") {
		t.Errorf("interp: want missing member error, got %v", ivErr)
	}
	_, vmErr := vmRun(dir, "main.spr")
	if vmErr == nil || !strings.Contains(vmErr.Error(), "no exported member 'nope'") {
		t.Errorf("vm: want missing member error, got %v", vmErr)
	}
}

func TestModuleIsReadOnly(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"
a["answer"] = 3`,
		"lib/a.spr": `let answer = 42`,
	})
	_, ivErr := interpRun(dir, "main.spr")
	if ivErr == nil || !strings.Contains(ivErr.Error(), "cannot assign to a member of a module") {
		t.Errorf("interp: want read-only error, got %v", ivErr)
	}
	_, vmErr := vmRun(dir, "main.spr")
	if vmErr == nil || !strings.Contains(vmErr.Error(), "cannot assign to a member of a module") {
		t.Errorf("vm: want read-only error, got %v", vmErr)
	}
}

func TestGraphCollectsFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"
let c = import "lib/c.spr"`,
		"lib/a.spr": `let b = import "b.spr"`,
		"lib/b.spr": `let x = 1`,
		"lib/c.spr": `let y = 2`,
	})
	mainPath := filepath.Join(dir, "main.spr")
	files, err := Graph(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	base := func(p string) string { return filepath.Base(p) }
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d: %v", len(files), files)
	}
	if base(files[0]) != "main.spr" {
		t.Errorf("first file: %v", files[0])
	}
	got := []string{base(files[1]), base(files[2]), base(files[3])}
	want := map[string]bool{"a.spr": true, "b.spr": true, "c.spr": true}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected graph member %q", g)
		}
	}
}

func TestGraphDetectsCycle(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr":  `let a = import "lib/a.spr"`,
		"lib/a.spr": `let b = import "b.spr"`,
		"lib/b.spr": `let a = import "a.spr"`,
	})
	_, err := Graph(filepath.Join(dir, "main.spr"))
	if err == nil || !strings.Contains(err.Error(), "circular import") {
		t.Fatalf("want circular import error, got %v", err)
	}
}

func TestGraphFailsOnBrokenModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr":  `let a = import "lib/a.spr"`,
		"lib/a.spr": `let x =`,
	})
	_, err := Graph(filepath.Join(dir, "main.spr"))
	if err == nil || !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("want parse error, got %v", err)
	}
}

func TestLoadOrderIsStable(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"main.spr": `let a = import "lib/a.spr"
let b = import "lib/b.spr"
let c = import "lib/a.spr"`,
		"lib/a.spr": `let x = 1`,
		"lib/b.spr": `let y = 2`,
	})
	full := filepath.Join(dir, "main.spr")
	text, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	file := source.NewFile(full, string(text))
	prog, _ := parser.Parse(file)
	var stdout, stderr strings.Builder
	loader := NewInterpLoader(strings.NewReader(""), &stdout, &stderr)
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	iv.SetModuleLoader(loader)
	if _, rerr := iv.Exec(file, prog); rerr != nil {
		t.Fatal(rerr)
	}
	got := loader.Files()
	if len(got) != 2 {
		t.Fatalf("want 2 loaded files, got %d: %v", len(got), got)
	}
	if filepath.Base(got[0]) != "a.spr" || filepath.Base(got[1]) != "b.spr" {
		t.Errorf("load order: %v", got)
	}
}
