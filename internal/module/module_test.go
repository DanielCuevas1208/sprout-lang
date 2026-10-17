package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/diag"
)

// writeFiles writes a map of relative paths to contents inside dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// errorMessages extracts the message of every error diagnostic.
func errorMessages(diags []diag.Diagnostic) []string {
	var msgs []string
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			msgs = append(msgs, d.Message)
		}
	}
	return msgs
}

// contains reports whether msgs holds a message containing want.
func contains(msgs []string, want string) bool {
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// loadEntry writes files, loads the entry, and returns the graph.
//
// Files under "lib/" are reachable through the project's lib directory.
func loadEntry(t *testing.T, files map[string]string, entry string) *Graph {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, files)
	g, diags := Load(filepath.Join(dir, entry), LoadOptions{
		LibDirs: []string{filepath.Join(dir, "lib")},
	})
	if hasErrors(diags) {
		t.Fatalf("load %s: unexpected errors: %v", entry, errorMessages(diags))
	}
	return g
}

func TestLoadCollectsExports(t *testing.T) {
	g := loadEntry(t, map[string]string{
		"main.spr":     "import \"util\"\nprint(util.greet())\n",
		"lib/util.spr": "export let NAME = \"sprout\"\nexport fn greet() { return \"hi\" }\nlet hidden = 1\n",
	}, "main.spr")

	if len(g.order) != 2 {
		t.Fatalf("graph order: got %d files, want 2", len(g.order))
	}
	util := g.order[1]
	if util.Spec != "util" {
		t.Errorf("module spec: got %q, want %q", util.Spec, "util")
	}
	if len(util.ExportNames) != 2 {
		t.Fatalf("export names: got %v", util.ExportNames)
	}
	if util.ExportNames[0] != "NAME" || util.ExportNames[1] != "greet" {
		t.Errorf("export names in declaration order: got %v", util.ExportNames)
	}
	if _, ok := util.Exports["hidden"]; ok {
		t.Error("hidden name should not be exported")
	}
}

func TestLoadLinking(t *testing.T) {
	g := loadEntry(t, map[string]string{
		"main.spr":  "import \"a\"\nimport \"b\"\n",
		"lib/a.spr": "import \"b\"\n",
		"lib/b.spr": "",
	}, "main.spr")

	main := g.order[0]
	if len(main.Imports) != 2 {
		t.Fatalf("main imports: got %d, want 2", len(main.Imports))
	}
	aFile := main.Imports[0].Target
	bFile := main.Imports[1].Target
	if aFile == bFile {
		t.Fatal("a and b should be distinct files")
	}
	// main imports "b", and a imports "b". Both link to the same file.
	if len(aFile.Imports) != 1 {
		t.Fatalf("a imports: got %d, want 1", len(aFile.Imports))
	}
	if aFile.Imports[0].Target != bFile {
		t.Error("main and a should share the same b file")
	}
	// b.spr loads once, so the graph holds three distinct files.
	seen := map[*File]bool{}
	for _, f := range g.order {
		seen[f] = true
	}
	if len(seen) != 3 {
		t.Errorf("distinct files: got %d, want 3", len(seen))
	}
}

func TestTopoOrdersDependenciesFirst(t *testing.T) {
	g := loadEntry(t, map[string]string{
		"main.spr":  "import \"a\"\n",
		"lib/a.spr": "import \"b\"\n",
		"lib/b.spr": "import \"c\"\n",
		"lib/c.spr": "",
	}, "main.spr")

	names := make([]string, 0, len(g.order))
	for _, f := range g.Topo() {
		names = append(names, BaseName(f.Path))
	}
	want := []string{"c", "b", "a", "main"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("topo order: got %v, want %v", names, want)
	}
}

func TestLoadMissingModule(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"main.spr": "import \"ghost\"\n"})
	g, diags := Load(filepath.Join(dir, "main.spr"), LoadOptions{})
	if g == nil {
		t.Fatal("graph should still exist for a missing module")
	}
	if !contains(errorMessages(diags), "cannot find module 'ghost'") {
		t.Errorf("expected missing module error, got %v", errorMessages(diags))
	}
}

func TestLoadDetectsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.spr":  "import \"a\"\n",
		"lib/a.spr": "import \"b\"\n",
		"lib/b.spr": "import \"a\"\n",
	})
	g, diags := Load(filepath.Join(dir, "main.spr"), LoadOptions{LibDirs: []string{filepath.Join(dir, "lib")}})
	if g == nil {
		t.Fatal("graph should still exist for a cyclic import")
	}
	if !contains(errorMessages(diags), "import cycle") {
		t.Errorf("expected cycle error, got %v", errorMessages(diags))
	}
}

