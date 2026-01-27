// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPMCIndirectSeq is a CAS-based MPMC queue for uintptr values.
//
// Uses 128-bit atomic operations to pack sequence and value into a single
// atomic entry, reducing atomics per operation from 2-3 to 1.
//
// This is the Compact variant using n slots. Use NewMPMCIndirect for the
// default FAA-based implementation with 2n slots and better scalability.
//
// Entry format: [lo=sequence | hi=value]
//
// Memory: n slots, 16 bytes per slot
type MPMCIndirectSeq struct {
	_        pad
	tail     atomix.Uint64 // Producer index
	_        pad
	head     atomix.Uint64 // Consumer index
	_        pad
	buffer   []mpmc128SeqSlot
	mask     uint64
	capacity uint64
}

type mpmc128SeqSlot struct {
	entry atomix.Uint128 // lo=seq, hi=value
	_     [64 - 16]byte  // Pad to cache line
}

// NewMPMCIndirectSeq creates a new CAS-based MPMC queue for uintptr values.
// Capacity rounds up to the next power of 2.
// This is the Compact variant. Use NewMPMCIndirect for the default FAA-based implementation.
func NewMPMCIndirectSeq(capacity int) *MPMCIndirectSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPMCIndirectSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	// Initialize: seq[i] = i (ready for write at round 0), val = 0
	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCIndirectSeq) Enqueue(elem uintptr) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		slot := &q.buffer[tail&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()
		diff := int64(seqLo) - int64(tail)

		if diff == 0 {
			// Slot ready for writing (seq == tail)
			// Single: atomically update seq AND store value
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, tail+1, uint64(elem)) {
				// Help advance tail for other producers
				q.tail.CompareAndSwapRelaxed(tail, tail+1)
				return nil
			}
		} else if diff < 0 {
			// Queue is full (slot from old round not yet consumed)
			return ErrWouldBlock
		}
		// diff > 0: another producer succeeded, retry with fresh tail
		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPMCIndirectSeq) Dequeue() (uintptr, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		slot := &q.buffer[head&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()
		diff := int64(seqLo) - int64(head+1)

		if diff == 0 {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, head+q.capacity, 0) {
				q.head.CompareAndSwapRelaxed(head, head+1)
				return uintptr(valHi), nil
			}
		} else if diff < 0 {
			return 0, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *MPMCIndirectSeq) Cap() int {
	return int(q.capacity)
}

// MPMCPtrSeq is a CAS-based MPMC queue for unsafe.Pointer values.
//
// Uses 128-bit atomic operations to pack sequence and pointer into a single
// atomic entry, reducing atomics per operation from 2-3 to 1.
//
// This is the Compact variant using n slots. Use NewMPMCPtr for the default
// FAA-based implementation with 2n slots and better scalability.
//
// Entry format: [lo=sequence | hi=pointer as uint64]
//
// Memory: n slots, 16 bytes per slot
type MPMCPtrSeq struct {
	_        pad
	tail     atomix.Uint64 // Producer index
	_        pad
	head     atomix.Uint64 // Consumer index
	_        pad
	buffer   []mpmc128SeqSlot // Reuse same slot type
	mask     uint64
	capacity uint64
}

// NewMPMCPtrSeq creates a new CAS-based MPMC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
// This is the Compact variant. Use NewMPMCPtr for the default FAA-based implementation.
func NewMPMCPtrSeq(capacity int) *MPMCPtrSeq {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	q := &MPMCPtrSeq{
		buffer:   make([]mpmc128SeqSlot, n),
		mask:     n - 1,
		capacity: n,
	}

	for i := uint64(0); i < n; i++ {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}

	return q
}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCPtrSeq) Enqueue(elem unsafe.Pointer) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		slot := &q.buffer[tail&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()
		diff := int64(seqLo) - int64(tail)

		if diff == 0 {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, tail+1, uint64(uintptr(elem))) {
				q.tail.CompareAndSwapRelaxed(tail, tail+1)
				return nil
			}
		} else if diff < 0 {
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *MPMCPtrSeq) Dequeue() (unsafe.Pointer, error) {
	sw := spin.Wait{}
	for {
		head := q.head.LoadAcquire()
		slot := &q.buffer[head&q.mask]
		seqLo, valHi := slot.entry.LoadAcquire()
		diff := int64(seqLo) - int64(head+1)

		if diff == 0 {
			if slot.entry.CompareAndSwapAcqRel(seqLo, valHi, head+q.capacity, 0) {
				q.head.CompareAndSwapRelaxed(head, head+1)
				return *(*unsafe.Pointer)(unsafe.Pointer(&valHi)), nil
			}
		} else if diff < 0 {
			return nil, ErrWouldBlock
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *MPMCPtrSeq) Cap() int {
	return int(q.capacity)
}
