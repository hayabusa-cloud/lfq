// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// emptyFlag marks a slot as empty. The remaining 63 bits store the round number.
const emptyFlag = 1 << 63

// MPMCCompactIndirect is a compact MPMC queue for uintptr values.
//
// Uses round-based empty detection: empty slots store (emptyFlag | round),
// filled slots store the value directly. This achieves 8 bytes per slot
// while allowing any 63-bit value (including zero) to be enqueued.
//
// Memory: 8 bytes per slot
type MPMCCompactIndirect struct {
	_        pad
	tail     atomix.Uint64
	_        pad
	head     atomix.Uint64
	_        pad
	buffer   []atomix.Uintptr
	mask     uint64
	capacity uint64
	order    uint64 // log2(capacity) for round calculation
}

// NewMPMCCompactIndirect creates a new compact MPMC queue.
// Capacity rounds up to the next power of 2.
// Values are limited to 63 bits (high bit reserved for empty flag).
func NewMPMCCompactIndirect(capacity int) *MPMCCompactIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	order := uint64(0)
	for (1 << order) < n {
		order++
	}

	q := &MPMCCompactIndirect{
		buffer:   make([]atomix.Uintptr, n),
		mask:     n - 1,
		capacity: n,
		order:    order,
	}

	for i := range q.buffer {
		q.buffer[i].StoreRelaxed(emptyFlag | 0)
	}

	return q
}

// Enqueue adds a value to the queue.
// Returns ErrWouldBlock if the queue is full.
// Values must fit in 63 bits (high bit must be 0).
func (q *MPMCCompactIndirect) Enqueue(elem uintptr) error {
	if elem&emptyFlag != 0 {
		panic("lfq: value exceeds 63 bits")
	}

	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()
		if tail != q.tail.LoadAcquire() {
			continue
		}
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		idx := tail & q.mask
		round := (tail >> q.order) & (emptyFlag - 1)
		expected := emptyFlag | uintptr(round)

		if q.buffer[idx].CompareAndSwapAcqRel(expected, elem) {
			q.tail.CompareAndSwapAcqRel(tail, tail+1)
			return nil
		}
		q.tail.CompareAndSwapAcqRel(tail, tail+1)
		sw.Once()
	}
}

// Dequeue removes and returns a value from the queue.
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPMCCompactIndirect) Dequeue() (uintptr, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		tail := q.tail.LoadAcquire()

		idx := head & q.mask
		elem := q.buffer[idx].LoadAcquire()
		if head != q.head.LoadAcquire() {
			continue
		}
		if head >= tail {
			return 0, ErrWouldBlock
		}
		nextRound := ((head >> q.order) + 1) & (emptyFlag - 1)
		nextEmpty := emptyFlag | uintptr(nextRound)
		if elem == nextEmpty {
			q.head.CompareAndSwapAcqRel(head, head+1)
			continue
		}
		if elem&emptyFlag != 0 {
			sw.Once()
			continue
		}
		if q.buffer[idx].CompareAndSwapAcqRel(elem, nextEmpty) {
			q.head.CompareAndSwapAcqRel(head, head+1)
			return elem, nil
		}

		q.head.CompareAndSwapAcqRel(head, head+1)
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *MPMCCompactIndirect) Cap() int {
	return int(q.capacity)
}
