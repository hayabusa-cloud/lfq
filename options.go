// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import "unsafe"

// Options configures queue creation and algorithm selection.
type Options struct {
	// Producer/Consumer constraints (determines queue type)
	singleProducer bool
	singleConsumer bool

	// Performance hints
	compact bool // Effort to save slots

	// Capacity (rounds up to next power of 2)
	capacity int
}

// Builder creates queues with fluent configuration.
//
// Builder provides a fluent API for configuring and creating queues.
// The builder automatically selects the algorithm based on
// producer/consumer constraints and performance hints.
//
// Example:
//
//	// SPSC queue (optimal for single producer/consumer)
//	q := lfq.BuildSPSC[Event](lfq.New(1024).SingleProducer().SingleConsumer())
//
//	// MPMC queue (default, general purpose)
//	q := lfq.BuildMPMC[Request](lfq.New(4096))
//
//	// Compact indirect queue for memory efficiency
//	q := lfq.New(8192).Compact().BuildIndirect()
type Builder struct {
	opts Options
}

// New creates a queue builder with the given capacity.
//
// Capacity rounds up to the next power of 2.
// For example, capacity=4 results in actual capacity=4, capacity=1000 results
// in actual capacity=1024.
//
// Panics if capacity < 2.
//
// Example:
//
//	// Create builder, then configure and build
//	b := lfq.New(1024)
//	q := lfq.BuildSPSC[int](b.SingleProducer().SingleConsumer())
//
//	// Or chain directly
//	q := lfq.BuildMPMC[int](lfq.New(1024))
func New(capacity int) *Builder {
	if capacity < 2 {
		panic("lfq: capacity must be >= 2")
	}
	return &Builder{opts: Options{capacity: capacity}}
}

// SingleProducer declares that only one goroutine will enqueue.
// Enables optimized algorithms for SPSC or SPMC patterns.
func (b *Builder) SingleProducer() *Builder {
	b.opts.singleProducer = true
	return b
}

// SingleConsumer declares that only one goroutine will dequeue.
// Enables optimized algorithms for SPSC or MPSC patterns.
func (b *Builder) SingleConsumer() *Builder {
	b.opts.singleConsumer = true
	return b
}

// Compact selects CAS-based algorithms with n physical slots instead of
// FAA-based algorithms with 2n slots.
//
// Trade-off: Half memory usage, reduced scalability under high contention.
//
// SPSC already uses n slots (Lamport ring buffer) and ignores Compact().
func (b *Builder) Compact() *Builder {
	b.opts.compact = true
	return b
}

// Build creates a Queue[T] with automatic algorithm selection.
//
// Algorithm selection:
//
//	SingleProducer + SingleConsumer → SPSC (Lamport ring buffer)
//	SingleProducer only             → SPMC (FAA default, CAS if Compact)
//	SingleConsumer only             → MPSC (FAA default, CAS if Compact)
//	Neither                         → MPMC (FAA default, CAS if Compact)
//
// Default: FAA-based algorithms with 2n physical slots (better scalability).
// Compact(): CAS-based algorithms with n slots (half memory footprint).
//
// For type-safe returns with concrete types, use:
//   - BuildSPSC[T](b) → *SPSC[T]
//   - BuildMPSC[T](b) → *MPSC[T] (or *MPSCSeq[T] if Compact)
//   - BuildSPMC[T](b) → *SPMC[T] (or *SPMCSeq[T] if Compact)
//   - BuildMPMC[T](b) → *MPMC[T] (or *MPMCSeq[T] if Compact)
func Build[T any](b *Builder) Queue[T] {
	switch {
	case b.opts.singleProducer && b.opts.singleConsumer:
		return NewSPSC[T](b.opts.capacity)
	case b.opts.singleProducer && b.opts.compact:
		return NewSPMCSeq[T](b.opts.capacity)
	case b.opts.singleProducer:
		return NewSPMC[T](b.opts.capacity)
	case b.opts.singleConsumer && b.opts.compact:
		return NewMPSCSeq[T](b.opts.capacity)
	case b.opts.singleConsumer:
		return NewMPSC[T](b.opts.capacity)
	case b.opts.compact:
		return NewMPMCSeq[T](b.opts.capacity)
	default:
		return NewMPMC[T](b.opts.capacity)
	}
}

