package interp

import (
	"fmt"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
)

// spawn runs a function in a child interpreter. The child owns its call
// frames, diagnostics, and builtin context, but closures keep their captured
// environment and channel values.
func (iv *Interpreter) spawn(fn object.Object, args []object.Object, pos source.Pos) (*object.Task, error) {
	if !isSpawnable(fn) {
		return nil, fmt.Errorf("spawn() expects a function, got %s", fn.Type())
	}
	task := object.NewTask()
	go func() {
		child := NewWithIO(iv.ctx.Stdin, iv.ctx.Stdout, iv.ctx.Stderr)
		child.ctx.Done = task.Done()
		child.ctx.Scheduler = iv.ctx.Scheduler
		child.resolver = iv.resolver
		var value object.Object
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				switch failure := recovered.(type) {
				case *runErr:
					err = fmt.Errorf("%s", failure.message)
				default:
					err = fmt.Errorf("task failed: %v", recovered)
				}
			}
			task.Complete(value, err)
		}()
		value = child.call(fn, args, pos)
	}()
	return task, nil
}

func isSpawnable(fn object.Object) bool {
	switch fn.(type) {
	case *Function, *object.BoundMethod, *runtime.Builtin:
		return true
	default:
		return false
	}
}
