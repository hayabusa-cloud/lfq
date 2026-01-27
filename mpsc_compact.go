// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPSCCompactIndirect is a compact MPSC queue for uintptr values.
//
// Uses round-based empty detection. Multiple producers use CAS,
// single consumer reads sequentially.
//
// Memory: 8 bytes per slot
type MPSCCompactIndirect struct {
	_        pad
	head     atomix.Uint64 // Consumer reads from here
	_        pad
	tail     atomix.Uint64 // Producers CAS here
	_        pad
	buffer   []atomix.Uintptr
	mask     uint64
	capacity uint64
	order    uint64
}

// NewMPSCCompactIndirect creates a new compact MPSC queue.
// Capacity rounds up to the next power of 2.
// Values are limited to 63 bits (high bit reserved for empty flag).
func NewMPSCCompactIndirect(capacity int) *MPSCCompactIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	order := uint64(0)
	for (1 << order) < n {
		order++
	}

	q := &MPSCCompactIndirect{
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

// Enqueue adds a value (multiple producers safe).
// Values must fit in 63 bits.
func (q *MPSCCompactIndirect) Enqueue(elem uintptr) error {
	if elem&emptyFlag != 0 {
		panic("lfq: value exceeds 63 bits")
	}

	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()

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

// Dequeue removes and returns a value (single consumer only).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPSCCompactIndirect) Dequeue() (uintptr, error) {
	head := q.head.LoadRelaxed()
	tail := q.tail.LoadAcquire()

	if head >= tail {
		return 0, ErrWouldBlock
	}

	idx := head & q.mask
	elem := q.buffer[idx].LoadAcquire()

	nextRound := ((head >> q.order) + 1) & (emptyFlag - 1)
	nextEmpty := emptyFlag | uintptr(nextRound)

	if elem&emptyFlag != 0 {
		return 0, ErrWouldBlock
	}

	q.buffer[idx].StoreRelease(nextEmpty)
	q.head.StoreRelease(head + 1)

	return elem, nil
}

// Cap returns queue capacity.
func (q *MPSCCompactIndirect) Cap() int {
	return int(q.capacity)
}
