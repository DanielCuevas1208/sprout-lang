// Package module_test exercises the loader against a real execution engine.
package module_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
)

func writeFile(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newLoader returns a loader that runs modules on the interpreter.
func newLoader() *module.Loader {
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	return module.New(iv)
}

func TestLoaderDetectsCycles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.spr", `import "b"`)
	writeFile(t, dir, "b.spr", `import "a"`)
	l := newLoader()

	_, err := l.Load("a", dir)
	if err == nil {
		t.Fatal("expected a circular import error")
	}
	if !strings.Contains(err.Error(), "circular import") {
		t.Errorf("error %q, want circular import", err)
	}
}

func TestCheckGraphDetectsCycles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.spr", `import "b"`)
	writeFile(t, dir, "b.spr", `import "a"`)
	l := module.New(nil)

	err := l.CheckGraph("a", dir)
	if err == nil {
		t.Fatal("expected a circular import error")
	}
	if !strings.Contains(err.Error(), "circular import") {
		t.Errorf("error %q, want circular import", err)
	}
}
