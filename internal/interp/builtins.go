package interp

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// BuiltinNames lists the predeclared standard library functions.
var BuiltinNames = []string{
	"print", "eprint", "write",
	"len", "str", "int", "float", "bool", "type",
	"range", "input",
	"push", "pop", "get", "set", "has",
	"keys", "values",
	"join", "split", "upper", "lower", "trim",
	"starts_with", "ends_with", "contains", "repeat",
	"map", "filter", "fold",
	"abs", "min", "max", "floor", "ceil", "round", "sqrt",
	"assert",
}

// RegisterBuiltins binds the standard library into iv's globals.
func RegisterBuiltins(iv *Interpreter) {
	builtins := []*Builtin{
		{Name: "print", MinArgs: 0, MaxArgs: -1, Fn: iv.builtinPrint},
		{Name: "eprint", MinArgs: 0, MaxArgs: -1, Fn: iv.builtinEprint},
		{Name: "write", MinArgs: 0, MaxArgs: -1, Fn: iv.builtinWrite},
		{Name: "len", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinLen},
		{Name: "str", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinStr},
		{Name: "int", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinInt},
		{Name: "float", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinFloat},
		{Name: "bool", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinBool},
		{Name: "type", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinType},
		{Name: "range", MinArgs: 2, MaxArgs: 3, Fn: iv.builtinRange},
		{Name: "input", MinArgs: 0, MaxArgs: 1, Fn: iv.builtinInput},
		{Name: "push", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinPush},
		{Name: "pop", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinPop},
		{Name: "get", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinGet},
		{Name: "set", MinArgs: 3, MaxArgs: 3, Fn: iv.builtinSet},
		{Name: "has", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinHas},
		{Name: "keys", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinKeys},
		{Name: "values", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinValues},
		{Name: "join", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinJoin},
		{Name: "split", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinSplit},
		{Name: "map", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinMap},
		{Name: "filter", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinFilter},
		{Name: "fold", MinArgs: 3, MaxArgs: 3, Fn: iv.builtinFold},
		{Name: "upper", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinUpper},
		{Name: "lower", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinLower},
		{Name: "trim", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinTrim},
		{Name: "starts_with", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinStartsWith},
		{Name: "ends_with", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinEndsWith},
		{Name: "contains", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinContains},
		{Name: "repeat", MinArgs: 2, MaxArgs: 2, Fn: iv.builtinRepeat},
		{Name: "abs", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinAbs},
		{Name: "min", MinArgs: 1, MaxArgs: -1, Fn: iv.builtinMin},
		{Name: "max", MinArgs: 1, MaxArgs: -1, Fn: iv.builtinMax},
		{Name: "floor", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinFloor},
		{Name: "ceil", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinCeil},
		{Name: "round", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinRound},
		{Name: "sqrt", MinArgs: 1, MaxArgs: 1, Fn: iv.builtinSqrt},
		{Name: "assert", MinArgs: 1, MaxArgs: 2, Fn: iv.builtinAssert},
	}
	for _, b := range builtins {
		iv.globals.Define(b.Name, b, true)
	}
}

func (iv *Interpreter) builtinPrint(args []object.Object, pos source.Pos) (object.Object, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	fmt.Fprintln(iv.stdout, strings.Join(parts, " "))
	return object.NilValue, nil
}

func (iv *Interpreter) builtinEprint(args []object.Object, pos source.Pos) (object.Object, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	fmt.Fprintln(iv.stderr, strings.Join(parts, " "))
	return object.NilValue, nil
}

func (iv *Interpreter) builtinWrite(args []object.Object, pos source.Pos) (object.Object, error) {
	for _, a := range args {
		fmt.Fprint(iv.stdout, a.String())
	}
	return object.NilValue, nil
}

func (iv *Interpreter) builtinLen(args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Str:
		return object.Int{Value: int64(utf8.RuneCountInString(v.Value))}, nil
	case *object.List:
		return object.Int{Value: int64(len(v.Elems))}, nil
	case *object.Map:
		return object.Int{Value: int64(len(v.Keys))}, nil
	}
	return nil, fmtErr("len() expects a string, list, or map, got %s", args[0].Type())
}

func (iv *Interpreter) builtinStr(args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Str{Value: args[0].String()}, nil
}

func (iv *Interpreter) builtinInt(args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Int:
		return v, nil
	case object.Float:
		return object.Int{Value: int64(v.Value)}, nil
	case object.Bool:
		if v.Value {
			return object.Int{Value: 1}, nil
		}
		return object.Int{Value: 0}, nil
	case object.Str:
		n, err := strconv.ParseInt(strings.TrimSpace(v.Value), 10, 64)
		if err != nil {
			return nil, fmtErr("int() cannot convert %q to an integer", v.Value)
		}
		return object.Int{Value: n}, nil
	}
	return nil, fmtErr("int() cannot convert a %s", args[0].Type())
}

