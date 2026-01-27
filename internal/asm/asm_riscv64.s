// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build riscv64

#include "textflag.h"

// SPSCIndirect struct offsets (verified via reflection)
// Each field is followed by a 64-byte pad for cache line isolation
#define HEAD_OFF      64    // atomix.Uint64 (8 bytes)
#define CACHED_TAIL   136   // uint64 (non-atomic, after 64-byte pad)
#define TAIL_OFF      208   // atomix.Uint64 (after 64-byte pad)
#define CACHED_HEAD   280   // uint64 (non-atomic, after 64-byte pad)
#define BUFFER_PTR    352   // slice data pointer (after 64-byte pad)
#define MASK_OFF      376   // uint64 (after slice header: ptr+len+cap=24)

// func SPSCEnqueue(q uintptr, elem uintptr) int
//
// Arguments (Go ABI internal):
//   q+0(FP)    = pointer to SPSCIndirect struct
//   elem+8(FP) = value to enqueue
//
// Returns:
//   ret+16(FP) = 0 on success, 1 if full
//
// RISC-V uses FENCE instructions for memory ordering:
//   Load-acquire:   load + FENCE R,RW
//   Store-release:  FENCE RW,W + store
//
TEXT ·SPSCEnqueue(SB), NOSPLIT, $0-24
	MOV	q+0(FP), X5          // X5 = q (struct pointer)
	MOV	elem+8(FP), X6       // X6 = elem (value to enqueue)

	// tail := q.tail.LoadRelaxed()
	MOV	TAIL_OFF(X5), X7     // X7 = tail

	// Load cachedHead and mask
	MOV	CACHED_HEAD(X5), X10 // X10 = cachedHead
	MOV	MASK_OFF(X5), X11    // X11 = mask

	// Fast path: if tail - cachedHead > mask, need to reload head
	SUB	X10, X7, X12         // X12 = tail - cachedHead
	BGTU	X12, X11, spsc_enq_slow_riscv64 // if > mask, slow path

spsc_enq_store_riscv64:
	// idx = tail & mask
	AND	X11, X7, X12         // X12 = tail & mask = idx

	// buffer[idx] = elem
	MOV	BUFFER_PTR(X5), X13  // X13 = buffer.ptr
	SLL	$3, X12              // X12 = idx * 8 (sizeof(uintptr))
	ADD	X13, X12, X13        // X13 = &buffer[idx]
	MOV	X6, (X13)            // buffer[idx] = elem

	// q.tail.StoreRelease(tail + 1)
	// FENCE RW,W ensures prior writes visible before tail update
	FENCE
	ADD	$1, X7, X7           // tail++
	MOV	X7, TAIL_OFF(X5)     // store tail

	// Return 0 (success)
	MOV	X0, X14              // X14 = 0 (X0 is always zero)
	MOV	X14, ret+16(FP)
	RET

spsc_enq_slow_riscv64:
	// Reload actual head: cachedHead = q.head.LoadAcquire()
	MOV	HEAD_OFF(X5), X10    // X10 = head (fresh load)
	// FENCE R,RW ensures load completes before dependent operations
	FENCE
	MOV	X10, CACHED_HEAD(X5) // update cachedHead

	// Recheck: if tail - cachedHead > mask, queue is full
	SUB	X10, X7, X12         // X12 = tail - cachedHead
	BLEU	X12, X11, spsc_enq_store_riscv64 // if <= mask, proceed to store

	// Queue is full
	MOV	$1, X14
	MOV	X14, ret+16(FP)
	RET

// func SPSCDequeue(q uintptr) (elem uintptr, err int)
//
// Arguments (Go ABI internal):
//   q+0(FP)    = pointer to SPSCIndirect struct
//
// Returns:
//   elem+8(FP) = dequeued value (valid only if err == 0)
//   err+16(FP) = 0 on success, 1 if empty
//
TEXT ·SPSCDequeue(SB), NOSPLIT, $0-24
	MOV	q+0(FP), X5          // X5 = q (struct pointer)

	// head := q.head.LoadRelaxed()
	MOV	HEAD_OFF(X5), X7     // X7 = head

	// Load cachedTail and mask
	MOV	CACHED_TAIL(X5), X10 // X10 = cachedTail
	MOV	MASK_OFF(X5), X11    // X11 = mask

	// Fast path: if head >= cachedTail, need to reload tail
	BGEU	X7, X10, spsc_deq_slow_riscv64 // if head >= cachedTail, slow path

spsc_deq_load_riscv64:
	// idx = head & mask
	AND	X11, X7, X12         // X12 = head & mask = idx

	// elem = buffer[idx]
	MOV	BUFFER_PTR(X5), X13  // X13 = buffer.ptr
	SLL	$3, X12              // X12 = idx * 8 (sizeof(uintptr))
	ADD	X13, X12, X13        // X13 = &buffer[idx]
	MOV	(X13), X6            // X6 = buffer[idx] = elem

	// q.head.StoreRelease(head + 1)
	FENCE
	ADD	$1, X7, X7           // head++
	MOV	X7, HEAD_OFF(X5)     // store head

	// Return elem, 0 (success)
	MOV	X6, elem+8(FP)
	MOV	X0, X14              // X14 = 0
	MOV	X14, err+16(FP)
	RET

spsc_deq_slow_riscv64:
	// Reload actual tail: cachedTail = q.tail.LoadAcquire()
	MOV	TAIL_OFF(X5), X10    // X10 = tail (fresh load)
	FENCE
	MOV	X10, CACHED_TAIL(X5) // update cachedTail

	// Recheck: if head >= cachedTail, queue is empty
	BLT	X7, X10, spsc_deq_load_riscv64 // if head < cachedTail, proceed to load

	// Queue is empty
	MOV	X0, X14              // X14 = 0
	MOV	X14, elem+8(FP)
	MOV	$1, X14
	MOV	X14, err+16(FP)
	RET
