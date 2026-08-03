// Package codec serializes compiled Sprout programs.
//
// The build tool writes a compiled program to a bytecode artifact. The run
// command reads that artifact back and executes it on the virtual machine.
// The format is stable and platform-independent: all integers are big-endian.
//
// The artifact stores the source text of the main file so that diagnostics
// keep their source lines. Modules are loaded from disk when the artifact
// runs, so an artifact needs its source tree.
package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// Magic is the file header of a Sprout bytecode artifact.
//
// The first four bytes spell "SPRB". The fifth byte is the format version.
var Magic = []byte{'S', 'P', 'R', 'B', 1}

// formatError marks malformed artifact data.
type formatError struct{ msg string }

func (e *formatError) Error() string { return e.msg }

func failf(format string, args ...any) error {
	return &formatError{msg: fmt.Sprintf(format, args...)}
}

// constant type tags.
const (
	constInt byte = iota + 1
	constFloat
	constString
	constFunction
)

// LooksLike reports whether data starts with the artifact magic.
func LooksLike(data []byte) bool {
	return bytes.HasPrefix(data, Magic)
}

// Encode writes prog to w in the artifact format.
func Encode(w io.Writer, prog *code.Program) error {
	e := &encoder{w: w}
	if _, err := w.Write(Magic); err != nil {
		return err
	}
	if err := e.string(prog.Text); err != nil {
		return err
	}
	return e.function(prog.Main)
}

// Decode reads a program from r in the artifact format.
func Decode(r io.Reader) (*code.Program, error) {
	d := &decoder{r: r}
	if err := d.expectMagic(); err != nil {
		return nil, err
	}
	text, err := d.string()
	if err != nil {
		return nil, err
	}
	main, err := d.function()
	if err != nil {
		return nil, err
	}
	return &code.Program{Main: main, Text: text}, nil
}

type encoder struct {
	w io.Writer
}

