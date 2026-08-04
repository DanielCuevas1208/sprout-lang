// Package-level tests that run multi-file projects on both engines.
package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/build"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// writeProject writes a map of relative paths into a fresh temp directory.
func writeProject(t *testing.T, files map[string]string) string {
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
	return dir
}

// loadProject loads and checks a project rooted at dir.
func loadProject(dir string) *module.Graph {
	g, diags := module.Load(filepath.Join(dir, "main.spr"), module.LoadOptions{
		LibDirs: []string{filepath.Join(dir, "lib")},
	})
	for _, d := range diags {
		if d.Severity == 0 {
			panic("load: " + d.Message)
		}
	}
	if diags := checker.CheckGraph(g); len(diags) > 0 {
		panic("check: " + diags[0].Message)
	}
	return g
}

// runGraphInterp runs a module graph on the tree-walking interpreter.
func runGraphInterp(g *module.Graph) (string, *interp.RunError) {
	var stdout strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	iv.SetModuleResolver(g.Resolve)
	_, rerr := iv.Exec(g.Entry.Source, g.Entry.Prog)
	return stdout.String(), rerr
}

// runGraphVM runs a module graph on the bytecode virtual machine.
func runGraphVM(g *module.Graph) (string, *vm.RunError) {
	compiled, err := compiler.CompileModules(g)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	var stdout strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	_, rerr := machine.Run(g.Entry.Source, compiled)
	return stdout.String(), rerr
}

// TestModuleParity runs a project with imports, exports, aliases, and
// closures on both engines and checks that they agree.
func TestModuleParity(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"main.spr": "import \"math\" as m\n" +
			"import \"state\"\n" +
			"print(m.twice(21))\n" +
			"print(state.bump(), state.bump(), state.bump())\n" +
			"print(m.scale(3))\n",
		"lib/math.spr": "import \"state\"\n" +
			"export fn twice(x) { return x * 2 }\n" +
			"export fn scale(x) { return x * state.bump() }\n",
		"lib/state.spr": "let n = 0\n" +
			"export fn bump() {\n" +
			"    n = n + 1\n" +
			"    return n\n" +
			"}\n",
	})
	g := loadProject(dir)

	ivOut, ivErr := runGraphInterp(g)
	vmOut, vmErr := runGraphVM(g)
	if ivErr != nil {
		t.Fatalf("interpreter error: %s", ivErr.Message)
	}
	if vmErr != nil {
		t.Fatalf("vm error: %s", vmErr.Message)
	}
	if ivOut != vmOut {
		t.Errorf("engines differ:\ninterp: %q\n    vm: %q", ivOut, vmOut)
	}
	// The module body must run once, so bump() sees shared state.
	if !strings.Contains(ivOut, "1 2 3") {
		t.Errorf("module state not shared: %q", ivOut)
	}
}

// TestBundleParity builds a project and runs the bundle on both engines.
func TestBundleParity(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"main.spr": "import \"math\" as m\n" +
			"print(m.square(5))\n" +
			"print(m.greet())\n",
		"lib/math.spr": "export fn square(x) { return x * x }\n" +
			"export fn greet() { return \"hello\" }\n",
	})
	g := loadProject(dir)

	bundle, err := build.Bundle(g)
	if err != nil {
		t.Fatal(err)
	}

	ivOut, ivErr := runBundleInterp(bundle)
	vmOut, vmErr := runBundleVM(bundle)
	if ivErr != nil {
		t.Fatalf("bundle interpreter error: %s", ivErr.Message)
	}
	if vmErr != nil {
		t.Fatalf("bundle vm error: %s", vmErr.Message)
	}
	if ivOut != vmOut {
		t.Errorf("bundles differ:\ninterp: %q\n    vm: %q", ivOut, vmOut)
	}
	if !strings.Contains(ivOut, "25") || !strings.Contains(ivOut, "hello") {
		t.Errorf("unexpected bundle output: %q", ivOut)
	}
}

