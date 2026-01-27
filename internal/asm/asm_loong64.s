// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build loong64

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
// LoongArch uses DBAR (Data BARrier) instructions for memory ordering:
//   Load-acquire:   load + DBAR 0x14 (LoadLoad+LoadStore)
//   Store-release:  DBAR 0x12 (StoreStore) + store
//
TEXT ·SPSCEnqueue(SB), NOSPLIT, $0-24
	MOVV	q+0(FP), R4          // R4 = q (struct pointer)
	MOVV	elem+8(FP), R5       // R5 = elem (value to enqueue)

	// tail := q.tail.LoadRelaxed()
	MOVV	TAIL_OFF(R4), R6     // R6 = tail

	// Load cachedHead and mask
	MOVV	CACHED_HEAD(R4), R7  // R7 = cachedHead
	MOVV	MASK_OFF(R4), R8     // R8 = mask

	// Fast path: if tail - cachedHead > mask, need to reload head
	// Compare unsigned: (tail - cachedHead) > mask
	SUBV	R7, R6, R9           // R9 = tail - cachedHead
	// SLTU sets R10 = 1 if R8 < R9 (unsigned), else 0
	SGTU	R9, R8, R10          // R10 = 1 if R9 > R8 (unsigned)
	BNE	R10, R0, spsc_enq_slow_loong64 // if R10 != 0, slow path

spsc_enq_store_loong64:
	// idx = tail & mask
	AND	R8, R6, R9           // R9 = tail & mask = idx

	// buffer[idx] = elem
	MOVV	BUFFER_PTR(R4), R10  // R10 = buffer.ptr
	SLLV	$3, R9               // R9 = idx * 8 (sizeof(uintptr))
	ADDV	R10, R9, R10         // R10 = &buffer[idx]
	MOVV	R5, (R10)            // buffer[idx] = elem

	// q.tail.StoreRelease(tail + 1)
	// DBAR ensures prior writes visible before tail update
	DBAR
	ADDV	$1, R6, R6           // tail++
	MOVV	R6, TAIL_OFF(R4)     // store tail

	// Return 0 (success)
	MOVV	R0, ret+16(FP)
	RET

spsc_enq_slow_loong64:
	// Reload actual head: cachedHead = q.head.LoadAcquire()
	MOVV	HEAD_OFF(R4), R7     // R7 = head (fresh load)
	// DBAR ensures load completes before dependent operations
	DBAR
	MOVV	R7, CACHED_HEAD(R4)  // update cachedHead

	// Recheck: if tail - cachedHead > mask, queue is full
	SUBV	R7, R6, R9           // R9 = tail - cachedHead
	SGTU	R9, R8, R10          // R10 = 1 if R9 > R8 (unsigned)
	BEQ	R10, R0, spsc_enq_store_loong64 // if R10 == 0, proceed to store

	// Queue is full
	MOVV	$1, R11
	MOVV	R11, ret+16(FP)
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
	MOVV	q+0(FP), R4          // R4 = q (struct pointer)

	// head := q.head.LoadRelaxed()
	MOVV	HEAD_OFF(R4), R6     // R6 = head

	// Load cachedTail and mask
	MOVV	CACHED_TAIL(R4), R7  // R7 = cachedTail
	MOVV	MASK_OFF(R4), R8     // R8 = mask

	// Fast path: if head >= cachedTail, need to reload tail
	// Compare unsigned: head >= cachedTail means !(head < cachedTail)
	SGTU	R7, R6, R10          // R10 = 1 if R7 > R6 (head < cachedTail)
	BEQ	R10, R0, spsc_deq_slow_loong64 // if R10 == 0, head >= cachedTail

spsc_deq_load_loong64:
	// idx = head & mask
	AND	R8, R6, R9           // R9 = head & mask = idx

	// elem = buffer[idx]
	MOVV	BUFFER_PTR(R4), R10  // R10 = buffer.ptr
	SLLV	$3, R9               // R9 = idx * 8 (sizeof(uintptr))
	ADDV	R10, R9, R10         // R10 = &buffer[idx]
	MOVV	(R10), R5            // R5 = buffer[idx] = elem

	// q.head.StoreRelease(head + 1)
	DBAR
	ADDV	$1, R6, R6           // head++
	MOVV	R6, HEAD_OFF(R4)     // store head

	// Return elem, 0 (success)
	MOVV	R5, elem+8(FP)
	MOVV	R0, err+16(FP)
	RET

spsc_deq_slow_loong64:
	// Reload actual tail: cachedTail = q.tail.LoadAcquire()
	MOVV	TAIL_OFF(R4), R7     // R7 = tail (fresh load)
	DBAR
	MOVV	R7, CACHED_TAIL(R4)  // update cachedTail

	// Recheck: if head >= cachedTail, queue is empty
	// head < cachedTail means proceed
	SGTU	R7, R6, R10          // R10 = 1 if cachedTail > head
	BNE	R10, R0, spsc_deq_load_loong64 // if R10 != 0, proceed to load

	// Queue is empty
	MOVV	R0, elem+8(FP)
	MOVV	$1, R11
	MOVV	R11, err+16(FP)
	RET
