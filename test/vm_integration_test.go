package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// vmRun executes a Sprout source file through the bytecode pipeline.
//
// It mirrors compileAndRun in integration_test.go but uses the bytecode
// virtual machine instead of the tree-walking interpreter.
func vmRun(t *testing.T, path string, stdin string) (string, *vm.RunError) {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	file := source.NewFile(path, string(text))
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("parse error in %s: %s", path, d.Message)
		}
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		t.Fatalf("check error in %s: %s", path, diags[0].Message)
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		t.Fatalf("compile error in %s: %s", path, err)
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(stdin), &stdout, &stderr)
	machine.SetModuleLoader(module.NewVMLoader(strings.NewReader(stdin), &stdout, &stderr))
	_, rerr := machine.Run(file, compiled)
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
