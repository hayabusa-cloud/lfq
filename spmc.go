// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// SPMC is an FAA-based single-producer multi-consumer bounded queue.
//
// Consumers use FAA to blindly claim positions (SCQ-style), requiring 2n
// physical slots for capacity n.
//
// Memory: 2n slots for capacity n (16+ bytes per slot)
type SPMC[T any] struct {
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	tail      atomix.Uint64 // Producer index (single producer writes, but consumers read)
	_         pad
	threshold atomix.Int64 // Livelock prevention for consumers
	_         pad
	buffer    []spmcSlot[T]
	capacity  uint64 // n (usable capacity)
	size      uint64 // 2n (physical slots)
	mask      uint64 // 2n - 1
}

type spmcSlot[T any] struct {
	cycle atomix.Uint64 // Round number
	data  T
	_     padShort
}

// NewSPMC creates a new FAA-based SPMC queue.
// Capacity rounds up to the next power of 2.
func NewSPMC[T any](capacity int) *SPMC[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &SPMC[T]{
		buffer:   make([]spmcSlot[T], size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	q.threshold.StoreRelaxed(3*int64(n) - 1)

	for i := uint64(0); i < size; i++ {
		q.buffer[i].cycle.StoreRelaxed(i / n)
	}

	return q
}

// Enqueue adds an element to the queue (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMC[T]) Enqueue(elem *T) error {
	tail := q.tail.LoadRelaxed()
	head := q.head.LoadAcquire()

	if tail >= head+q.capacity {
		return ErrWouldBlock
	}

	cycle := tail / q.capacity
	slot := &q.buffer[tail&q.mask]

	slotCycle := slot.cycle.LoadAcquire()

	if slotCycle != cycle {
		return ErrWouldBlock
	}

	slot.data = *elem
	slot.cycle.StoreRelease(cycle + 1)
	q.tail.StoreRelaxed(tail + 1)

	q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *SPMC[T]) Dequeue() (T, error) {
	if q.threshold.LoadRelaxed() < 0 {
		var zero T
		return zero, ErrWouldBlock
	}

	sw := spin.Wait{}
	for {
		myHead := q.head.AddAcqRel(1) - 1

		slot := &q.buffer[myHead&q.mask]
		expectedCycle := myHead/q.capacity + 1
		slotCycle := slot.cycle.LoadAcquire()

		if slotCycle == expectedCycle {
			elem := slot.data
			var zero T
			slot.data = zero
			nextEnqCycle := (myHead + q.size) / q.capacity
			slot.cycle.StoreRelease(nextEnqCycle)
			return elem, nil
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot for future enqueuers
			nextEnqCycle := (myHead + q.size) / q.capacity
			slot.cycle.CompareAndSwapAcqRel(slotCycle, nextEnqCycle)

			tail := q.tail.LoadRelaxed()
			if tail <= myHead+1 {
				q.catchup(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				var zero T
				return zero, ErrWouldBlock
			}
			if q.threshold.AddAcqRel(-1) <= 0 {
				var zero T
				return zero, ErrWouldBlock
			}
		}
		sw.Once()
	}
}

func (q *SPMC[T]) catchup(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *SPMC[T]) Cap() int {
	return int(q.capacity)
}
