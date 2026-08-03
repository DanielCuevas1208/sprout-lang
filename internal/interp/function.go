package interp

import (
	"fmt"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/object"
)

// Function is a user-defined function with its captured environment.
type Function struct {
	Name   string
	Params []*ast.Param
	Body   *ast.Block
	Env    *Env
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
