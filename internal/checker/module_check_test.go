package checker

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

// moduleExports builds a fake export table for an imported module.
func moduleExports(names ...string) func(string) (map[string]source.Pos, bool) {
	return func(spec string) (map[string]source.Pos, bool) {
		exports := make(map[string]source.Pos)
		for _, n := range names {
			exports[n] = source.Pos{Line: 1, Column: 1, Offset: 0}
		}
		return exports, true
	}
}

// checkModule runs CheckContext as a module file.
func checkModule(t *testing.T, exports func(string) (map[string]source.Pos, bool), src string) []string {
	t.Helper()
	file := source.NewFile("mod.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, "parse: "+d.Message)
		}
		return msgs
	}
	var msgs []string
	for _, d := range CheckContext(Context{IsModule: true, ModuleExports: exports}, file, prog) {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

// expectModuleError checks that a module file produces an error containing want.
func expectModuleError(t *testing.T, exports func(string) (map[string]source.Pos, bool), src, want string) {
	t.Helper()
	msgs := checkModule(t, exports, src)
	found := false
	for _, m := range msgs {
		if strings.Contains(m, want) {
			found = true
		}
	}
	if !found {
		t.Errorf("module check %q: expected error containing %q, got %v", src, want, msgs)
	}
}

func TestImportDeclaresModule(t *testing.T) {
	msgs := checkModule(t, moduleExports("greet"), "import \"util\"\nprint(util.greet())\n")
	if len(msgs) != 0 {
		t.Errorf("expected clean import, got %v", msgs)
	}
}

func TestImportAlias(t *testing.T) {
	msgs := checkModule(t, moduleExports("x"), "import \"util\" as u\nprint(u.x)\n")
	if len(msgs) != 0 {
		t.Errorf("expected clean aliased import, got %v", msgs)
	}
}

func TestImportUnexportedMember(t *testing.T) {
	expectModuleError(t, moduleExports("greet"),
		"import \"util\"\nprint(util.secret)\n",
		"has no exported member 'secret'")
}

func TestImportExportedMember(t *testing.T) {
	msgs := checkModule(t, moduleExports("greet", "secret"), "import \"util\"\nprint(util.secret)\n")
	if len(msgs) != 0 {
		t.Errorf("expected exported member to pass, got %v", msgs)
	}
}

func TestImportMissingModule(t *testing.T) {
	expectModuleError(t, nil, "import \"ghost\"\n", "cannot find module 'ghost'")
}

func TestImportInsideFunction(t *testing.T) {
	expectModuleError(t, moduleExports("x"),
		"fn f() { import \"util\" }\n",
		"top level")
}

func TestImportDuplicateName(t *testing.T) {
	expectModuleError(t, moduleExports("x"),
		"import \"a\"\nimport \"a\"\n",
		"duplicate declaration")
}

func TestExportAllowedAtTopLevel(t *testing.T) {
	msgs := checkModule(t, moduleExports(), "export let x = 1\nexport fn f() { return 2 }\n")
	if len(msgs) != 0 {
		t.Errorf("expected exports to pass in a module, got %v", msgs)
	}
}

func TestExportInsideFunction(t *testing.T) {
	expectModuleError(t, moduleExports(), "fn f() { export let x = 1 }\n", "top level")
}

func TestMemberOnNonModuleValue(t *testing.T) {
	// A runtime module value cannot be validated statically. The checker
	// leaves it to the runtime rather than rejecting a valid bundle.
	msgs := checkModule(t, moduleExports("x"),
		"let m = module(\"m\", {})\nprint(m.x)\n")
	if len(msgs) != 0 {
		t.Errorf("expected dynamic member access to pass, got %v", msgs)
	}
}
