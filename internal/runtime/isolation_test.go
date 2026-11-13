package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/source"
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
		{name: "nested closure", value: &captureStub{values: []object.Object{&object.Map{Vals: map[string]object.Object{"answer": object.Int{Value: 42}}}}}, wantErr: "mutable map"},
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

func TestValidateChannelValue(t *testing.T) {
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
		{name: "result scalar", value: object.Result{Ok: true, Value: object.Str{Value: "ready"}}},
		{name: "list", value: &object.List{Elems: []object.Object{object.Int{Value: 1}}}, wantErr: "mutable list"},
		{name: "map", value: &object.Map{Vals: map[string]object.Object{"answer": object.Int{Value: 42}}}, wantErr: "mutable map"},
		{name: "struct", value: structValue, wantErr: "mutable struct"},
		{name: "nested result", value: object.Result{Ok: true, Value: &object.List{}}, wantErr: "mutable list"},
		{name: "nested closure", value: &captureStub{values: []object.Object{&object.Map{Vals: map[string]object.Object{"answer": object.Int{Value: 42}}}}}, wantErr: "mutable map"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChannelValue(tc.value)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateChannelValue() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateChannelValue() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuiltinSendRejectsMutableValueBeforeBlocking(t *testing.T) {
	ch := object.NewChannel(0)
	done := make(chan error, 1)
	go func() {
		_, err := builtinSend(&Context{}, []object.Object{ch, &object.List{}}, source.Pos{})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "mutable list") {
			t.Fatalf("builtinSend() error = %v, want mutable list rejection", err)
		}
	case <-time.After(time.Second):
		t.Fatal("builtinSend() blocked before rejecting mutable value")
	}
}
