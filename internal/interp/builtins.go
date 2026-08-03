package interp

import (
	"github.com/sprout-lang/sprout/internal/runtime"
)

// BuiltinNames lists the predeclared standard library functions.
//
// The names come from the shared runtime package, which both execution
// engines use. The checker consults this list to validate programs.
var BuiltinNames = runtime.Names

// RegisterBuiltins binds the standard library into iv's globals.
func RegisterBuiltins(iv *Interpreter) {
	runtime.Register(func(b *runtime.Builtin) {
		iv.globals.Define(b.Name, b, true)
	})
}
