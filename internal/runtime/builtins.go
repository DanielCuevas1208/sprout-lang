// This file implements the standard library functions of Sprout.
//
// Every builtin follows the same shape. It reads its arguments, does its
// work, and returns a value. It returns an error to report a problem. The
// engine turns that error into a source diagnostic.
//
// The higher-order builtins map, filter, and fold call user functions.
// They use ctx.Call, the engine hook for invoking a function value.
package runtime

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

func builtinPrint(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	writeLine(ctx.Stdout, strings.Join(parts, " "))
	return object.NilValue, nil
}

func builtinEprint(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.String()
	}
	writeLine(ctx.Stderr, strings.Join(parts, " "))
	return object.NilValue, nil
}

func builtinWrite(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	for _, a := range args {
		writeString(ctx.Stdout, a.String())
	}
	return object.NilValue, nil
}

func builtinLen(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Str:
		return object.Int{Value: int64(utf8.RuneCountInString(v.Value))}, nil
	case *object.List:
		return object.Int{Value: int64(len(v.Elems))}, nil
	case *object.Map:
		return object.Int{Value: int64(len(v.Keys))}, nil
	}
	return nil, FmtErr("len() expects a string, list, or map, got %s", args[0].Type())
}

func builtinStr(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Str{Value: args[0].String()}, nil
}

func builtinInt(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
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
			return nil, FmtErr("int() cannot convert %q to an integer", v.Value)
		}
		return object.Int{Value: n}, nil
	}
	return nil, FmtErr("int() cannot convert a %s", args[0].Type())
}

func builtinFloat(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
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
			return nil, FmtErr("float() cannot convert %q to a number", v.Value)
		}
		return object.Float{Value: f}, nil
	}
	return nil, FmtErr("float() cannot convert a %s", args[0].Type())
}

func builtinBool(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Bool{Value: Truthy(args[0])}, nil
}

func builtinType(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return object.Str{Value: string(args[0].Type())}, nil
}

func builtinRange(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	start, ok := AsIntIndex(args[0])
	if !ok {
		return nil, FmtErr("range() start must be an integer, got %s", args[0].Type())
	}
	end, ok := AsIntIndex(args[1])
	if !ok {
		return nil, FmtErr("range() end must be an integer, got %s", args[1].Type())
	}
	step := int64(1)
	if len(args) == 3 {
		step, ok = AsIntIndex(args[2])
		if !ok {
			return nil, FmtErr("range() step must be an integer, got %s", args[2].Type())
		}
		if step <= 0 {
			return nil, FmtErr("range() step must be a positive integer, got %d", step)
		}
	}
	return object.Range{Start: start, End: end, Step: step}, nil
}

func builtinInput(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if len(args) == 1 {
		if prompt, ok := args[0].(object.Str); ok {
			writeString(ctx.Stdout, prompt.Value)
		}
	}
	if ctx.Input == nil {
		ctx.Input = bufio.NewReader(ctx.Stdin)
	}
	line, err := ctx.Input.ReadString('\n')
	if err != nil && len(line) == 0 {
		return nil, FmtErr("input() reached the end of input")
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return object.Str{Value: line}, nil
}

func builtinPush(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, FmtErr("push() expects a list, got %s", args[0].Type())
	}
	list.Elems = append(list.Elems, args[1])
	return list, nil
}

func builtinPop(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, FmtErr("pop() expects a list, got %s", args[0].Type())
	}
	if len(list.Elems) == 0 {
		return nil, FmtErr("pop() on an empty list")
	}
	last := list.Elems[len(list.Elems)-1]
	list.Elems = list.Elems[:len(list.Elems)-1]
	return last, nil
}

func builtinGet(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	switch c := args[0].(type) {
	case *object.List:
		i, ok := AsIntIndex(args[1])
		if !ok {
			return nil, FmtErr("get() list index must be an integer, got %s", args[1].Type())
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return object.NilValue, nil
		}
		return c.Elems[i], nil
	case object.Str:
		i, ok := AsIntIndex(args[1])
		if !ok {
			return nil, FmtErr("get() string index must be an integer, got %s", args[1].Type())
		}
		runes := []rune(c.Value)
		if i < 0 || i >= int64(len(runes)) {
			return object.NilValue, nil
		}
		return object.Str{Value: string(runes[i])}, nil
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, FmtErr("get() map key must be a string, got %s", args[1].Type())
		}
		if v, exists := c.Get(key.Value); exists {
			return v, nil
		}
		return object.NilValue, nil
	}
	return nil, FmtErr("get() expects a list, string, or map, got %s", args[0].Type())
}

