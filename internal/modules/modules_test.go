package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes files under a fresh temp directory and returns its path.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadEntry(t *testing.T, dir, entry string) *Graph {
	t.Helper()
	g, err := (&Loader{}).Load(filepath.Join(dir, entry))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestLoadSingleFile(t *testing.T) {
	dir := writeTree(t, map[string]string{"app.spr": "print(1)\n"})
	g := loadEntry(t, dir, "app.spr")
	if len(g.Units) != 1 {
		t.Fatalf("unit count: %d", len(g.Units))
	}
	if g.Entry != 0 {
		t.Errorf("entry index: %d", g.Entry)
	}
	if g.HasErrors() {
		t.Errorf("errors: %v", g.Diags())
	}
}

func TestLoadModuleGraph(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr":      "import \"lib/math\" as m\nprint(m.square(2))\n",
		"lib/math.spr": "fn square(n) { return n * n }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	if len(g.Units) != 2 {
		t.Fatalf("unit count: %d", len(g.Units))
	}
	// The entry is last; the module comes first.
	if g.Entry != 1 {
		t.Fatalf("entry index: %d", g.Entry)
	}
	entry := g.Units[g.Entry]
	if len(entry.Imports) != 1 {
		t.Fatalf("import count: %d", len(entry.Imports))
	}
	if entry.Imports[0].Alias != "m" || entry.Imports[0].Target != 0 {
		t.Errorf("import: %+v", entry.Imports[0])
	}
	if got := ExportedNames(g.Units[0].Prog); len(got) != 1 || got[0] != "square" {
		t.Errorf("exports: %v", got)
	}
}

func TestLoadDirectoryModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr":           "import \"lib/math\"\nprint(math.square(3))\n",
		"lib/math/main.spr": "fn square(n) { return n * n }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	if len(g.Units) != 2 {
		t.Fatalf("unit count: %d", len(g.Units))
	}
}

func TestLoadDefaultAlias(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr":  "import \"math\"\nprint(math.square(2))\n",
		"math.spr": "fn square(n) { return n * n }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	alias := g.Units[g.Entry].Imports[0].Alias
	if alias != "math" {
		t.Errorf("default alias: %q", alias)
	}
}

func TestLoadSharedModule(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr": "import \"a\"\nimport \"b\"\nprint(a.f(), b.g())\n",
		"a.spr":   "import \"lib\" as l\nfn f() { return l.value }\n",
		"b.spr":   "import \"lib\" as l\nfn g() { return l.value }\n",
		"lib.spr": "let value = 42\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	// lib loads once even though two modules import it.
	if len(g.Units) != 4 {
		t.Fatalf("unit count: %d (lib should load once)", len(g.Units))
	}
	// lib is the first unit, so both a and b point at it.
	if g.Units[1].Imports[0].Target != 0 || g.Units[2].Imports[0].Target != 0 {
		t.Errorf("shared target not deduplicated: %v %v", g.Units[1].Imports[0].Target, g.Units[2].Imports[0].Target)
	}
}

func TestLoadMissingModule(t *testing.T) {
	dir := writeTree(t, map[string]string{"app.spr": "import \"nowhere\"\nprint(1)\n"})
	g := loadEntry(t, dir, "app.spr")
	if !g.HasErrors() {
		t.Fatal("expected an error")
	}
	found := false
	for _, d := range g.Diags() {
		if strings.Contains(d.Message, "cannot find module \"nowhere\"") {
			found = true
		}
	}
	if !found {
		t.Errorf("diags: %v", g.Diags())
	}
}

func TestLoadImportCycle(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr": "import \"a\"\nprint(a.f())\n",
		"a.spr":   "import \"b\" as b\nfn f() { return b.g() }\n",
		"b.spr":   "import \"a\" as a\nfn g() { return 1 }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if !g.HasErrors() {
		t.Fatal("expected a cycle error")
	}
	found := false
	for _, d := range g.Diags() {
		if strings.Contains(d.Message, "import cycle") {
			found = true
		}
	}
	if !found {
		t.Errorf("diags: %v", g.Diags())
	}
}

func TestLoadSelfImportCycle(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr": "import \"a\"\nprint(1)\n",
		"a.spr":   "import \"a\"\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if !g.HasErrors() {
		t.Fatal("expected a cycle error")
	}
}

func TestLoadEntryDirFallback(t *testing.T) {
	// A nested module resolves a bare spec against the entry directory.
	dir := writeTree(t, map[string]string{
		"app.spr":       "import \"lib/stats\"\nprint(1)\n",
		"lib/stats.spr": "import \"lib/math\" as m\nfn total() { return m.square(2) }\n",
		"lib/math.spr":  "fn square(n) { return n * n }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	if len(g.Units) != 3 {
		t.Fatalf("unit count: %d", len(g.Units))
	}
}

func TestLoadSearchPath(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr": "import \"shared/math\"\nprint(math.square(2))\n",
	})
	lib := writeTree(t, map[string]string{"shared/math.spr": "fn square(n) { return n * n }\n"})
	l := &Loader{SearchPaths: []string{lib}}
	g, err := l.Load(filepath.Join(dir, "app.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if g.HasErrors() {
		t.Fatalf("errors: %v", g.Diags())
	}
	if len(g.Units) != 2 {
		t.Fatalf("unit count: %d", len(g.Units))
	}
}

func TestEntryMissing(t *testing.T) {
	dir := writeTree(t, map[string]string{"app.spr": "print(1)\n"})
	if _, err := (&Loader{}).Load(filepath.Join(dir, "nope.spr")); err == nil {
		t.Fatal("expected an error for a missing entry")
	}
}

func TestExportedNamesSkipsImports(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr": "import \"x\" as x\nlet a = 1\nfn b() { }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	names := ExportedNames(g.Units[g.Entry].Prog)
	if strings.Join(names, ",") != "a,b" {
		t.Errorf("exported names: %v", names)
	}
}

func TestMemberErrorDetectedAtCheck(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.spr":  "import \"math\"\nprint(math.nope)\n",
		"math.spr": "fn square(n) { return n * n }\n",
	})
	g := loadEntry(t, dir, "app.spr")
	if !g.HasErrors() {
		t.Fatal("expected a check error for a missing member")
	}
	found := false
	for _, d := range g.Diags() {
		if strings.Contains(d.Message, "module 'math' has no member 'nope'") {
			found = true
		}
	}
	if !found {
		t.Errorf("diags: %v", g.Diags())
	}
}