// runBundleInterp runs bundle source on the interpreter.
func runBundleInterp(src string) (string, *interp.RunError) {
	var stdout strings.Builder
	file := source.NewFile("bundle.spr", src)
	prog, _ := parser.Parse(file)
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	_, rerr := iv.Exec(file, prog)
	return stdout.String(), rerr
}

// runBundleVM runs bundle source on the virtual machine.
func runBundleVM(src string) (string, *vm.RunError) {
	var stdout strings.Builder
	file := source.NewFile("bundle.spr", src)
	prog, _ := parser.Parse(file)
	if diags := checker.Check(file, prog); len(diags) > 0 {
		return "", &vm.RunError{Message: diags[0].Message}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &vm.RunError{Message: err.Error()}
	}
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &strings.Builder{})
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}

// TestModuleCycleParity checks that both engines reject the same cycle.
func TestModuleCycleParity(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"main.spr":  "import \"a\"\nprint(\"done\")\n",
		"lib/a.spr": "import \"b\"\n",
		"lib/b.spr": "import \"a\"\n",
	})
	_, diags := module.Load(filepath.Join(dir, "main.spr"), module.LoadOptions{
		LibDirs: []string{filepath.Join(dir, "lib")},
	})
	found := false
	for _, d := range diags {
		if d.Severity == 0 && strings.Contains(d.Message, "import cycle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an import cycle error, got %v", diags)
	}
}

// TestModuleMissingParity checks that both engines report a missing module.
func TestModuleMissingParity(t *testing.T) {
	dir := writeProject(t, map[string]string{
		"main.spr": "import \"ghost\"\n",
	})
	_, diags := module.Load(filepath.Join(dir, "main.spr"), module.LoadOptions{})
	found := false
	for _, d := range diags {
		if d.Severity == 0 && strings.Contains(d.Message, "cannot find module") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a missing module error, got %v", diags)
	}
}

// TestExampleProjectRunsEverywhere runs the project example on both engines
// and on the generated bundle. All four paths must match the golden output.
func TestExampleProjectRunsEverywhere(t *testing.T) {
	root := filepath.Join("..", "examples", "project")
	m, err := module.LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	g, diags := module.Load(m.EntryPath(root), module.LoadOptions{LibDirs: m.LibDirs(root)})
	for _, d := range diags {
		if d.Severity == 0 {
			t.Fatalf("load: %s", d.Message)
		}
	}
	if diags := checker.CheckGraph(g); len(diags) > 0 {
		t.Fatalf("check: %s", diags[0].Message)
	}

	goldenData, err := os.ReadFile(filepath.Join("golden", "project.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := string(goldenData)

	ivOut, ivErr := runGraphInterp(g)
	if ivErr != nil {
		t.Fatalf("interpreter: %s", ivErr.Message)
	}
	if ivOut != want {
		t.Errorf("interpreter output differs:\n got:\n%q\nwant:\n%q", ivOut, want)
	}

	vmOut, vmErr := runGraphVM(g)
	if vmErr != nil {
		t.Fatalf("vm: %s", vmErr.Message)
	}
	if vmOut != want {
		t.Errorf("vm output differs:\n got:\n%q\nwant:\n%q", vmOut, want)
	}

	bundle, err := build.Bundle(g)
	if err != nil {
		t.Fatal(err)
	}
	bundleIV, rerr := runBundleInterp(bundle)
	if rerr != nil {
		t.Fatalf("bundle interpreter: %s", rerr.Message)
	}
	if bundleIV != want {
		t.Errorf("bundle interpreter output differs:\n got:\n%q\nwant:\n%q", bundleIV, want)
	}
	bundleVM, vErr := runBundleVM(bundle)
	if vErr != nil {
		t.Fatalf("bundle vm: %s", vErr.Message)
	}
	if bundleVM != want {
		t.Errorf("bundle vm output differs:\n got:\n%q\nwant:\n%q", bundleVM, want)
	}
}
