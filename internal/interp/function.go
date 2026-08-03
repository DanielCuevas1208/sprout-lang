// Package interp is the tree-walking interpreter for Sprout.
package interp

import (
	"fmt"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
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
// It is the shared runtime.Builtin value. The interpreter registers these
// values into its globals and calls them through the runtime Context.
type Builtin = runtime.Builtin
