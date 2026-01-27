// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// SPMCCompactIndirect is a compact SPMC queue for uintptr values.
//
// Uses round-based empty detection. Single producer writes sequentially,
// multiple consumers use CAS.
//
// Memory: 8 bytes per slot
type SPMCCompactIndirect struct {
	_        pad
	head     atomix.Uint64 // Consumers CAS here
	_        pad
	tail     atomix.Uint64 // Producer writes here
	_        pad
	buffer   []atomix.Uintptr
	mask     uint64
	capacity uint64
	order    uint64
}

// NewSPMCCompactIndirect creates a new compact SPMC queue.
// Capacity rounds up to the next power of 2.
// Values are limited to 63 bits (high bit reserved for empty flag).
func NewSPMCCompactIndirect(capacity int) *SPMCCompactIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	order := uint64(0)
	for (1 << order) < n {
		order++
	}

	q := &SPMCCompactIndirect{
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

// Enqueue adds a value (single producer only).
// Values must fit in 63 bits. Returns ErrWouldBlock if the queue is full.
func (q *SPMCCompactIndirect) Enqueue(elem uintptr) error {
	if elem&emptyFlag != 0 {
		panic("lfq: value exceeds 63 bits")
	}

	tail := q.tail.LoadRelaxed()
	head := q.head.LoadAcquire()

	if tail >= head+q.capacity {
		return ErrWouldBlock
	}

	idx := tail & q.mask
	round := (tail >> q.order) & (emptyFlag - 1)
	expected := emptyFlag | uintptr(round)

	if !q.buffer[idx].CompareAndSwapAcqRel(expected, elem) {
		return ErrWouldBlock
	}
	q.tail.StoreRelease(tail + 1)

	return nil
}

// Dequeue removes and returns a value (multiple consumers safe).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *SPMCCompactIndirect) Dequeue() (uintptr, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		tail := q.tail.LoadAcquire()

		if head >= tail {
			return 0, ErrWouldBlock
		}

		idx := head & q.mask
		elem := q.buffer[idx].LoadAcquire()

		if head != q.head.LoadAcquire() {
			continue
		}

		currentRound := (head >> q.order) & (emptyFlag - 1)
		nextRound := (currentRound + 1) & (emptyFlag - 1)
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

// Cap returns queue capacity.
func (q *SPMCCompactIndirect) Cap() int {
	return int(q.capacity)
}
