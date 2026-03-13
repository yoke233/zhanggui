package flow

import "sync"

// Semaphore limits concurrent step executions.
type Semaphore struct {
	ch chan struct{}
	wg sync.WaitGroup
}

// NewSemaphore creates a semaphore with the given concurrency limit.
func NewSemaphore(limit int) *Semaphore {
	if limit <= 0 {
		limit = 1
	}
	return &Semaphore{ch: make(chan struct{}, limit)}
}

// Acquire blocks until a slot is available.
func (s *Semaphore) Acquire() {
	s.ch <- struct{}{}
	s.wg.Add(1)
}

// Release frees a slot.
func (s *Semaphore) Release() {
	<-s.ch
	s.wg.Done()
}

// Wait blocks until all acquired slots are released.
func (s *Semaphore) Wait() {
	s.wg.Wait()
}

// Capacity returns the maximum number of concurrent slots.
func (s *Semaphore) Capacity() int {
	if s == nil {
		return 0
	}
	return cap(s.ch)
}
