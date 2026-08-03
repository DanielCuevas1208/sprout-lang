package module

import (
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

func TestResolveAppendsSuffix(t *testing.T) {
	fs := NewMemFS()
	fs.Add("lib/greeting.spr", "")
	l := NewLoader(fs)

	path, ok := l.Resolve("main.spr", "lib/greeting")
	if !ok {
		t.Fatal("expected the module to resolve")
	}
	if path != "lib/greeting.spr" {
		t.Errorf("resolved path: got %q", path)
	}
}

func TestResolveKeepsSuffix(t *testing.T) {
	fs := NewMemFS()
	fs.Add("greeting.spr", "")
	l := NewLoader(fs)

	path, ok := l.Resolve("main.spr", "greeting.spr")
	if !ok {
		t.Fatal("expected the module to resolve")
	}
	if path != "greeting.spr" {
		t.Errorf("resolved path: got %q", path)
	}
}

func TestResolveRelativeToImporter(t *testing.T) {
	fs := NewMemFS()
	fs.Add("sub/lib/math.spr", "")
	l := NewLoader(fs)

	path, ok := l.Resolve("sub/main.spr", "lib/math")
	if !ok {
		t.Fatal("expected the module to resolve")
	}
	if path != "sub/lib/math.spr" {
		t.Errorf("resolved path: got %q", path)
	}
}

func TestResolveMissing(t *testing.T) {
	l := NewLoader(NewMemFS())
	if _, ok := l.Resolve("main.spr", "missing"); ok {
		t.Error("expected a missing module to fail to resolve")
	}
}

func TestLoaderCaches(t *testing.T) {
	fs := NewMemFS()
	fs.Add("greeting.spr", "")
	l := NewLoader(fs)

	path, ok := l.Resolve("main.spr", "greeting")
	if !ok {
		t.Fatal("expected the module to resolve")
	}
	if _, ok := l.Cached(path); ok {
		t.Fatal("expected an empty cache")
	}
	l.Cache(path, object.NewModule("greeting", path))
	if m, ok := l.Cached(path); !ok || m.Name != "greeting" {
		t.Errorf("cache hit: got %+v, ok=%v", m, ok)
	}
}

func TestMarkLoadingRejectsCycles(t *testing.T) {
	l := NewLoader(NewMemFS())
	if !l.MarkLoading("a.spr") {
		t.Fatal("expected the first mark to succeed")
	}
	if l.MarkLoading("a.spr") {
		t.Error("expected the second mark to fail")
	}
	l.DoneLoading("a.spr")
	if !l.MarkLoading("a.spr") {
		t.Error("expected the mark to succeed after done")
	}
}

func TestName(t *testing.T) {
	l := NewLoader(NewMemFS())
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"lib/greeting.spr", "greeting", true},
		{"greeting.spr", "greeting", true},
		{"my-lib.spr", "", false},
		{"lib.spr", "lib", true},
	}
	for _, c := range cases {
		got, ok := l.Name(c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("Name(%q): got (%q, %v), want (%q, %v)", c.path, got, ok, c.want, c.ok)
		}
	}
}

func TestValidName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"greeting", true},
		{"_private", true},
		{"mod2", true},
		{"2mod", false},
		{"my-mod", false},
		{"my mod", false},
		{"", false},
		{"if", false},
	}
	for _, c := range cases {
		if got := ValidName(c.name); got != c.want {
			t.Errorf("ValidName(%q): got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExportsInSourceOrder(t *testing.T) {
	file := source.NewFile("mod.spr", `
let first = 1
fn second() { return 2 }
let third = 3
print("side effect")
const fourth = 4
`)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		t.Fatalf("parse errors: %v", diags)
	}
	got := Exports(prog)
	want := []string{"first", "second", "third", "fourth"}
	if len(got) != len(want) {
		t.Fatalf("exports: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("export %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMemFSReadsAddedFiles(t *testing.T) {
	fs := NewMemFS()
	fs.Add("a.spr", "hello")
	text, err := fs.ReadFile("a.spr")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(text) != "hello" {
		t.Errorf("read: got %q", string(text))
	}
	if _, err := fs.ReadFile("missing.spr"); err == nil {
		t.Error("expected a read error for a missing file")
	}
}
