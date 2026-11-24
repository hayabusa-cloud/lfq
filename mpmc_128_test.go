// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// MPMCIndirect Tests
// =============================================================================

func TestMPMCIndirect128BasicOperations(t *testing.T) {
	q := lfq.NewMPMCIndirect(4)
	_, err := q.Dequeue()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

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

func TestMPMCIndirect128WrapAround(t *testing.T) {
	q := lfq.NewMPMCIndirect(4)

	// Fill and drain multiple times to test wrap-around
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

func TestMPMCIndirect128ZeroValue(t *testing.T) {
	q := lfq.NewMPMCIndirect(4)

	// Zero is a valid uintptr value
	if err := q.Enqueue(0); err != nil {
		t.Fatalf("enqueue 0: %v", err)
	}

	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if val != 0 {
		t.Fatalf("got %d, want 0", val)
	}
}

func TestMPMCIndirect128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewMPMCIndirect(1)
}

// =============================================================================
// MPMCPtr Tests
// =============================================================================

func TestMPMCPtr128BasicOperations(t *testing.T) {
	q := lfq.NewMPMCPtr(4)
	_, err := q.Dequeue()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	vals := packedPtrTestValues[:]
	for i := range vals {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Test full enqueue
	extra := 999
	if err := q.Enqueue(unsafe.Pointer(&extra)); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("full enqueue: got %v, want ErrWouldBlock", err)
	}

	// Test FIFO order and pointer identity
	for i := range vals {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if ptr != unsafe.Pointer(&vals[i]) {
			t.Fatalf("dequeue %d: pointer mismatch", i)
		}
		if *(*int)(ptr) != vals[i] {
			t.Fatalf("dequeue %d: got %d, want %d", i, *(*int)(ptr), vals[i])
		}
	}
}

func TestMPMCPtr128RollsBackFullRingClaim(t *testing.T) {
	const capacity = 2
	const slots = 2 * capacity
	q, tail := lfq.MPMCPtrWithAdmittedBurstForTest(capacity)
	for range slots {
		if err := q.Enqueue(nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Enqueue(nil); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("enqueue on full ring = %v, want ErrWouldBlock", err)
	}
	if got := tail(); got != slots {
		t.Fatalf("tail after rejected enqueue = %d, want %d", got, slots)
	}
}

func TestMPMCPtr128NilPointer(t *testing.T) {
	q := lfq.NewMPMCPtr(4)

	// nil is a valid unsafe.Pointer value
	if err := q.Enqueue(nil); err != nil {
		t.Fatalf("enqueue nil: %v", err)
	}

	ptr, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if ptr != nil {
		t.Fatalf("got %v, want nil", ptr)
	}
}

func TestMPMCPtr128Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for capacity < 2")
		}
	}()
	lfq.NewMPMCPtr(1)
}

// =============================================================================
// Concurrent Tests (these will be moved to lockfree_test.go for !race builds)
// =============================================================================

