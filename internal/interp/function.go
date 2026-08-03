package interp

import (
	"fmt"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// Function is a user-defined function with its captured environment.
type Function struct {
	Name   string
	Params []*ast.Param
	Body   *ast.Block
	Env    *Env
}

func (f *Function) Type() object.Type { return object.TypeFunction }

func (f *Function) String() string {
	var params []string
	for _, p := range f.Params {
		params = append(params, p.Name.Name)
	}
	if f.Name != "" {
		return fmt.Sprintf("<fn %s (%s)>", f.Name, strings.Join(params, ", "))
	}
	return fmt.Sprintf("<fn (%s)>", strings.Join(params, ", "))
}

// Builtin is a function implemented in Go.
//
// Fn is a bound method on the interpreter, so it can reach the streams and
// call other functions. pos is the source position of the call site.
type Builtin struct {
	Name    string
	MinArgs int
	// MaxArgs is the maximum number of arguments, or -1 for no limit.
	MaxArgs int
	Fn      func(args []object.Object, pos source.Pos) (object.Object, error)
}

func (b *Builtin) Type() object.Type { return object.TypeFunction }
func (b *Builtin) String() string    { return "<builtin " + b.Name + ">" }
