// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import (
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/spin"
)

// MPMCIndirect is an FAA-based MPMC queue for uintptr values.
//
// Uses 128-bit atomic operations to pack cycle and value into a single
// atomic entry, reducing atomics per operation from 2-3 to 1.
// Based on SCQ algorithm (Nikolaev, DISC 2019) with 2n slots for capacity n.
//
// Entry format: [lo=cycle | hi=value]
//
// Memory: 2n slots, 16 bytes per slot (cycle + value in single Uint128)
type MPMCIndirect struct {
	_         pad
	tail      atomix.Uint64 // Producer index (FAA)
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	threshold atomix.Int64 // Livelock prevention
	_         pad
	draining  atomix.Bool // Drain mode: skip threshold check
	_         pad
	buffer    []mpmc128Slot
	capacity  uint64 // n (usable capacity)
	size      uint64 // 2n (physical slots)
	mask      uint64 // 2n - 1
}

type mpmc128Slot struct {
	entry atomix.Uint128 // lo=cycle, hi=value
	_     [64 - 16]byte  // Pad to cache line
}

// NewMPMCIndirect creates a new FAA-based MPMC queue for uintptr values.
// Capacity rounds up to the next power of 2.
// Physical slot count is 2n for capacity n.
func NewMPMCIndirect(capacity int) *MPMCIndirect {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &MPMCIndirect{
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

// Drain signals that no more enqueues will occur.
// After Drain is called, Dequeue skips the threshold check to allow
// consumers to drain all remaining items without producer pressure.
func (q *MPMCIndirect) Drain() {
	q.draining.StoreRelease(true)
}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCIndirect) Enqueue(elem uintptr) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		myTail := q.tail.AddAcqRel(1) - 1

		slot := &q.buffer[myTail&q.mask]
		expectedCycle := myTail / q.capacity

		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			if slot.entry.CompareAndSwapAcqRel(expectedCycle, valHi, expectedCycle+1, uint64(elem)) {
				q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)
				return nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			return ErrWouldBlock // Queue full
		}

		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPMCIndirect) Dequeue() (uintptr, error) {
	// Early exit via threshold (livelock prevention)
	// Skip threshold check in drain mode
	if !q.draining.LoadAcquire() && q.threshold.LoadRelaxed() < 0 {
		return 0, ErrWouldBlock
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
				return uintptr(valHi), nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			// SCQ slot repair: advance stale slot for future enqueuers
			nextEnqCycle := (myHead + q.size) / q.capacity
			slot.entry.CompareAndSwapAcqRel(slotCycle, valHi, nextEnqCycle, 0)

			tail := q.tail.LoadAcquire()
			if tail <= myHead+1 {
				q.catchup(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				return 0, ErrWouldBlock
			}
			if q.threshold.AddAcqRel(-1) <= 0 && !q.draining.LoadAcquire() {
				return 0, ErrWouldBlock
			}
		}

		sw.Once()
	}
}

func (q *MPMCIndirect) catchup(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *MPMCIndirect) Cap() int {
	return int(q.capacity)
}

// MPMCPtr is an FAA-based MPMC queue for unsafe.Pointer values.
//
// Uses 128-bit atomic operations to pack cycle and pointer into a single
// atomic entry. Based on SCQ algorithm with 2n slots for capacity n.
//
// Entry format: [lo=cycle | hi=pointer as uint64]
//
// Memory: 2n slots, 16 bytes per slot
type MPMCPtr struct {
	_         pad
	tail      atomix.Uint64 // Producer index (FAA)
	_         pad
	head      atomix.Uint64 // Consumer index (FAA)
	_         pad
	threshold atomix.Int64 // Livelock prevention
	_         pad
	draining  atomix.Bool // Drain mode: skip threshold check
	_         pad
	buffer    []mpmc128Slot // Reuse same slot type
	capacity  uint64        // n (usable capacity)
	size      uint64        // 2n (physical slots)
	mask      uint64        // 2n - 1
}

// NewMPMCPtr creates a new FAA-based MPMC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewMPMCPtr(capacity int) *MPMCPtr {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q := &MPMCPtr{
		buffer:   make([]mpmc128Slot, size),
		capacity: n,
		size:     size,
		mask:     size - 1,
	}

	q.threshold.StoreRelaxed(3*int64(n) - 1)

	// Initialize slots based on their first use position's cycle
	for i := uint64(0); i < size; i++ {
		q.buffer[i].entry.StoreRelaxed(i/n, 0)
	}

	return q
}

// Drain signals that no more enqueues will occur.
// After Drain is called, Dequeue skips the threshold check to allow
// consumers to drain all remaining items without producer pressure.
func (q *MPMCPtr) Drain() {
	q.draining.StoreRelease(true)
}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCPtr) Enqueue(elem unsafe.Pointer) error {
	sw := spin.Wait{}
	for {
		tail := q.tail.LoadAcquire()
		head := q.head.LoadAcquire()
		if tail >= head+q.capacity {
			return ErrWouldBlock
		}

		myTail := q.tail.AddAcqRel(1) - 1

		slot := &q.buffer[myTail&q.mask]
		expectedCycle := myTail / q.capacity

		slotCycle, valHi := slot.entry.LoadAcquire()

		if slotCycle == expectedCycle {
			if slot.entry.CompareAndSwapAcqRel(expectedCycle, valHi, expectedCycle+1, uint64(uintptr(elem))) {
				q.threshold.StoreRelaxed(3*int64(q.capacity) - 1)
				return nil
			}
		}

		if int64(slotCycle) < int64(expectedCycle) {
			return ErrWouldBlock // Queue full
		}

		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *MPMCPtr) Dequeue() (unsafe.Pointer, error) {
	// Early exit via threshold (livelock prevention)
	// Skip threshold check in drain mode
	if !q.draining.LoadAcquire() && q.threshold.LoadRelaxed() < 0 {
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

			tail := q.tail.LoadAcquire()
			if tail <= myHead+1 {
				q.catchupPtr(tail, myHead+1)
				q.threshold.AddAcqRel(-1)
				return nil, ErrWouldBlock
			}
			if q.threshold.AddAcqRel(-1) <= 0 && !q.draining.LoadAcquire() {
				return nil, ErrWouldBlock
			}
		}

		sw.Once()
	}
}

func (q *MPMCPtr) catchupPtr(tail, head uint64) {
	for tail < head {
		if q.tail.CompareAndSwapRelaxed(tail, head) {
			break
		}
		tail = q.tail.LoadRelaxed()
		head = q.head.LoadRelaxed()
	}
}

// Cap returns the queue capacity.
func (q *MPMCPtr) Cap() int {
	return int(q.capacity)
}
