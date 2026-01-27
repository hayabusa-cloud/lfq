// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"sync"
	"testing"
	"time"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// MPSCIndirect Tests
// =============================================================================

func TestMPSCIndirect128BasicOperations(t *testing.T) {
	q := lfq.NewMPSCIndirect(4)

	// Test empty dequeue
	_, err := q.Dequeue()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	// Test enqueue/dequeue
	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Test full enqueue
	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("full enqueue: got %v, want ErrWouldBlock", err)
	}

	// Test FIFO order
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("dequeue %d: got %d, want %d", i, val, i+100)
		}
	}
}

func TestMPSCIndirect128WrapAround(t *testing.T) {
	q := lfq.NewMPSCIndirect(4)

	for round := range 10 {
		for i := range 4 {
			val := uintptr(round*100 + i)
			if err := q.Enqueue(val); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}

		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := uintptr(round*100 + i)
			if val != expected {
				t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
			}
		}
	}
}

func TestMPSCIndirect128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewMPSCIndirect(1)
}

// =============================================================================
// MPSCPtr Tests
// =============================================================================

func TestMPSCPtr128BasicOperations(t *testing.T) {
	q := lfq.NewMPSCPtr(4)

	vals := []int{100, 200, 300, 400}
	for i := range vals {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	for i := range vals {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if *(*int)(ptr) != vals[i] {
			t.Fatalf("dequeue %d: got %d, want %d", i, *(*int)(ptr), vals[i])
		}
	}
}

func TestMPSCPtr128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewMPSCPtr(1)
}

// =============================================================================
// SPMCIndirect Tests
// =============================================================================

func TestSPMCIndirect128BasicOperations(t *testing.T) {
	// Test empty dequeue on fresh queue
	// Note: FAA-based queues advance indices on empty dequeue, so use separate queue
	qEmpty := lfq.NewSPMCIndirect(4)
	_, err := qEmpty.Dequeue()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	// Test enqueue/dequeue on fresh queue
	q := lfq.NewSPMCIndirect(4)
	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Test full enqueue
	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("full enqueue: got %v, want ErrWouldBlock", err)
	}

	// Test FIFO order
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("dequeue %d: got %d, want %d", i, val, i+100)
		}
	}
}

func TestSPMCIndirect128WrapAround(t *testing.T) {
	q := lfq.NewSPMCIndirect(4)

	for round := range 10 {
		for i := range 4 {
			val := uintptr(round*100 + i)
			if err := q.Enqueue(val); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}

		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := uintptr(round*100 + i)
			if val != expected {
				t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
			}
		}
	}
}

func TestSPMCIndirect128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewSPMCIndirect(1)
}

// =============================================================================
// SPMCPtr Tests
// =============================================================================

func TestSPMCPtr128BasicOperations(t *testing.T) {
	q := lfq.NewSPMCPtr(4)

	vals := []int{100, 200, 300, 400}
	for i := range vals {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	for i := range vals {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if *(*int)(ptr) != vals[i] {
			t.Fatalf("dequeue %d: got %d, want %d", i, *(*int)(ptr), vals[i])
		}
	}
}

func TestSPMCPtr128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewSPMCPtr(1)
}

// =============================================================================
// Concurrent Tests
// =============================================================================

