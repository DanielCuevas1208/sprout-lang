// Package runtime defines the value semantics and standard library of Sprout.
//
// Both execution engines use this package. The tree-walking interpreter and
// the bytecode virtual machine share the same arithmetic, comparison,
// indexing, iteration, and builtin behavior. This keeps the two engines
// consistent and keeps the standard library in one place.
//
// An engine provides a Context with its streams and a Call hook. The Call
// hook runs a function value with a set of arguments. Engines use the hook
// to call user functions from higher-order builtins such as map and fold.
package runtime

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// Context carries the engine hooks that builtins need.
type Context struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Input is the buffered reader used by input(). Engines leave it nil.
	// The input() builtin creates it lazily from Stdin.
	Input *bufio.Reader

	// Call invokes fn with args. Engines use it to call user functions.
	// The hook reports an error by panicking with the engine's signal.
	Call func(fn object.Object, args []object.Object, pos source.Pos) object.Object
}

// Builtin is one standard library function.
//
// A Builtin is a runtime value, so it implements object.Object. Engines put
// these values into their globals and call Fn to run them.
type Builtin struct {
	Name    string
	MinArgs int
	// MaxArgs is the maximum number of arguments, or -1 for no limit.
	MaxArgs int
	Fn      func(ctx *Context, args []object.Object, pos source.Pos) (object.Object, error)
}

// Type reports the runtime type of a builtin.
func (b *Builtin) Type() object.Type { return object.TypeFunction }

// String renders a builtin for display.
func (b *Builtin) String() string { return "<builtin " + b.Name + ">" }

// CheckArgs verifies that len(args) fits the argument contract.
func (b *Builtin) CheckArgs(args []object.Object, name string) error {
	if len(args) < b.MinArgs || (b.MaxArgs >= 0 && len(args) > b.MaxArgs) {
		return FmtErr("function '%s' expects %d arguments, got %d", name, b.MinArgs, len(args))
	}
	return nil
}

// Builtins lists the standard library in registration order.
var Builtins = []*Builtin{
	{Name: "print", MinArgs: 0, MaxArgs: -1, Fn: builtinPrint},
	{Name: "eprint", MinArgs: 0, MaxArgs: -1, Fn: builtinEprint},
	{Name: "write", MinArgs: 0, MaxArgs: -1, Fn: builtinWrite},
	{Name: "len", MinArgs: 1, MaxArgs: 1, Fn: builtinLen},
	{Name: "str", MinArgs: 1, MaxArgs: 1, Fn: builtinStr},
	{Name: "int", MinArgs: 1, MaxArgs: 1, Fn: builtinInt},
	{Name: "float", MinArgs: 1, MaxArgs: 1, Fn: builtinFloat},
	{Name: "bool", MinArgs: 1, MaxArgs: 1, Fn: builtinBool},
	{Name: "type", MinArgs: 1, MaxArgs: 1, Fn: builtinType},
	{Name: "range", MinArgs: 2, MaxArgs: 3, Fn: builtinRange},
	{Name: "input", MinArgs: 0, MaxArgs: 1, Fn: builtinInput},
	{Name: "push", MinArgs: 2, MaxArgs: 2, Fn: builtinPush},
	{Name: "pop", MinArgs: 1, MaxArgs: 1, Fn: builtinPop},
	{Name: "get", MinArgs: 2, MaxArgs: 2, Fn: builtinGet},
	{Name: "set", MinArgs: 3, MaxArgs: 3, Fn: builtinSet},
	{Name: "has", MinArgs: 2, MaxArgs: 2, Fn: builtinHas},
	{Name: "keys", MinArgs: 1, MaxArgs: 1, Fn: builtinKeys},
	{Name: "values", MinArgs: 1, MaxArgs: 1, Fn: builtinValues},
	{Name: "join", MinArgs: 2, MaxArgs: 2, Fn: builtinJoin},
	{Name: "split", MinArgs: 2, MaxArgs: 2, Fn: builtinSplit},
	{Name: "map", MinArgs: 2, MaxArgs: 2, Fn: builtinMap},
	{Name: "filter", MinArgs: 2, MaxArgs: 2, Fn: builtinFilter},
	{Name: "fold", MinArgs: 3, MaxArgs: 3, Fn: builtinFold},
	{Name: "upper", MinArgs: 1, MaxArgs: 1, Fn: builtinUpper},
	{Name: "lower", MinArgs: 1, MaxArgs: 1, Fn: builtinLower},
	{Name: "trim", MinArgs: 1, MaxArgs: 1, Fn: builtinTrim},
	{Name: "starts_with", MinArgs: 2, MaxArgs: 2, Fn: builtinStartsWith},
	{Name: "ends_with", MinArgs: 2, MaxArgs: 2, Fn: builtinEndsWith},
	{Name: "contains", MinArgs: 2, MaxArgs: 2, Fn: builtinContains},
	{Name: "repeat", MinArgs: 2, MaxArgs: 2, Fn: builtinRepeat},
	{Name: "abs", MinArgs: 1, MaxArgs: 1, Fn: builtinAbs},
	{Name: "min", MinArgs: 1, MaxArgs: -1, Fn: builtinMin},
	{Name: "max", MinArgs: 1, MaxArgs: -1, Fn: builtinMax},
	{Name: "floor", MinArgs: 1, MaxArgs: 1, Fn: builtinFloor},
	{Name: "ceil", MinArgs: 1, MaxArgs: 1, Fn: builtinCeil},
	{Name: "round", MinArgs: 1, MaxArgs: 1, Fn: builtinRound},
	{Name: "sqrt", MinArgs: 1, MaxArgs: 1, Fn: builtinSqrt},
	{Name: "assert", MinArgs: 1, MaxArgs: 2, Fn: builtinAssert},
	{Name: "module", MinArgs: 2, MaxArgs: 2, Fn: builtinModule},
}

