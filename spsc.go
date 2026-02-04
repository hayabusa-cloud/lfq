// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
)

// SPSC is a single-producer single-consumer bounded queue.
//
// Based on Lamport's ring buffer with cached index optimization.
// The producer caches the consumer's dequeue index, and vice versa,
// reducing cross-core cache line traffic.
//
// Memory: O(capacity) with minimal per-slot overhead
type SPSC[T any] struct {
	_          pad
	head       atomix.Uint64 // Consumer reads from here
	_          pad
	cachedTail uint64 // Consumer's cached view of tail
	_          pad
	tail       atomix.Uint64 // Producer writes here
	_          pad
	cachedHead uint64 // Producer's cached view of head
	_          pad
	buffer     []T
	mask       uint64
}

// NewSPSC creates a new SPSC queue.
// Capacity rounds up to the next power of 2.
func NewSPSC[T any](capacity int) *SPSC[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	return &SPSC[T]{
		buffer: make([]T, n),
		mask:   n - 1,
	}
}

// Enqueue adds an element to the queue (producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPSC[T]) Enqueue(elem *T) error {
	tail := q.tail.LoadRelaxed()
	if tail-q.cachedHead > q.mask {
		q.cachedHead = q.head.LoadAcquire()
		if tail-q.cachedHead > q.mask {
			return ErrWouldBlock
		}
	}

	q.buffer[tail&q.mask] = *elem
	q.tail.StoreRelease(tail + 1)
	return nil
}

// Dequeue removes and returns an element (consumer only).
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *SPSC[T]) Dequeue() (T, error) {
	head := q.head.LoadRelaxed()
	if head >= q.cachedTail {
		q.cachedTail = q.tail.LoadAcquire()
		if head >= q.cachedTail {
			var zero T
			return zero, ErrWouldBlock
		}
	}

	elem := q.buffer[head&q.mask]
	var zero T
	q.buffer[head&q.mask] = zero
	q.head.StoreRelease(head + 1)
	return elem, nil
}

// Cap returns the queue capacity.
func (q *SPSC[T]) Cap() int {
	return int(q.mask + 1)
}

// SPSCIndirect is a SPSC queue for uintptr values.
type SPSCIndirect struct {
	_          pad
	head       atomix.Uint64
	_          pad
	cachedTail uint64
	_          pad
	tail       atomix.Uint64
	_          pad
	cachedHead uint64
	_          pad
	buffer     []uintptr
	mask       uint64
}

// NewSPSCIndirect creates a new SPSC queue for uintptr values.
// Capacity rounds up to the next power of 2.
func NewSPSCIndirect(capacity int) *SPSCIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	return &SPSCIndirect{
		buffer: make([]uintptr, n),
		mask:   n - 1,
	}
}

// Cap returns the queue capacity.
func (q *SPSCIndirect) Cap() int {
	return int(q.mask + 1)
}

// SPSCPtr is a SPSC queue for unsafe.Pointer values.
// Useful for zero-copy pointer passing between goroutines.
type SPSCPtr struct {
	_          pad
	head       atomix.Uint64
	_          pad
	cachedTail uint64
	_          pad
	tail       atomix.Uint64
	_          pad
	cachedHead uint64
	_          pad
	buffer     []unsafe.Pointer
	mask       uint64
}

// NewSPSCPtr creates a new SPSC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewSPSCPtr(capacity int) *SPSCPtr {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	return &SPSCPtr{
		buffer: make([]unsafe.Pointer, n),
		mask:   n - 1,
	}
}

// Enqueue adds an element (producer only).
func (q *SPSCPtr) Enqueue(elem unsafe.Pointer) error {
	tail := q.tail.LoadRelaxed()

	if tail-q.cachedHead > q.mask {
		q.cachedHead = q.head.LoadAcquire()
		if tail-q.cachedHead > q.mask {
			return ErrWouldBlock
		}
	}
	// Pointer arithmetic avoids slice bounds checking in hot path.
	// Equivalent to q.buffer[tail&q.mask] = elem
	*(*unsafe.Pointer)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(q.buffer)), int(tail&q.mask)*ptrSize)) = elem
	q.tail.StoreRelease(tail + 1)
	return nil
}

// Dequeue removes and returns an element (consumer only).
func (q *SPSCPtr) Dequeue() (unsafe.Pointer, error) {
	head := q.head.LoadRelaxed()

	if head >= q.cachedTail {
		q.cachedTail = q.tail.LoadAcquire()
		if head >= q.cachedTail {
			return nil, ErrWouldBlock
		}
	}
	// Pointer arithmetic avoids slice bounds checking in hot path.
	// Equivalent to elem := q.buffer[head&q.mask]
	elem := *(*unsafe.Pointer)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(q.buffer)), int(head&q.mask)*ptrSize))
	q.head.StoreRelease(head + 1)
	return elem, nil
}

// Cap returns the queue capacity.
func (q *SPSCPtr) Cap() int {
	return int(q.mask + 1)
}
