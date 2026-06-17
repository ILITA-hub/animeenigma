package service

import (
	"sync"
	"testing"
	"time"
)

// TestFireSignal_SpawnsWhenSlotAvailable verifies the happy path: when the
// semaphore has a free slot, fireSignal returns true and actually runs fn.
func TestFireSignal_SpawnsWhenSlotAvailable(t *testing.T) {
	r := &RawResolver{serveSignalSem: make(chan struct{}, 2)}

	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	if !r.fireSignal(func() { ran = true; wg.Done() }) {
		t.Fatal("fireSignal returned false with a free slot, want true")
	}
	wg.Wait()
	if !ran {
		t.Fatal("fireSignal did not run fn")
	}
}

// TestFireSignal_DropsWhenSaturated is the WR-01 regression: when every slot is
// held by an in-flight signal, fireSignal must DROP (return false, run nothing)
// rather than block or spawn an unbounded goroutine. We fill the semaphore with
// blocked workers, then assert the next call drops.
func TestFireSignal_DropsWhenSaturated(t *testing.T) {
	const cap = 4
	r := &RawResolver{serveSignalSem: make(chan struct{}, cap)}

	release := make(chan struct{})
	started := make(chan struct{}, cap)
	// Saturate every slot with a goroutine that blocks until released.
	for i := 0; i < cap; i++ {
		if !r.fireSignal(func() {
			started <- struct{}{}
			<-release
		}) {
			t.Fatalf("fireSignal %d returned false while filling, want true", i)
		}
	}
	// Wait until all cap workers have actually acquired their slot.
	for i := 0; i < cap; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for saturating workers to start")
		}
	}

	// Semaphore is now full → the next signal must drop, and fn must NOT run.
	dropRan := false
	if r.fireSignal(func() { dropRan = true }) {
		t.Fatal("fireSignal returned true while saturated, want false (drop-on-full)")
	}
	// Give a dropped fn no chance to have been scheduled.
	time.Sleep(20 * time.Millisecond)
	if dropRan {
		t.Fatal("fireSignal ran fn while saturated, want drop (fn never runs)")
	}

	// Release the workers; a slot frees up and a subsequent signal spawns again.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		var wg sync.WaitGroup
		wg.Add(1)
		if r.fireSignal(func() { wg.Done() }) {
			wg.Wait()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fireSignal never recovered a free slot after release")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestFireSignal_NilSemaphoreDrops guards that a zero-value resolver (nil
// semaphore) drops rather than panicking on a nil-channel send.
func TestFireSignal_NilSemaphoreDrops(t *testing.T) {
	r := &RawResolver{}
	if r.fireSignal(func() {}) {
		t.Fatal("fireSignal with nil semaphore returned true, want false (drop)")
	}
}
