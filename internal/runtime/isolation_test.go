package runtime

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/object"
)

type captureStub struct {
	values []object.Object
}

func (c *captureStub) Type() object.Type { return object.TypeFunction }
func (c *captureStub) String() string    { return "<capture>" }
func (c *captureStub) TaskCaptureValues() []object.Object {
	return c.values
}

func TestValidateTaskCapture(t *testing.T) {
	structValue := &object.Struct{
		StructType: &object.StructType{Name: "Box"},
		Values:     map[string]object.Object{"value": object.Int{Value: 1}},
	}
	cases := []struct {
		name    string
		value   object.Object
		wantErr string
	}{
		{name: "scalar", value: object.Int{Value: 1}},
		{name: "channel", value: object.NewChannel(1)},
		{name: "task", value: object.NewTask()},
		{name: "list", value: &object.List{Elems: []object.Object{object.Int{Value: 1}}}, wantErr: "mutable list"},
		{name: "map", value: &object.Map{Vals: map[string]object.Object{"answer": object.Int{Value: 42}}}, wantErr: "mutable map"},
		{name: "struct", value: structValue, wantErr: "mutable struct"},
		{name: "nested result", value: object.Result{Ok: true, Value: &object.List{}}, wantErr: "mutable list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTaskCapture(&captureStub{values: []object.Object{tc.value}})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTaskCapture() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateTaskCapture() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
