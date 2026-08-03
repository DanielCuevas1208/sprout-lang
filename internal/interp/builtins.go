package interp

import (
	"github.com/sprout-lang/sprout/internal/runtime"
)

// BuiltinNames lists the predeclared standard library functions.
//
// The names come from the shared runtime package, which both execution
// engines use. It remains exported for compatibility with external callers.
var BuiltinNames = runtime.Names

// RegisterBuiltins binds the standard library into env.
func RegisterBuiltins(env *Env) {
	runtime.Register(func(b *runtime.Builtin) {
		env.Define(b.Name, b, true)
	})
}
