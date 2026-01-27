// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build arm64

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
// ARM64 uses LDAR/STLR for acquire/release semantics.
// Producer only writes tail, consumer only writes head.
//
TEXT ·SPSCEnqueue(SB), NOSPLIT, $0-24
	MOVD	q+0(FP), R0          // R0 = q (struct pointer)
	MOVD	elem+8(FP), R1       // R1 = elem (value to enqueue)

	// tail := q.tail.LoadRelaxed()
	MOVD	TAIL_OFF(R0), R2     // R2 = tail

	// Load cachedHead and mask
	MOVD	CACHED_HEAD(R0), R3  // R3 = cachedHead
	MOVD	MASK_OFF(R0), R4     // R4 = mask

	// Fast path: if tail - cachedHead > mask, need to reload head
	SUB	R3, R2, R5           // R5 = tail - cachedHead
	CMP	R4, R5
	BHI	spsc_enq_slow_arm64  // if tail - cachedHead > mask, slow path

spsc_enq_store_arm64:
	// idx = tail & mask
	AND	R4, R2, R5           // R5 = tail & mask = idx

	// buffer[idx] = elem
	MOVD	BUFFER_PTR(R0), R6   // R6 = buffer.ptr
	LSL	$3, R5               // R5 = idx * 8 (sizeof(uintptr))
	ADD	R5, R6, R6           // R6 = &buffer[idx]
	MOVD	R1, (R6)             // buffer[idx] = elem

	// q.tail.StoreRelease(tail + 1)
	ADD	$1, R2, R2           // tail++
	ADD	$TAIL_OFF, R0, R7    // R7 = &q.tail
	STLR	R2, (R7)             // store-release tail

	// Return 0 (success)
	MOVD	ZR, ret+16(FP)
	RET

spsc_enq_slow_arm64:
	// Reload actual head: cachedHead = q.head.LoadAcquire()
	ADD	$HEAD_OFF, R0, R7    // R7 = &q.head
	LDAR	(R7), R3             // R3 = head (load-acquire)
	MOVD	R3, CACHED_HEAD(R0)  // update cachedHead

	// Recheck: if tail - cachedHead > mask, queue is full
	SUB	R3, R2, R5           // R5 = tail - cachedHead
	CMP	R4, R5
	BLS	spsc_enq_store_arm64 // if <= mask, proceed to store

	// Queue is full
	MOVD	$1, R8
	MOVD	R8, ret+16(FP)
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
	MOVD	q+0(FP), R0          // R0 = q (struct pointer)

	// head := q.head.LoadRelaxed()
	MOVD	HEAD_OFF(R0), R2     // R2 = head

	// Load cachedTail and mask
	MOVD	CACHED_TAIL(R0), R3  // R3 = cachedTail
	MOVD	MASK_OFF(R0), R4     // R4 = mask

	// Fast path: if head >= cachedTail, need to reload tail
	CMP	R3, R2               // compare head with cachedTail
	BHS	spsc_deq_slow_arm64  // if head >= cachedTail, slow path

spsc_deq_load_arm64:
	// idx = head & mask
	AND	R4, R2, R5           // R5 = head & mask = idx

	// elem = buffer[idx]
	MOVD	BUFFER_PTR(R0), R6   // R6 = buffer.ptr
	LSL	$3, R5               // R5 = idx * 8 (sizeof(uintptr))
	ADD	R5, R6, R6           // R6 = &buffer[idx]
	MOVD	(R6), R1             // R1 = buffer[idx] = elem

	// q.head.StoreRelease(head + 1)
	ADD	$1, R2, R2           // head++
	ADD	$HEAD_OFF, R0, R7    // R7 = &q.head
	STLR	R2, (R7)             // store-release head

	// Return elem, 0 (success)
	MOVD	R1, elem+8(FP)
	MOVD	ZR, err+16(FP)
	RET

spsc_deq_slow_arm64:
	// Reload actual tail: cachedTail = q.tail.LoadAcquire()
	ADD	$TAIL_OFF, R0, R7    // R7 = &q.tail
	LDAR	(R7), R3             // R3 = tail (load-acquire)
	MOVD	R3, CACHED_TAIL(R0)  // update cachedTail

	// Recheck: if head >= cachedTail, queue is empty
	CMP	R3, R2               // compare head with cachedTail
	BLO	spsc_deq_load_arm64  // if head < cachedTail, proceed to load

	// Queue is empty
	MOVD	ZR, elem+8(FP)
	MOVD	$1, R8
	MOVD	R8, err+16(FP)
	RET
