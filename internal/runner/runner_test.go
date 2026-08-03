package runner

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

// moduleProgram returns the entry path of a two-file program.
func moduleProgram(t *testing.T) string {
	t.Helper()
	dir := writeTree(t, map[string]string{
		"math.spr": `fn square(n) { return n * n }
fn double(n) { return n * 2 }`,
		"app.spr": "import \"math\" as m\nprint(m.square(5))\nprint(m.double(21))\n",
	})
	return filepath.Join(dir, "app.spr")
}

func TestRunInterpModules(t *testing.T) {
	g, diags := Load(moduleProgram(t))
	if len(diags) > 0 {
		t.Fatalf("load errors: %v", diags)
	}
	var stdout, stderr strings.Builder
	if rerr := RunInterp(g, strings.NewReader(""), &stdout, &stderr); rerr != nil {
		t.Fatalf("run: %s", rerr.Message)
	}
	if stdout.String() != "25\n42\n" {
		t.Errorf("output: %q", stdout.String())
	}
}

func TestRunVMModules(t *testing.T) {
	g, diags := Load(moduleProgram(t))
	if len(diags) > 0 {
		t.Fatalf("load errors: %v", diags)
	}
	var stdout, stderr strings.Builder
	if rerr := RunVM(g, strings.NewReader(""), &stdout, &stderr); rerr != nil {
		t.Fatalf("run: %s", rerr.Message)
	}
	if stdout.String() != "25\n42\n" {
		t.Errorf("output: %q", stdout.String())
	}
}

func TestEnginesAgreeOnModules(t *testing.T) {
	g, diags := Load(moduleProgram(t))
	if len(diags) > 0 {
		t.Fatalf("load errors: %v", diags)
	}

	var ivOut, ivErr strings.Builder
	if rerr := RunInterp(g, strings.NewReader(""), &ivOut, &ivErr); rerr != nil {
		t.Fatalf("interpreter: %s", rerr.Message)
	}
	var vmOut, vmErr strings.Builder
	if rerr := RunVM(g, strings.NewReader(""), &vmOut, &vmErr); rerr != nil {
		t.Fatalf("vm: %s", rerr.Message)
	}
	if ivOut.String() != vmOut.String() {
		t.Errorf("engines differ:\ninterp: %q\n    vm: %q", ivOut.String(), vmOut.String())
	}
}

func TestLoadReportsErrors(t *testing.T) {
	dir := writeTree(t, map[string]string{"app.spr": "import \"missing\"\n"})
	_, diags := Load(filepath.Join(dir, "app.spr"))
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "cannot find module") {
			found = true
		}
	}
	if !found {
		t.Errorf("diags: %v", diags)
	}
}

func TestLoadMissingEntry(t *testing.T) {
	dir := writeTree(t, map[string]string{"app.spr": "print(1)\n"})
	_, diags := Load(filepath.Join(dir, "nope.spr"))
	if len(diags) == 0 {
		t.Fatal("expected an error for a missing entry")
	}
}

func TestCompileGraph(t *testing.T) {
	g, diags := Load(moduleProgram(t))
	if len(diags) > 0 {
		t.Fatalf("load errors: %v", diags)
	}
	prog, err := Compile(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(prog.Modules) != 1 {
		t.Fatalf("module count: %d", len(prog.Modules))
	}
}
