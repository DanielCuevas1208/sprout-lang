// Package object defines the runtime values of the Sprout language.
package object

import (
	"fmt"
	"strconv"
	"strings"
)

// Type names a kind of runtime value.
type Type string

const (
	TypeInt      Type = "int"
	TypeFloat    Type = "float"
	TypeString   Type = "string"
	TypeBool     Type = "bool"
	TypeNil      Type = "nil"
	TypeList     Type = "list"
	TypeMap      Type = "map"
	TypeFunction Type = "function"
	TypeRange    Type = "range"
	TypeModule   Type = "module"
	TypeStruct   Type = "struct"
	TypeStructTp Type = "struct type"
	TypeMethod   Type = "method"
	TypeResult   Type = "result"
	TypeChannel  Type = "channel"
	TypeTask     Type = "task"
)

func (t Type) String() string { return string(t) }

// Object is a runtime value.
type Object interface {
	Type() Type
	// String renders the value for display.
	String() string
}

// Int is a 64-bit integer.
type Int struct{ Value int64 }

func (i Int) Type() Type     { return TypeInt }
func (i Int) String() string { return strconv.FormatInt(i.Value, 10) }

// Float is a 64-bit floating-point number.
type Float struct{ Value float64 }

func (f Float) Type() Type { return TypeFloat }
func (f Float) String() string {
	s := strconv.FormatFloat(f.Value, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// Str is a string of bytes.
type Str struct{ Value string }

func (s Str) Type() Type     { return TypeString }
func (s Str) String() string { return s.Value }
func (s Str) Repr() string   { return strconv.Quote(s.Value) }

// Bool is a boolean.
type Bool struct{ Value bool }

func (b Bool) Type() Type { return TypeBool }
func (b Bool) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// Nil is the absence of a value.
type Nil struct{}

// NilValue is the single nil value.
var NilValue = Nil{}

func (Nil) Type() Type     { return TypeNil }
func (Nil) String() string { return "nil" }

// List is an ordered collection of values.
//
// Lists are mutable and shared, so the language uses a pointer to a List.
type List struct {
	Elems []Object
}

func (l *List) Type() Type { return TypeList }
func (l *List) String() string {
	parts := make([]string, len(l.Elems))
	for i, e := range l.Elems {
		parts[i] = e.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (l *List) Repr() string {
	parts := make([]string, len(l.Elems))
	for i, e := range l.Elems {
		parts[i] = Repr(e)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Map is an ordered map with string keys.
//
// Insertion order is preserved so output is deterministic.
type Map struct {
	Keys []string
	Vals map[string]Object
}

func (m *Map) Type() Type { return TypeMap }

func (m *Map) Set(key string, value Object) {
	if _, ok := m.Vals[key]; !ok {
		m.Keys = append(m.Keys, key)
	}
	m.Vals[key] = value
}

func (m *Map) Get(key string) (Object, bool) {
	v, ok := m.Vals[key]
	return v, ok
}

func (m *Map) Delete(key string) bool {
	if _, ok := m.Vals[key]; !ok {
		return false
	}
	delete(m.Vals, key)
	for i, k := range m.Keys {
		if k == key {
			m.Keys = append(m.Keys[:i], m.Keys[i+1:]...)
			break
		}
	}
	return true
}

func (m *Map) String() string {
	var b strings.Builder
	b.WriteString("{")
	for i, k := range m.Keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(m.Vals[k].String())
	}
	b.WriteString("}")
	return b.String()
}

func (m *Map) Repr() string {
	var b strings.Builder
	b.WriteString("{")
	for i, k := range m.Keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(k))
		b.WriteString(": ")
		b.WriteString(Repr(m.Vals[k]))
	}
	b.WriteString("}")
	return b.String()
}

// Range is a half-open interval of integers [Start, End).
type Range struct {
	Start, End, Step int64
}

func (r Range) Type() Type { return TypeRange }
func (r Range) String() string {
	if r.Step == 1 {
		return fmt.Sprintf("range(%d, %d)", r.Start, r.End)
	}
	return fmt.Sprintf("range(%d, %d, %d)", r.Start, r.End, r.Step)
}

// Module is the value produced by importing a Sprout file.
//
// Its Exports map holds the current value of each exported name. Member
// access reads from this map. Modules are immutable to the importer.
type Module struct {
	Name    string
	Exports map[string]Object
}

func (m *Module) Type() Type { return TypeModule }
func (m *Module) String() string {
	return fmt.Sprintf("<module %s>", m.Name)
}

// StructType is the value that a struct declaration creates.
//
// It carries the field names in declaration order and the method table. A
// struct declaration binds the type to a name; a struct literal calls the
// type to build an instance.
type StructType struct {
	Name    string
	Fields  []string
	Methods map[string]Object
}

func (t *StructType) Type() Type { return TypeStructTp }
func (t *StructType) String() string {
	return "<struct " + t.Name + ">"
}

// HasField reports whether the type declares a field with name.
func (t *StructType) HasField(name string) bool {
	for _, f := range t.Fields {
		if f == name {
			return true
		}
	}
	return false
}

// HasMethod reports whether the type declares a method with name.
func (t *StructType) HasMethod(name string) bool {
	if t.Methods == nil {
		return false
	}
	_, ok := t.Methods[name]
	return ok
}

// Construct builds an instance from positional values.
//
// Values map to fields in declaration order. Missing trailing fields hold
// nil; extra values are an error.
func (t *StructType) Construct(positional []Object) (*Struct, error) {
	if len(positional) > len(t.Fields) {
		return nil, fmt.Errorf("struct '%s' expects at most %d fields, got %d", t.Name, len(t.Fields), len(positional))
	}
	s := &Struct{StructType: t, Values: make(map[string]Object)}
	for i, v := range positional {
		s.Values[t.Fields[i]] = v
	}
	return s, nil
}

// ConstructNamed builds an instance from named field values.
//
// An unknown field name is an error; a missing field holds nil.
func (t *StructType) ConstructNamed(names []string, values []Object) (*Struct, error) {
	s := &Struct{StructType: t, Values: make(map[string]Object)}
	for i, name := range names {
		if !t.HasField(name) {
			return nil, fmt.Errorf("struct '%s' has no field '%s'", t.Name, name)
		}
		s.Values[name] = values[i]
	}
	return s, nil
}

// Struct is one instance of a struct type.
type Struct struct {
	StructType *StructType
	Values     map[string]Object
}

func (s *Struct) Type() Type { return TypeStruct }

// String renders the instance like its source literal: Point{x: 1, y: 2}.
func (s *Struct) String() string {
	var b strings.Builder
	b.WriteString(s.StructType.Name)
	b.WriteString("{")
	for i, f := range s.StructType.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f)
		b.WriteString(": ")
		if v, ok := s.Values[f]; ok {
			b.WriteString(v.String())
		} else {
			b.WriteString("nil")
		}
	}
	b.WriteString("}")
	return b.String()
}

// Repr renders the instance with nested values in literal form.
func (s *Struct) Repr() string {
	var b strings.Builder
	b.WriteString(s.StructType.Name)
	b.WriteString("{")
	for i, f := range s.StructType.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f)
		b.WriteString(": ")
		if v, ok := s.Values[f]; ok {
			b.WriteString(Repr(v))
		} else {
			b.WriteString("nil")
		}
	}
	b.WriteString("}")
	return b.String()
}

