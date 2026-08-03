package interp

import (
	"fmt"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
)

// Function is a user-defined function with its captured environment.
//
// File is the source file that defined the function. The interpreter switches
// to it while the body runs, so diagnostics and import resolution point at
// the function's own file even when the function crosses module boundaries.
type Function struct {
	Name   string
	Params []*ast.Param
	Body   *ast.Block
	Env    *Env
	File   *source.File
}

// Type reports the runtime type of a function.
func (f *Function) Type() object.Type { return object.TypeFunction }

// String renders a function for display.
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