func TestLoadSelfImportCycle(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.spr": "import \"main\"\n",
	})
	_, diags := Load(filepath.Join(dir, "main.spr"), LoadOptions{})
	if !contains(errorMessages(diags), "import cycle") {
		t.Errorf("expected self-import cycle error, got %v", errorMessages(diags))
	}
}

func TestLoadParseErrorInModule(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.spr":   "import \"broken\"\n",
		"broken.spr": "let x = =\n",
	})
	_, diags := Load(filepath.Join(dir, "main.spr"), LoadOptions{})
	if !contains(errorMessages(diags), "expected") {
		t.Errorf("expected parse error from module, got %v", errorMessages(diags))
	}
}

func TestLoadEntryNotReadable(t *testing.T) {
	dir := t.TempDir()
	_, diags := Load(filepath.Join(dir, "missing.spr"), LoadOptions{})
	if !contains(errorMessages(diags), "cannot read") {
		t.Errorf("expected read error, got %v", errorMessages(diags))
	}
}

func TestResolverRelativeAndBare(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"main.spr":     "",
		"lib/util.spr": "",
	})
	r := NewResolver([]string{filepath.Join(dir, "lib")})

	// A dot specifier resolves relative to the importing directory.
	rel, ok := r.Resolve("./lib/util", dir)
	if !ok || rel != filepath.Join(dir, "lib", "util.spr") {
		t.Errorf("relative resolve: got %q, %v", rel, ok)
	}

	// A bare specifier searches the importing directory, then lib dirs.
	bare, ok := r.Resolve("util", filepath.Join(dir, "anything"))
	if !ok || bare != filepath.Join(dir, "lib", "util.spr") {
		t.Errorf("bare resolve: got %q, %v", bare, ok)
	}

	// From the project root, the bare specifier still reaches the lib dir.
	if _, ok := r.Resolve("util", dir); !ok {
		t.Error("bare specifier should fall back to the lib dirs")
	}
}

func TestResolverAddsSuffix(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"lib/extra.spr": ""})
	r := NewResolver([]string{filepath.Join(dir, "lib")})
	p, ok := r.Resolve("extra", dir)
	if !ok || p != filepath.Join(dir, "lib", "extra.spr") {
		t.Errorf("suffix resolve: got %q, %v", p, ok)
	}
}

func TestModuleName(t *testing.T) {
	cases := map[string]string{
		"lib/util":       "util",
		"./lib/util.spr": "util",
		"util.spr":       "util",
		"/":              "module",
		"":               "module",
	}
	for spec, want := range cases {
		if got := ModuleName(spec); got != want {
			t.Errorf("ModuleName(%q): got %q, want %q", spec, got, want)
		}
	}
}

func TestGraphKeyIsStable(t *testing.T) {
	g := loadEntry(t, map[string]string{
		"main.spr": "import \"util\"\n",
		"util.spr": "",
	}, "main.spr")
	mainKey, utilKey := g.Key(g.order[0]), g.Key(g.order[1])
	if mainKey != "module000" || utilKey != "module001" {
		t.Errorf("keys: got %q, %q", mainKey, utilKey)
	}
}

func TestManifestDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sprout.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != filepath.Base(dir) {
		t.Errorf("default name: got %q", m.Name)
	}
	if m.Entry != "main.spr" {
		t.Errorf("default entry: got %q", m.Entry)
	}
}

func TestFindManifestWalksUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sprout.toml"), []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root, m, err := FindManifest(sub)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("project root: got %q, want %q", root, dir)
	}
	if m.Name != "demo" {
		t.Errorf("manifest name: got %q", m.Name)
	}
}

func TestManifestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := DefaultManifest("roundtrip")
	if err := m.Write(dir); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "roundtrip" || got.Entry != "main.spr" {
		t.Errorf("round trip manifest: got %+v", got)
	}
	if len(got.Lib) != 1 || got.Lib[0] != "lib" {
		t.Errorf("round trip lib: got %v", got.Lib)
	}
}

func TestExportNamesDeterministic(t *testing.T) {
	g := loadEntry(t, map[string]string{
		"main.spr":  "import \"m\"\n",
		"lib/m.spr": "export fn z() { return 1 }\nexport let a = 2\nexport fn m() { return 3 }\n",
	}, "main.spr")
	m := g.order[1]
	want := []string{"z", "a", "m"}
	if len(m.ExportNames) != len(want) {
		t.Fatalf("export names: got %v", m.ExportNames)
	}
	for i := range want {
		if m.ExportNames[i] != want[i] {
			t.Errorf("export order: got %v, want %v", m.ExportNames, want)
		}
	}
}
