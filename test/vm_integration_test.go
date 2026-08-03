package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/runner"
	"github.com/sprout-lang/sprout/internal/vm"
)

// vmRun executes a Sprout program through the bytecode pipeline.
//
// It mirrors compileAndRun in integration_test.go but uses the bytecode
// virtual machine instead of the tree-walking interpreter.
func vmRun(t *testing.T, path string, stdin string) (string, *vm.RunError) {
	t.Helper()
	g, diags := runner.Load(path)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("load error in %s: %s", path, d.Message)
		}
	}
	var stdout, stderr strings.Builder
	rerr := runner.RunVM(g, strings.NewReader(stdin), &stdout, &stderr)
	return stdout.String(), rerr
}

// TestExamplesMatchGoldensOnVM runs every example on the bytecode VM and
// compares its output with the golden files.
//
// The goldens were recorded from the interpreter. Matching them proves the
// two engines agree on the whole example suite.
func TestExamplesMatchGoldensOnVM(t *testing.T) {
	examples, err := filepath.Glob(filepath.Join("..", "examples", "*.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) == 0 {
		t.Fatal("no example programs found")
	}

	for _, ex := range examples {
		name := strings.TrimSuffix(filepath.Base(ex), ".spr")
		t.Run(name, func(t *testing.T) {
			stdin := ""
			stdinPath := filepath.Join("golden", name+".stdin")
			if data, err := os.ReadFile(stdinPath); err == nil {
				stdin = string(data)
			}
			goldenPath := filepath.Join("golden", name+".txt")
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("missing golden file %s: %v", goldenPath, err)
			}
			got, rerr := vmRun(t, ex, stdin)
			if rerr != nil {
				t.Fatalf("runtime error: %s", rerr.Message)
			}
			if got != string(golden) {
				t.Errorf("output mismatch for %s:\n got:\n%q\nwant:\n%q", name, got, string(golden))
			}
		})
	}
}
