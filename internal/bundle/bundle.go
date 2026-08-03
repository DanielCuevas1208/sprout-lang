// Package bundle serializes compiled Sprout programs.
//
// The build command uses this package to write a bytecode bundle to disk and
// the run commands use it to load one back. The format is deterministic: the
// same program always produces the same bytes, so a bundle can be rebuilt
// and compared in tests.
//
// A bundle embeds the source text of every file in the program. The VM uses
// that text to render runtime diagnostics with the same gutter and caret as
// a source run.
package bundle

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

const (
	magic   = "SPROUTBC"
	version = 1
)

// object tags in the constant pool.
const (
	tagInt   = 0
	tagFloat = 1
	tagStr   = 2
	tagBool  = 3
	tagNil   = 4
	tagFn    = 5
)

// Encode writes p as a bytecode bundle.
func Encode(p *code.Program) ([]byte, error) {
	e := &encoder{}
	e.str(magic)
	e.byte(version)

	e.u32(uint32(len(p.Files)))
	for _, f := range p.Files {
		e.str(f.Name)
		e.str(f.Text)
	}

	e.function(p.Main)

	e.u32(uint32(len(p.Modules)))
	for _, m := range p.Modules {
		e.str(m.Path)
		e.u32(uint32(len(m.Exports)))
		for _, ex := range m.Exports {
			e.str(ex)
		}
		e.function(m.Init)
	}
	return e.buf, nil
}

// Decode reads a bytecode bundle written by Encode.
func Decode(b []byte) (*code.Program, error) {
	d := &decoder{buf: b}
	if got := d.str(); got != magic {
		return nil, fmt.Errorf("bundle: not a Sprout bytecode bundle")
	}
	if v := d.byte(); v != version {
		return nil, fmt.Errorf("bundle: unsupported version %d", v)
	}

	p := &code.Program{}

	nf := int(d.u32())
	for i := 0; i < nf; i++ {
		name := d.str()
		text := d.str()
		p.Files = append(p.Files, source.NewFile(name, text))
	}

	p.Main = d.function()

	nm := int(d.u32())
	for i := 0; i < nm; i++ {
		mod := &code.Module{Path: d.str()}
		ne := int(d.u32())
		for j := 0; j < ne; j++ {
			mod.Exports = append(mod.Exports, d.str())
		}
		mod.Init = d.function()
		p.Modules = append(p.Modules, mod)
	}

	if d.err != nil {
		return nil, fmt.Errorf("bundle: %w", d.err)
	}
	return p, nil
}

// encoder appends big-endian values to a byte slice.
type encoder struct {
	buf []byte
}

func (e *encoder) byte(v byte) { e.buf = append(e.buf, v) }

func (e *encoder) u32(v uint32) {
	e.buf = binary.BigEndian.AppendUint32(e.buf, v)
}

func (e *encoder) u64(v uint64) {
	e.buf = binary.BigEndian.AppendUint64(e.buf, v)
}

func (e *encoder) str(s string) {
	e.u32(uint32(len(s)))
	e.buf = append(e.buf, s...)
}

func (e *encoder) pos(p source.Pos) {
	e.u32(uint32(p.Line))
	e.u32(uint32(p.Column))
	e.u32(uint32(p.Offset))
}

func (e *encoder) function(fn *code.Function) {
	e.str(fn.Name)
	e.str(fn.FileName)
	e.u32(uint32(fn.FileIdx))
	e.u32(uint32(fn.NumSlots))

	e.u32(uint32(len(fn.ParamNames)))
	for _, p := range fn.ParamNames {
		e.str(p)
	}

	e.u32(uint32(len(fn.Code)))
	e.buf = append(e.buf, fn.Code...)

	e.u32(uint32(len(fn.Positions)))
	for _, p := range fn.Positions {
		e.pos(p)
	}

	e.u32(uint32(len(fn.Consts)))
	for _, c := range fn.Consts {
		e.obj(c)
	}
}

func (e *encoder) obj(o object.Object) {
	switch v := o.(type) {
	case object.Int:
		e.byte(tagInt)
		e.u64(uint64(v.Value))
	case object.Float:
		e.byte(tagFloat)
		e.u64(math.Float64bits(v.Value))
	case object.Str:
		e.byte(tagStr)
		e.str(v.Value)
	case object.Bool:
		e.byte(tagBool)
		if v.Value {
			e.byte(1)
		} else {
			e.byte(0)
		}
	case object.Nil:
		e.byte(tagNil)
	case *code.Function:
		e.byte(tagFn)
		e.function(v)
	default:
		// A value of another type cannot reach the constant pool.
		panic(fmt.Sprintf("bundle: cannot encode a %s constant", o.Type()))
	}
}

// decoder reads values out of a byte slice with bounds checks.
type decoder struct {
	buf []byte
	off int
	err error
}

func (d *decoder) need(n int) bool {
	if d.err != nil {
		return false
	}
	if d.off+n > len(d.buf) {
		d.err = fmt.Errorf("truncated data")
		return false
	}
	return true
}

func (d *decoder) byte() byte {
	if !d.need(1) {
		return 0
	}
	v := d.buf[d.off]
	d.off++
	return v
}

func (d *decoder) u32() uint32 {
	if !d.need(4) {
		return 0
	}
	v := binary.BigEndian.Uint32(d.buf[d.off:])
	d.off += 4
	return v
}

func (d *decoder) u64() uint64 {
	if !d.need(8) {
		return 0
	}
	v := binary.BigEndian.Uint64(d.buf[d.off:])
	d.off += 8
	return v
}

func (d *decoder) str() string {
	n := int(d.u32())
	if !d.need(n) {
		return ""
	}
	s := string(d.buf[d.off : d.off+n])
	d.off += n
	return s
}

func (d *decoder) position() source.Pos {
	return source.Pos{
		Line:   int(d.u32()),
		Column: int(d.u32()),
		Offset: int(d.u32()),
	}
}

func (d *decoder) function() *code.Function {
	fn := &code.Function{
		Name:     d.str(),
		FileName: d.str(),
		FileIdx:  int(d.u32()),
		NumSlots: int(d.u32()),
	}

	np := int(d.u32())
	for i := 0; i < np; i++ {
		fn.ParamNames = append(fn.ParamNames, d.str())
	}

	nc := int(d.u32())
	if d.need(nc) {
		fn.Code = append([]byte(nil), d.buf[d.off:d.off+nc]...)
		d.off += nc
	}

	npos := int(d.u32())
	for i := 0; i < npos; i++ {
		fn.Positions = append(fn.Positions, d.position())
	}

	nk := int(d.u32())
	for i := 0; i < nk; i++ {
		fn.Consts = append(fn.Consts, d.obj())
	}
	return fn
}

func (d *decoder) obj() object.Object {
	tag := d.byte()
	switch tag {
	case tagInt:
		return object.Int{Value: int64(d.u64())}
	case tagFloat:
		return object.Float{Value: math.Float64frombits(d.u64())}
	case tagStr:
		return object.Str{Value: d.str()}
	case tagBool:
		return object.Bool{Value: d.byte() == 1}
	case tagNil:
		return object.NilValue
	case tagFn:
		return d.function()
	default:
		if d.err == nil {
			d.err = fmt.Errorf("unknown constant tag %d", tag)
		}
		return nil
	}
}
