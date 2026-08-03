package bundle

import (
	"bytes"
	"github.com/sprout-lang/sprout/internal/code"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/modules"
	"github.com/sprout-lang/sprout/internal/vm"
)

// buildGraph compiles a small two-file program and encodes it.
func buildGraph(t *testing.T) []byte {
	t.Helper()
	prog := compileGraph(t)
	data, err := Encode(prog)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// compileGraph builds a two-file program once.
func compileGraph(t *testing.T) *code.Program {
	t.Helper()
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("math.spr", "fn square(n) { return n * n }\n")
	write("app.spr", "import \"math\" as m\nprint(m.square(6))\nprint(7 / 2)\n")

	g, err := (&modules.Loader{}).Load(filepath.Join(dir, "app.spr"))
	if err != nil {
		t.Fatal(err)
	}
	if g.HasErrors() {
		t.Fatalf("load errors: %v", g.Diags())
	}
	prog, err := compiler.CompileGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	return prog
}

// runBundle decodes data and runs it on the VM.
func runBundle(t *testing.T, data []byte) (string, error) {
	t.Helper()
	prog, err := Decode(data)
	if err != nil {
		return "", err
	}
	var stdout, stderr strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &stdout, &stderr)
	_, rerr := machine.Run(prog.EntryFile(), prog)
	if rerr != nil {
		return stdout.String(), rerr
	}
	return stdout.String(), nil
}

func TestRoundTrip(t *testing.T) {
	data := buildGraph(t)
	out, err := runBundle(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if out != "36\n3\n" {
		t.Errorf("output: %q", out)
	}
}

func TestRoundTripIdempotent(t *testing.T) {
	data := buildGraph(t)
	prog, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encode(prog)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Error("encoding a decoded program changed the bytes")
	}
}

func TestEncodeDeterministic(t *testing.T) {
	prog := compileGraph(t)
	data1, err := Encode(prog)
	if err != nil {
		t.Fatal(err)
	}
	data2, err := Encode(prog)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data1, data2) {
		t.Error("encoding the same program twice changed the bytes")
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"bad magic", []byte("NOTSPROUTBC")},
		{"short magic", []byte("SPRO")},
		{"bad version", append([]byte(magic), 99)},
	}
	for _, c := range cases {
		if _, err := Decode(c.data); err == nil {
			t.Errorf("%s: expected an error, got none", c.name)
		}
	}
}

func TestDecodeTruncated(t *testing.T) {
	data := buildGraph(t)
	// Cut the bundle at several points; decoding must fail cleanly.
	for _, n := range []int{1, 9, 20, len(data) - 3, len(data) - 1} {
		if _, err := Decode(data[:n]); err == nil {
			t.Errorf("cut at %d: expected an error", n)
		}
	}
}
