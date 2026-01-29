package execution

import "fmt"

type Registry struct{ m map[string]Workflow }

func NewRegistry() *Registry { return &Registry{m: map[string]Workflow{}} }

func (r *Registry) Register(w Workflow) error {
	if w == nil || w.Name() == "" {
		return fmt.Errorf("invalid workflow")
	}
	if _, ok := r.m[w.Name()]; ok {
		return fmt.Errorf("workflow exists: %s", w.Name())
	}
	r.m[w.Name()] = w
	return nil
}

func (r *Registry) Get(name string) (Workflow, error) {
	w, ok := r.m[name]
	if !ok {
		return nil, fmt.Errorf("unknown workflow: %s", name)
	}
	return w, nil
}
