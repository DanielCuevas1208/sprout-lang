package runtime

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
)

func mustValue(t *testing.T, fn func() (object.Object, error)) object.Object {
	t.Helper()
	v, err := fn()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return v
}

func mustErr(t *testing.T, fn func() (object.Object, error)) error {
	t.Helper()
	v, err := fn()
	if err == nil {
		t.Fatalf("expected an error, got %s", v.String())
	}
	return err
}

func intVal(n int64) object.Object { return object.Int{Value: n} }
func floatVal(f float64) object.Object {
	return object.Float{Value: f}
}

func TestArithmeticValues(t *testing.T) {
	cases := []struct {
		name string
		got  object.Object
		want string
	}{
		{"int add", mustValue(t, func() (object.Object, error) { return Add(intVal(2), intVal(3)) }), "5"},
		{"mixed add", mustValue(t, func() (object.Object, error) { return Add(intVal(2), floatVal(2.5)) }), "4.5"},
		{"string add", mustValue(t, func() (object.Object, error) { return Add(object.Str{Value: "a"}, object.Str{Value: "b"}) }), "ab"},
		{"int sub", mustValue(t, func() (object.Object, error) { return Sub(intVal(7), intVal(10)) }), "-3"},
		{"int mul", mustValue(t, func() (object.Object, error) { return Mul(intVal(6), intVal(7)) }), "42"},
		{"int div", mustValue(t, func() (object.Object, error) { return Div(intVal(7), intVal(2)) }), "3"},
		{"float div", mustValue(t, func() (object.Object, error) { return Div(intVal(7), floatVal(2)) }), "3.5"},
		{"int mod", mustValue(t, func() (object.Object, error) { return Mod(intVal(7), intVal(3)) }), "1"},
		{"int pow", mustValue(t, func() (object.Object, error) { return Pow(intVal(2), intVal(10)) }), "1024"},
		{"pow of pow", mustValue(t, func() (object.Object, error) {
			exponent, err := Pow(intVal(3), intVal(2))
			if err != nil {
				return nil, err
			}
			return Pow(intVal(2), exponent)
		}), "512"},
		{"float pow", mustValue(t, func() (object.Object, error) { return Pow(floatVal(2), floatVal(0.5)) }), "1.4142135623730951"},
		{"negative pow", mustValue(t, func() (object.Object, error) { return Pow(intVal(2), intVal(-1)) }), "0.5"},
	}
	for _, c := range cases {
		if c.got.String() != c.want {
			t.Errorf("%s: got %s, want %s", c.name, c.got.String(), c.want)
		}
	}
}

func TestArithmeticErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"add type", mustErr(t, func() (object.Object, error) { return Add(intVal(1), object.Str{Value: "a"}) }), "cannot add"},
		{"div by zero", mustErr(t, func() (object.Object, error) { return Div(intVal(1), intVal(0)) }), "cannot divide by zero"},
		{"mod by zero", mustErr(t, func() (object.Object, error) { return Mod(intVal(1), intVal(0)) }), "remainder by zero"},
		{"div non-number", mustErr(t, func() (object.Object, error) { return Div(object.NilValue, intVal(1)) }), "cannot divide a nil"},
		{"pow non-number", mustErr(t, func() (object.Object, error) { return Pow(object.NilValue, intVal(1)) }), "cannot raise"},
	}
	for _, c := range cases {
		if !strings.Contains(c.err.Error(), c.want) {
			t.Errorf("%s: error %q does not contain %q", c.name, c.err.Error(), c.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b object.Object
		want int
	}{
		{intVal(1), intVal(2), -1},
		{intVal(2), intVal(1), 1},
		{intVal(1), intVal(1), 0},
		{floatVal(1.5), intVal(2), -1},
		{object.Str{Value: "a"}, object.Str{Value: "b"}, -1},
		{object.Str{Value: "b"}, object.Str{Value: "a"}, 1},
	}
	for _, c := range cases {
		got, ok := Compare(c.a, c.b)
		if !ok {
			t.Errorf("compare %s and %s: not comparable", c.a, c.b)
			continue
		}
		if got != c.want {
			t.Errorf("compare %s and %s: got %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if _, ok := Compare(intVal(1), object.NilValue); ok {
		t.Error("int and nil should not be comparable")
	}
}

func TestEqual(t *testing.T) {
	listA := &object.List{Elems: []object.Object{intVal(1), object.Str{Value: "a"}}}
	listB := &object.List{Elems: []object.Object{intVal(1), object.Str{Value: "a"}}}
	listC := &object.List{Elems: []object.Object{intVal(2), object.Str{Value: "a"}}}

	mapA := &object.Map{Vals: map[string]object.Object{"k": intVal(1)}}
	mapA.Set("k", intVal(1))
	mapB := &object.Map{Vals: map[string]object.Object{"k": intVal(1)}}
	mapB.Set("k", intVal(1))

	cases := []struct {
		name string
		a, b object.Object
		want bool
	}{
		{"int equal", intVal(1), intVal(1), true},
		{"int different", intVal(1), intVal(2), false},
		{"int and float", intVal(1), floatVal(1.0), true},
		{"string equal", object.Str{Value: "x"}, object.Str{Value: "x"}, true},
		{"string different", object.Str{Value: "x"}, object.Str{Value: "y"}, false},
		{"nil equal", object.NilValue, object.NilValue, true},
		{"list deep equal", listA, listB, true},
		{"list deep different", listA, listC, false},
		{"map equal", mapA, mapB, true},
		{"nil and value", object.NilValue, intVal(1), false},
	}
	for _, c := range cases {
		if got := Equal(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIndex(t *testing.T) {
	list := &object.List{Elems: []object.Object{intVal(10), intVal(20)}}
	if v := mustValue(t, func() (object.Object, error) { return IndexGet(list, intVal(1)) }); v.String() != "20" {
		t.Errorf("list index: got %s", v)
	}
	if _, err := IndexGet(list, intVal(5)); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Errorf("list out of range: %v", err)
	}
	if _, err := IndexGet(list, object.Str{Value: "x"}); err == nil || !strings.Contains(err.Error(), "integer") {
		t.Errorf("list bad index: %v", err)
	}
	m := &object.Map{Vals: map[string]object.Object{}}
	m.Set("a", intVal(1))
	if v := mustValue(t, func() (object.Object, error) { return IndexGet(m, object.Str{Value: "a"}) }); v.String() != "1" {
		t.Errorf("map index: got %s", v)
	}
	if v := mustValue(t, func() (object.Object, error) { return IndexGet(m, object.Str{Value: "missing"}) }); v.Type() != object.TypeNil {
		t.Errorf("map missing: got %s", v)
	}
	if _, err := IndexGet(m, intVal(1)); err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("map bad key: %v", err)
	}
	if _, err := IndexGet(object.NilValue, intVal(0)); err == nil || !strings.Contains(err.Error(), "cannot index") {
		t.Errorf("cannot index nil: %v", err)
	}
}

func TestSetIndex(t *testing.T) {
	list := &object.List{Elems: []object.Object{intVal(10), intVal(20)}}
	if v := mustValue(t, func() (object.Object, error) { return SetIndex(list, intVal(0), intVal(99)) }); v.String() != "99" {
		t.Errorf("set index value: got %s", v)
	}
	if list.Elems[0].String() != "99" {
		t.Errorf("list not updated: %s", list)
	}
	m := &object.Map{Vals: map[string]object.Object{}}
	mustValue(t, func() (object.Object, error) { return SetIndex(m, object.Str{Value: "k"}, intVal(7)) })
	if v, ok := m.Get("k"); !ok || v.String() != "7" {
		t.Errorf("map not updated: %v", m)
	}
	if _, err := SetIndex(list, intVal(9), intVal(1)); err == nil {
		t.Error("expected out of range error")
	}
}

func TestModuleIndex(t *testing.T) {
	mod := &object.Module{
		Name:    "lib/a.spr",
		Exports: map[string]object.Object{"answer": intVal(42)},
	}
	if v := mustValue(t, func() (object.Object, error) { return IndexGet(mod, object.Str{Value: "answer"}) }); v.String() != "42" {
		t.Errorf("module member: got %s", v)
	}
	if _, err := IndexGet(mod, object.Str{Value: "nope"}); err == nil || !strings.Contains(err.Error(), "no exported member") {
		t.Errorf("module missing member: %v", err)
	}
	if _, err := IndexGet(mod, intVal(0)); err == nil || !strings.Contains(err.Error(), "string") {
		t.Errorf("module bad member: %v", err)
	}
	if _, err := SetIndex(mod, object.Str{Value: "answer"}, intVal(1)); err == nil || !strings.Contains(err.Error(), "cannot assign") {
		t.Errorf("module write: %v", err)
	}
}

func TestSequence(t *testing.T) {
	list := &object.List{Elems: []object.Object{intVal(1), intVal(2)}}
	seq, err := Sequence(list)
	if err != nil || len(seq) != 2 {
		t.Fatalf("list sequence: %v %v", seq, err)
	}
	seq, err = Sequence(object.Str{Value: "hé"})
	if err != nil || len(seq) != 2 {
		t.Fatalf("string sequence: %v %v", seq, err)
	}
	seq, err = Sequence(object.Range{Start: 0, End: 5, Step: 2})
	if err != nil || len(seq) != 3 || seq[2].String() != "4" {
		t.Fatalf("range sequence: %v %v", seq, err)
	}
	if _, err := Sequence(object.NilValue); err == nil || !strings.Contains(err.Error(), "cannot iterate") {
		t.Errorf("nil sequence: %v", err)
	}
}

func TestTruthy(t *testing.T) {
	if Truthy(object.NilValue) {
		t.Error("nil should be falsy")
	}
	if Truthy(object.Bool{Value: false}) {
		t.Error("false should be falsy")
	}
	for _, v := range []object.Object{
		object.Bool{Value: true},
		intVal(0),
		floatVal(0),
		object.Str{Value: ""},
		&object.List{},
	} {
		if !Truthy(v) {
			t.Errorf("%s should be truthy", v)
		}
	}
}

func TestBuiltinArgContract(t *testing.T) {
	b := &Builtin{Name: "set", MinArgs: 3, MaxArgs: 3}
	if err := b.CheckArgs([]object.Object{intVal(1), intVal(2)}, "set"); err == nil {
		t.Error("too few args should fail")
	}
	if err := b.CheckArgs([]object.Object{intVal(1), intVal(2), intVal(3), intVal(4)}, "set"); err == nil {
		t.Error("too many args should fail")
	}
	if err := b.CheckArgs([]object.Object{intVal(1), intVal(2), intVal(3)}, "set"); err != nil {
		t.Errorf("valid args failed: %v", err)
	}
}
