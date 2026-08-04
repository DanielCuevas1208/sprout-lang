package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// fakeRunner records the modules it runs and returns canned exports.
type fakeRunner struct {
	ran     []string
	exports map[string]map[string]object.Object
}

func (f *fakeRunner) RunModule(path string, file *source.File, prog *ast.Program) (map[string]object.Object, error) {
	f.ran = append(f.ran, path)
	if m, ok := f.exports[path]; ok {
		return m, nil
	}
	return map[string]object.Object{}, nil
}

// writeModule writes a Sprout source file in dir and returns its path.
func writeModule(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoaderLoadsEachModuleOnce(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "a.spr", "export let x = 1")
	runner := &fakeRunner{exports: map[string]map[string]object.Object{
		filepath.Join(dir, "a.spr"): {"x": object.Int{Value: 1}},
	}}
	l := New(runner)

	m1, err := l.Load("a", dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	m2, err := l.Load("a.spr", dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(runner.ran) != 1 {
		t.Errorf("module ran %d times, want 1", len(runner.ran))
	}
	if m1 != m2 {
		t.Errorf("expected cached module, got two distinct loads")
	}
}

func TestLoaderResolvesExtension(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "math.spr", "export let x = 1")
	runner := &fakeRunner{}
	l := New(runner)

	if _, err := l.Load("math", dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(runner.ran) != 1 || filepath.Base(runner.ran[0]) != "math.spr" {
		t.Errorf("runner saw %v, want math.spr", runner.ran)
	}
}

func TestLoaderResolvesSubdirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "lib")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModule(t, sub, "tools.spr", "export let x = 1")
	runner := &fakeRunner{}
	l := New(runner)

	if _, err := l.Load("lib/tools", dir); err != nil {
		t.Fatalf("load from subdirectory: %v", err)
	}
	// A second path to the same file resolves to the same module.
	if _, err := l.Load("tools", sub); err != nil {
		t.Fatalf("load relative to the module's own directory: %v", err)
	}
	if len(runner.ran) != 1 {
		t.Errorf("module ran %d times, want 1", len(runner.ran))
	}
}

func TestLoaderMissingModule(t *testing.T) {
	dir := t.TempDir()
	l := New(&fakeRunner{})
	_, err := l.Load("missing", dir)
	if err == nil {
		t.Fatal("expected an error for a missing module")
	}
	if !strings.Contains(err.Error(), "missing.spr") {
		t.Errorf("error %q does not name the module", err)
	}
}

func TestLoaderRejectsBrokenModule(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "bad.spr", "fn f( { }")
	l := New(&fakeRunner{})

	_, err := l.Load("bad", dir)
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if !strings.Contains(err.Error(), "bad.spr") {
		t.Errorf("error %q does not name the module", err)
	}
}

func TestCheckGraphFindsImportedErrors(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "lib")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeModule(t, sub, "broken.spr", "export let x = undefined_name")
	writeModule(t, dir, "main.spr", `import "lib/broken"`)
	l := New(nil)

	err := l.CheckGraph("main", dir)
	if err == nil {
		t.Fatal("expected the broken module to fail the check")
	}
	if !strings.Contains(err.Error(), "undefined name") {
		t.Errorf("error %q does not mention the broken name", err)
	}
}
