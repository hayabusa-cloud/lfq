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
// Each slot packs its sequence and full-width value into one atomic pair.
type MPMCIndirect struct {
	_        pad
	tail     atomix.Uint64 // Successful publications plus outstanding claims.
	tailHint atomix.Uint64 // Lower bound on consecutive publications.
	_        pad
	head     atomix.Uint64 // Successful removals plus outstanding claims.
	headHint atomix.Uint64 // Lower bound on consecutive removals.
	_        pad
	buffer   []mpmc128Slot
	capacity uint64 // n (usable capacity)
	mask     uint64 // 2n - 1
}

type mpmc128Slot struct {
	entry atomix.Uint128 // lo=sequence, hi=value
	_     [64 - 16]byte  // Pad to cache line
}

// NewMPMCIndirect creates a new FAA-based MPMC queue for uintptr values.
// Capacity rounds up to the next power of 2.
// Physical slot count is 2n for capacity n.
func NewMPMCIndirect(capacity int) *MPMCIndirect {
	q := &MPMCIndirect{}
	q.Init(capacity)
	return q
}

// Init initializes a zero-value MPMCIndirect queue in place.
// Capacity rounds up to the next power of 2.
func (q *MPMCIndirect) Init(capacity int) {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}

	n := uint64(roundToPow2(capacity))
	size := n * 2

	q.buffer = make([]mpmc128Slot, size)
	q.capacity = n
	q.mask = size - 1

	for i := range size {
		q.buffer[i].entry.StoreRelaxed(i, 0)
	}
}

// Drain is a no-op. Consumers can drain the queue after producers finish.
func (q *MPMCIndirect) Drain() {}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCIndirect) Enqueue(elem uintptr) error {
	if !q.hasRoom() {
		return ErrWouldBlock
	}
	ticket := q.tail.AddAcqRel(1) - 1
	var sw spin.Wait
	for {
		pos, seq, previous := q.position(q.tailHint.LoadAcquire(), ticket, publishPhase)
		if seq < pos { // The previous occupant still fills this physical slot.
			q.tail.AddAcqRel(^uint64(0))
			return ErrWouldBlock
		}
		if q.buffer[pos&q.mask].entry.CompareAndSwapAcqRel(pos, previous, pos+1, uint64(elem)) {
			q.tailHint.StoreRelease(pos + 1)
			return nil
		}
		sw.Once()
	}
}

// Dequeue removes and returns an element from the queue.
// Returns (0, ErrWouldBlock) if the queue is empty.
func (q *MPMCIndirect) Dequeue() (uintptr, error) {
	head := q.headHint.LoadAcquire()
	if q.tail.LoadAcquire() <= head {
		return 0, ErrWouldBlock
	}
	ticket := q.head.AddAcqRel(1) - 1
	var sw spin.Wait
	for {
		pos, seq, word := q.position(head, ticket, consumePhase)
		if seq <= pos {
			q.head.AddAcqRel(^uint64(0))
			q.headHint.StoreRelease(pos)
			return 0, ErrWouldBlock
		}
		if q.buffer[pos&q.mask].entry.CompareAndSwapAcqRel(seq, word, pos+uint64(len(q.buffer)), 0) {
			q.headHint.StoreRelease(pos + 1)
			return uintptr(word), nil
		}
		sw.Once()
		head = q.headHint.LoadAcquire()
	}
}

const (
	publishPhase uint64 = iota
	consumePhase
)

// A slot is free at t, occupied at t+1, and free again at t+len(buffer).
// A transfer may use an earlier outstanding FAA ticket if its owner paused.
// Successive publications and successive removals each form a contiguous prefix.
func (q *MPMCIndirect) position(start, ticket, phase uint64) (pos, seq, word uint64) {
	pos = start
	if ticket > pos {
		prev, _ := q.buffer[(ticket-1)&q.mask].entry.LoadAcquire()
		if prev > ticket-1+phase {
			pos = ticket
		}
	}

	var sw spin.Wait
	for {
		seq, word = q.buffer[pos&q.mask].entry.LoadAcquire()
		if seq <= pos+phase {
			return
		}
		pos++
		// A delayed hint can be many laps behind. At most one ring of
		// positions can separate this slot's tag from either frontier.
		if size := uint64(len(q.buffer)); seq > size {
			pos = max(pos, seq-size)
		}
		sw.Once()
	}
}

// A raw FAA distance includes outstanding claims. Before refusing a producer,
// check whether the item one nominal capacity behind has actually been removed.
func (q *MPMCIndirect) hasRoom() bool {
	var sw spin.Wait
	for {
		head := q.headHint.LoadAcquire()
		tail := q.tail.LoadAcquire()
		if tail < head+q.capacity {
			return true
		}
		pos, _, _ := q.position(q.tailHint.LoadAcquire(), tail, publishPhase)
		if pos < q.capacity {
			return true
		}
		old := pos - q.capacity
		seq, _ := q.buffer[old&q.mask].entry.LoadAcquire()
		if seq <= old+1 {
			return false
		}
		seq, _ = q.buffer[pos&q.mask].entry.LoadAcquire()
		if seq <= pos {
			return true
		}
		sw.Once()
	}
}

// Cap returns the queue capacity.
func (q *MPMCIndirect) Cap() int {
	return int(q.capacity)
}

// MPMCPtr is an FAA-based MPMC queue for unsafe.Pointer values.
// Keep each Go pointee live at a stable address until its consumer takes ownership.
type MPMCPtr MPMCIndirect

// NewMPMCPtr creates a new FAA-based MPMC queue for unsafe.Pointer values.
// Capacity rounds up to the next power of 2.
func NewMPMCPtr(capacity int) *MPMCPtr {
	q := &MPMCPtr{}
	q.Init(capacity)
	return q
}

// Init initializes a zero-value MPMCPtr queue in place.
// Capacity rounds up to the next power of 2.
func (q *MPMCPtr) Init(capacity int) {
	(*MPMCIndirect)(q).Init(capacity)
}

// Drain is a no-op. Consumers can drain the queue after producers finish.
func (q *MPMCPtr) Drain() {}

// Enqueue adds an element to the queue.
// Returns ErrWouldBlock if the queue is full.
func (q *MPMCPtr) Enqueue(elem unsafe.Pointer) error {
	return (*MPMCIndirect)(q).Enqueue(uintptr(elem))
}

// Dequeue removes and returns an element from the queue.
// Returns (nil, ErrWouldBlock) if the queue is empty.
func (q *MPMCPtr) Dequeue() (unsafe.Pointer, error) {
	word, err := (*MPMCIndirect)(q).Dequeue()
	return *(*unsafe.Pointer)(unsafe.Pointer(&word)), err
}

// Cap returns the queue capacity.
func (q *MPMCPtr) Cap() int {
	return int(q.capacity)
}
