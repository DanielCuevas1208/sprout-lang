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
	// File is the source file that defined the function. Runtime errors and
	// stack frames use it so diagnostics point at the right module.
	File *source.File
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
