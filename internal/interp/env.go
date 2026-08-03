package interp

import (
	"github.com/sprout-lang/sprout/internal/object"
)

// binding is one declared name in an environment.
type binding struct {
	value   object.Object
	isConst bool
}

// Env is a lexical scope that chains to its parent.
//
// Name lookup walks the chain from the innermost scope outward. This gives
// Sprout closures the standard shared-environment semantics.
type Env struct {
	parent *Env
	vars   map[string]*binding
}

// NewEnv returns an environment whose parent is parent.
func NewEnv(parent *Env) *Env {
	return &Env{parent: parent, vars: make(map[string]*binding)}
}

// Define binds name to value in this environment.
func (e *Env) Define(name string, value object.Object, isConst bool) {
	e.vars[name] = &binding{value: value, isConst: isConst}
}

// Get returns the value bound to name, searching enclosing scopes.
func (e *Env) Get(name string) (object.Object, error) {
	b := e.lookup(name)
	if b == nil {
		return nil, fmtErr("undefined name '%s'", name)
	}
	return b.value, nil
}

// Assign stores value into the nearest binding of name.
func (e *Env) Assign(name string, value object.Object) error {
	b := e.lookup(name)
	if b == nil {
		return fmtErr("cannot assign to undefined name '%s'", name)
	}
	if b.isConst {
		return fmtErr("cannot assign to constant '%s'", name)
	}
	b.value = value
	return nil
}

// IsConst reports whether name is bound to a constant.
func (e *Env) IsConst(name string) bool {
	b := e.lookup(name)
	return b != nil && b.isConst
}

func (e *Env) lookup(name string) *binding {
	for env := e; env != nil; env = env.parent {
		if b, ok := env.vars[name]; ok {
			return b
		}
	}
	return nil
}
