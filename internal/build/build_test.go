package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// loadGraph writes a project, loads it, and checks it.
func loadGraph(t *testing.T, files map[string]string, entry string) *module.Graph {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g, diags := module.Load(filepath.Join(dir, entry), module.LoadOptions{
		LibDirs: []string{filepath.Join(dir, "lib")},
	})
	for _, d := range diags {
		if d.Severity == 0 {
			t.Fatalf("load error: %s", d.Message)
		}
	}
	if diags := checker.CheckGraph(g); len(diags) > 0 {
		t.Fatalf("check error: %s", diags[0].Message)
	}
	return g
}

// runSrc runs bundle text on the interpreter.
func runInterp(bundle string) (string, error) {
	var stdout strings.Builder
	file := source.NewFile("bundle.spr", bundle)
	prog, _ := parser.Parse(file)
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

// runBundle runs bundle text on the virtual machine.
func runVM(bundle string) (string, error) {
	var stdout strings.Builder
	file := source.NewFile("bundle.spr", bundle)
	prog, _ := parser.Parse(file)
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return "", &checkerErr{diags[0].Message}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", err
	}
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

type checkerErr struct{ msg string }

func (e *checkerErr) Error() string { return e.msg }

// TestBundleRunsOnBothEngines builds a project and runs the bundle on the
// interpreter and the VM. The output must match the output of the original
// project on both engines.
func TestBundleRunsOnBothEngines(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":     "import \"math\" as m\nprint(m.double(21))\nprint(m.greet())\n",
		"lib/math.spr": "export fn double(x) { return x * 2 }\nexport fn greet() { return \"hi\" }\n",
	}, "main.spr")

	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}

	want := "42\nhi\n"
	for name, run := range map[string]func(string) (string, error){
		"interp": runInterp,
		"vm":     runVM,
	} {
		got, rerr := run(bundle)
		if rerr != nil {
			t.Errorf("%s: bundle failed: %v", name, rerr)
			continue
		}
		if got != want {
			t.Errorf("%s: bundle output: got %q, want %q", name, got, want)
		}
	}
}

// TestBundleHasNoImports proves the bundle is self-contained.
func TestBundleHasNoImports(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":  "import \"a\"\nimport \"b\"\nprint(a.x, b.y)\n",
		"lib/a.spr": "export let x = 1\n",
		"lib/b.spr": "import \"a\"\nexport let y = a.x + 1\n",
	}, "main.spr")

	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bundle, "import ") {
		t.Error("bundle must not contain import statements")
	}
	file := source.NewFile("bundle.spr", bundle)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == 0 {
			t.Fatalf("bundle does not parse: %s", d.Message)
		}
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		t.Fatalf("bundle does not check: %s", diags[0].Message)
	}
}

// TestBundleIsDeterministic builds the same graph twice.
func TestBundleIsDeterministic(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr": "import \"util\"\nprint(util.value)\n",
		"util.spr": "export let value = 7\n",
	}, "main.spr")

	first, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("Bundle output is not deterministic")
	}
}

// TestBundleModuleCachedOnce proves a module body runs exactly once, even
// when two files import it.
func TestBundleModuleCachedOnce(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":      "import \"state\"\nimport \"user\"\nprint(state.ready)\n",
		"lib/state.spr": "print(\"state loaded\")\nexport let ready = true\n",
		"lib/user.spr":  "import \"state\"\n",
	}, "main.spr")

	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := runInterp(bundle)
	if rerr != nil {
		t.Fatalf("bundle failed: %v", rerr)
	}
	if got != "state loaded\ntrue\n" {
		t.Errorf("output: got %q", got)
	}
}

// TestBundleRejectsReservedNames guards the bundle helper names.
func TestBundleRejectsReservedNames(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr": "let __sprout_load = 1\n",
	}, "main.spr")
	if _, err := Bundle(g); err == nil {
		t.Fatal("expected an error for a reserved helper name")
	}
}

// TestBundleModuleWithClosureState keeps state inside a module closure.
func TestBundleModuleWithClosureState(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":        "import \"counter\"\nprint(counter.next())\nprint(counter.next())\n",
		"lib/counter.spr": "let n = 0\nexport fn next() {\n    n = n + 1\n    return n\n}\n",
	}, "main.spr")

	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := runInterp(bundle)
	if rerr != nil {
		t.Fatalf("bundle failed: %v", rerr)
	}
	if got != "1\n2\n" {
		t.Errorf("closure state: got %q, want %q", got, "1\n2\n")
	}
}

// TestBundleEmptyModule handles a module with no exports.
func TestBundleEmptyModule(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":      "import \"noise\"\nprint(\"done\")\n",
		"lib/noise.spr": "print(\"side effect\")\n",
	}, "main.spr")

	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	got, rerr := runInterp(bundle)
	if rerr != nil {
		t.Fatalf("bundle failed: %v", rerr)
	}
	if got != "side effect\ndone\n" {
		t.Errorf("output: got %q", got)
	}
}

// TestSourceBodyStripsExports verifies the printer used by the bundler.
func TestSourceBodyStripsExports(t *testing.T) {
	g := loadGraph(t, map[string]string{
		"main.spr":  "import \"m\"\nprint(m.x)\n",
		"lib/m.spr": "export let x = 1\nlet hidden = 2\n",
	}, "main.spr")
	bundle, err := Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bundle, "export ") {
		t.Error("bundle must not contain export modifiers")
	}
	if !strings.Contains(bundle, "module(") {
		t.Error("bundle must build module values with module()")
	}
	// The module map quotes its keys. A private name never appears quoted.
	if strings.Contains(bundle, "\"hidden\"") {
		t.Error("module map must not include private names")
	}
}
