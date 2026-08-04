package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/vm"
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

// moduleProgram builds a small module graph for the tests.
func moduleProgram(t *testing.T, dir string) {
	writeModule(t, dir, "lib/greetings.spr",
		"export fn greet(name) { return \"hello, \" + name }\nexport const punc = \"!\"\n")
	writeModule(t, dir, "lib/stat.spr",
		"import \"greetings.spr\"\nexport fn shout(name) { return greet(name) + punc }\n")
}

// runBundleInterp loads the bundle rooted at entry and runs it on the
// interpreter.
func runBundleInterp(t *testing.T, entry string) (string, *interp.RunError) {
	t.Helper()
	bundle, diags := module.Load(entry)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("load error: %s", d.Message)
		}
	}
	if cdiags := checker.CheckBundle(bundle); len(cdiags) > 0 {
		t.Fatalf("check error: %s", cdiags[0].Message)
	}
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := iv.ExecBundle(bundle)
	return stdout.String(), rerr
}

// runBundleVM loads the bundle rooted at entry and runs it on the VM.
func runBundleVM(t *testing.T, entry string) (string, *vm.RunError) {
	t.Helper()
	bundle, diags := module.Load(entry)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("load error: %s", d.Message)
		}
	}
	if cdiags := checker.CheckBundle(bundle); len(cdiags) > 0 {
		t.Fatalf("check error: %s", cdiags[0].Message)
	}
	compiled, err := compiler.CompileBundle(bundle)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(bundle.Source(), compiled)
	return stdout.String(), rerr
}

// TestModulesAgree runs module programs on both engines and checks that the
// output matches. Modules must behave identically on the interpreter and the
// bytecode virtual machine.
func TestModulesAgree(t *testing.T) {
	cases := []string{
		// Direct import of all exports.
		`import "lib/greetings.spr"
print(greet("world") + punc)`,
		// Alias import.
		`import calc from "lib/calc.spr"
print(calc["add"](2, 3))
print(calc["mul"](3, 4))`,
		// Transitive imports through a shared module.
		`import "lib/stat.spr"
print(shout("deep"))`,
		// A module is initialized once even when shared.
		`import "lib/stat.spr"
import "lib/greetings.spr"
print(shout("a"), greet("b"))`,
		// Imported closures keep module state.
		`import counter from "lib/counter.spr"
print(counter["next"](), counter["next"]())`,
		// The import can follow the use, because imports are hoisted.
		`print(greet("early"))
import "lib/greetings.spr"`,
	}

	for _, src := range cases {
		dir := t.TempDir()
		moduleProgram(t, dir)
		writeModule(t, dir, "lib/calc.spr",
			"export fn add(a, b) { return a + b }\nexport fn mul(a, b) { return a * b }\n")
		writeModule(t, dir, "lib/counter.spr",
			"let count = 0\n"+
				"export fn next() {\n"+
				"\tcount = count + 1\n"+
				"\treturn count\n"+
				"}\n")
		writeModule(t, dir, "main.spr", src)

		entry := filepath.Join(dir, "main.spr")
		ivOut, ivErr := runBundleInterp(t, entry)
		vmOut, vmErr := runBundleVM(t, entry)
		if ivErr != nil {
			t.Errorf("interpreter error for:\n%s\n  %v", src, ivErr)
			continue
		}
		if vmErr != nil {
			t.Errorf("vm error for:\n%s\n  %v", src, vmErr)
			continue
		}
		if ivOut != vmOut {
			t.Errorf("engines differ for:\n%s\ninterp: %q\n    vm: %q", src, ivOut, vmOut)
		}
	}
}

// TestModulesAgreeOnErrors checks that module errors stop both engines with
// the same message.
func TestModulesAgreeOnErrors(t *testing.T) {
	dir := t.TempDir()
	moduleProgram(t, dir)
	writeModule(t, dir, "lib/boom.spr",
		"export fn kaboom() {\n\treturn 1 / 0\n}\n")
	writeModule(t, dir, "main.spr",
		"import \"lib/boom.spr\"\nprint(kaboom())\n")

	_, ivErr := runBundleInterp(t, filepath.Join(dir, "main.spr"))
	_, vmErr := runBundleVM(t, filepath.Join(dir, "main.spr"))
	if ivErr == nil || vmErr == nil {
		t.Fatalf("expected both engines to fail, interp: %v, vm: %v", ivErr, vmErr)
	}
	if ivErr.Error() != vmErr.Error() {
		t.Errorf("error messages differ: interp %q, vm %q", ivErr.Error(), vmErr.Error())
	}
}

// TestBuildProducesRunnableBundle exercises the whole build flow: it renders
// a bundle, writes it to disk, and runs the result on both engines.
func TestBuildProducesRunnableBundle(t *testing.T) {
	dir := t.TempDir()
	moduleProgram(t, dir)
	writeModule(t, dir, "main.spr", `import "lib/stat.spr"
print(shout("built"))`)

	bundle, diags := module.Load(filepath.Join(dir, "main.spr"))
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			t.Fatalf("load error: %s", d.Message)
		}
	}
	outPath := filepath.Join(dir, "dist", "main.bundle.spr")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte(bundle.Render()), 0o644); err != nil {
		t.Fatal(err)
	}

	// The bundle is a single file, so it runs without the source modules.
	ivOut, ivErr := runBundleInterp(t, outPath)
	if ivErr != nil {
		t.Fatalf("interpreter error on bundle: %v", ivErr)
	}
	if !strings.Contains(ivOut, "hello, built!") {
		t.Errorf("bundle output: %q", ivOut)
	}
	vmOut, vmErr := runBundleVM(t, outPath)
	if vmErr != nil {
		t.Fatalf("vm error on bundle: %v", vmErr)
	}
	if !strings.Contains(vmOut, "hello, built!") {
		t.Errorf("bundle vm output: %q", vmOut)
	}
}
