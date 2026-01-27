// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// SPMCIndirect is an FAA-based SPMC queue for uintptr values.
//
// Uses 128-bit atomic operations to pack cycle and value into a single
// atomic entry. Based on SCQ algorithm with 2n slots for capacity n.
//
// Entry format: [lo=cycle | hi=value]
//
// Memory: 2n slots, 16 bytes per slot
type SPMCIndirect struct {
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	tail      atomix.Uint64 // Producer index (single producer writes, but consumers read)
	_         pad
	threshold atomix.Int64 // Livelock prevention
	_         pad
	buffer    []mpmc128Slot
	capacity  uint64
	size      uint64
	mask      uint64
}

// NewSPMCIndirect creates a new FAA-based SPMC queue for uintptr values.
// Capacity rounds up to the next power of 2.
func NewSPMCIndirect(capacity int) *SPMCIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &SPMCIndirect{
		buffer:   make([]mpmc128Slot, size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	q.threshold.StoreRelaxed(3*int64(n) - 1)

	// Initialize slots based on their first use position's cycle
	// Slots 0 to n-1: first used at positions 0-(n-1), cycle 0
	// Slots n to 2n-1: first used at positions n-(2n-1), cycle 1
	for i := uint64(0); i < size; i++ {
		q.buffer[i].entry.StoreRelaxed(i/n, 0)
	}

	return q
}

// Enqueue adds an element to the queue (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMCIndirect) Enqueue(elem uintptr) error {
	tail := q.tail.LoadRelaxed()
	head := q.head.LoadAcquire()

	// Check if full
	if tail >= head+q.capacity {
		return ErrWouldBlock
	}

	cycle := tail / q.capacity
	slot := &q.buffer[tail&q.mask]

	slotCycle, _ := slot.entry.LoadAcquire()

	if slotCycle != cycle {
		return ErrWouldBlock
	}

	// Write value and advance cycle
	slot.entry.StoreRelease(cycle+1, uint64(elem))
	q.tail.StoreRelaxed(tail + 1)

	// Reset threshold on successful enqueue (helps dequeue)
	q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *SPMCIndirect) Dequeue() (uintptr, error) {
	// Early exit via threshold (livelock prevention)
	if q.threshold.LoadRelaxed() < 0 {
		return 0, ErrWouldBlock
	}

	sw := spin.Wait{}
	for {
		// FAA to blindly claim position (true SCQ)
		myHead := q.head.AddAcqRel(1) - 1

		slot := &q.buffer[myHead&q.mask]
		expectedCycle := myHead/q.capacity + 1 // Filled slots have cycle+1

		// Check slot and try to read atomically (128-bit CAS)
		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			// Slot contains data - atomically read and mark consumed
			nextEnqCycle := (myHead + q.size) / q.capacity
			if slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, nextEnqCycle, 0) {
				return uintptr(valHi), nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot for future enqueuers
			nextEnqCycle := (myHead + q.size) / q.capacity
			slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, nextEnqCycle, 0)

			// Slot not yet filled - queue might be empty
			tail := q.tail.LoadRelaxed()
			if tail <= myHead+1 {
				// Queue is empty, help reset indices
				q.catchup(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				return 0, ErrWouldBlock
			}
			// Decrement threshold for livelock prevention
			if q.threshold.AddAcqRel(-1) <= 0 {
				return 0, ErrWouldBlock
			}
		}

		sw.Once()
	}
}

func (q *SPMCIndirect) catchup(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *SPMCIndirect) Cap() int {
	return int(q.capacity)
}

// SPMCPtr is an FAA-based SPMC queue for unsafe.Pointer values.
//
// Uses 128-bit atomic operations. Based on SCQ algorithm with 2n slots.
//
// Entry format: [lo=cycle | hi=pointer as uint64]
//
// Memory: 2n slots, 16 bytes per slot
type SPMCPtr struct {
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	tail      atomix.Uint64 // Producer index (single producer writes, but consumers read)
	_         pad
	threshold atomix.Int64 // Livelock prevention
	_         pad
	buffer    []mpmc128Slot
	capacity  uint64
	size      uint64
	mask      uint64
}

// NewSPMCPtr creates a new FAA-based SPMC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewSPMCPtr(capacity int) *SPMCPtr {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &SPMCPtr{
		buffer:   make([]mpmc128Slot, size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	q.threshold.StoreRelaxed(3*int64(n) - 1)

	for i := uint64(0); i < size; i++ {
		q.buffer[i].entry.StoreRelaxed(i/n, 0)
	}

	return q
}

// Enqueue adds an element to the queue (single producer only).
// Returns ErrWouldBlock if the queue is full.
func (q *SPMCPtr) Enqueue(elem unsafe.Pointer) error {
	tail := q.tail.LoadRelaxed()
	head := q.head.LoadAcquire()

	if tail >= head+q.capacity {
		return ErrWouldBlock
	}

	cycle := tail / q.capacity
	slot := &q.buffer[tail&q.mask]

	slotCycle, _ := slot.entry.LoadAcquire()

	if slotCycle != cycle {
		return ErrWouldBlock
	}

	slot.entry.StoreRelease(cycle+1, uint64(uintptr(elem)))
	q.tail.StoreRelaxed(tail + 1)

	q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)

	return nil
}

// Dequeue removes and returns an element (multiple consumers safe).
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *SPMCPtr) Dequeue() (unsafe.Pointer, error) {
	if q.threshold.LoadRelaxed() < 0 {
		return nil, ErrWouldBlock
	}

	sw := spin.Wait{}
	for {
		myHead := q.head.AddAcqRel(1) - 1

		slot := &q.buffer[myHead&q.mask]
		expectedCycle := myHead/q.capacity + 1
		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			nextEnqCycle := (myHead + q.size) / q.capacity
			if slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, nextEnqCycle, 0) {
				return *(*unsafe.Pointer)(unsafe.Pointer(&valHi)), nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot for future enqueuers
			nextEnqCycle := (myHead + q.size) / q.capacity
			slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, nextEnqCycle, 0)

			tail := q.tail.LoadRelaxed()
			if tail <= myHead+1 {
				q.catchupPtr(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				return nil, ErrWouldBlock
			}
			if q.threshold.AddAcqRel(-1) <= 0 {
				return nil, ErrWouldBlock
			}
		}

		sw.Once()
	}
}

func (q *SPMCPtr) catchupPtr(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *SPMCPtr) Cap() int {
	return int(q.capacity)
}
