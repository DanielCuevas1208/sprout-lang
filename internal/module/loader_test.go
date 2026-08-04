package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
)

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

// loadErrors returns the error messages for the bundle rooted at main.spr.
func loadErrors(t *testing.T, dir string) ([]string, *Bundle) {
	t.Helper()
	bundle, diags := Load(filepath.Join(dir, "main.spr"))
	var msgs []string
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return msgs, bundle
}

func expectLoadClean(t *testing.T, dir string) *Bundle {
	t.Helper()
	msgs, bundle := loadErrors(t, dir)
	if len(msgs) > 0 {
		t.Fatalf("load: unexpected errors: %v", msgs)
	}
	return bundle
}

func TestLoadBuildsGraph(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "lib/a.spr", "export fn a() { return 1 }\n")
	writeModule(t, dir, "lib/b.spr", "import \"a.spr\"\nexport fn b() { return a() + 1 }\n")
	writeModule(t, dir, "main.spr", "import \"lib/b.spr\"\nprint(b())\n")
	bundle := expectLoadClean(t, dir)

	if !bundle.Main().Main {
		t.Error("entry file not marked as main")
	}
	if len(bundle.Files) != 3 {
		t.Fatalf("file count: %d, want 3", len(bundle.Files))
	}
	if bundle.Files[0].Path != filepath.Join(dir, "main.spr") {
		t.Errorf("main path: %q", bundle.Files[0].Path)
	}
	// The dependency (a) must be resolved to the module that b imports.
	var bFile *File
	for _, f := range bundle.Files {
		if strings.HasSuffix(f.Path, "b.spr") {
			bFile = f
		}
	}
	if bFile == nil {
		t.Fatal("b.spr not loaded")
	}
	// a.spr is loaded only once despite the shared dependency.
	loaded := 0
	for _, f := range bundle.Files {
		if strings.HasSuffix(f.Path, "a.spr") {
			loaded++
		}
	}
	if loaded != 1 {
		t.Errorf("a.spr loaded %d times", loaded)
	}
}

func TestLoadCollectsExports(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr",
		"export let count = 1\nexport const pi = 3\nexport fn f() { return 1 }\nlet hidden = 2\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\n")
	bundle := expectLoadClean(t, dir)
	var m *File
	for _, f := range bundle.Files {
		if strings.HasSuffix(f.Path, "m.spr") {
			m = f
		}
	}
	if m == nil {
		t.Fatal("m.spr not loaded")
	}
	want := []string{"count", "pi", "f"}
	if len(m.Exports) != len(want) {
		t.Fatalf("exports: %v, want %v", m.Exports, want)
	}
	for i := range want {
		if m.Exports[i] != want[i] {
			t.Errorf("export %d: %q, want %q", i, m.Exports[i], want[i])
		}
	}
}

func TestLoadResolvesExtension(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 1\n")
	// The import omits the .spr extension.
	writeModule(t, dir, "main.spr", "import \"m\"\n")
	bundle := expectLoadClean(t, dir)
	if len(bundle.Files) != 2 {
		t.Fatalf("file count: %d, want 2", len(bundle.Files))
	}
}

func TestLoadSetsImportIndex(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 1\n")
	writeModule(t, dir, "main.spr", "import m2 from \"m.spr\"\n")
	bundle := expectLoadClean(t, dir)
	main := bundle.Files[0]
	imp, ok := main.Prog.Stmts[0].(*ast.ImportStmt)
	if !ok {
		t.Fatalf("expected an import statement")
	}
	if imp.Module < 0 || imp.Module >= len(bundle.Files) {
		t.Errorf("import index %d out of range", imp.Module)
	}
	if !strings.HasSuffix(bundle.Files[imp.Module].Path, "m.spr") {
		t.Errorf("import index points at %q", bundle.Files[imp.Module].Path)
	}
}

func TestLoadMissingModule(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "main.spr", "import \"nope.spr\"\n")
	msgs, _ := loadErrors(t, dir)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "cannot find module 'nope.spr'") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a missing-module error, got %v", msgs)
	}
}

func TestLoadDuplicateImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "m.spr", "export let x = 1\n")
	writeModule(t, dir, "main.spr", "import \"m.spr\"\nimport \"m.spr\"\n")
	msgs, _ := loadErrors(t, dir)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "duplicate import") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a duplicate-import error, got %v", msgs)
	}
}

func TestLoadCycle(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "a.spr", "import \"b.spr\"\nexport let a = 1\n")
	writeModule(t, dir, "b.spr", "import \"a.spr\"\nexport let b = 1\n")
	writeModule(t, dir, "main.spr", "import \"a.spr\"\n")
	msgs, _ := loadErrors(t, dir)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "import cycle") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a cycle error, got %v", msgs)
	}
}

func TestLoadSelfImport(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "main.spr", "import \"main.spr\"\n")
	msgs, _ := loadErrors(t, dir)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, "import cycle") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a self-import cycle error, got %v", msgs)
	}
}