// BoundMethod is a method value with its receiver bound.
//
// Reading a method from an instance, as in p.area, returns a BoundMethod.
// Calling it runs the method with self set to the receiver.
type BoundMethod struct {
	Name     string
	Receiver Object
	Method   Object
}

func (m *BoundMethod) Type() Type { return TypeMethod }
func (m *BoundMethod) String() string {
	return "<method " + m.Name + ">"
}

// Result is a value that carries either a success value or an error message.
//
// Sprout has no exceptions. A function that can fail returns a Result. The
// ok() and err() builtins build results, and a match expression takes them
// apart. An ok result holds a value; an error result holds a message string.
// Results are immutable and compare by value.
type Result struct {
	Ok      bool
	Value   Object
	Message string
}

func (r Result) Type() Type { return TypeResult }
func (r Result) String() string {
	if r.Ok {
		return "ok(" + r.Value.String() + ")"
	}
	return "err(" + strconv.Quote(r.Message) + ")"
}
func (r Result) Repr() string {
	if r.Ok {
		return "ok(" + Repr(r.Value) + ")"
	}
	return "err(" + strconv.Quote(r.Message) + ")"
}

// Repr renders o in a form that is close to its source literal.
func Repr(o Object) string {
	if r, ok := o.(interface{ Repr() string }); ok {
		return r.Repr()
	}
	return o.String()
}
