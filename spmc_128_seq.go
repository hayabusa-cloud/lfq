// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// SPMCIndirectSeq is a single-producer multi-consumer queue for uintptr values.
//
// The single producer writes sequentially. Consumers use CAS to claim slots.
//
// Entry format: [lo=sequence | hi=value]
//
// Memory: 16 bytes per slot (sequence + value in single Uint128)
type SPMCIndirectSeq struct {
	_        pad
	head     atomix.Uint64 // Consumers CAS here
	_        pad
	tail     atomix.Uint64 // Producer writes here
	_        pad
	buffer   []mpmc128SeqSlot // Reuse MPMC slot type
	mask     uint64
	capacity uint64
}

// NewSPMCIndirectSeq creates a new SPMC queue for uintptr values.
// Capacity rounds up to the next power of 2.
func NewSPMCIndirectSeq(capacity int) *SPMCIndirectSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &SPMCIndirectSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMCIndirectSeq) Enqueue(elem uintptr) error {
	tail := q.tail.LoadRelaxed()
	slot := &q.buffer[tail&q.mask]
	seqLo, _ := slot.entry.LoadAcquire()

	if seqLo != tail {
		return ErrWouldBlock
	}

	slot.entry.StoreRelease(tail+1, uint64(elem))
	q.tail.StoreRelease(tail + 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *SPMCIndirectSeq) Dequeue() (uintptr, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		tail := q.tail.LoadAcquire()

		if head >= tail {
			return 0, ErrWouldBlock
		}

		slot := &q.buffer[head&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()

		if seqLo == head+1 {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, head+q.capacity, 0) {
				q.head.CompareAndSwapRelaxed(head, head+1)
				return uintptr(valHi), nil
			}
		} else if seqLo < head+1 {
			return 0, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *SPMCIndirectSeq) Cap() int {
	return int(q.capacity)
}

// SPMCPtrSeq is a single-producer multi-consumer queue for unsafe.Pointer values.
//
// Entry format: [lo=sequence | hi=pointer as uint64]
//
// Memory: 16 bytes per slot
type SPMCPtrSeq struct {
	_        pad
	head     atomix.Uint64 // Consumers CAS here
	_        pad
	tail     atomix.Uint64 // Producer writes here
	_        pad
	buffer   []mpmc128SeqSlot // Reuse MPMC slot type
	mask     uint64
	capacity uint64
}

// NewSPMCPtrSeq creates a new SPMC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewSPMCPtrSeq(capacity int) *SPMCPtrSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &SPMCPtrSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMCPtrSeq) Enqueue(elem unsafe.Pointer) error {
	tail := q.tail.LoadRelaxed()
	slot := &q.buffer[tail&q.mask]
	seqLo, _ := slot.entry.LoadAcquire()

	if seqLo != tail {
		return ErrWouldBlock
	}

	slot.entry.StoreRelease(tail+1, uint64(uintptr(elem)))
	q.tail.StoreRelease(tail + 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *SPMCPtrSeq) Dequeue() (unsafe.Pointer, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		tail := q.tail.LoadAcquire()

		if head >= tail {
			return nil, ErrWouldBlock
		}

		slot := &q.buffer[head&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()

		if seqLo == head+1 {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, head+q.capacity, 0) {
				q.head.CompareAndSwapRelaxed(head, head+1)
				return *(*unsafe.Pointer)(unsafe.Pointer(&valHi)), nil
			}
		} else if seqLo < head+1 {
			return nil, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *SPMCPtrSeq) Cap() int {
	return int(q.capacity)
}
