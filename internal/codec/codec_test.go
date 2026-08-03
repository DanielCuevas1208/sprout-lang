package codec

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

func roundTrip(t *testing.T, prog *code.Program) *code.Program {
	t.Helper()
	var buf bytes.Buffer
	if err := Encode(&buf, prog); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !LooksLike(buf.Bytes()) {
		t.Fatal("encoded data does not start with the magic")
	}
	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

// compileSrc parses and compiles src for the round-trip tests.
func compileSrc(t *testing.T, src string) *code.Program {
	t.Helper()
	file := source.NewFile("prog.spr", src)
	prog, _ := parser.Parse(file)
	p, err := compiler.Compile(file, prog)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return p
}

func TestRoundTripLiterals(t *testing.T) {
	p := compileSrc(t, "print(1 + 2.5, \"hi\")\n")
	got := roundTrip(t, p)
	if !reflect.DeepEqual(p, got) {
		t.Errorf("round trip changed the program:\n got: %+v\nwant: %+v", got, p)
	}
}

func TestRoundTripFunctions(t *testing.T) {
	p := compileSrc(t, `
fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}
print(fib(10))
`)
	got := roundTrip(t, p)
	if !reflect.DeepEqual(p, got) {
		t.Errorf("round trip changed the program")
	}
}

func TestRoundTripModules(t *testing.T) {
	p := compileSrc(t, `import "./math"
export let top = 1
print(math.square(5))
`)
	got := roundTrip(t, p)
	if len(got.Main.Exports) != 1 {
		t.Fatalf("exports: %v", got.Main.Exports)
	}
	if got.Main.Exports["top"] != 1 {
		t.Errorf("top export slot: %d", got.Main.Exports["top"])
	}
	if !reflect.DeepEqual(p, got) {
		t.Errorf("round trip changed the program")
	}
}

func TestRoundTripText(t *testing.T) {
	p := compileSrc(t, "print(1)\n")
	p.Text = "print(1)\n"
	got := roundTrip(t, p)
	if got.Text != p.Text {
		t.Errorf("text: got %q, want %q", got.Text, p.Text)
	}
}

func TestDecodeRejectsTruncated(t *testing.T) {
	p := compileSrc(t, "print(1 + 2)\n")
	var buf bytes.Buffer
	if err := Encode(&buf, p); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	// Chop bytes from the end until the reader fails cleanly.
	for n := len(data) - 1; n > 0; n-- {
		if _, err := Decode(bytes.NewReader(data[:n])); err == nil {
			t.Fatalf("decode accepted %d bytes, want an error", n)
		}
	}
}

func TestDecodeRejectsNonArtifact(t *testing.T) {
	if _, err := Decode(bytes.NewReader([]byte("hello, world"))); err == nil {
		t.Fatal("expected an error for non-artifact data")
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	p := compileSrc(t, `import "./a"
import "./b"
export let x = 1
export let y = 2
`)
	enc := func() []byte {
		var buf bytes.Buffer
		if err := Encode(&buf, p); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	first, second := enc(), enc()
	if !bytes.Equal(first, second) {
		t.Error("encoding is not deterministic")
	}
}
