// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPSCIndirect is an FAA-based MPSC queue for uintptr values.
//
// Uses 128-bit atomic operations to pack cycle and value into a single
// atomic entry. Based on SCQ algorithm with 2n slots for capacity n.
//
// Entry format: [lo=cycle | hi=value]
//
// Memory: 2n slots, 16 bytes per slot
type MPSCIndirect struct {
	_        pad
	head     atomix.Uint64 // Consumer index (single consumer writes, but producers read)
	_        pad
	tail     atomix.Uint64 // Producer index (FAA)
	_        pad
	buffer   []mpmc128Slot
	capacity uint64
	size     uint64
	mask     uint64
}

// NewMPSCIndirect creates a new FAA-based MPSC queue for uintptr values.
// Capacity rounds up to the next power of 2.
func NewMPSCIndirect(capacity int) *MPSCIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &MPSCIndirect{
		buffer:   make([]mpmc128Slot, size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	// Initialize slots based on their first use position's cycle
	// Slots 0 to n-1: first used at positions 0-(n-1), cycle 0
	// Slots n to 2n-1: first used at positions n-(2n-1), cycle 1
	for i := uint64(0); i < size; i++ {
		q.buffer[i].entry.StoreRelaxed(i/n, 0)
	}

	return q
}

// Enqueue adds an element to the queue (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSCIndirect) Enqueue(elem uintptr) error {
	sw := spin.Wait{}
	for {
		// Early check: if queue appears full, don't waste a position
		tail := q.tail.LoadAcquire()
		head := q.head.LoadRelaxed() // Atomic read (written by consumer)
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		// FAA to blindly claim position (true SCQ)
		myTail := q.tail.AddAcqRel(1) - 1

		slot := &q.buffer[myTail&q.mask]
		expectedCycle := myTail / q.capacity

		// Check slot and try to write atomically (128-bit CAS)
		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			// Slot ready - atomically update cycle AND store value
			if slot.entry.CompareAndSwapAcqRel(expectedCycle, valHi, expectedCycle+1, uint64(elem)) {
				return nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot so dequeue can skip this position
			slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, expectedCycle+1, valHi)
			return ErrWouldBlock
		}

		// slotCycle > expectedCycle or CAS failed: another producer used this slot
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPSCIndirect) Dequeue() (uintptr, error) {
	head := q.head.LoadRelaxed()
	cycle := head / q.capacity
	slot := &q.buffer[head&q.mask]

	slotCycle, valHi := slot.entry.LoadAcquire()

	if slotCycle != cycle+1 {
		return 0, ErrWouldBlock
	}

	nextEnqCycle := (head + q.size) / q.capacity
	slot.entry.StoreRelease(nextEnqCycle, 0)
	q.head.StoreRelaxed(head + 1)

	return uintptr(valHi), nil
}

// Cap returns the queue capacity.
func (q *MPSCIndirect) Cap() int {
	return int(q.capacity)
}

// MPSCPtr is an FAA-based MPSC queue for unsafe.Pointer values.
//
// Uses 128-bit atomic operations. Based on SCQ algorithm with 2n slots.
//
// Entry format: [lo=cycle | hi=pointer as uint64]
//
// Memory: 2n slots, 16 bytes per slot
type MPSCPtr struct {
	_        pad
	head     atomix.Uint64 // Consumer index (single consumer writes, but producers read)
	_        pad
	tail     atomix.Uint64 // Producer index (FAA)
	_        pad
	buffer   []mpmc128Slot
	capacity uint64
	size     uint64
	mask     uint64
}

// NewMPSCPtr creates a new FAA-based MPSC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewMPSCPtr(capacity int) *MPSCPtr {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &MPSCPtr{
		buffer:   make([]mpmc128Slot, size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	for i := uint64(0); i < size; i++ {
		q.buffer[i].entry.StoreRelaxed(i/n, 0)
	}

	return q
}

// Enqueue adds an element to the queue (multiple producers safe).
// Returns ErrWouldBlock if the queue is full.
func (q *MPSCPtr) Enqueue(elem unsafe.Pointer) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadRelaxed()
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		myTail := q.tail.AddAcqRel(1) - 1

		slot := &q.buffer[myTail&q.mask]
		expectedCycle := myTail / q.capacity

		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			if slot.entry.CompareAndSwapAcqRel(expectedCycle, valHi, expectedCycle+1, uint64(uintptr(elem))) {
				return nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot so dequeue can skip this position
			slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, expectedCycle+1, valHi)
			return ErrWouldBlock
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element (single consumer only).
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *MPSCPtr) Dequeue() (unsafe.Pointer, error) {
	head := q.head.LoadRelaxed()
	cycle := head / q.capacity
	slot := &q.buffer[head&q.mask]

	slotCycle, valHi := slot.entry.LoadAcquire()

	if slotCycle != cycle+1 {
		return nil, ErrWouldBlock
	}

	nextEnqCycle := (head + q.size) / q.capacity
	slot.entry.StoreRelease(nextEnqCycle, 0)
	q.head.StoreRelaxed(head + 1)

	return *(*unsafe.Pointer)(unsafe.Pointer(&valHi)), nil
}

// Cap returns the queue capacity.
func (q *MPSCPtr) Cap() int {
	return int(q.capacity)
}
