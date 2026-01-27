// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPMCSeq is a CAS-based multi-producer multi-consumer bounded queue.
//
// Uses per-slot sequence numbers which provide:
//   - Full ABA safety via sequence-based validation
//   - Works with both distinct and non-distinct values
//   - Good performance under moderate contention
//
// This is the Compact variant using n slots (vs 2n for FAA-based default).
// Use NewMPMC for the default FAA-based implementation with better scalability.
//
// Memory: n slots (16+ bytes per slot)
type MPMCSeq[T any] struct {
	_        pad
	tail     atomix.Uint64 // Producer index
	_        pad
	head     atomix.Uint64 // Consumer index
	_        pad
	buffer   []mpmcSeqSlot[T]
	mask     uint64
	capacity uint64
}

type mpmcSeqSlot[T any] struct {
	seq  atomix.Uint64
	data T
	_    padShort // Pad to cache line
}

// NewMPMCSeq creates a new CAS-based MPMC queue.
// Capacity rounds up to the next power of 2.
// This is the Compact variant. Use NewMPMC for the default FAA-based implementation.
func NewMPMCSeq[T any](capacity int) *MPMCSeq[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPMCSeq[T]{
		buffer:   make([]mpmcSeqSlot[T], n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].seq.StoreRelaxed(i)
	}

	return q
}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCSeq[T]) Enqueue(elem *T) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		slot := &q.buffer[tail&q.mask]
		seq := slot.seq.LoadAcquire()
		diff := int64(seq) - int64(tail)

		if diff == 0 {
			if q.tail.CompareAndSwapAcqRel(tail, tail+1) {
				slot.data = *elem
				slot.seq.StoreRelease(tail + 1)
				return nil
			}
		} else if diff < 0 {
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *MPMCSeq[T]) Dequeue() (T, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		slot := &q.buffer[head&q.mask]
		seq := slot.seq.LoadAcquire()
		diff := int64(seq) - int64(head+1)

		if diff == 0 {
			if q.head.CompareAndSwapAcqRel(head, head+1) {
				elem := slot.data
				var zero T
				slot.data = zero
				slot.seq.StoreRelease(head + q.capacity)
				return elem, nil
			}
		} else if diff < 0 {
			var zero T
			return zero, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *MPMCSeq[T]) Cap() int {
	return int(q.capacity)
}
