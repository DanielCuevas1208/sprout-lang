package object

import "testing"

func TestModuleMembers(t *testing.T) {
	m := NewModule("greeting", "lib/greeting.spr")
	m.Set("pi", Int{Value: 3})
	m.Set("hi", Str{Value: "hello"})
	m.Set("pi", Int{Value: 5})

	if m.Type() != TypeModule {
		t.Errorf("module type: got %q", m.Type())
	}
	if m.String() != "<module greeting>" {
		t.Errorf("module string: got %q", m.String())
	}
	if v, ok := m.Get("pi"); !ok || v.String() != "5" {
		t.Errorf("module pi: got %v, ok=%v", v, ok)
	}
	if _, ok := m.Get("missing"); ok {
		t.Error("expected a missing member")
	}

	keys := m.Keys()
	want := []string{"pi", "hi"}
	if len(keys) != len(want) {
		t.Fatalf("keys: got %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key %d: got %q, want %q", i, keys[i], want[i])
		}
	}
}
