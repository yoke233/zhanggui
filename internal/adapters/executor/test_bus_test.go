package executor

import membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"

func NewMemBus() *membus.Bus {
	return membus.NewBus()
}