func builtinSet(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	value := args[2]
	switch c := args[0].(type) {
	case *object.List:
		i, ok := AsIntIndex(args[1])
		if !ok {
			return nil, FmtErr("set() list index must be an integer, got %s", args[1].Type())
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return nil, FmtErr("set() list index %d out of range (length %d)", i, len(c.Elems))
		}
		c.Elems[i] = value
		return c, nil
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, FmtErr("set() map key must be a string, got %s", args[1].Type())
		}
		c.Set(key.Value, value)
		return c, nil
	}
	return nil, FmtErr("set() expects a list or map, got %s", args[0].Type())
}

func builtinHas(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	switch c := args[0].(type) {
	case *object.Map:
		key, ok := args[1].(object.Str)
		if !ok {
			return nil, FmtErr("has() map key must be a string, got %s", args[1].Type())
		}
		_, exists := c.Get(key.Value)
		return object.Bool{Value: exists}, nil
	case *object.List:
		i, ok := AsIntIndex(args[1])
		if !ok {
			return nil, FmtErr("has() list index must be an integer, got %s", args[1].Type())
		}
		return object.Bool{Value: i >= 0 && i < int64(len(c.Elems))}, nil
	case object.Str:
		i, ok := AsIntIndex(args[1])
		if !ok {
			return nil, FmtErr("has() string index must be an integer, got %s", args[1].Type())
		}
		return object.Bool{Value: i >= 0 && i < int64(utf8.RuneCountInString(c.Value))}, nil
	}
	return nil, FmtErr("has() expects a list, string, or map, got %s", args[0].Type())
}

func builtinKeys(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	m, ok := args[0].(*object.Map)
	if !ok {
		return nil, FmtErr("keys() expects a map, got %s", args[0].Type())
	}
	keys := make([]object.Object, len(m.Keys))
	for i, k := range m.Keys {
		keys[i] = object.Str{Value: k}
	}
	return &object.List{Elems: keys}, nil
}

func builtinValues(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	m, ok := args[0].(*object.Map)
	if !ok {
		return nil, FmtErr("values() expects a map, got %s", args[0].Type())
	}
	vals := make([]object.Object, len(m.Keys))
	for i, k := range m.Keys {
		vals[i] = m.Vals[k]
	}
	return &object.List{Elems: vals}, nil
}

func builtinJoin(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	sep, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("join() separator must be a string, got %s", args[0].Type())
	}
	list, ok := args[1].(*object.List)
	if !ok {
		return nil, FmtErr("join() expects a list, got %s", args[1].Type())
	}
	parts := make([]string, len(list.Elems))
	for i, e := range list.Elems {
		parts[i] = e.String()
	}
	return object.Str{Value: strings.Join(parts, sep.Value)}, nil
}

func builtinSplit(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("split() expects a string, got %s", args[0].Type())
	}
	sep, ok := args[1].(object.Str)
	if !ok {
		return nil, FmtErr("split() separator must be a string, got %s", args[1].Type())
	}
	if sep.Value == "" {
		return nil, FmtErr("split() separator must not be empty")
	}
	parts := strings.Split(s.Value, sep.Value)
	elems := make([]object.Object, len(parts))
	for i, p := range parts {
		elems[i] = object.Str{Value: p}
	}
	return &object.List{Elems: elems}, nil
}

// builtinMap applies a function to every element of a list.
func builtinMap(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, FmtErr("map() expects a list, got %s", args[0].Type())
	}
	out := make([]object.Object, len(list.Elems))
	for i, e := range list.Elems {
		out[i] = ctx.Call(args[1], []object.Object{e}, pos)
	}
	return &object.List{Elems: out}, nil
}

// builtinFilter keeps the elements for which the predicate returns truthy.
func builtinFilter(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, FmtErr("filter() expects a list, got %s", args[0].Type())
	}
	var out []object.Object
	for _, e := range list.Elems {
		if Truthy(ctx.Call(args[1], []object.Object{e}, pos)) {
			out = append(out, e)
		}
	}
	return &object.List{Elems: out}, nil
}

// builtinFold reduces a list with a binary function and an initial value.
func builtinFold(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	list, ok := args[0].(*object.List)
	if !ok {
		return nil, FmtErr("fold() expects a list, got %s", args[0].Type())
	}
	acc := args[1]
	for _, e := range list.Elems {
		acc = ctx.Call(args[2], []object.Object{acc, e}, pos)
	}
	return acc, nil
}

func builtinUpper(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.ToUpper, "upper()")
}

func builtinLower(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.ToLower, "lower()")
}

func builtinTrim(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return stringOp(args, strings.TrimSpace, "trim()")
}

func stringOp(args []object.Object, fn func(string) string, name string) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("%s expects a string, got %s", name, args[0].Type())
	}
	return object.Str{Value: fn(s.Value)}, nil
}

