package runtime

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/sprout-lang/sprout/internal/object"
)

// TaskCapture exposes the values reachable from a user function's lexical
// environment. Engines implement it for their closure representations.
type TaskCapture interface {
	TaskCaptureValues() []object.Object
}

// ValidateTaskCapture enforces the task isolation policy.
//
// Spawned functions may capture scalar values, channels, and tasks. They may
// not capture lists, maps, or struct instances. Those values are mutable and
// would otherwise create a data race between a parent and child task.
func ValidateTaskCapture(fn object.Object) error {
	if kind := mutableCapture(fn, make(map[visitKey]struct{})); kind != "" {
		return fmt.Errorf("spawn() cannot capture mutable %s; use a channel to share task state", kind)
	}
	return nil
}

type visitKey struct {
	typeOf reflect.Type
	ptr    uintptr
}

func mutableCapture(value object.Object, seen map[visitKey]struct{}) string {
	if value == nil {
		return ""
	}
	if key, ok := pointerKey(value); ok {
		if _, exists := seen[key]; exists {
			return ""
		}
		seen[key] = struct{}{}
	}

	switch v := value.(type) {
	case *object.List:
		return "list"
	case *object.Map:
		return "map"
	case *object.Struct:
		return "struct"
	case object.Result:
		return mutableCapture(v.Value, seen)
	case *object.Module:
		keys := make([]string, 0, len(v.Exports))
		for key := range v.Exports {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if kind := mutableCapture(v.Exports[key], seen); kind != "" {
				return kind
			}
		}
	case *object.StructType:
		names := make([]string, 0, len(v.Methods))
		for name := range v.Methods {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if kind := mutableCapture(v.Methods[name], seen); kind != "" {
				return kind
			}
		}
	case *object.BoundMethod:
		if kind := mutableCapture(v.Receiver, seen); kind != "" {
			return kind
		}
		return mutableCapture(v.Method, seen)
	case TaskCapture:
		for _, captured := range v.TaskCaptureValues() {
			if kind := mutableCapture(captured, seen); kind != "" {
				return kind
			}
		}
	}
	return ""
}

func pointerKey(value object.Object) (visitKey, bool) {
	ref := reflect.ValueOf(value)
	if ref.Kind() != reflect.Ptr {
		return visitKey{}, false
	}
	return visitKey{typeOf: ref.Type(), ptr: ref.Pointer()}, true
}
