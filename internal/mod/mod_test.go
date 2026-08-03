package mod

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// writeModule writes a module file and returns its directory.
func writeModule(t *testing.T, dir, name, src string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// diagTexts returns the message of each diagnostic.
func diagTexts(diags []diag.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Message)
	}
	return out
}

func TestExports(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "util.spr", `export fn shout(s) { return upper(s) }
let secret = 1
export const N = 2`)

	r := NewResolver()
	exports, diags, err := r.ModuleExports("util.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagTexts(diags))
	}
	if !exports["shout"] || !exports["N"] {
		t.Errorf("expected shout and N, got %v", exports)
	}
	if exports["secret"] {
		t.Error("secret must not be exported")
	}
}

func TestMissingModule(t *testing.T) {
	dir := t.TempDir()
	r := NewResolver()
	_, _, err := r.ModuleExports("nope.spr", filepath.Join(dir, "main.spr"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cannot find module") {
		t.Errorf("error: %v", err)
	}
}

func TestImportCycle(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "a.spr", `import "./b.spr"`)
	writeModule(t, dir, "b.spr", `import "./a.spr"`)
	writeModule(t, dir, "main.spr", `import "./a.spr"`)

	r := NewResolver()
	_, diags, err := r.ModuleExports("main.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) == 0 {
		t.Fatal("expected a cycle diagnostic")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "import cycle") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an import cycle diagnostic, got %v", diagTexts(diags))
	}
}

func TestModuleDiagnosticsReportedOnce(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "bad.spr", "print(undefined_thing)\n")
	writeModule(t, dir, "main.spr", `import "./bad.spr"
import "./bad.spr"`)

	r := NewResolver()
	_, diags, err := r.ModuleExports("main.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %d: %v", len(diags), diagTexts(diags))
	}
	if !strings.Contains(diags[0].Message, "undefined name") {
		t.Errorf("diagnostic: %v", diags[0].Message)
	}
}

func TestNestedModuleDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "deep.spr", "let boom = missing_value\n")
	writeModule(t, dir, "mid.spr", `import "./deep.spr"`)
	writeModule(t, dir, "main.spr", `import "./mid.spr"`)

	r := NewResolver()
	_, diags, err := r.ModuleExports("main.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %d: %v", len(diags), diagTexts(diags))
	}
	if !strings.Contains(diags[0].Message, "undefined name") {
		t.Errorf("diagnostic: %v", diags[0].Message)
	}
}

func TestMemberValidation(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "util.spr", "export fn shout(s) { return s }\n")
	writeModule(t, dir, "main.spr", `import "./util.spr"
print(util.shout("hi"))
print(util.nope)`)

	r := NewResolver()
	_, diags, err := r.ModuleExports("main.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %d: %v", len(diags), diagTexts(diags))
	}
	if !strings.Contains(diags[0].Message, "does not export 'nope'") {
		t.Errorf("diagnostic: %v", diags[0].Message)
	}
}

// TestLoaderLoadsOnce verifies that a module executes exactly once and that
// the exported values are visible.
func TestLoaderLoadsOnce(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "counter.spr", `let n = 0
export fn bump() { n = n + 1 }
export fn value() { return n }`)

	calls := 0
	loader := NewLoader(func(file *source.File, prog *ast.Program) (map[string]object.Object, error) {
		calls++
		exports := map[string]object.Object{
			"bump":  object.Str{Value: "bump"},
			"value": object.Str{Value: "value"},
		}
		return exports, nil
	})

	m1, err := loader.Load("counter.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	m2, err := loader.Load("counter.spr", filepath.Join(dir, "main.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if m1 != m2 {
		t.Error("the loader must return the same module value")
	}
	if calls != 1 {
		t.Errorf("module body ran %d times, want 1", calls)
	}
	if v, ok := m1.Get("bump"); !ok || v.String() != "bump" {
		t.Errorf("exported bump: %v, %v", v, ok)
	}
}

func TestExportedNames(t *testing.T) {
	file := source.NewFile("m.spr", `let a = 1
export let b = 2
fn c() { }
export fn d() { }
export const e = 3`)
	prog, _ := parser.Parse(file)
	got := ExportedNames(prog)
	want := []string{"b", "d", "e"}
	if len(got) != len(want) {
		t.Fatalf("names: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
