// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPSCSeq is a CAS-based multi-producer single-consumer bounded queue.
//
// Producers use CAS to claim slots. The single consumer reads sequentially.
//
// This is the Compact variant using n slots (vs 2n for FAA-based default).
// Use NewMPSC for the default FAA-based implementation with better scalability.
//
// Memory: n slots (16 bytes per slot)
type MPSCSeq[T any] struct {
	_        pad
	head     atomix.Uint64 // Consumer reads from here
	_        pad
	tail     atomix.Uint64 // Producers CAS here
	_        pad
	buffer   []mpscSeqSlot[T]
	mask     uint64
	capacity uint64
}

type mpscSeqSlot[T any] struct {
	seq  atomix.Uint64
	data T
	_    padShort // Pad to cache line
}

// NewMPSCSeq creates a new CAS-based MPSC queue.
// Capacity rounds up to the next power of 2.
// This is the Compact variant. Use NewMPSC for the default FAA-based implementation.
func NewMPSCSeq[T any](capacity int) *MPSCSeq[T] {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPSCSeq[T]{
		buffer:   make([]mpscSeqSlot[T], n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].seq.StoreRelaxed(i)
	}

	return q
}

// Enqueue adds an element to the queue (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSCSeq[T]) Enqueue(elem *T) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()

		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		slot := &q.buffer[tail&q.mask]
		seq := slot.seq.LoadAcquire()

		if seq == tail {
			if q.tail.CompareAndSwapAcqRel(tail, tail+1) {
				slot.data = *elem
				slot.seq.StoreRelease(tail + 1)
				return nil
			}
		} else if seq < tail {
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (zero-value, ErrWouldBlock) if the queue is empty.
func (q *MPSCSeq[T]) Dequeue() (T, error) {
	head := q.head.LoadRelaxed()
	slot := &q.buffer[head&q.mask]
	seq := slot.seq.LoadAcquire()

	if seq != head+1 {
		var zero T
		return zero, ErrWouldBlock
	}

	elem := slot.data
	var zero T
	slot.data = zero
	slot.seq.StoreRelease(head + q.capacity)
	q.head.StoreRelease(head + 1)

	return elem, nil
}

// Cap returns the queue capacity.
func (q *MPSCSeq[T]) Cap() int {
	return int(q.capacity)
}