func (iv *Interpreter) builtinFloat(args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Float:
		return v, nil
	case object.Int:
		return object.Float{Value: float64(v.Value)}, nil
	case object.Bool:
		if v.Value {
			return object.Float{Value: 1}, nil
		}
		return object.Float{Value: 0}, nil
	case object.Str:
		f, err := strconv.ParseFloat(strings.TrimSpace(v.Value), 64)
		if err != nil {
			return nil, fmtErr("float() cannot convert %q to a number", v.Value)
		}
		return object.Float{Value: f}, nil
	}
	return nil, fmtErr("float() cannot convert a %s", args[0].Type())
}

func (iv *Interpreter) builtinBool(args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Bool{Value: truthy(args[0])}, nil
}

func (iv *Interpreter) builtinType(args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Str{Value: string(args[0].Type())}, nil
}

func (iv *Interpreter) builtinRange(args []object.Object, pos source.Pos) (object.Object, error) {
	start, ok := asIntIndex(args[0])
	if !ok {
		return nil, fmtErr("range() start must be an integer, got %s", args[0].Type())
	}
	end, ok := asIntIndex(args[1])
	if !ok {
		return nil, fmtErr("range() end must be an integer, got %s", args[1].Type())
	}
	step := int64(1)
	if len(args) == 3 {
		step, ok = asIntIndex(args[2])
		if !ok {
			return nil, fmtErr("range() step must be an integer, got %s", args[2].Type())
		}
		if step <= 0 {
			return nil, fmtErr("range() step must be a positive integer, got %d", step)
		}
	}
	return object.Range{Start: start, End: end, Step: step}, nil
}

func (iv *Interpreter) builtinInput(args []object.Object, pos source.Pos) (object.Object, error) {
	if len(args) == 1 {
		if prompt, ok := args[0].(object.Str); ok {
			fmt.Fprint(iv.stdout, prompt.Value)
		}
	}
	if iv.input == nil {
		iv.input = bufio.NewReader(iv.stdin)
	}
	line, err := iv.input.ReadString('\n')
	if err != nil && len(line) == 0 {
		return nil, fmtErr("input() reached the end of input")
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return object.Str{Value: line}, nil
}

func (iv *Interpreter) builtinPush(args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, fmtErr("push() expects a list, got %s", args[0].Type())
	}
	list.Elems = append(list.Elems, args[1])
	return list, nil
}

func (iv *Interpreter) builtinPop(args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, fmtErr("pop() expects a list, got %s", args[0].Type())
	}
	if len(list.Elems) == 0 {
		return nil, fmtErr("pop() on an empty list")
	}
	last := list.Elems[len(list.Elems)-1]
	list.Elems = list.Elems[:len(list.Elems)-1]
	return last, nil
}

func (iv *Interpreter) builtinGet(args []object.Object, pos source.Pos) (object.Object, error) {
	switch c := args[0].(type) {
	case *object.List:
		i, ok := asIntIndex(args[1])
		if !ok {
			return nil, fmtErr("get() list index must be an integer, got %s", args[1].Type())
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return object.NilValue, nil
		}
		return c.Elems[i], nil
	case object.Str:
		i, ok := asIntIndex(args[1])
		if !ok {
			return nil, fmtErr("get() string index must be an integer, got %s", args[1].Type())
		}
		runes := []rune(c.Value)
		if i < 0 || i >= int64(len(runes)) {
			return object.NilValue, nil
		}
		return object.Str{Value: string(runes[i])}, nil
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, fmtErr("get() map key must be a string, got %s", args[1].Type())
		}
		if v, exists := c.Get(key.Value); exists {
			return v, nil
		}
		return object.NilValue, nil
	}
	return nil, fmtErr("get() expects a list, string, or map, got %s", args[0].Type())
}

func (iv *Interpreter) builtinSet(args []object.Object, pos source.Pos) (object.Object, error) {
	value := args[2]
	switch c := args[0].(type) {
	case *object.List:
		i, ok := asIntIndex(args[1])
		if !ok {
			return nil, fmtErr("set() list index must be an integer, got %s", args[1].Type())
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return nil, fmtErr("set() list index %d out of range (length %d)", i, len(c.Elems))
		}
		c.Elems[i] = value
		return c, nil
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, fmtErr("set() map key must be a string, got %s", args[1].Type())
		}
		c.Set(key.Value, value)
		return c, nil
	}
	return nil, fmtErr("set() expects a list or map, got %s", args[0].Type())
}

