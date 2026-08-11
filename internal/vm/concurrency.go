package vm

import (
	"fmt"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
)

// spawn runs a function in a child VM. The child owns its stack and frames,
// while closures keep their captured environment and channel values.
func (vm *VM) spawn(fn object.Object, args []object.Object, pos source.Pos) (*object.Task, error) {
	if !isSpawnable(fn) {
		return nil, fmt.Errorf("spawn() expects a function, got %s", fn.Type())
	}
	task := object.NewTask()
	go func() {
		child := NewWithIO(vm.ctx.Stdin, vm.ctx.Stdout, vm.ctx.Stderr)
		child.ctx.Done = task.Done()
		var value object.Object
		var err error
		defer func() {
			if recovered := recover(); recovered != nil {
				switch failure := recovered.(type) {
				case *vmErr:
					err = fmt.Errorf("%s", failure.msg)
				default:
					err = fmt.Errorf("task failed: %v", recovered)
				}
			}
			task.Complete(value, err)
		}()
		value = child.callValue(fn, args, pos)
	}()
	return task, nil
}

func isSpawnable(fn object.Object) bool {
	switch fn.(type) {
	case *Closure, *object.BoundMethod, *runtime.Builtin:
		return true
	default:
		return false
	}
}