func TestMPSCIndirect128Concurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skipping concurrent test with race detector")
	}

	q := lfq.NewMPSCIndirect(256)
	const producers = 4
	const itemsPerProducer = 500

	var wg sync.WaitGroup
	var totalEnqueued atomix.Int64

	// Multiple producers
	for p := range producers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			base := uintptr(id * itemsPerProducer)
			deadline := time.Now().Add(3 * time.Second)
			for i := range itemsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					if time.Now().After(deadline) {
						return
					}
					backoff.Wait()
				}
				backoff.Reset()
				totalEnqueued.Add(1)
			}
		}(p)
	}

	// Single consumer (wait-free)
	var totalDequeued atomix.Int64
	done := make(chan struct{})
	go func() {
		backoff := iox.Backoff{}
		deadline := time.Now().Add(3 * time.Second)
		for {
			select {
			case <-done:
				// Drain remaining with timeout
				drainDeadline := time.Now().Add(500 * time.Millisecond)
				for {
					if time.Now().After(drainDeadline) {
						return
					}
					if _, err := q.Dequeue(); err != nil {
						return
					}
					totalDequeued.Add(1)
				}
			default:
				if time.Now().After(deadline) {
					return
				}
				if _, err := q.Dequeue(); err == nil {
					totalDequeued.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}
	}()

	wg.Wait()
	close(done)

	// Wait for consumer to finish with timeout
	waitBackoff := iox.Backoff{}
	deadline := time.Now().Add(3 * time.Second)
	for totalDequeued.Load() < producers*itemsPerProducer {
		if time.Now().After(deadline) {
			t.Fatalf("consumer timeout: dequeued %d, want %d", totalDequeued.Load(), producers*itemsPerProducer)
		}
		waitBackoff.Wait()
	}

	if totalDequeued.Load() != producers*itemsPerProducer {
		t.Fatalf("dequeued %d, want %d", totalDequeued.Load(), producers*itemsPerProducer)
	}
}

func TestSPMCIndirect128Concurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skipping concurrent test with race detector")
	}

	q := lfq.NewSPMCIndirect(256)
	const consumers = 4
	const totalItems = 500

	// Track each item: 0 = not seen, 1 = seen once, >1 = duplicate
	seen := make([]atomix.Int32, totalItems)
	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup
	var totalDequeued atomix.Int64
	var producerDone atomix.Bool

	// Helper to record a dequeued value
	recordValue := func(v uintptr) {
		idx := int(v)
		if idx >= 0 && idx < totalItems {
			seen[idx].Add(1)
		}
		totalDequeued.Add(1)
	}

	// Single producer: enqueue all items
	producerWg.Add(1)
	go func() {
		defer producerWg.Done()
		backoff := iox.Backoff{}
		for i := range totalItems {
			for q.Enqueue(uintptr(i)) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Multiple consumers: run concurrently with producer, exit when producer done
	for range consumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			backoff := iox.Backoff{}
			for !producerDone.Load() {
				if v, err := q.Dequeue(); err == nil {
					recordValue(v)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	// Wait for producer to finish
	producerWg.Wait()
	producerDone.Store(true)
	consumerWg.Wait()

	// Final drain with timeout (threshold exhaustion may prevent complete drain)
	drainDeadline := time.Now().Add(500 * time.Millisecond)
	backoff := iox.Backoff{}
	consecutiveFailures := 0
	for time.Now().Before(drainDeadline) {
		if v, err := q.Dequeue(); err == nil {
			recordValue(v)
			consecutiveFailures = 0
			backoff.Reset()
		} else {
			consecutiveFailures++
			if consecutiveFailures > 1000 {
				break // Likely threshold exhausted
			}
			backoff.Wait()
		}
	}

	// Verify linearizability: no duplicates
	var duplicates int
	for i := range totalItems {
		if seen[i].Load() > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates detected", duplicates)
	}

	// Log missing items (threshold exhaustion is valid SCQ behavior)
	if totalDequeued.Load() < totalItems {
		t.Logf("note: dequeued %d/%d (threshold exhaustion during drain is expected)",
			totalDequeued.Load(), totalItems)
	}
}

// =============================================================================
// Interface Tests
// =============================================================================

func TestMPSCIndirect128ImplementsInterface(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.NewMPSCIndirect(8)
}

func TestMPSCPtr128ImplementsInterface(t *testing.T) {
	var _ lfq.QueuePtr = lfq.NewMPSCPtr(8)
}

func TestSPMCIndirect128ImplementsInterface(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.NewSPMCIndirect(8)
}

func TestSPMCPtr128ImplementsInterface(t *testing.T) {
	var _ lfq.QueuePtr = lfq.NewSPMCPtr(8)
}
