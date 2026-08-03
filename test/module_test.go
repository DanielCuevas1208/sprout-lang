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
	"github.com/sprout-lang/sprout/internal/mod"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

// writeSource writes a Sprout file and returns its path.
func writeSource(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runProject runs mainPath on the interpreter and returns its output and a
// runtime error when one occurs.
func runProject(path string, stdin string) (string, *interp.RunError) {
	text, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	file := source.NewFile(path, string(text))
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", &interp.RunError{Message: d.Message, File: file, Pos: d.Pos}
		}
	}
	if diags := checker.CheckWith(file, prog, mod.NewResolver()); len(diags) > 0 {
		return "", &interp.RunError{Message: diags[0].Message, File: file, Pos: diags[0].Pos}
	}
	var stdout, stderr strings.Builder
	iv := interp.NewWithIO(strings.NewReader(stdin), &stdout, &stderr)
	_, rerr := iv.Exec(file, prog)
	return stdout.String(), rerr
}

// runProjectVM runs mainPath on the bytecode virtual machine.
func runProjectVM(path string, stdin string) (string, *vm.RunError) {
	text, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	file := source.NewFile(path, string(text))
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", &vm.RunError{Message: d.Message, File: file, Pos: d.Pos}
		}
	}
	if diags := checker.CheckWith(file, prog, mod.NewResolver()); len(diags) > 0 {
		return "", &vm.RunError{Message: diags[0].Message, File: file, Pos: diags[0].Pos}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &vm.RunError{Message: err.Error(), File: file}
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(stdin), &stdout, &stderr)
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}

// TestModuleProjectMatchesGolden runs the multi-file example on both engines
// and compares the output with the recorded golden file.
func TestModuleProjectMatchesGolden(t *testing.T) {
	main := filepath.Join("..", "examples", "project", "main.spr")
	golden, err := os.ReadFile(filepath.Join("golden", "project.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := string(golden)

	got, rerr := runProject(main, "")
	if rerr != nil {
		t.Fatalf("interpreter: %s", rerr.Message)
	}
	if got != want {
		t.Errorf("interpreter output:\n got: %q\nwant: %q", got, want)
	}

	vmGot, vmErr := runProjectVM(main, "")
	if vmErr != nil {
		t.Fatalf("vm: %s", vmErr.Message)
	}
	if vmGot != want {
		t.Errorf("vm output:\n got: %q\nwant: %q", vmGot, want)
	}
}

// TestModuleEngineParity runs a temp-dir project on both engines and checks
// that their output and errors match.
func TestModuleEngineParity(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "lib.spr", `export fn twice(n) { return n * 2 }
export const GREETING = "hi"`)
	writeSource(t, dir, "main.spr", `import "./lib.spr"
print(lib.twice(21))
print(lib.GREETING)
print(type(lib))`)

	main := filepath.Join(dir, "main.spr")
	ivOut, ivErr := runProject(main, "")
	vmOut, vmErr := runProjectVM(main, "")
	if ivErr != nil {
		t.Fatalf("interpreter error: %s", ivErr.Message)
	}
	if vmErr != nil {
		t.Fatalf("vm error: %s", vmErr.Message)
	}
	if ivOut != vmOut {
		t.Errorf("engines differ:\ninterp: %q\n    vm: %q", ivOut, vmOut)
	}
	if ivOut != "42\nhi\nmodule\n" {
		t.Errorf("unexpected output: %q", ivOut)
	}
}

// TestModuleRuntimeErrorPointsAtModule checks that a runtime error inside a
// module is reported at the module's location, not at the import site.
func TestModuleRuntimeErrorPointsAtModule(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "boom.spr", `fn divide() {
	return 1 / 0
}
export fn start() { return divide() }`)
	writeSource(t, dir, "main.spr", `import "./boom.spr"
boom.start()`)

	main := filepath.Join(dir, "main.spr")
	_, ivErr := runProject(main, "")
	if ivErr == nil {
		t.Fatal("expected a runtime error")
	}
	if !strings.Contains(ivErr.Message, "cannot divide by zero") {
		t.Errorf("message: %s", ivErr.Message)
	}
	if !strings.Contains(ivErr.File.Name, "boom.spr") {
		t.Errorf("error file: %s, want boom.spr", ivErr.File.Name)
	}

	_, vmErr := runProjectVM(main, "")
	if vmErr == nil {
		t.Fatal("expected a runtime error on the vm")
	}
	if !strings.Contains(vmErr.Message, "cannot divide by zero") {
		t.Errorf("vm message: %s", vmErr.Message)
	}
	if !strings.Contains(vmErr.File.Name, "boom.spr") {
		t.Errorf("vm error file: %s, want boom.spr", vmErr.File.Name)
	}
}

