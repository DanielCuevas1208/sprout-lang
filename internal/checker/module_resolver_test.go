// Package checker_test exercises the checker against real module projects.
//
// It lives outside the checker package because it needs the mod resolver,
// and mod imports checker.
package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/mod"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

func writeFile(t *testing.T, dir, name, src string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checkProject(t *testing.T, dir, src string) []string {
	t.Helper()
	file := source.NewFile(filepath.Join(dir, "main.spr"), src)
	prog, parseDiags := parser.Parse(file)
	diags := append([]diag.Diagnostic{}, parseDiags...)
	diags = append(diags, checker.CheckWith(file, prog, mod.NewResolver())...)
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func find(t *testing.T, msgs []string, want string) {
	t.Helper()
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return
		}
	}
	t.Errorf("expected a message containing %q, got %v", want, msgs)
}

func TestCheckValidProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "util.spr", "export fn shout(s) { return upper(s) }\n")
	msgs := checkProject(t, dir, `import "./util.spr"
print(util.shout("hi"))`)
	if len(msgs) != 0 {
		t.Errorf("expected a clean project, got %v", msgs)
	}
}

func TestCheckUnexportedMember(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "util.spr", "export fn shout(s) { return s }\n")
	msgs := checkProject(t, dir, `import "./util.spr"
print(util.whisper("hi"))`)
	find(t, msgs, "does not export 'whisper'")
}

func TestCheckMissingModule(t *testing.T) {
	dir := t.TempDir()
	msgs := checkProject(t, dir, `import "./gone.spr"`)
	find(t, msgs, "cannot find module")
}

func TestCheckTransitiveError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "deep.spr", "let oops = missing\n")
	writeFile(t, dir, "mid.spr", `import "./deep.spr"`)
	msgs := checkProject(t, dir, `import "./mid.spr"`)
	find(t, msgs, "undefined name 'missing'")
}

func TestCheckImportCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.spr", `import "./b.spr"`)
	writeFile(t, dir, "b.spr", `import "./a.spr"`)
	msgs := checkProject(t, dir, `import "./a.spr"`)
	find(t, msgs, "import cycle")
}
