// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPSC is an FAA-based multi-producer single-consumer bounded queue.
//
// Producers use FAA to blindly claim positions (SCQ-style), requiring 2n
// physical slots for capacity n.
//
// Memory: 2n slots for capacity n (16+ bytes per slot)
type MPSC[T any] struct {
	_        pad
	head     atomix.Uint64 // Consumer index (single consumer writes, but producers read)
	_        pad
	tail     atomix.Uint64 // Producer index (FAA)
	_        pad
	draining atomix.Bool // Drain mode: no more enqueues
	_        pad
	buffer   []mpscSlot[T]
	capacity uint64 // n (usable capacity)
	size     uint64 // 2n (physical slots)
	mask     uint64 // 2n - 1
}

type mpscSlot[T any] struct {
	cycle atomix.Uint64 // Round number
	data  T
	_     padShort
}

// NewMPSC creates a new FAA-based MPSC queue.
// Capacity rounds up to the next power of 2.
func NewMPSC[T any](capacity int) *MPSC[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &MPSC[T]{
		buffer:   make([]mpscSlot[T], size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	for i := uint64(0); i < size; i++ {
		q.buffer[i].cycle.StoreRelaxed(i / n)
	}

	return q
}

// Drain signals that no more enqueues will occur.
// This is a hint for graceful shutdown — the caller ensures no further
// enqueues will be attempted after calling Drain.
func (q *MPSC[T]) Drain() {
	q.draining.StoreRelease(true)
}

// Enqueue adds an element to the queue (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSC[T]) Enqueue(elem *T) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadRelaxed()
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		myTail := q.tail.AddAcqRel(1) - 1

		slot := &q.buffer[myTail&q.mask]
		expectedCycle := myTail / q.capacity

		slotCycle := slot.cycle.LoadAcquire()

		if slotCycle == expectedCycle {
			slot.data = *elem
			slot.cycle.StoreRelease(expectedCycle + 1)
			return nil
		}

		if int64(slotCycle) < int64(expectedCycle) {
			return ErrWouldBlock // Queue full
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *MPSC[T]) Dequeue() (T, error) {
	head := q.head.LoadRelaxed()
	cycle := head / q.capacity
	slot := &q.buffer[head&q.mask]

	slotCycle := slot.cycle.LoadAcquire()

	if slotCycle != cycle+1 {
		var zero T
		return zero, ErrWouldBlock
	}

	elem := slot.data
	var zero T
	slot.data = zero
	nextEnqCycle := (head + q.size) / q.capacity
	slot.cycle.StoreRelease(nextEnqCycle)
	q.head.StoreRelaxed(head + 1)

	return elem, nil
}

// Cap returns the queue capacity.
func (q *MPSC[T]) Cap() int {
	return int(q.capacity)
}