// TestModuleNotFoundError checks that a missing module fails cleanly.
func TestModuleNotFoundError(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "main.spr", `import "./missing.spr"`)

	main := filepath.Join(dir, "main.spr")
	_, ivErr := runProject(main, "")
	if ivErr == nil {
		t.Fatal("expected an error for a missing module")
	}
	if !strings.Contains(ivErr.Message, "cannot find module") {
		t.Errorf("message: %s", ivErr.Message)
	}

	_, vmErr := runProjectVM(main, "")
	if vmErr == nil {
		t.Fatal("expected an error for a missing module on the vm")
	}
	if !strings.Contains(vmErr.Message, "cannot find module") {
		t.Errorf("vm message: %s", vmErr.Message)
	}
}

// TestModuleCycleError checks that an import cycle is reported.
func TestModuleCycleError(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "a.spr", `import "./b.spr"`)
	writeSource(t, dir, "b.spr", `import "./a.spr"`)
	writeSource(t, dir, "main.spr", `import "./a.spr"`)

	main := filepath.Join(dir, "main.spr")
	_, ivErr := runProject(main, "")
	if ivErr == nil {
		t.Fatal("expected a cycle error")
	}
	if !strings.Contains(ivErr.Message, "import cycle") {
		t.Errorf("message: %s", ivErr.Message)
	}
}

// TestImportAlias verifies that "import as" binds a chosen name.
func TestImportAlias(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "numbers.spr", "export fn triple(n) { return n * 3 }\n")
	writeSource(t, dir, "main.spr", `import "./numbers.spr" as n3
print(n3.triple(5))`)

	main := filepath.Join(dir, "main.spr")
	got, ivErr := runProject(main, "")
	if ivErr != nil {
		t.Fatalf("interpreter error: %s", ivErr.Message)
	}
	if got != "15\n" {
		t.Errorf("output: %q", got)
	}
	vmGot, vmErr := runProjectVM(main, "")
	if vmErr != nil {
		t.Fatalf("vm error: %s", vmErr.Message)
	}
	if vmGot != "15\n" {
		t.Errorf("vm output: %q", vmGot)
	}
}

// TestBuildAndRunArtifact builds the module project into a bytecode artifact
// and runs that artifact on the virtual machine.
func TestBuildAndRunArtifact(t *testing.T) {
	main := filepath.Join("..", "examples", "project", "main.spr")
	golden, err := os.ReadFile(filepath.Join("golden", "project.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := string(golden)

	out := filepath.Join(t.TempDir(), "project.sprb")
	_, errOut, code := runCLI(t, "", "build", "-o", out, main)
	if code != 0 {
		t.Fatalf("build failed (%d): %s", code, errOut)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("artifact not written: %v", err)
	}

	got, errOut, code := runCLI(t, "", "run", out)
	if code != 0 {
		t.Fatalf("run failed (%d): %s", code, errOut)
	}
	if got != want {
		t.Errorf("artifact output:\n got: %q\nwant: %q", got, want)
	}
}

// TestBuildFailsOnBadProject checks that a build reports project errors.
func TestBuildFailsOnBadProject(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "lib.spr", "export fn real() { return 1 }\n")
	writeSource(t, dir, "main.spr", `import "./lib.spr"
print(lib.ghost)`)

	out := filepath.Join(dir, "bad.sprb")
	_, errOut, code := runCLI(t, "", "build", "-o", out, filepath.Join(dir, "main.spr"))
	if code == 0 {
		t.Fatal("build should fail for a bad project")
	}
	if !strings.Contains(errOut, "does not export") {
		t.Errorf("stderr: %q", errOut)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("a failed build must not write an artifact")
	}
}