func (iv *Interpreter) builtinHas(args []object.Object, pos source.Pos) (object.Object, error) {
	switch c := args[0].(type) {
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, fmtErr("has() map key must be a string, got %s", args[1].Type())
		}
		_, exists := c.Get(key.Value)
		return object.Bool{Value: exists}, nil
	case *object.List:
		i, ok := asIntIndex(args[1])
		if !ok {
			return nil, fmtErr("has() list index must be an integer, got %s", args[1].Type())
		}
		return object.Bool{Value: i >= 0 && i < int64(len(c.Elems))}, nil
	case object.Str:
		i, ok := asIntIndex(args[1])
		if !ok {
			return nil, fmtErr("has() string index must be an integer, got %s", args[1].Type())
		}
		return object.Bool{Value: i >= 0 && i < int64(utf8.RuneCountInString(c.Value))}, nil
	}
	return nil, fmtErr("has() expects a list, string, or map, got %s", args[0].Type())
}

func (iv *Interpreter) builtinKeys(args []object.Object, pos source.Pos) (object.Object, error) {
	m, ok := args[0].(*object.Map)
	if !ok {
		return nil, fmtErr("keys() expects a map, got %s", args[0].Type())
	}
	keys := make([]object.Object, len(m.Keys))
	for i, k := range m.Keys {
		keys[i] = object.Str{Value: k}
	}
	return &object.List{Elems: keys}, nil
}

func (iv *Interpreter) builtinValues(args []object.Object, pos source.Pos) (object.Object, error) {
	m, ok := args[0].(*object.Map)
	if !ok {
		return nil, fmtErr("values() expects a map, got %s", args[0].Type())
	}
	vals := make([]object.Object, len(m.Keys))
	for i, k := range m.Keys {
		vals[i] = m.Vals[k]
	}
	return &object.List{Elems: vals}, nil
}

func (iv *Interpreter) builtinJoin(args []object.Object, pos source.Pos) (object.Object, error) {
	sep, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("join() separator must be a string, got %s", args[0].Type())
	}
	list, ok := args[1].(*object.List)
	if !ok {
		return nil, fmtErr("join() expects a list, got %s", args[1].Type())
	}
	parts := make([]string, len(list.Elems))
	for i, e := range list.Elems {
		parts[i] = e.String()
	}
	return object.Str{Value: strings.Join(parts, sep.Value)}, nil
}

func (iv *Interpreter) builtinSplit(args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("split() expects a string, got %s", args[0].Type())
	}
	sep, ok := args[1].(object.Str)
	if !ok {
		return nil, fmtErr("split() separator must be a string, got %s", args[1].Type())
	}
	if sep.Value == "" {
		return nil, fmtErr("split() separator must not be empty")
	}
	parts := strings.Split(s.Value, sep.Value)
	elems := make([]object.Object, len(parts))
	for i, p := range parts {
		elems[i] = object.Str{Value: p}
	}
	return &object.List{Elems: elems}, nil
}

// builtinMap applies a function to every element of a list.
func (iv *Interpreter) builtinMap(args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, fmtErr("map() expects a list, got %s", args[0].Type())
	}
	out := make([]object.Object, len(list.Elems))
	for i, e := range list.Elems {
		out[i] = iv.call(args[1], []object.Object{e}, pos)
	}
	return &object.List{Elems: out}, nil
}

// builtinFilter keeps the elements for which the predicate returns truthy.
func (iv *Interpreter) builtinFilter(args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, fmtErr("filter() expects a list, got %s", args[0].Type())
	}
	var out []object.Object
	for _, e := range list.Elems {
		if truthy(iv.call(args[1], []object.Object{e}, pos)) {
			out = append(out, e)
		}
	}
	return &object.List{Elems: out}, nil
}

// builtinFold reduces a list with a binary function and an initial value.
func (iv *Interpreter) builtinFold(args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, fmtErr("fold() expects a list, got %s", args[0].Type())
	}
	acc := args[1]
	for _, e := range list.Elems {
		acc = iv.call(args[2], []object.Object{acc, e}, pos)
	}
	return acc, nil
}

func (iv *Interpreter) builtinUpper(args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.ToUpper, "upper()")
}

func (iv *Interpreter) builtinLower(args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.ToLower, "lower()")
}

func (iv *Interpreter) builtinTrim(args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.TrimSpace, "trim()")
}