func builtinStartsWith(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return prefixOp(args, strings.HasPrefix, "starts_with()")
}

func builtinEndsWith(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return prefixOp(args, strings.HasSuffix, "ends_with()")
}

func prefixOp(args []object.Object, fn func(string, string) bool, name string) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("%s expects a string, got %s", name, args[0].Type())
	}
	part, ok := args[1].(object.Str)
	if !ok {
		return nil, FmtErr("%s expects a string, got %s", name, args[1].Type())
	}
	return object.Bool{Value: fn(s.Value, part.Value)}, nil
}

func builtinContains(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("contains() expects a string, got %s", args[0].Type())
	}
	sub, ok := args[1].(object.Str)
	if !ok {
		return nil, FmtErr("contains() expects a string, got %s", args[1].Type())
	}
	return object.Bool{Value: strings.Contains(s.Value, sub.Value)}, nil
}

func builtinRepeat(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	s, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("repeat() expects a string, got %s", args[0].Type())
	}
	n, ok := AsIntIndex(args[1])
	if !ok {
		return nil, FmtErr("repeat() count must be an integer, got %s", args[1].Type())
	}
	if n < 0 {
		return nil, FmtErr("repeat() count must not be negative")
	}
	return object.Str{Value: strings.Repeat(s.Value, int(n))}, nil
}

func builtinAbs(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	switch v := args[0].(type) {
	case object.Int:
		if v.Value < 0 {
			return object.Int{Value: -v.Value}, nil
		}
		return v, nil
	case object.Float:
		return object.Float{Value: math.Abs(v.Value)}, nil
	}
	return nil, FmtErr("abs() expects a number, got %s", args[0].Type())
}

func builtinMin(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return minMax(args, true)
}

func builtinMax(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	return minMax(args, false)
}

func minMax(args []object.Object, wantMin bool) (object.Object, error) {
	best := args[0]
	if _, _, ok := AsNumber(best); !ok {
		return nil, FmtErr("min() and max() expect numbers, got %s", best.Type())
	}
	bestIsFloat := isFloat(best)
	for _, a := range args[1:] {
		if _, _, ok := AsNumber(a); !ok {
			return nil, FmtErr("min() and max() expect numbers, got %s", a.Type())
		}
		cmp, ok := Compare(a, best)
		if !ok {
			return nil, FmtErr("cannot compare %s with %s", a.Type(), best.Type())
		}
		if (wantMin && cmp < 0) || (!wantMin && cmp > 0) {
			best = a
		}
		bestIsFloat = bestIsFloat || isFloat(a)
	}
	if bestIsFloat {
		return object.Float{Value: AsFloat(best)}, nil
	}
	return object.Int{Value: AsInt(best)}, nil
}

func builtinFloor(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := AsNumber(args[0]); !ok {
		return nil, FmtErr("floor() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Floor(AsFloat(args[0])))}, nil
}

func builtinCeil(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := AsNumber(args[0]); !ok {
		return nil, FmtErr("ceil() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Ceil(AsFloat(args[0])))}, nil
}

func builtinRound(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := AsNumber(args[0]); !ok {
		return nil, FmtErr("round() expects a number, got %s", args[0].Type())
	}
	return object.Int{Value: int64(math.Round(AsFloat(args[0])))}, nil
}

func builtinSqrt(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if _, _, ok := AsNumber(args[0]); !ok {
		return nil, FmtErr("sqrt() expects a number, got %s", args[0].Type())
	}
	f := AsFloat(args[0])
	if f < 0 {
		return nil, FmtErr("sqrt() of a negative number")
	}
	return object.Float{Value: math.Sqrt(f)}, nil
}

func builtinAssert(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	if Truthy(args[0]) {
		return object.NilValue, nil
	}
	if len(args) == 2 {
		if msg, ok := args[1].(object.Str); ok {
			return nil, FmtErr("assertion failed: %s", msg.Value)
		}
	}
	return nil, FmtErr("assertion failed")
}

// builtinModule wraps a map into a module value.
//
// The build tool uses it to construct modules inside a bundle. Programs can
// use it to create module values dynamically.
func builtinModule(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error) {
	name, ok := args[0].(object.Str)
	if !ok {
		return nil, FmtErr("module() name must be a string, got %s", args[0].Type())
	}
	m, ok := args[1].(*object.Map)
	if !ok {
		return nil, FmtErr("module() exports must be a map, got %s", args[1].Type())
	}
	exports := make(map[string]object.Object, len(m.Keys))
	for _, k := range m.Keys {
		exports[k] = m.Vals[k]
	}
	return &object.Module{Name: name.Value, Exports: exports}, nil
}

func writeLine(w io.Writer, s string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintln(w, s)
}

func writeString(w io.Writer, s string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprint(w, s)
}
