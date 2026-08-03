package runtime

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

func noopPos() source.Pos { return source.Pos{Line: 1, Column: 1} }

func intVal(i int64) object.Object     { return object.Int{Value: i} }
func floatVal(f float64) object.Object { return object.Float{Value: f} }
func strVal(s string) object.Object    { return object.Str{Value: s} }
func boolVal(b bool) object.Object     { return object.Bool{Value: b} }

func TestTruthy(t *testing.T) {
	cases := []struct {
		val  object.Object
		want bool
	}{
		{object.NilValue, false},
		{boolVal(false), false},
		{boolVal(true), true},
		{intVal(0), true},
		{intVal(1), true},
		{strVal(""), true},
		{&object.List{}, true},
	}
	for _, c := range cases {
		if got := Truthy(c.val); got != c.want {
			t.Errorf("Truthy(%v) = %v, want %v", c.val, got, c.want)
		}
	}
}

func TestEqual(t *testing.T) {
	cases := []struct {
		a, b object.Object
		want bool
	}{
		{intVal(1), intVal(1), true},
		{intVal(1), intVal(2), false},
		{intVal(1), floatVal(1), true},
		{intVal(1), floatVal(1.5), false},
		{strVal("a"), strVal("a"), true},
		{strVal("a"), strVal("b"), false},
		{boolVal(true), boolVal(true), true},
		{object.NilValue, object.NilValue, true},
		{intVal(1), strVal("1"), false},
		{&object.List{Elems: []object.Object{intVal(1), intVal(2)}},
			&object.List{Elems: []object.Object{intVal(1), intVal(2)}}, true},
		{&object.List{Elems: []object.Object{intVal(1)}},
			&object.List{Elems: []object.Object{intVal(2)}}, false},
	}
	for _, c := range cases {
		if got := Equal(c.a, c.b); got != c.want {
			t.Errorf("Equal(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestArithmetic(t *testing.T) {
	add, err := Add(intVal(2), intVal(3))
	if err != nil || add != intVal(5) {
		t.Errorf("Add = %v, %v", add, err)
	}
	mixed, err := Add(intVal(2), floatVal(0.5))
	if err != nil || mixed != floatVal(2.5) {
		t.Errorf("Add mixed = %v, %v", mixed, err)
	}
	cat, err := Add(strVal("a"), strVal("b"))
	if err != nil || cat != strVal("ab") {
		t.Errorf("Add strings = %v, %v", cat, err)
	}
	if _, err := Add(intVal(1), strVal("a")); err == nil {
		t.Errorf("Add int+string should fail")
	}

	div, err := Div(intVal(7), intVal(2))
	if err != nil || div != intVal(3) {
		t.Errorf("Div = %v, %v", div, err)
	}
	if _, err := Div(intVal(1), intVal(0)); err == nil {
		t.Errorf("Div by zero should fail")
	}
	pow, err := Pow(intVal(2), intVal(10))
	if err != nil || pow != intVal(1024) {
		t.Errorf("Pow = %v, %v", pow, err)
	}
	mod, err := Mod(intVal(7), intVal(3))
	if err != nil || mod != intVal(1) {
		t.Errorf("Mod = %v, %v", mod, err)
	}
}

func TestCompare(t *testing.T) {
	if got, ok := Compare(intVal(1), intVal(2)); !ok || got >= 0 {
		t.Errorf("Compare(1,2) = %d, %v", got, ok)
	}
	if got, ok := Compare(strVal("a"), strVal("b")); !ok || got >= 0 {
		t.Errorf("Compare(a,b) = %d, %v", got, ok)
	}
	if _, ok := Compare(intVal(1), strVal("a")); ok {
		t.Errorf("Compare(int,string) should not be comparable")
	}
}

func TestIndexGetSet(t *testing.T) {
	l := &object.List{Elems: []object.Object{intVal(1), intVal(2), intVal(3)}}
	if v, err := IndexGet(l, intVal(1)); err != nil || v != intVal(2) {
		t.Errorf("IndexGet = %v, %v", v, err)
	}
	if _, err := IndexGet(l, intVal(5)); err == nil {
		t.Errorf("IndexGet out of range should fail")
	}
	if _, err := IndexGet(l, strVal("x")); err == nil {
		t.Errorf("IndexGet with string index should fail")
	}
	if _, err := SetIndex(l, intVal(0), intVal(9)); err != nil {
		t.Errorf("SetIndex = %v", err)
	}
	if l.Elems[0] != intVal(9) {
		t.Errorf("SetIndex did not store value")
	}
	if _, err := IndexGet(intVal(5), intVal(0)); err == nil {
		t.Errorf("IndexGet of int should fail")
	}
}

func TestSequence(t *testing.T) {
	items, err := Sequence(&object.List{Elems: []object.Object{intVal(1), intVal(2)}})
	if err != nil || len(items) != 2 {
		t.Errorf("Sequence list = %v, %v", items, err)
	}
	items, err = Sequence(object.Str{Value: "ab"})
	if err != nil || len(items) != 2 || items[0] != strVal("a") {
		t.Errorf("Sequence string = %v, %v", items, err)
	}
	items, err = Sequence(object.Range{Start: 0, End: 3})
	if err != nil || len(items) != 3 || items[2] != intVal(2) {
		t.Errorf("Sequence range = %v, %v", items, err)
	}
}

func TestBuiltinRegisterAndNames(t *testing.T) {
	if len(Builtins) == 0 {
		t.Fatal("no builtins registered")
	}
	if len(Names) != len(Builtins) {
		t.Fatalf("Names has %d entries, Builtins has %d", len(Names), len(Builtins))
	}
	seen := map[string]bool{}
	for i, b := range Builtins {
		if b.Name != Names[i] {
			t.Errorf("name mismatch at %d: %q vs %q", i, b.Name, Names[i])
		}
		if seen[b.Name] {
			t.Errorf("duplicate builtin name %q", b.Name)
		}
		seen[b.Name] = true
		if b.MinArgs > b.MaxArgs && b.MaxArgs >= 0 {
			t.Errorf("builtin %s has invalid arg range", b.Name)
		}
		if b.Type() != object.TypeFunction {
			t.Errorf("builtin %s has type %s", b.Name, b.Type())
		}
	}
}

func TestBuiltinValues(t *testing.T) {
	var out, errOut strings.Builder
	ctx := &Context{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut}

	if v, err := callBuiltin(t, ctx, "len", []object.Object{strVal("héllo")}); err != nil || v != intVal(5) {
		t.Errorf("len = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "int", []object.Object{strVal("42")}); err != nil || v != intVal(42) {
		t.Errorf("int = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "float", []object.Object{strVal("2.5")}); err != nil || v != floatVal(2.5) {
		t.Errorf("float = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "type", []object.Object{intVal(1)}); err != nil || v != strVal("int") {
		t.Errorf("type = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "min", []object.Object{intVal(3), intVal(1), intVal(2)}); err != nil || v != intVal(1) {
		t.Errorf("min = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "max", []object.Object{intVal(3), intVal(1), intVal(2)}); err != nil || v != intVal(3) {
		t.Errorf("max = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "sqrt", []object.Object{intVal(144)}); err != nil || v != floatVal(12) {
		t.Errorf("sqrt = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "upper", []object.Object{strVal("hi")}); err != nil || v != strVal("HI") {
		t.Errorf("upper = %v, %v", v, err)
	}
	if v, err := callBuiltin(t, ctx, "split", []object.Object{strVal("a,b"), strVal(",")}); err != nil {
		t.Errorf("split error: %v", err)
	} else if s := v.String(); s != "[a, b]" {
		t.Errorf("split = %q", s)
	}
	if v, err := callBuiltin(t, ctx, "abs", []object.Object{intVal(-3)}); err != nil || v != intVal(3) {
		t.Errorf("abs = %v, %v", v, err)
	}
	if _, err := callBuiltin(t, ctx, "abs", []object.Object{strVal("x")}); err == nil {
		t.Errorf("abs of string should fail")
	}
}

func TestBuiltinPrint(t *testing.T) {
	var out strings.Builder
	ctx := &Context{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	if _, err := callBuiltin(t, ctx, "print", []object.Object{intVal(1), strVal("two"), boolVal(true)}); err != nil {
		t.Fatalf("print error: %v", err)
	}
	if out.String() != "1 two true\n" {
		t.Errorf("print output = %q", out.String())
	}
}

func TestBuiltinArity(t *testing.T) {
	var out strings.Builder
	ctx := &Context{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out}
	for _, b := range Builtins {
		if err := b.CheckArgs(make([]object.Object, b.MinArgs), b.Name); err != nil {
			t.Errorf("%s: min arity rejected: %v", b.Name, err)
		}
		if b.MinArgs > 0 && b.CheckArgs(make([]object.Object, b.MinArgs-1), b.Name) == nil {
			t.Errorf("%s: too-few args accepted", b.Name)
		}
		if b.MaxArgs >= 0 && b.CheckArgs(make([]object.Object, b.MaxArgs+1), b.Name) == nil {
			t.Errorf("%s: too-many args accepted", b.Name)
		}
	}
	_ = ctx
}

func callBuiltin(t *testing.T, ctx *Context, name string, args []object.Object) (object.Object, error) {
	t.Helper()
	for _, b := range Builtins {
		if b.Name != name {
			continue
		}
		if err := b.CheckArgs(args, name); err != nil {
			return nil, err
		}
		return b.Fn(ctx, args, noopPos())
	}
	t.Fatalf("no builtin named %q", name)
	return nil, nil
}