func TestMPMCIndirect128ConcurrentSingleProducerConsumer(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skipping concurrent test with race detector")
	}

	q := lfq.NewMPMCIndirect(1024)
	const count = 1000

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer: waits for consumer to drain (external wait → iox.Backoff)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		deadline := time.Now().Add(3 * time.Second)
		for i := range count {
			for q.Enqueue(uintptr(i)) != nil {
				if time.Now().After(deadline) {
					return
				}
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Consumer: waits for producer to enqueue (external wait → iox.Backoff)
	var received atomix.Int64
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		deadline := time.Now().Add(3 * time.Second)
		for received.Load() < count {
			if time.Now().After(deadline) {
				return
			}
			if _, err := q.Dequeue(); err == nil {
				received.Add(1)
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	if received.Load() != count {
		t.Fatalf("received %d, want %d", received.Load(), count)
	}
}

func TestMPMCIndirect128ConcurrentMPMC(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skipping concurrent test with race detector")
	}

	q := lfq.NewMPMCIndirect(256)
	const producers = 4
	const consumers = 4
	const itemsPerProducer = 200
	const totalItems = producers * itemsPerProducer

	// Track each item: 0 = not seen, 1 = seen once, >1 = duplicate
	seen := make([]atomix.Int32, totalItems)
	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup
	var totalDequeued atomix.Int64
	var producersDone atomix.Bool

	// Helper to record a dequeued value
	recordValue := func(v uintptr) {
		idx := int(v)
		if idx >= 0 && idx < totalItems {
			seen[idx].Add(1)
		}
		totalDequeued.Add(1)
	}

	// Producers: enqueue all items with backoff when full
	for p := range producers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			backoff := iox.Backoff{}
			base := id * itemsPerProducer
			for i := range itemsPerProducer {
				v := uintptr(base + i)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(p)
	}

	// Consumers run until the producers finish.
	for range consumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			backoff := iox.Backoff{}
			for !producersDone.Load() {
				if v, err := q.Dequeue(); err == nil {
					recordValue(v)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	// Wait for all producers to finish
	producerWg.Wait()
	producersDone.Store(true)
	consumerWg.Wait()

	// All workers have joined. Check for an extra value as well as missing ones.
	for totalDequeued.Load() <= totalItems {
		if v, err := q.Dequeue(); err == nil {
			recordValue(v)
		} else {
			break
		}
	}

	if got := totalDequeued.Load(); got != totalItems {
		t.Fatalf("dequeued %d/%d", got, totalItems)
	}
	for i := range totalItems {
		if n := seen[i].Load(); n != 1 {
			t.Fatalf("value %d delivered %d times", i, n)
		}
	}
	for i := range q.Cap() {
		if err := q.Enqueue(0); err != nil {
			t.Fatalf("refill at %d/%d: %v", i, q.Cap(), err)
		}
	}
	if err := q.Enqueue(0); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("full enqueue: %v", err)
	}
}

func TestMPMCPtr128ConcurrentMPMC(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skipping concurrent test with race detector")
	}

	q := lfq.NewMPMCPtr(256)
	const producers = 4
	const consumers = 4
	const itemsPerProducer = 200
	const totalItems = producers * itemsPerProducer

	// Pre-allocate items to avoid GC issues
	items := make([]int, totalItems)
	for i := range items {
		items[i] = i
	}

	// Track each item: 0 = not seen, 1 = seen once, >1 = duplicate
	seen := make([]atomix.Int32, totalItems)
	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup
	var totalDequeued atomix.Int64
	var producersDone atomix.Bool

	// Helper to record a dequeued value
	recordValue := func(ptr unsafe.Pointer) {
		if ptr != nil {
			val := *(*int)(ptr)
			if val >= 0 && val < totalItems {
				seen[val].Add(1)
			}
		}
		totalDequeued.Add(1)
	}

	// Producers: enqueue all items
	for p := range producers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			backoff := iox.Backoff{}
			base := id * itemsPerProducer
			for i := range itemsPerProducer {
				for q.Enqueue(unsafe.Pointer(&items[base+i])) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(p)
	}

	// Consumers: run concurrently with producers, exit when producers done
	for range consumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			backoff := iox.Backoff{}
			for !producersDone.Load() {
				if ptr, err := q.Dequeue(); err == nil {
					recordValue(ptr)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	// Wait for all producers to finish
	producerWg.Wait()
	producersDone.Store(true)
	consumerWg.Wait()

	// All workers have joined. Check for an extra value as well as missing ones.
	for totalDequeued.Load() <= totalItems {
		if ptr, err := q.Dequeue(); err == nil {
			recordValue(ptr)
		} else {
			break
		}
	}

	if got := totalDequeued.Load(); got != totalItems {
		t.Fatalf("dequeued %d/%d", got, totalItems)
	}
	for i := range totalItems {
		if n := seen[i].Load(); n != 1 {
			t.Fatalf("value %d delivered %d times", i, n)
		}
	}
	runtime.KeepAlive(items)
}

// =============================================================================
// Interface Tests
// =============================================================================

func TestMPMCIndirect128ImplementsInterface(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.NewMPMCIndirect(8)
}

func TestMPMCPtr128ImplementsInterface(t *testing.T) {
	var _ lfq.QueuePtr = lfq.NewMPMCPtr(8)
}

func TestMPMCSeqHelpsStalledCursors(t *testing.T) {
	words, ptrs := lfq.StalledMPMCSeqForTest()
	if err := words.Enqueue(7); err != nil {
		t.Fatal(err)
	}
	if got, err := words.Dequeue(); got != 7 || err != nil {
		t.Fatalf("dequeue after stalled workers = (%d, %v), want (7, nil)", got, err)
	}
	if err := ptrs.Enqueue(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := ptrs.Dequeue(); got != nil || err != nil {
		t.Fatalf("dequeue after stalled workers = (%p, %v), want (nil, nil)", got, err)
	}
}
