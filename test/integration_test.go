// Package test holds end-to-end tests that run example programs.
package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// compileAndRun runs a Sprout source through the full pipeline.
func compileAndRun(t *testing.T, path string, stdin string) (string, *interp.RunError) {
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
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(stdin), &stdout, &stderr)
	iv.SetModuleLoader(module.NewInterpLoader(strings.NewReader(stdin), &stdout, &stderr))
	_, rerr := iv.Exec(file, prog)
	return stdout.String(), rerr
}

func TestExamplesMatchGoldens(t *testing.T) {
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
			// When a "<name>.stdin" file exists, feed it to the program.
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
			got, rerr := compileAndRun(t, ex, stdin)
			if rerr != nil {
				t.Fatalf("runtime error: %s", rerr.Message)
			}
			want := string(golden)
			if got != want {
				t.Errorf("output mismatch for %s:\n got:\n%q\nwant:\n%q", name, got, want)
			}
		})
	}
}

func TestExampleProduceExpectedValues(t *testing.T) {
	cases := []struct {
		name    string
		example string
		want    string
	}{
		{"fizzbuzz", "fizzbuzz.spr", "fizzbuzz"},
		{"primes", "primes.spr", "[2, 3, 5, 7, 11, 13, 17, 19, 23, 29]"},
		{"fib", "fibonacci.spr", "total calls to compute fib(10): 453"},
	}
	for _, c := range cases {
		got, rerr := compileAndRun(t, filepath.Join("..", "examples", c.example), "")
		if rerr != nil {
			t.Fatalf("%s: runtime error: %s", c.example, rerr.Message)
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: output %q does not contain %q", c.example, got, c.want)
		}
	}
}
