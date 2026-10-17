package vm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// runWithModules compiles and runs src on the VM from a temporary directory
// that also holds the given module files.
func runWithModules(t *testing.T, src string, files map[string]string) (string, *RunError) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entry := filepath.Join(dir, "main.spr")
	file := source.NewFile(entry, src)
	prog, diags := parser.Parse(file)
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return "", &RunError{Message: "parse error: " + d.Message}
		}
	}
	if cdiags := checker.Check(file, prog); len(cdiags) > 0 {
		return "", &RunError{Message: "check error: " + cdiags[0].Message}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", &RunError{Message: "compile error: " + err.Error()}
	}
	var stdout, stderr strings.Builder
	machine := NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(file, compiled)
	return stdout.String(), rerr
}

func expectVMModuleOutput(t *testing.T, src string, files map[string]string, want string) {
	t.Helper()
	got, rerr := runWithModules(t, src, files)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != want {
		t.Errorf("output:\n got: %q\nwant: %q", got, want)
	}
}

func expectVMModuleError(t *testing.T, src string, files map[string]string, want string) {
	t.Helper()
	_, rerr := runWithModules(t, src, files)
	if rerr == nil {
		t.Fatalf("expected error containing %q, got none", want)
	}
	if !strings.Contains(rerr.Message, want) {
		t.Errorf("error %q does not contain %q", rerr.Message, want)
	}
}

func TestVMImportBindsExports(t *testing.T) {
	files := map[string]string{
		"math.spr": "export fn square(x) { return x * x }\nexport const answer = 42\n",
	}
	expectVMModuleOutput(t, `
import "math"
print(math["square"](7))
print(math["answer"])
`, files, "49\n42\n")
}

func TestVMImportWithAlias(t *testing.T) {
	files := map[string]string{
		"tools.spr": "export fn double(x) { return x * 2 }\n",
	}
	expectVMModuleOutput(t, `
import "tools.spr" as t
print(t["double"](21))
`, files, "42\n")
}

func TestVMModuleStateIsShared(t *testing.T) {
	files := map[string]string{
		"bank.spr": "let balance = 0\nexport fn deposit(amount) { balance = balance + amount }\nexport fn total() { return balance }\n",
	}
	expectVMModuleOutput(t, `
import "bank"
bank["deposit"](10)
bank["deposit"](5)
print(bank["total"]())
`, files, "15\n")
}

func TestVMModulePrivatesAreHidden(t *testing.T) {
	files := map[string]string{
		"bank.spr": "let balance = 0\nexport fn deposit(amount) { balance = balance + amount }\nexport fn total() { return balance }\nfn hidden() { return 99 }\n",
	}
	expectVMModuleOutput(t, `
import "bank"
print(bank["balance"])
print(bank["hidden"])
print(bank["deposit"] == nil)
`, files, "nil\nnil\nfalse\n")
}

func TestVMModuleRunsOnce(t *testing.T) {
	files := map[string]string{
		"log.spr": "print(\"module start\")\nexport let x = 1\n",
	}
	expectVMModuleOutput(t, `
import "log"
import "log" as again
print("done")
`, files, "module start\ndone\n")
}

func TestVMNestedImports(t *testing.T) {
	files := map[string]string{
		"lib/outer.spr": `import "inner"
export fn go() { return inner["value"] }`,
		"lib/inner.spr": "export let value = 7\n",
	}
	expectVMModuleOutput(t, `
import "lib/outer"
print(outer["go"]())
`, files, "7\n")
}

func TestVMCircularImportIsRejected(t *testing.T) {
	files := map[string]string{
		"a.spr": `import "b"`,
		"b.spr": `import "a"`,
	}
	expectVMModuleError(t, `import "a"`, files, "circular import")
}

func TestVMMissingModuleIsRejected(t *testing.T) {
	expectVMModuleError(t, `import "missing"`, nil, "cannot read module")
}

func TestVMModuleTypeAndReadOnly(t *testing.T) {
	files := map[string]string{
		"m.spr": "export let x = 1\n",
	}
	expectVMModuleOutput(t, `
import "m"
print(type(m))
print(m["x"])
`, files, "module\n1\n")

	_, rerr := runWithModules(t, `
import "m"
m["x"] = 99
`, files)
	if rerr == nil {
		t.Fatal("expected an error for writing into a module")
	}
	if !strings.Contains(rerr.Message, "module") {
		t.Errorf("error %q does not mention the module", rerr.Message)
	}
}
