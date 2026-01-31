// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPMC is an FAA-based multi-producer multi-consumer bounded queue.
//
// Based on the SCQ (Scalable Circular Queue) algorithm by Nikolaev (DISC 2019).
// Uses Fetch-And-Add to blindly increment position counters, requiring 2n
// physical slots for capacity n. This approach scales better under high
// contention compared to CAS-based alternatives.
//
// Cycle-based slot validation provides ABA safety: each slot tracks which
// "cycle" (round) it belongs to via cycle = position / capacity.
//
// Memory: 2n slots for capacity n (16+ bytes per slot)
type MPMC[T any] struct {
	_         pad
	tail      atomix.Uint64 // Producer index (FAA)
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	threshold atomix.Int64 // Livelock prevention for dequeue
	_         pad
	draining  atomix.Bool // Drain mode: skip threshold check
	_         pad
	buffer    []mpmcSlot[T]
	capacity  uint64 // n (usable capacity)
	size      uint64 // 2n (physical slots)
	mask      uint64 // 2n - 1
}

type mpmcSlot[T any] struct {
	cycle atomix.Uint64 // Round number for this slot
	data  T
	_     padShort // Pad to cache line
}

// NewMPMC creates a new FAA-based MPMC queue.
// Capacity rounds up to the next power of 2.
// Physical slot count is 2n for capacity n (SCQ requirement).
func NewMPMC[T any](capacity int) *MPMC[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2 // 2n physical slots

	q := &MPMC[T]{
		buffer:   make([]mpmcSlot[T], size),
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

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMC[T]) Enqueue(elem *T) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()
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
			q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)
			return nil
		}

		if int64(slotCycle) < int64(expectedCycle) {
			return ErrWouldBlock // Queue full
		}

		sw.Once()
	}
}

// Drain signals that no more enqueues will occur.
// After Drain is called, Dequeue skips the threshold check to allow
// consumers to drain all remaining items without producer pressure.
func (q *MPMC[T]) Drain() {
	q.draining.StoreRelease(true)
}

// Dequeue removes and returns an element from the queue.
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *MPMC[T]) Dequeue() (T, error) {
	// Early exit via threshold (livelock prevention)
	// Skip threshold check in drain mode
	if !q.draining.LoadAcquire() && q.threshold.LoadRelaxed() < 0 {
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

			tail := q.tail.LoadAcquire()
			if tail <= myHead+1 {
				q.catchup(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				var zero T
				return zero, ErrWouldBlock
			}
			if q.threshold.AddAcqRel(-1) <= 0 && !q.draining.LoadAcquire() {
				var zero T
				return zero, ErrWouldBlock
			}
		}
		sw.Once()
	}
}

func (q *MPMC[T]) catchup(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *MPMC[T]) Cap() int {
	return int(q.capacity)
}