func stringOp(args []object.Object, fn func(string) string, name string) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("%s expects a string, got %s", name, args[0].Type())
	}
	return object.Str{Value: fn(s.Value)}, nil
}

func (iv *Interpreter) builtinStartsWith(args []object.Object, pos source.Pos) (object.Object, error) {
	return prefixOp(args, strings.HasPrefix, "starts_with()")
}

func (iv *Interpreter) builtinEndsWith(args []object.Object, pos source.Pos) (object.Object, error) {
	return prefixOp(args, strings.HasSuffix, "ends_with()")
}

func prefixOp(args []object.Object, fn func(string, string) bool, name string) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("%s expects a string, got %s", name, args[0].Type())
	}
	part, ok := args[1].(object.Str)
	if !ok {
		return nil, fmtErr("%s expects a string, got %s", name, args[1].Type())
	}
	return object.Bool{Value: fn(s.Value, part.Value)}, nil
}

func (iv *Interpreter) builtinContains(args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("contains() expects a string, got %s", args[0].Type())
	}
	sub, ok := args[1].(object.Str)
	if !ok {
		return nil, fmtErr("contains() expects a string, got %s", args[1].Type())
	}
	return object.Bool{Value: strings.Contains(s.Value, sub.Value)}, nil
}

func (iv *Interpreter) builtinRepeat(args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, fmtErr("repeat() expects a string, got %s", args[0].Type())
	}
	n, ok := asIntIndex(args[1])
	if !ok {
		return nil, fmtErr("repeat() count must be an integer, got %s", args[1].Type())
	}
	if n < 0 {
		return nil, fmtErr("repeat() count must not be negative")
	}
	return object.Str{Value: strings.Repeat(s.Value, int(n))}, nil
}

func (iv *Interpreter) builtinAbs(args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Int:
		if v.Value < 0 {
			return object.Int{Value: -v.Value}, nil
		}
		return v, nil
	case object.Float:
		return object.Float{Value: math.Abs(v.Value)}, nil
	}
	return nil, fmtErr("abs() expects a number, got %s", args[0].Type())
}

func (iv *Interpreter) builtinMin(args []object.Object, pos source.Pos) (object.Object, error) {
	return minMax(args, true)
}

func (iv *Interpreter) builtinMax(args []object.Object, pos source.Pos) (object.Object, error) {
	return minMax(args, false)
}

func minMax(args []object.Object, wantMin bool) (object.Object, error) {
	best := args[0]
	if _, _, ok := asNumber(best); !ok {
		return nil, fmtErr("min() and max() expect numbers, got %s", best.Type())
	}
	bestIsFloat := isFloat(best)
	for _, a := range args[1:] {
		if _, _, ok := asNumber(a); !ok {
			return nil, fmtErr("min() and max() expect numbers, got %s", a.Type())
		}
		cmp, ok := compareValues(a, best)
		if !ok {
			return nil, fmtErr("cannot compare %s with %s", a.Type(), best.Type())
		}
		if (wantMin && cmp < 0) || (!wantMin && cmp > 0) {
			best = a
		}
		bestIsFloat = bestIsFloat || isFloat(a)
	}
	if bestIsFloat {
		return object.Float{Value: asFloat(best)}, nil
	}
	return object.Int{Value: asInt(best)}, nil
}

func (iv *Interpreter) builtinFloor(args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := asNumber(args[0]); !ok {
		return nil, fmtErr("floor() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Floor(asFloat(args[0])))}, nil
}

func (iv *Interpreter) builtinCeil(args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := asNumber(args[0]); !ok {
		return nil, fmtErr("ceil() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Ceil(asFloat(args[0])))}, nil
}

func (iv *Interpreter) builtinRound(args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := asNumber(args[0]); !ok {
		return nil, fmtErr("round() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Round(asFloat(args[0])))}, nil
}

func (iv *Interpreter) builtinSqrt(args []object.Object, pos source.Pos) (object.Object, error) {
	f := asFloat(args[0])
	if _, _, ok := asNumber(args[0]); !ok {
		return nil, fmtErr("sqrt() expects a number, got %s", args[0].Type())
	}
	if f < 0 {
		return nil, fmtErr("sqrt() of a negative number")
	}
	return object.Float{Value: math.Sqrt(f)}, nil
}

func (iv *Interpreter) builtinAssert(args []object.Object, pos source.Pos) (object.Object, error) {
	if truthy(args[0]) {
		return object.NilValue, nil
	}
	if len(args) == 2 {
		if msg, ok := args[1].(object.Str); ok {
			return nil, fmtErr("assertion failed: %s", msg.Value)
		}
	}
	return nil, fmtErr("assertion failed")
}
