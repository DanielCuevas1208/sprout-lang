package interp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// writeModule writes a Sprout file inside dir, creating parent directories.
func writeModule(t *testing.T, dir, name, src string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runWithModules runs src as a file in a temporary directory that also holds
// the given module files.
func runWithModules(t *testing.T, src string, files map[string]string) (string, *RunError) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeModule(t, dir, name, content)
	}
	io := &testIO{}
	iv := NewWithIO(strings.NewReader(""), &io.out, &io.err)
	entry := filepath.Join(dir, "main.spr")
	file := source.NewFile(entry, src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		t.Fatalf("parse errors: %v", diags)
	}
	_, rerr := iv.Exec(file, prog)
	return io.out.String(), rerr
}

func expectModuleOutput(t *testing.T, src string, files map[string]string, want string) {
	t.Helper()
	got, rerr := runWithModules(t, src, files)
	if rerr != nil {
		t.Fatalf("runtime error: %s", rerr.Message)
	}
	if got != want {
		t.Errorf("output:\n got: %q\nwant: %q", got, want)
	}
}

func expectModuleError(t *testing.T, src string, files map[string]string, want string) {
	t.Helper()
	_, rerr := runWithModules(t, src, files)
	if rerr == nil {
		t.Fatalf("expected error containing %q, got none", want)
	}
	if !strings.Contains(rerr.Message, want) {
		t.Errorf("error %q does not contain %q", rerr.Message, want)
	}
}

func TestImportBindsExports(t *testing.T) {
	files := map[string]string{
		"math.spr": "export fn square(x) { return x * x }\nexport const answer = 42\n",
	}
	expectModuleOutput(t, `
import "math"
print(math["square"](7))
print(math["answer"])
`, files, "49\n42\n")
}

func TestImportWithAlias(t *testing.T) {
	files := map[string]string{
		"tools.spr": "export fn double(x) { return x * 2 }\n",
	}
	expectModuleOutput(t, `
import "tools.spr" as t
print(t["double"](21))
`, files, "42\n")
}

func TestModulePrivatesAreHidden(t *testing.T) {
	files := map[string]string{
		"bank.spr": "let balance = 0\nexport fn deposit(amount) { balance = balance + amount }\nexport fn total() { return balance }\nfn hidden() { return 99 }\n",
	}
	// Private names, both values and functions, stay hidden from importers.
	expectModuleOutput(t, `
import "bank"
print(bank["balance"])
print(bank["hidden"])
print(bank["deposit"] == nil)
`, files, "nil\nnil\nfalse\n")
}

func TestModuleStateIsShared(t *testing.T) {
	files := map[string]string{
		"bank.spr": "let balance = 0\nexport fn deposit(amount) { balance = balance + amount }\nexport fn total() { return balance }\n",
	}
	// Exported functions share the module's scope, so state persists.
	expectModuleOutput(t, `
import "bank"
bank["deposit"](10)
bank["deposit"](5)
print(bank["total"]())
`, files, "15\n")
}

func TestModuleRunsOnce(t *testing.T) {
	files := map[string]string{
		"log.spr": "print(\"module start\")\nexport let x = 1\n",
	}
	expectModuleOutput(t, `
import "log"
import "log"
import "log" as again
print("done")
`, files, "module start\ndone\n")
}

func TestModuleIdentityIsStable(t *testing.T) {
	files := map[string]string{
		"m.spr": "export let x = 1\n",
	}
	// Every import of one module shares the same value.
	expectModuleOutput(t, `
import "m"
import "m" as same
print(m == same)
`, files, "true\n")
}

func TestNestedImports(t *testing.T) {
	files := map[string]string{
		"lib/outer.spr": `import "inner"
export fn go() { return inner["value"] }`,
		"lib/inner.spr": "export let value = 7\n",
	}
	expectModuleOutput(t, `
import "lib/outer"
print(outer["go"]())
`, files, "7\n")
}

func TestModuleErrors(t *testing.T) {
	expectModuleError(t, `import "missing"`, nil, "cannot read module")
	expectModuleError(t, `
import "a"
`, map[string]string{"a.spr": "fn broken( { }"}, "module")
	expectModuleError(t, `
import "a"
`, map[string]string{"a.spr": "export let x = undefined_name\n"}, "undefined name")
}

func TestCircularImportIsRejected(t *testing.T) {
	files := map[string]string{
		"a.spr": `import "b"`,
		"b.spr": `import "a"`,
	}
	expectModuleError(t, `import "a"`, files, "circular import")
}

func TestModuleTypeAndReadOnly(t *testing.T) {
	files := map[string]string{
		"m.spr": "export let x = 1\n",
	}
	expectModuleOutput(t, `
import "m"
print(type(m))
print(m["x"])
`, files, "module\n1\n")

	// Index writes into a module are rejected.
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

func TestImportedFunctionCallsHelpers(t *testing.T) {
	files := map[string]string{
		"area.spr": "fn half(n) { return n / 2 }\nexport fn circle_area(r) { return 3 * half(r) * r }\n",
	}
	// Exported functions can call the module's private helpers.
	expectModuleOutput(t, `
import "area"
print(area["circle_area"](4))
`, files, "24\n")
}
