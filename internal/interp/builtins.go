package interp

import (
	"github.com/sprout-lang/sprout/internal/runtime"
)

// BuiltinNames lists the predeclared standard library functions.
//
// The names come from the shared runtime package, which both execution
// engines use. The checker consults this list to validate programs.
var BuiltinNames = runtime.Names

// RegisterBuiltins binds the standard library into iv's builtin scope.
//
// Builtins live in their own environment. The global scope and every module
// scope use it as their parent, so modules can call the standard library
// without seeing the entry file's top-level names.
func RegisterBuiltins(iv *Interpreter) {
	runtime.Register(func(b *runtime.Builtin) {
		iv.builtins.Define(b.Name, b, true)
	})
}