// BuildSPSC creates an SPSC queue with compile-time type safety.
// Panics if builder is not configured with SingleProducer().SingleConsumer().
func BuildSPSC[T any](b *Builder) *SPSC[T] {
	if !b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildSPSC requires SingleProducer().SingleConsumer()")
	}
	return NewSPSC[T](b.opts.capacity)
}

// BuildMPSC creates an MPSC queue with compile-time type safety.
// Panics if builder is not configured with SingleConsumer() only.
func BuildMPSC[T any](b *Builder) Queue[T] {
	if b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildMPSC requires SingleConsumer() without SingleProducer()")
	}
	if b.opts.compact {
		return NewMPSCSeq[T](b.opts.capacity)
	}
	return NewMPSC[T](b.opts.capacity)
}

// BuildSPMC creates an SPMC queue with compile-time type safety.
// Panics if builder is not configured with SingleProducer() only.
func BuildSPMC[T any](b *Builder) Queue[T] {
	if !b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildSPMC requires SingleProducer() without SingleConsumer()")
	}
	if b.opts.compact {
		return NewSPMCSeq[T](b.opts.capacity)
	}
	return NewSPMC[T](b.opts.capacity)
}

// BuildMPMC creates an MPMC queue with compile-time type safety.
// Panics if builder has any constraints set.
func BuildMPMC[T any](b *Builder) Queue[T] {
	if b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildMPMC requires no constraints")
	}
	if b.opts.compact {
		return NewMPMCSeq[T](b.opts.capacity)
	}
	return NewMPMC[T](b.opts.capacity)
}

// BuildIndirect creates a QueueIndirect for uintptr values.
//
// Algorithm selection:
//   - SPSC (SingleProducer + SingleConsumer) → Lamport ring buffer
//   - Compact() → CAS-based algorithms (n slots, values limited to 63 bits)
//   - Default → FAA-based algorithms (2n slots)
func (b *Builder) BuildIndirect() QueueIndirect {
	switch {
	case b.opts.singleProducer && b.opts.singleConsumer:
		return NewSPSCIndirect(b.opts.capacity)
	case b.opts.compact && b.opts.singleProducer:
		return NewSPMCCompactIndirect(b.opts.capacity)
	case b.opts.compact && b.opts.singleConsumer:
		return NewMPSCCompactIndirect(b.opts.capacity)
	case b.opts.compact:
		return NewMPMCCompactIndirect(b.opts.capacity)
	case b.opts.singleProducer:
		return NewSPMCIndirect(b.opts.capacity)
	case b.opts.singleConsumer:
		return NewMPSCIndirect(b.opts.capacity)
	default:
		return NewMPMCIndirect(b.opts.capacity)
	}
}

// BuildIndirectSPSC creates an SPSC queue for uintptr values.
func (b *Builder) BuildIndirectSPSC() *SPSCIndirect {
	if !b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildIndirectSPSC requires SingleProducer().SingleConsumer()")
	}
	return NewSPSCIndirect(b.opts.capacity)
}

// BuildIndirectMPSC creates an MPSC queue for uintptr values.
// Panics if builder is not configured with SingleConsumer() only.
func (b *Builder) BuildIndirectMPSC() QueueIndirect {
	if b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildIndirectMPSC requires SingleConsumer() without SingleProducer()")
	}
	if b.opts.compact {
		return NewMPSCCompactIndirect(b.opts.capacity)
	}
	return NewMPSCIndirect(b.opts.capacity)
}

// BuildIndirectSPMC creates an SPMC queue for uintptr values.
// Panics if builder is not configured with SingleProducer() only.
func (b *Builder) BuildIndirectSPMC() QueueIndirect {
	if !b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildIndirectSPMC requires SingleProducer() without SingleConsumer()")
	}
	if b.opts.compact {
		return NewSPMCCompactIndirect(b.opts.capacity)
	}
	return NewSPMCIndirect(b.opts.capacity)
}

