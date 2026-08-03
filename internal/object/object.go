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

// Module is the namespace value of a loaded module.
//
// A module caches its exports. User code reads them with dot or index
// access. Modules are read-only.
type Module struct {
	// Name is the resolved path of the module source file.
	Name string
	// Exports holds the module's top-level declarations.
	Exports map[string]Object
}

func (m *Module) Type() Type { return TypeModule }

// String renders a module for display, using the file base name.
func (m *Module) String() string {
	base := m.Name
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".spr")
	return "<module " + base + ">"
}

// Repr renders o in a form that is close to its source literal.
func Repr(o Object) string {
	if r, ok := o.(interface{ Repr() string }); ok {
		return r.Repr()
	}
	return o.String()
}
