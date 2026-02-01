// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package lfq provides bounded FIFO queue implementations.
//
// The package offers multiple queue variants optimized for different
// producer/consumer patterns:
//
//   - SPSC: Single-Producer Single-Consumer
//   - MPSC: Multi-Producer Single-Consumer
//   - SPMC: Single-Producer Multi-Consumer
//   - MPMC: Multi-Producer Multi-Consumer
//
// # Quick Start
//
// Direct constructors (recommended for most cases):
//
//	q := lfq.NewSPSC[Event](1024)
//	q := lfq.NewMPMC[*Request](4096)
//
// Builder API auto-selects algorithm based on constraints:
//
//	q := lfq.Build[Event](lfq.New(1024).SingleProducer().SingleConsumer())  // → SPSC
//	q := lfq.Build[Event](lfq.New(1024).SingleConsumer())                   // → MPSC
//	q := lfq.Build[Event](lfq.New(1024).SingleProducer())                   // → SPMC
//	q := lfq.Build[Event](lfq.New(1024))                                    // → MPMC
//
// # Basic Usage
//
// All queues share the same interface for enqueueing and dequeueing:
//
//	// Create a queue
//	q := lfq.NewMPMC[int](1024)
//
//	// Enqueue (non-blocking)
//	value := 42
//	err := q.Enqueue(&value)
//	if lfq.IsWouldBlock(err) {
//	    // Queue is full - handle backpressure
//	}
//
//	// Dequeue (non-blocking)
//	elem, err := q.Dequeue()
//	if lfq.IsWouldBlock(err) {
//	    // Queue is empty - try again later
//	}
//
// # Common Patterns
//
// Pipeline Stage (SPSC):
//
//	// Stage 1 → Queue → Stage 2
//	q := lfq.NewSPSC[Data](1024)
//
//	go func() { // Producer (Stage 1)
//	    backoff := iox.Backoff{}
//	    for data := range input {
//	        for q.Enqueue(&data) != nil {
//	            backoff.Wait()
//	        }
//	        backoff.Reset()
//	    }
//	}()
//
//	go func() { // Consumer (Stage 2)
//	    backoff := iox.Backoff{}
//	    for {
//	        data, err := q.Dequeue()
//	        if err != nil {
//	            backoff.Wait()
//	            continue
//	        }
//	        backoff.Reset()
//	        process(data)
//	    }
//	}()
//
// Event Aggregation (MPSC):
//
//	// Multiple event sources → Single processor
//	q := lfq.NewMPSC[Event](4096)
//
//	// Multiple producers (event sources)
//	for sensor := range slices.Values(sensors) {
//	    go func(s Sensor) {
//	        for ev := range s.Events() {
//	            q.Enqueue(&ev)
//	        }
//	    }(sensor)
//	}
//
//	// Single consumer (aggregator)
//	go func() {
//	    for {
//	        ev, err := q.Dequeue()
//	        if err == nil {
//	            aggregate(ev)
//	        }
//	    }
//	}()
//
// Work Distribution (SPMC):
//
//	// Single dispatcher → Multiple workers
//	q := lfq.NewSPMC[Task](1024)
//
//	// Single producer (dispatcher)
//	go func() {
//	    backoff := iox.Backoff{}
//	    for task := range tasks {
//	        for q.Enqueue(&task) != nil {
//	            backoff.Wait()
//	        }
//	        backoff.Reset()
//	    }
//	}()
//
//	// Multiple consumers (workers)
//	for range numWorkers {
//	    go func() {
//	        for {
//	            task, err := q.Dequeue()
//	            if err == nil {
//	                task.Execute()
//	            }
//	        }
//	    }()
//	}
//
// Worker Pool (MPMC):
//
//	// Multiple submitters → Multiple workers
//	q := lfq.NewMPMC[Job](4096)
//
//	// Workers
//	for range numWorkers {
//	    go func() {
//	        for {
//	            job, err := q.Dequeue()
//	            if err == nil {
//	                job.Run()
//	            }
//	        }
//	    }()
//	}
//
//	// Submit jobs from anywhere
//	func Submit(j Job) error {
//	    return q.Enqueue(&j)
//	}
//
// # Queue Variants
//
// Three queue flavors are available for different use cases:
//
//	Build[T]        - Generic type-safe queue for any type
//	BuildIndirect() - Queue for uintptr values (pool indices, handles)
//	BuildPtr()      - Queue for unsafe.Pointer (zero-copy pointer passing)
//
// When to use Indirect:
//
//	// Buffer pool with index-based access
//	pool := make([][]byte, 1024)
//	freeList := lfq.NewSPSCIndirect(1024)
//
//	// Initialize free list with buffer indices
//	for i := range pool {
//	    pool[i] = make([]byte, 4096)
//	    freeList.Enqueue(uintptr(i))
//	}
//
//	// Allocate: get index from free list
//	idx, err := freeList.Dequeue()
//	buf := pool[idx]
//
//	// Free: return index to free list
//	freeList.Enqueue(idx)
//
// When to use Ptr:
//
//	// Zero-copy object passing between goroutines
//	q := lfq.NewMPMCPtr(1024)
//
//	// Producer creates object once
//	msg := &Message{Data: largePayload}
//	q.Enqueue(unsafe.Pointer(msg))
//
//	// Consumer receives same pointer - no copy
//	ptr, _ := q.Dequeue()
//	msg := (*Message)(ptr)
//
// # Algorithm Selection
//
// The builder selects algorithms based on constraints and Compact() hint:
//
// Default (FAA-based, 2n slots for capacity n):
//
//	SPSC: Lamport ring buffer (n slots, already optimal)
//	MPSC: FAA producers, sequential consumer
//	SPMC: Sequential producer, FAA consumers
//	MPMC: FAA-based SCQ algorithm
//
// With Compact() (CAS-based, n slots for capacity n):
//
//	SPSC: Same as default (already optimal)
//	MPSC: CAS producers, sequential consumer
//	SPMC: Sequential producer, CAS consumers
//	MPMC: Sequence-based algorithm
//
// FAA (Fetch-And-Add) scales better under high contention but requires
// 2n physical slots. Use Compact() when memory efficiency is critical.
//
// Type-safe builder functions enforce constraints at compile time:
//
//	BuildSPSC[T](b) → *SPSC[T]    // Requires SP + SC
//	BuildMPSC[T](b) → Queue[T]   // Requires SC only
//	BuildSPMC[T](b) → Queue[T]   // Requires SP only
//	BuildMPMC[T](b) → Queue[T]   // Requires no constraints
//
// # Performance Hints
//
// Compact() selects CAS-based algorithms with n physical slots (vs 2n for
// FAA-based default). Use when memory efficiency is more important than
// contention scalability:
//
//	// Compact mode - CAS-based, n slots (works with all queue types)
//	q := lfq.Build[Event](lfq.New(4096).Compact())
//	q := lfq.New(4096).Compact().BuildIndirect()
//	q := lfq.New(4096).Compact().BuildPtr()
//
// SPSC variants already use n slots (Lamport ring buffer) and ignore Compact().
//
// # Error Handling
//
// Queues return [ErrWouldBlock] when operations cannot proceed. This error
// is sourced from [code.hybscloud.com/iox] for ecosystem consistency.
//
//	// Retry loop with backoff
//	backoff := iox.Backoff{}
//	for {
//	    err := q.Enqueue(&item)
//	    if err == nil {
//	        backoff.Reset()
//	        break
//	    }
//	    if !lfq.IsWouldBlock(err) {
//	        return err // Unexpected error
//	    }
//	    backoff.Wait()
//	}
//
// For semantic error classification (delegates to iox):
//
//	lfq.IsWouldBlock(err)  // true if queue full/empty
//	lfq.IsSemantic(err)    // true if control flow signal
//	lfq.IsNonFailure(err)  // true if nil or ErrWouldBlock
//
// # Capacity and Length
//
// Capacity rounds up to the next power of 2:
//
//	q := lfq.NewMPMC[int](3)     // Actual capacity: 4
//	q := lfq.NewMPMC[int](4)     // Actual capacity: 4
//	q := lfq.NewMPMC[int](1000)  // Actual capacity: 1024
//	q := lfq.NewMPMC[int](1024)  // Actual capacity: 1024
//
// Minimum capacity is 2 (already a power of 2). Panic if capacity < 2.
//
// Length is intentionally not provided because accurate counts in lock-free
// algorithms require expensive cross-core synchronization. Track counts in
// application logic when needed.
//
// # Thread Safety
//
// All queue operations are thread-safe within their access pattern constraints:
//
//   - SPSC: One producer goroutine, one consumer goroutine
//   - MPSC: Multiple producer goroutines, one consumer goroutine
//   - SPMC: One producer goroutine, multiple consumer goroutines
//   - MPMC: Multiple producer and consumer goroutines
//
// Violating these constraints (e.g., multiple producers on SPSC) causes
// undefined behavior including data corruption and races.
//
// # Graceful Shutdown
//
// FAA-based queues (MPMC, SPMC, MPSC) include a threshold mechanism to prevent
// livelock. This mechanism may cause Dequeue to return [ErrWouldBlock] even when
// items remain, waiting for producer activity to reset the threshold.
//
// For graceful shutdown scenarios where producers have finished but consumers
// need to drain remaining items, use the [Drainer] interface:
//
//	// Producer goroutines finish
//	prodWg.Wait()
//
//	// Signal no more enqueues will occur
//	if d, ok := q.(lfq.Drainer); ok {
//	    d.Drain()
//	}
//
//	// Consumers can now drain all remaining items
//	// without threshold blocking
//
// After Drain is called, Dequeue skips threshold checks, allowing consumers
// to fully drain the queue. Drain is a hint — the caller must ensure no
// further Enqueue calls will be made.
//
// SPSC queues do not implement [Drainer] as they have no threshold mechanism.
// The type assertion naturally handles this case.
//
// # Race Detection
//
// Go's race detector is not designed for lock-free algorithm verification.
// The race detector tracks explicit synchronization primitives (mutex, channels,
// WaitGroup) but cannot observe happens-before relationships established through
// atomic memory orderings (acquire-release semantics).
//
// Lock-free queues use sequence numbers with acquire-release semantics to
// protect non-atomic data fields. These algorithms are correct, but the race
// detector may report false positives because it cannot track synchronization
// provided by atomic operations on separate variables.
//
// For lock-free algorithm correctness verification, use:
//   - Formal verification tools (TLA+, SPIN)
//   - Stress testing without race detector
//   - Memory model analysis
//
// Tests incompatible with race detection are excluded via //go:build !race.
//
// # Dependencies
//
// This package uses [code.hybscloud.com/iox] for semantic errors,
// [code.hybscloud.com/atomix] for atomic primitives with explicit
// memory ordering, and [code.hybscloud.com/spin] for CPU pause instructions.
package lfq
