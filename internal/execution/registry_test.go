package execution

import "testing"

type testWorkflow struct{ name string }

func (w *testWorkflow) Name() string { return w.name }
func (w *testWorkflow) Run(ctx Context) (Result, error) {
	return Result{}, nil
}

func TestRegistry_Get_Unknown(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatalf("expected error for unknown workflow")
	}
}

func TestRegistry_Register_Invalid(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(nil); err == nil {
		t.Fatalf("expected error for nil workflow")
	}
	if err := r.Register(&testWorkflow{name: ""}); err == nil {
		t.Fatalf("expected error for empty workflow name")
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(&testWorkflow{name: "demo"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(&testWorkflow{name: "demo"}); err == nil {
		t.Fatalf("expected error for duplicate workflow")
	}
}

func TestRegistry_Get_OK(t *testing.T) {
	r := NewRegistry()
	want := &testWorkflow{name: "demo"}
	if err := r.Register(want); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Get("demo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("expected same workflow instance")
	}
}
