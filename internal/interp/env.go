package interp

import (
	"sync"

	"github.com/sprout-lang/sprout/internal/object"
)

// binding is one declared name in an environment.
type binding struct {
	value   object.Object
	isConst bool
}

// Env is a lexical scope that chains to its parent.
//
// Name lookup walks the chain from the innermost scope outward. A lock keeps
// captured environments safe when a child task runs beside its parent.
type Env struct {
	parent *Env
	mu     sync.RWMutex
	vars   map[string]*binding
}

// NewEnv returns an environment whose parent is parent.
func NewEnv(parent *Env) *Env {
	return &Env{parent: parent, vars: make(map[string]*binding)}
}

// Define binds name to value in this environment.
func (e *Env) Define(name string, value object.Object, isConst bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vars[name] = &binding{value: value, isConst: isConst}
}

// Get returns the value bound to name, searching enclosing scopes.
func (e *Env) Get(name string) (object.Object, error) {
	for env := e; env != nil; env = env.parent {
		env.mu.RLock()
		b, ok := env.vars[name]
		if ok {
			value := b.value
			env.mu.RUnlock()
			return value, nil
		}
		env.mu.RUnlock()
	}
	return nil, fmtErr("undefined name '%s'", name)
}

// Assign stores value into the nearest binding of name.
func (e *Env) Assign(name string, value object.Object) error {
	for env := e; env != nil; env = env.parent {
		env.mu.Lock()
		b, ok := env.vars[name]
		if ok {
			if b.isConst {
				env.mu.Unlock()
				return fmtErr("cannot assign to constant '%s'", name)
			}
			b.value = value
			env.mu.Unlock()
			return nil
		}
		env.mu.Unlock()
	}
	return fmtErr("cannot assign to undefined name '%s'", name)
}

// IsConst reports whether name is bound to a constant.
func (e *Env) IsConst(name string) bool {
	for env := e; env != nil; env = env.parent {
		env.mu.RLock()
		b, ok := env.vars[name]
		env.mu.RUnlock()
		if ok {
			return b.isConst
		}
	}
	return false
}