// BuildIndirectMPMC creates an MPMC queue for uintptr values.
// Panics if builder has any constraints set.
func (b *Builder) BuildIndirectMPMC() QueueIndirect {
	if b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildIndirectMPMC requires no constraints")
	}
	if b.opts.compact {
		return NewMPMCCompactIndirect(b.opts.capacity)
	}
	return NewMPMCIndirect(b.opts.capacity)
}

// BuildPtr creates a QueuePtr for unsafe.Pointer values.
//
// Algorithm selection:
//  1. SPSC (SingleProducer + SingleConsumer) → Lamport ring buffer
//  2. Other configurations → FAA default (2n slots), CAS if Compact (n slots)
//
// Default: FAA-based algorithms with 2n physical slots (better scalability).
// Compact(): CAS-based algorithms with n slots (half memory footprint).
func (b *Builder) BuildPtr() QueuePtr {
	switch {
	case b.opts.singleProducer && b.opts.singleConsumer:
		return NewSPSCPtr(b.opts.capacity)
	case b.opts.singleProducer && b.opts.compact:
		return NewSPMCPtrSeq(b.opts.capacity)
	case b.opts.singleProducer:
		return NewSPMCPtr(b.opts.capacity)
	case b.opts.singleConsumer && b.opts.compact:
		return NewMPSCPtrSeq(b.opts.capacity)
	case b.opts.singleConsumer:
		return NewMPSCPtr(b.opts.capacity)
	case b.opts.compact:
		return NewMPMCPtrSeq(b.opts.capacity)
	default:
		return NewMPMCPtr(b.opts.capacity)
	}
}

// BuildPtrSPSC creates an SPSC queue for unsafe.Pointer values.
// Panics if builder is not configured with SingleProducer().SingleConsumer().
func (b *Builder) BuildPtrSPSC() *SPSCPtr {
	if !b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildPtrSPSC requires SingleProducer().SingleConsumer()")
	}
	return NewSPSCPtr(b.opts.capacity)
}

// BuildPtrMPSC creates an MPSC queue for unsafe.Pointer values.
// Panics if builder is not configured with SingleConsumer() only.
func (b *Builder) BuildPtrMPSC() QueuePtr {
	if b.opts.singleProducer || !b.opts.singleConsumer {
		panic("lfq: BuildPtrMPSC requires SingleConsumer() without SingleProducer()")
	}
	if b.opts.compact {
		return NewMPSCPtrSeq(b.opts.capacity)
	}
	return NewMPSCPtr(b.opts.capacity)
}

// BuildPtrSPMC creates an SPMC queue for unsafe.Pointer values.
// Panics if builder is not configured with SingleProducer() only.
func (b *Builder) BuildPtrSPMC() QueuePtr {
	if !b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildPtrSPMC requires SingleProducer() without SingleConsumer()")
	}
	if b.opts.compact {
		return NewSPMCPtrSeq(b.opts.capacity)
	}
	return NewSPMCPtr(b.opts.capacity)
}

// BuildPtrMPMC creates an MPMC queue for unsafe.Pointer values.
// Panics if builder has any constraints set.
func (b *Builder) BuildPtrMPMC() QueuePtr {
	if b.opts.singleProducer || b.opts.singleConsumer {
		panic("lfq: BuildPtrMPMC requires no constraints")
	}
	if b.opts.compact {
		return NewMPMCPtrSeq(b.opts.capacity)
	}
	return NewMPMCPtr(b.opts.capacity)
}

// roundToPow2 rounds n up to the next power of 2.
func roundToPow2(n int) int {
	if n < 2 {
		return 2
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}

// ptrSize is the size of a pointer in bytes.
const ptrSize = int(unsafe.Sizeof(uintptr(0)))

// pad is cache line padding to prevent false sharing.
type pad [64]byte

// padShort is padding to fill cache line after 8-byte field.
type padShort [64 - 8]byte

// padPtr is padding to fill cache line after pointer-sized field.
type padPtr [64 - ptrSize]byte