func (e *encoder) u8(v byte) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *encoder) i32(v int32) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *encoder) i64(v int64) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *encoder) f64(v float64) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *encoder) string(s string) error {
	if err := e.i32(int32(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(e.w, s)
	return err
}

func (e *encoder) function(fn *code.Function) error {
	if err := e.string(fn.Name); err != nil {
		return err
	}
	if err := e.string(fn.FileName); err != nil {
		return err
	}
	if err := e.i32(int32(len(fn.ParamNames))); err != nil {
		return err
	}
	for _, p := range fn.ParamNames {
		if err := e.string(p); err != nil {
			return err
		}
	}
	if err := e.i32(int32(fn.NumSlots)); err != nil {
		return err
	}
	if err := e.i32(int32(len(fn.Code))); err != nil {
		return err
	}
	if _, err := e.w.Write(fn.Code); err != nil {
		return err
	}

	// Exports: count, then name and slot pairs. Keys are sorted so a build
	// produces the same bytes every time.
	exportNames := make([]string, 0, len(fn.Exports))
	for name := range fn.Exports {
		exportNames = append(exportNames, name)
	}
	sort.Strings(exportNames)
	if err := e.i32(int32(len(exportNames))); err != nil {
		return err
	}
	for _, name := range exportNames {
		if err := e.string(name); err != nil {
			return err
		}
		if err := e.i32(int32(fn.Exports[name])); err != nil {
			return err
		}
	}

	// Positions: count, then offset and source position tuples.
	// Only instruction starts carry a position, so store them as pairs.
	positions := validPositions(fn)
	if err := e.i32(int32(len(positions))); err != nil {
		return err
	}
	for _, p := range positions {
		if err := e.i32(int32(p.offset)); err != nil {
			return err
		}
		if err := e.i32(int32(p.pos.Line)); err != nil {
			return err
		}
		if err := e.i32(int32(p.pos.Column)); err != nil {
			return err
		}
		if err := e.i32(int32(p.pos.Offset)); err != nil {
			return err
		}
	}

	if err := e.i32(int32(len(fn.Consts))); err != nil {
		return err
	}
	for _, c := range fn.Consts {
		if err := e.constant(c); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) constant(v object.Object) error {
	switch c := v.(type) {
	case object.Int:
		if err := e.u8(constInt); err != nil {
			return err
		}
		return e.i64(c.Value)
	case object.Float:
		if err := e.u8(constFloat); err != nil {
			return err
		}
		return e.f64(c.Value)
	case object.Str:
		if err := e.u8(constString); err != nil {
			return err
		}
		return e.string(c.Value)
	case *code.Function:
		if err := e.u8(constFunction); err != nil {
			return err
		}
		return e.function(c)
	}
	return failf("cannot encode a %s constant", v.Type())
}

// posEntry pairs an instruction offset with its source position.
type posEntry struct {
	offset int
	pos    source.Pos
}

// validPositions returns the offsets that start an instruction.
func validPositions(fn *code.Function) []posEntry {
	var out []posEntry
	for i, p := range fn.Positions {
		if p.IsValid() {
			out = append(out, posEntry{offset: i, pos: p})
		}
	}
	return out
}

type decoder struct {
	r io.Reader
}

func (d *decoder) expectMagic() error {
	buf := make([]byte, len(Magic))
	if _, err := io.ReadFull(d.r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return failf("not a Sprout bytecode artifact")
		}
		return err
	}
	if !bytes.Equal(buf, Magic) {
		return failf("not a Sprout bytecode artifact")
	}
	return nil
}

func (d *decoder) u8() (byte, error) {
	var v byte
	if err := binary.Read(d.r, binary.BigEndian, &v); err != nil {
		return 0, failf("truncated artifact")
	}
	return v, nil
}

func (d *decoder) i32() (int32, error) {
	var v int32
	if err := binary.Read(d.r, binary.BigEndian, &v); err != nil {
		return 0, failf("truncated artifact")
	}
	return v, nil
}

func (d *decoder) i64() (int64, error) {
	var v int64
	if err := binary.Read(d.r, binary.BigEndian, &v); err != nil {
		return 0, failf("truncated artifact")
	}
	return v, nil
}

func (d *decoder) f64() (float64, error) {
	var v float64
	if err := binary.Read(d.r, binary.BigEndian, &v); err != nil {
		return 0, failf("truncated artifact")
	}
	return v, nil
}

func (d *decoder) string() (string, error) {
	n, err := d.i32()
	if err != nil {
		return "", err
	}
	if n < 0 || n > 1<<26 {
		return "", failf("invalid string length in artifact")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(d.r, buf); err != nil {
		return "", failf("truncated artifact")
	}
	return string(buf), nil
}

func (d *decoder) function() (*code.Function, error) {
	fn := &code.Function{}
	var err error
	if fn.Name, err = d.string(); err != nil {
		return nil, err
	}
	if fn.FileName, err = d.string(); err != nil {
		return nil, err
	}
	n, err := d.i32()
	if err != nil {
		return nil, err
	}
	if n < 0 || n > 1<<20 {
		return nil, failf("invalid parameter count in artifact")
	}
	if n > 0 {
		fn.ParamNames = make([]string, n)
	}
	for i := int32(0); i < n; i++ {
		if fn.ParamNames[i], err = d.string(); err != nil {
			return nil, err
		}
	}
	slots, err := d.i32()
	if err != nil {
		return nil, err
	}
	fn.NumSlots = int(slots)

	codeLen, err := d.i32()
	if err != nil {
		return nil, err
	}
	if codeLen < 0 || codeLen > 1<<28 {
		return nil, failf("invalid instruction stream in artifact")
	}
	fn.Code = make([]byte, codeLen)
	if _, err := io.ReadFull(d.r, fn.Code); err != nil {
		return nil, failf("truncated artifact")
	}

	exportCount, err := d.i32()
	if err != nil {
		return nil, err
	}
	if exportCount > 0 {
		fn.Exports = make(map[string]int, exportCount)
	}
	for i := int32(0); i < exportCount; i++ {
		name, err := d.string()
		if err != nil {
			return nil, err
		}
		slot, err := d.i32()
		if err != nil {
			return nil, err
		}
		fn.Exports[name] = int(slot)
	}

	posCount, err := d.i32()
	if err != nil {
		return nil, err
	}
	if posCount < 0 || posCount > 1<<26 {
		return nil, failf("invalid position count in artifact")
	}
	for i := int32(0); i < posCount; i++ {
		off, err := d.i32()
		if err != nil {
			return nil, err
		}
		line, err := d.i32()
		if err != nil {
			return nil, err
		}
		col, err := d.i32()
		if err != nil {
			return nil, err
		}
		offset, err := d.i32()
		if err != nil {
			return nil, err
		}
		p := source.Pos{Line: int(line), Column: int(col), Offset: int(offset)}
		fn.Positions = appendPos(fn.Positions, int(off), p)
	}

	constCount, err := d.i32()
	if err != nil {
		return nil, err
	}
	if constCount < 0 || constCount > 1<<20 {
		return nil, failf("invalid constant count in artifact")
	}
	for i := int32(0); i < constCount; i++ {
		c, err := d.constant()
		if err != nil {
			return nil, err
		}
		fn.Consts = append(fn.Consts, c)
	}
	return fn, nil
}

func (d *decoder) constant() (object.Object, error) {
	tag, err := d.u8()
	if err != nil {
		return nil, err
	}
	switch tag {
	case constInt:
		v, err := d.i64()
		if err != nil {
			return nil, err
		}
		return object.Int{Value: v}, nil
	case constFloat:
		v, err := d.f64()
		if err != nil {
			return nil, err
		}
		return object.Float{Value: v}, nil
	case constString:
		s, err := d.string()
		if err != nil {
			return nil, err
		}
		return object.Str{Value: s}, nil
	case constFunction:
		return d.function()
	}
	return nil, failf("unknown constant type %d in artifact", tag)
}

// appendPos writes p at offset, extending positions with zero entries.
func appendPos(positions []source.Pos, offset int, p source.Pos) []source.Pos {
	for len(positions) <= offset {
		positions = append(positions, source.Pos{})
	}
	positions[offset] = p
	return positions
}
