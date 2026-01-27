// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// SPMCSeq is a CAS-based single-producer multi-consumer bounded queue.
//
// The single producer writes sequentially. Consumers use CAS to claim slots.
//
// This is the Compact variant using n slots (vs 2n for FAA-based default).
// Use NewSPMC for the default FAA-based implementation with better scalability.
//
// Memory: n slots (16 bytes per slot)
type SPMCSeq[T any] struct {
	_        pad
	head     atomix.Uint64 // Consumers CAS here
	_        pad
	tail     atomix.Uint64 // Producer writes here
	_        pad
	buffer   []spmcSeqSlot[T]
	mask     uint64
	capacity uint64
}

type spmcSeqSlot[T any] struct {
	seq  atomix.Uint64
	data T
	_    padShort // Pad to cache line
}

// NewSPMCSeq creates a new CAS-based SPMC queue.
// Capacity rounds up to the next power of 2.
// This is the Compact variant. Use NewSPMC for the default FAA-based implementation.
func NewSPMCSeq[T any](capacity int) *SPMCSeq[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &SPMCSeq[T]{
		buffer:   make([]spmcSeqSlot[T], n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].seq.StoreRelaxed(i)
	}

	return q
}

// Enqueue adds an element to the queue (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMCSeq[T]) Enqueue(elem *T) error {
	tail := q.tail.LoadRelaxed()
	slot := &q.buffer[tail&q.mask]
	seq := slot.seq.LoadAcquire()

	if seq != tail {
		return ErrWouldBlock
	}

	slot.data = *elem
	slot.seq.StoreRelease(tail + 1)
	q.tail.StoreRelease(tail + 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *SPMCSeq[T]) Dequeue() (T, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		tail := q.tail.LoadAcquire()

		if head >= tail {
			var zero T
			return zero, ErrWouldBlock
		}

		slot := &q.buffer[head&q.mask]
		seq := slot.seq.LoadAcquire()

		if seq == head+1 {
			if q.head.CompareAndSwapAcqRel(head, head+1) {
				elem := slot.data
				var zero T
				slot.data = zero
				slot.seq.StoreRelease(head + q.capacity)
				return elem, nil
			}
		} else if seq < head+1 {
			var zero T
			return zero, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *SPMCSeq[T]) Cap() int {
	return int(q.capacity)
}
