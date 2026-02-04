// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// SPSC Indirect - Basic Operations
// =============================================================================

// TestSPSCIndirectBasic tests basic SPSCIndirect (Lamport ring buffer) operations.
// SPSCIndirect provides wait-free operations for both enqueue and dequeue.
func TestSPSCIndirectBasic(t *testing.T) {
	q := lfq.NewSPSCIndirect(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	// Enqueue to capacity
	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	// Full queue returns ErrWouldBlock
	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Dequeue in FIFO order
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	// Empty queue returns ErrWouldBlock
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestSPSCIndirectZeroValue tests that SPSCIndirect correctly handles zero values.
// Zero is a valid value distinct from empty.
func TestSPSCIndirectZeroValue(t *testing.T) {
	q := lfq.NewSPSCIndirect(4)

	// Enqueue zeros
	for range 4 {
		if err := q.Enqueue(0); err != nil {
			t.Fatalf("Enqueue(0): %v", err)
		}
	}

	// Full queue
	if err := q.Enqueue(0); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Dequeue zeros
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != 0 {
			t.Fatalf("Dequeue(%d): got %d, want 0", i, val)
		}
	}

	// Empty
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestSPSCIndirectWraparound tests index wraparound over multiple cycles.
func TestSPSCIndirectWraparound(t *testing.T) {
	q := lfq.NewSPSCIndirect(4)

	for cycle := range 10 {
		// Fill
		for i := range 4 {
			v := uintptr(cycle*100 + i)
			if err := q.Enqueue(v); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
		}

		// Drain in FIFO order
		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			expected := uintptr(cycle*100 + i)
			if val != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, expected)
			}
		}
	}
}

// =============================================================================
// SPSC Indirect - Concurrent Tests (1 Producer, 1 Consumer)
// =============================================================================

// TestSPSCIndirectConcurrent tests concurrent access with exactly one producer
// and one consumer (the SPSC contract).
func TestSPSCIndirectConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: SPSC uses cross-variable memory ordering")
	}

	const itemCount = 100000
	q := lfq.NewSPSCIndirect(64)

	var wg sync.WaitGroup
	var producerDone atomix.Bool
	var consumerErr error
	var consumed atomix.Int64

	// Producer: single goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer producerDone.Store(true)
		backoff := iox.Backoff{}
		for i := range itemCount {
			for q.Enqueue(uintptr(i+1)) != nil { // +1 to distinguish from zero
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Consumer: single goroutine, verify FIFO order
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		expected := uintptr(1)
		for expected <= itemCount {
			val, err := q.Dequeue()
			if err == nil {
				if val != expected {
					consumerErr = errors.New("FIFO violation")
					return
				}
				expected++
				consumed.Add(1)
				backoff.Reset()
			} else {
				if producerDone.Load() && consumed.Load() == itemCount {
					return
				}
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	if consumerErr != nil {
		t.Fatalf("consumer error: %v", consumerErr)
	}
	if got := consumed.Load(); got != itemCount {
		t.Fatalf("consumed %d, want %d", got, itemCount)
	}
}

// TestSPSCIndirectConcurrentHighThroughput tests SPSC under sustained load.
func TestSPSCIndirectConcurrentHighThroughput(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: SPSC uses cross-variable memory ordering")
	}

	const (
		itemCount = 1000000
		timeout   = 10 * time.Second
	)
	q := lfq.NewSPSCIndirect(1024)

	var wg sync.WaitGroup
	var timedOut atomix.Bool
	var consumed atomix.Int64
	deadline := time.Now().Add(timeout)

	// Producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range itemCount {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			for q.Enqueue(uintptr(i)) != nil {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for consumed.Load() < itemCount {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			if _, err := q.Dequeue(); err == nil {
				consumed.Add(1)
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	if timedOut.Load() {
		t.Fatalf("timeout: consumed %d/%d", consumed.Load(), itemCount)
	}
	if got := consumed.Load(); got != itemCount {
		t.Fatalf("consumed %d, want %d", got, itemCount)
	}
}

// =============================================================================
// SPSC Indirect - Stress Tests
// =============================================================================

// TestSPSCIndirectStressFillDrain repeatedly fills and drains the queue.
func TestSPSCIndirectStressFillDrain(t *testing.T) {
	q := lfq.NewSPSCIndirect(16)

	for cycle := range 5000 {
		// Fill to capacity
		for i := range 16 {
			if err := q.Enqueue(uintptr(i)); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
		}

		// Verify full
		if err := q.Enqueue(0); !errors.Is(err, lfq.ErrWouldBlock) {
			t.Fatalf("cycle %d: expected ErrWouldBlock on full", cycle)
		}

		// Drain completely
		for i := range 16 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			if val != uintptr(i) {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, i)
			}
		}

		// Verify empty
		if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
			t.Fatalf("cycle %d: expected ErrWouldBlock on empty", cycle)
		}
	}
}

// TestSPSCIndirectStressPartialFillDrain tests interleaved partial operations.
func TestSPSCIndirectStressPartialFillDrain(t *testing.T) {
	q := lfq.NewSPSCIndirect(16)
	var nextEnq, nextDeq uintptr

	for cycle := range 10000 {
		// Enqueue 4 items
		for i := range 4 {
			if err := q.Enqueue(nextEnq); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
			nextEnq++
		}

		// Dequeue 4 items (balanced)
		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			if val != nextDeq {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, nextDeq)
			}
			nextDeq++
		}
	}
}

// =============================================================================
// SPSC Indirect - Panic Tests
// =============================================================================

// TestSPSCIndirectPanicOnSmallCapacity tests that constructor panics for capacity < 2.
func TestSPSCIndirectPanicOnSmallCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{"One", 1},
		{"Zero", 0},
		{"Negative", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for capacity < 2")
				}
			}()
			lfq.NewSPSCIndirect(tt.capacity)
		})
	}
}

// =============================================================================
// SPSC Indirect - Builder API
// =============================================================================

// TestSPSCIndirectBuilder tests creating SPSCIndirect via the Builder API.
func TestSPSCIndirectBuilder(t *testing.T) {
	q := lfq.New(7).SingleProducer().SingleConsumer().BuildIndirectSPSC()

	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	// Basic operation
	if err := q.Enqueue(42); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if val != 42 {
		t.Fatalf("got %d, want 42", val)
	}
}

// TestSPSCIndirectBuilderInterface tests that SPSCIndirect satisfies QueueIndirect.
func TestSPSCIndirectBuilderInterface(t *testing.T) {
	q := lfq.New(4).SingleProducer().SingleConsumer().BuildIndirect()

	// Should be SPSCIndirect, verify via interface
	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	if err := q.Enqueue(100); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if val != 100 {
		t.Fatalf("got %d, want 100", val)
	}
}
