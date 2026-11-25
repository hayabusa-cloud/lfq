// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import "unsafe"

// Queue is the combined producer-consumer interface for a FIFO queue.
//
// Queue provides non-blocking Enqueue and Dequeue operations. Both operations
// return [ErrWouldBlock] when they cannot proceed.
//
// The interface intentionally excludes length because accurate counts in
// lock-free algorithms require expensive cross-core synchronization.
// Track counts in application logic when needed.
//
// Example:
//
//	q := lfq.NewSPSC[int](1024)
//
//	// Enqueue
//	if err := q.Enqueue(new(42)); err != nil {
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
	// Returns nil on success or [ErrWouldBlock] if the queue cannot accept elem.
	//
	// Thread safety depends on queue type:
	//   - SPSC: single producer only
	//   - MPSC/MPMC: concurrent producers within the concrete queue's limits
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
	// Returns the zero value and [ErrWouldBlock] if the next item is unavailable.
	//
	// Thread safety depends on queue type:
	//   - SPSC: single consumer only
	//   - MPSC: single consumer only
	//   - SPMC/MPMC: see the concrete queue's consumer contract
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
// In this SPSC pool, one goroutine borrows buffers and one returns them.
// After the last use of a buffer, return its index exactly once.
//
//	pool := make([][]byte, 1024)
//	freeList := lfq.NewSPSCIndirect(len(pool))
//	for i := range pool {
//		pool[i] = make([]byte, 4096)
//		freeList.Enqueue(uintptr(i))
//	}
//
//	if idx, err := freeList.Dequeue(); err == nil {
//		copy(pool[idx], "hello")
//		freeList.Enqueue(idx)
//	}
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
	// Returns 0 and [ErrWouldBlock] if the next item is unavailable.
	Dequeue() (uintptr, error)
}

// QueuePtr is the combined interface for unsafe.Pointer queues.
//
// A successful enqueue transfers ownership. The consumer receives the same
// pointer. Packed pointer queues store address bits, so the caller must keep
// each Go object live at a stable address until its consumer takes ownership.
//
// The interface intentionally excludes length because accurate counts in
// lock-free algorithms require expensive cross-core synchronization.
//
// A package-level array provides stable storage. The producer may reuse a
// slot after the consumer returns it.
//
//	type Message struct {
//		Data []byte
//	}
//
//	var messages [1024]Message
//	var q = lfq.NewMPMCPtr(len(messages))
//
//	func publish(i int) error {
//		return q.Enqueue(unsafe.Pointer(&messages[i]))
//	}
//
//	func receive() (*Message, error) {
//		ptr, err := q.Dequeue()
//		return (*Message)(ptr), err
//	}
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
	// Returns nil and [ErrWouldBlock] if the next item is unavailable.
	Dequeue() (unsafe.Pointer, error)
}

// Drainer signals that no more enqueues will occur.
//
// FAA-based queues (MPMC, SPMC, MPSC) implement this interface.
// SPSC queues do not implement Drainer as they have no threshold mechanism.
//
// When sharing [MPMC], [SPMC], [SPMCIndirect] or [SPMCPtr], use a fresh
// queue for one batch of at most Cap() items. Wait for every producer's final
// Enqueue to return, then call Drain. The first Dequeue on the queue must
// follow Drain. The same consumer continues until [ErrWouldBlock].
//
// The FAA SPMC queues and generic FAA MPMC queue skip their dequeue
// threshold after Drain. MPMCIndirect and MPMCPtr can always drain
// published values.
//
// Example:
//
//	prodWg.Wait()  // Wait for producers to finish
//	// For a queue value initialized with Init, use its address as q.
//	if d, ok := any(q).(lfq.Drainer); ok {
//		d.Drain()
//	}
//	// Drain the batch in this goroutine.
type Drainer interface {
	// Drain signals that all producers have finished. For the batch protocol
	// in [Drainer], call it before the first Dequeue.
	Drain()
}
