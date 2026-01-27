// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPSCIndirectSeq is a multi-producer single-consumer queue for uintptr values.
//
// Producers use CAS to claim slots. The single consumer reads sequentially.
//
// Entry format: [lo=sequence | hi=value]
//
// Memory: 16 bytes per slot (sequence + value in single Uint128)
type MPSCIndirectSeq struct {
	_        pad
	head     atomix.Uint64 // Consumer reads from here
	_        pad
	tail     atomix.Uint64 // Producers CAS here
	_        pad
	buffer   []mpmc128SeqSlot // Reuse MPMC slot type
	mask     uint64
	capacity uint64
}

// NewMPSCIndirectSeq creates a new MPSC queue for uintptr values.
// Capacity rounds up to the next power of 2.
func NewMPSCIndirectSeq(capacity int) *MPSCIndirectSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPSCIndirectSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element to the queue (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSCIndirectSeq) Enqueue(elem uintptr) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()

		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		slot := &q.buffer[tail&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()

		if seqLo == tail {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, tail+1, uint64(elem)) {
				q.tail.CompareAndSwapRelaxed(tail, tail+1)
				return nil
			}
		} else if seqLo < tail {
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPSCIndirectSeq) Dequeue() (uintptr, error) {
	head := q.head.LoadRelaxed()
	slot := &q.buffer[head&q.mask]
	seqLo, valHi := slot.entry.LoadAcquire()

	if seqLo != head+1 {
		return 0, ErrWouldBlock
	}

	slot.entry.StoreRelease(head+q.capacity, 0)
	q.head.StoreRelease(head + 1)

	return uintptr(valHi), nil
}

// Cap returns the queue capacity.
func (q *MPSCIndirectSeq) Cap() int {
	return int(q.capacity)
}

// MPSCPtrSeq is a multi-producer single-consumer queue for unsafe.Pointer values.
//
// Entry format: [lo=sequence | hi=pointer as uint64]
//
// Memory: 16 bytes per slot
type MPSCPtrSeq struct {
	_        pad
	head     atomix.Uint64 // Consumer reads from here
	_        pad
	tail     atomix.Uint64 // Producers CAS here
	_        pad
	buffer   []mpmc128SeqSlot // Reuse MPMC slot type
	mask     uint64
	capacity uint64
}

// NewMPSCPtrSeq creates a new MPSC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewMPSCPtrSeq(capacity int) *MPSCPtrSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPSCPtrSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSCPtrSeq) Enqueue(elem unsafe.Pointer) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()

		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		slot := &q.buffer[tail&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()

		if seqLo == tail {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, tail+1, uint64(uintptr(elem))) {
				q.tail.CompareAndSwapRelaxed(tail, tail+1)
				return nil
			}
		} else if seqLo < tail {
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *MPSCPtrSeq) Dequeue() (unsafe.Pointer, error) {
	head := q.head.LoadRelaxed()
	slot := &q.buffer[head&q.mask]
	seqLo, valHi := slot.entry.LoadAcquire()

	if seqLo != head+1 {
		return nil, ErrWouldBlock
	}

	slot.entry.StoreRelease(head+q.capacity, 0)
	q.head.StoreRelease(head + 1)

	return *(*unsafe.Pointer)(unsafe.Pointer(&valHi)), nil
}

// Cap returns the queue capacity.
func (q *MPSCPtrSeq) Cap() int {
	return int(q.capacity)
}
