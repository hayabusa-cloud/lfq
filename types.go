// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import "unsafe"

// Queue is the combined producer-consumer interface for a FIFO queue.
//
// Queue provides non-blocking Enqueue and Dequeue operations. Both operations
// return ErrWouldBlock when they cannot proceed (queue full or empty).
//
// The interface intentionally excludes length because accurate counts in
// lock-free algorithms require expensive cross-core synchronization.
// Track counts in application logic when needed.
//
// Example:
//
//	q := lfq.NewMPMC[int](1024)
//
//	// Enqueue
//	val := 42
//	if err := q.Enqueue(&val); err != nil {
//	    // Handle full queue
//	}
//
//	// Dequeue
//	elem, err := q.Dequeue()
//	if err == nil {
//	    fmt.Println(elem)
//	}
type Queue[T any] interface {
	Producer[T]
	Consumer[T]
	Cap() int
}

// Producer is the interface for enqueueing elements.
//
// Producer provides non-blocking enqueue operations. The element is passed
// by pointer to avoid copying large structs. The queue stores a copy of
// the pointed-to value, so the original can be modified after Enqueue returns.
type Producer[T any] interface {
	// Enqueue adds an element to the queue (non-blocking).
	// The element is copied into the queue's internal buffer.
	// Returns nil on success, ErrWouldBlock if the queue is full.
	//
	// Thread safety depends on queue type:
	//   - SPSC: single producer only
	//   - MPSC/MPMC: multiple producers safe
	//   - SPMC: single producer only
	Enqueue(elem *T) error
}

// Consumer is the interface for dequeueing elements.
//
// Consumer provides non-blocking dequeue operations. The element is returned
// by value (copied from the queue's internal buffer). The original slot is
// cleared to allow garbage collection of referenced objects.
//
// For large types (>512 bytes), consider using QueuePtr or QueueIndirect
// instead to avoid copy overhead.
type Consumer[T any] interface {
	// Dequeue removes and returns an element from the queue (non-blocking).
	// Returns the dequeued element on success.
	// Returns (zero-value, ErrWouldBlock) if the queue is empty.
	//
	// Thread safety depends on queue type:
	//   - SPSC: single consumer only
	//   - MPSC: single consumer only
	//   - SPMC/MPMC: multiple consumers safe
	Dequeue() (T, error)
}

// QueueIndirect is the combined interface for indirect (uintptr) queues.
//
// QueueIndirect passes indices or handles instead of full objects. This is
// useful for buffer pools, object pools, or any index-based data structure.
//
// The interface intentionally excludes length because accurate counts in
// lock-free algorithms require expensive cross-core synchronization.
//
// Example (buffer pool):
//
//	pool := make([][]byte, 1024)
//	freeList := lfq.NewSPSCIndirect(1024)
//
//	// Initialize pool
//	for i := range pool {
//	    pool[i] = make([]byte, 4096)
//	    freeList.Enqueue(uintptr(i))
//	}
//
//	// Allocate
//	idx, _ := freeList.Dequeue()
//	buf := pool[idx]
//
//	// Free
//	freeList.Enqueue(idx)
type QueueIndirect interface {
	ProducerIndirect
	ConsumerIndirect
	Cap() int
}

// ProducerIndirect enqueues uintptr values (non-blocking).
type ProducerIndirect interface {
	// Enqueue adds an element to the queue.
	// Returns ErrWouldBlock immediately if the queue is full.
	Enqueue(elem uintptr) error
}

// ConsumerIndirect dequeues uintptr values (non-blocking).
type ConsumerIndirect interface {
	// Dequeue removes and returns an element from the queue.
	// Returns (0, ErrWouldBlock) immediately if the queue is empty.
	Dequeue() (uintptr, error)
}

// QueuePtr is the combined interface for unsafe.Pointer queues.
//
// QueuePtr passes pointers directly without copying. This enables zero-copy
// transfer of objects between goroutines. The producer creates an object,
// enqueues its pointer, and the consumer receives the same pointer.
//
// Ownership semantics: The producer transfers ownership to the consumer.
// After enqueueing, the producer should not access the object.
//
// The interface intentionally excludes length because accurate counts in
// lock-free algorithms require expensive cross-core synchronization.
//
// Example:
//
//	type Message struct {
//	    Data []byte
//	}
//
//	q := lfq.NewMPMCPtr(1024)
//
//	// Producer
//	msg := &Message{Data: largePayload}
//	q.Enqueue(unsafe.Pointer(msg))
//	// msg ownership transferred - do not use msg after this
//
//	// Consumer
//	ptr, _ := q.Dequeue()
//	msg := (*Message)(ptr)
//	// msg is now owned by consumer
type QueuePtr interface {
	ProducerPtr
	ConsumerPtr
	Cap() int
}

// ProducerPtr enqueues unsafe.Pointer values (non-blocking).
type ProducerPtr interface {
	// Enqueue adds an element to the queue.
	// Returns ErrWouldBlock immediately if the queue is full.
	Enqueue(elem unsafe.Pointer) error
}

// ConsumerPtr dequeues unsafe.Pointer values (non-blocking).
type ConsumerPtr interface {
	// Dequeue removes and returns an element from the queue.
	// Returns (nil, ErrWouldBlock) immediately if the queue is empty.
	Dequeue() (unsafe.Pointer, error)
}

// Drainer signals that no more enqueues will occur.
//
// FAA-based queues (MPMC, SPMC, MPSC) implement this interface.
// SPSC queues do not implement Drainer as they have no threshold mechanism.
//
// Call Drain after all producers have finished to allow consumers to
// drain remaining items without threshold blocking.
//
// Example:
//
//	prodWg.Wait()  // Wait for producers to finish
//	if d, ok := q.(lfq.Drainer); ok {
//	    d.Drain()
//	}
//	// Consumers can now drain all remaining items
type Drainer interface {
	// Drain signals that no more enqueues will occur.
	// After Drain is called, Dequeue skips threshold checks, allowing
	// consumers to drain all remaining items without producer pressure.
	//
	// Drain is a hint — the caller must ensure no further Enqueue calls
	// will be made after calling Drain.
	Drain()
}