// Names lists the standard library function names.
var Names = func() []string {
	names := make([]string, len(Builtins))
	for i, b := range Builtins {
		names[i] = b.Name
	}
	return names
}()

// Register calls define for every builtin.
//
// Engines use this to install the standard library into their globals.
func Register(define func(b *Builtin)) {
	for _, b := range Builtins {
		define(b)
	}
}

// FmtErr builds an error with a formatted message.
func FmtErr(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// Truthy reports whether o counts as true in a condition.
//
// Only nil and false are falsy.
func Truthy(o object.Object) bool {
	switch v := o.(type) {
	case object.Nil:
		return false
	case object.Bool:
		return v.Value
	}
	return true
}

// Equal reports whether a and b have the same value.
func Equal(a, b object.Object) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type() != b.Type() {
		an, af, aNum := AsNumber(a)
		bn, bf, bNum := AsNumber(b)
		if aNum && bNum {
			if isFloat(a) || isFloat(b) {
				return af == bf
			}
			return an == bn
		}
		return false
	}
	switch x := a.(type) {
	case object.Int:
		return x.Value == b.(object.Int).Value
	case object.Float:
		return x.Value == b.(object.Float).Value
	case object.Str:
		return x.Value == b.(object.Str).Value
	case object.Bool:
		return x.Value == b.(object.Bool).Value
	case object.Nil:
		return true
	case object.Range:
		y := b.(object.Range)
		return x == y
	case *object.List:
		y := b.(*object.List)
		if len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !Equal(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *object.Map:
		y := b.(*object.Map)
		if len(x.Keys) != len(y.Keys) {
			return false
		}
		for i, k := range x.Keys {
			if k != y.Keys[i] {
				return false
			}
			if !Equal(x.Vals[k], y.Vals[k]) {
				return false
			}
		}
		return true
	case object.Object:
		return a == b
	}
	return false
}

// AsNumber views o as a number. It returns the int and float forms and
// whether o is a number.
func AsNumber(o object.Object) (int64, float64, bool) {
	switch v := o.(type) {
	case object.Int:
		return v.Value, float64(v.Value), true
	case object.Float:
		return 0, v.Value, true
	}
	return 0, 0, false
}

// AsInt returns the int value of o, or 0 when o is not an int.
func AsInt(o object.Object) int64 {
	if v, ok := o.(object.Int); ok {
		return v.Value
	}
	return 0
}

// AsFloat returns the float value of o, or 0 when o is not a number.
func AsFloat(o object.Object) float64 {
	if v, ok := o.(object.Float); ok {
		return v.Value
	}
	if v, ok := o.(object.Int); ok {
		return float64(v.Value)
	}
	return 0
}

// AsIntIndex views o as an index into a container.
func AsIntIndex(o object.Object) (int64, bool) {
	if v, ok := o.(object.Int); ok {
		return v.Value, true
	}
	return 0, false
}

func isFloat(o object.Object) bool {
	_, ok := o.(object.Float)
	return ok
}

// Compare orders a and b. It reports whether the values are comparable.
func Compare(a, b object.Object) (int, bool) {
	if _, af, aNum := AsNumber(a); aNum {
		if _, bf, bNum := AsNumber(b); bNum {
			switch {
			case isFloat(a), isFloat(b):
				return compareFloats(af, bf), true
			default:
				return compareInts(AsInt(a), AsInt(b)), true
			}
		}
	}
	if as, aStr := a.(object.Str); aStr {
		if bs, bStr := b.(object.Str); bStr {
			return strings.Compare(as.Value, bs.Value), true
		}
	}
	return 0, false
}

func compareInts(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func compareFloats(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Add sums two values.
func Add(a, b object.Object) (object.Object, error) {
	switch x := a.(type) {
	case object.Int:
		switch y := b.(type) {
		case object.Int:
			return object.Int{Value: x.Value + y.Value}, nil
		case object.Float:
			return object.Float{Value: float64(x.Value) + y.Value}, nil
		}
	case object.Float:
		switch y := b.(type) {
		case object.Int:
			return object.Float{Value: x.Value + float64(y.Value)}, nil
		case object.Float:
			return object.Float{Value: x.Value + y.Value}, nil
		}
	case object.Str:
		if y, ok := b.(object.Str); ok {
			return object.Str{Value: x.Value + y.Value}, nil
		}
	}
	return nil, FmtErr("cannot add %s and %s", a.Type(), b.Type())
}

// Sub subtracts b from a.
func Sub(a, b object.Object) (object.Object, error) {
	if _, af, aNum := AsNumber(a); aNum {
		if _, bf, bNum := AsNumber(b); bNum {
			if _, aF := a.(object.Float); aF {
				return object.Float{Value: af - bf}, nil
			}
			if _, bF := b.(object.Float); bF {
				return object.Float{Value: af - bf}, nil
			}
			return object.Int{Value: AsInt(a) - AsInt(b)}, nil
		}
	}
	return nil, FmtErr("cannot subtract %s from %s", b.Type(), a.Type())
}

// Mul multiplies two values.
func Mul(a, b object.Object) (object.Object, error) {
	if _, af, aNum := AsNumber(a); aNum {
		if _, bf, bNum := AsNumber(b); bNum {
			if _, aF := a.(object.Float); aF {
				return object.Float{Value: af * bf}, nil
			}
			if _, bF := b.(object.Float); bF {
				return object.Float{Value: af * bf}, nil
			}
			return object.Int{Value: AsInt(a) * AsInt(b)}, nil
		}
	}
	return nil, FmtErr("cannot multiply %s and %s", a.Type(), b.Type())
}

// Div divides a by b.
func Div(a, b object.Object) (object.Object, error) {
	if _, _, aNum := AsNumber(a); !aNum {
		return nil, FmtErr("cannot divide a %s", a.Type())
	}
	bn, bf, bNum := AsNumber(b)
	if !bNum {
		return nil, FmtErr("cannot divide by a %s", b.Type())
	}
	if (bn == 0 && !isFloat(b)) || (bNum && isFloat(b) && bf == 0) {
		return nil, FmtErr("cannot divide by zero")
	}
	if _, aF := a.(object.Float); aF {
		return object.Float{Value: AsFloat(a) / bf}, nil
	}
	if _, bF := b.(object.Float); bF {
		return object.Float{Value: AsFloat(a) / bf}, nil
	}
	return object.Int{Value: AsInt(a) / bn}, nil
}

// Mod takes the remainder of a divided by b.
func Mod(a, b object.Object) (object.Object, error) {
	if _, _, aNum := AsNumber(a); !aNum {
		return nil, FmtErr("cannot take the remainder of a %s", a.Type())
	}
	if _, _, bNum := AsNumber(b); !bNum {
		return nil, FmtErr("cannot take the remainder by a %s", b.Type())
	}
	if _, aF := a.(object.Float); aF {
		bf := AsFloat(b)
		if bf == 0 {
			return nil, FmtErr("cannot take the remainder by zero")
		}
		return object.Float{Value: floatMod(AsFloat(a), bf)}, nil
	}
	if _, bF := b.(object.Float); bF {
		if bf := AsFloat(b); bf == 0 {
			return nil, FmtErr("cannot take the remainder by zero")
		}
		return object.Float{Value: floatMod(AsFloat(a), AsFloat(b))}, nil
	}
	bn := AsInt(b)
	if bn == 0 {
		return nil, FmtErr("cannot take the remainder by zero")
	}
	return object.Int{Value: AsInt(a) % bn}, nil
}

// Pow raises a to the power b.
func Pow(a, b object.Object) (object.Object, error) {
	if _, _, aNum := AsNumber(a); !aNum {
		return nil, FmtErr("cannot raise a %s to a power", a.Type())
	}
	if _, _, bNum := AsNumber(b); !bNum {
		return nil, FmtErr("cannot raise to a %s power", b.Type())
	}
	if _, aF := a.(object.Float); aF {
		return object.Float{Value: floatPow(AsFloat(a), AsFloat(b))}, nil
	}
	if _, bF := b.(object.Float); bF {
		return object.Float{Value: floatPow(AsFloat(a), AsFloat(b))}, nil
	}
	exponent := AsInt(b)
	if exponent < 0 {
		return object.Float{Value: floatPow(AsFloat(a), AsFloat(b))}, nil
	}
	return object.Int{Value: intPow(AsInt(a), exponent)}, nil
}

// IndexGet reads an element from a container.
func IndexGet(container, idx object.Object) (object.Object, error) {
	switch c := container.(type) {
	case *object.List:
		i, ok := AsIntIndex(idx)
		if !ok {
			return nil, FmtErr("list index must be an integer")
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return nil, FmtErr("list index %d out of range (length %d)", i, len(c.Elems))
		}
		return c.Elems[i], nil
	case object.Str:
		i, ok := AsIntIndex(idx)
		if !ok {
			return nil, FmtErr("string index must be an integer")
		}
		runes := []rune(c.Value)
		if i < 0 || i >= int64(len(runes)) {
			return nil, FmtErr("string index %d out of range (length %d)", i, len(runes))
		}
		return object.Str{Value: string(runes[i])}, nil
	case *object.Map:
		ks, ok := idx.(object.Str)
		if !ok {
			return nil, FmtErr("map key must be a string")
		}
		if v, exists := c.Get(ks.Value); exists {
			return v, nil
		}
		return object.NilValue, nil
	case *object.Module:
		ks, ok := idx.(object.Str)
		if !ok {
			return nil, FmtErr("module member must be a string")
		}
		if v, exists := c.Exports[ks.Value]; exists {
			return v, nil
		}
		return nil, FmtErr("module '%s' has no exported member '%s'", c.Name, ks.Value)
	}
	return nil, FmtErr("cannot index a %s", container.Type())
}

// SetIndex stores value into a container and returns value.
func SetIndex(container, idx, value object.Object) (object.Object, error) {
	switch c := container.(type) {
	case *object.List:
		i, ok := AsIntIndex(idx)
		if !ok {
			return nil, FmtErr("list index must be an integer")
		}
		if i < 0 || i >= int64(len(c.Elems)) {
			return nil, FmtErr("list index %d out of range (length %d)", i, len(c.Elems))
		}
		c.Elems[i] = value
		return value, nil
	case *object.Map:
		ks, ok := idx.(object.Str)
		if !ok {
			return nil, FmtErr("map key must be a string")
		}
		c.Set(ks.Value, value)
		return value, nil
	case *object.Module:
		return nil, FmtErr("cannot assign to a member of module '%s'", c.Name)
	}
	return nil, FmtErr("cannot assign to an index of a %s", container.Type())
}

// Sequence materializes an iterable value as a slice.
func Sequence(o object.Object) ([]object.Object, error) {
	switch v := o.(type) {
	case *object.List:
		return v.Elems, nil
	case object.Str:
		runes := []rune(v.Value)
		out := make([]object.Object, len(runes))
		for i, r := range runes {
			out[i] = object.Str{Value: string(r)}
		}
		return out, nil
	case object.Range:
		var out []object.Object
		step := v.Step
		if step == 0 {
			step = 1
		}
		for i := v.Start; i < v.End; i += step {
			out = append(out, object.Int{Value: i})
		}
		return out, nil
	}
	return nil, FmtErr("cannot iterate a %s", o.Type())
}

func floatMod(a, b float64) float64 {
	return math.Mod(a, b)
}

func floatPow(a, b float64) float64 {
	return math.Pow(a, b)
}

// IntPow raises base to a non-negative integer exponent.
func IntPow(base, exponent int64) int64 {
	result := int64(1)
	for exponent > 0 {
		if exponent&1 == 1 {
			result *= base
		}
		exponent >>= 1
		if exponent > 0 {
			base *= base
		}
	}
	return result
}

func intPow(base, exponent int64) int64 {
	return IntPow(base, exponent)
}
