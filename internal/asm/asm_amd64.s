// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build amd64

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
// On x86-64 TSO: MOV provides sufficient ordering for SPSC.
// Producer only writes tail, consumer only writes head.
// Cross-core reads use plain MOV (TSO guarantees visibility).
//
TEXT ·SPSCEnqueue(SB), NOSPLIT, $0-24
    MOVQ    q+0(FP), DI          // DI = q (struct pointer)
    MOVQ    elem+8(FP), SI       // SI = elem (value to enqueue)

    // tail := q.tail.LoadRelaxed()
    MOVQ    TAIL_OFF(DI), AX     // AX = tail

    // Load cachedHead and mask
    MOVQ    CACHED_HEAD(DI), BX  // BX = cachedHead
    MOVQ    MASK_OFF(DI), CX     // CX = mask

    // Fast path: if tail - cachedHead > mask, need to reload head
    MOVQ    AX, DX               // DX = tail
    SUBQ    BX, DX               // DX = tail - cachedHead
    CMPQ    DX, CX               // compare with mask
    JA      spsc_enq_slow        // if tail - cachedHead > mask, slow path

spsc_enq_store:
    // idx = tail & mask (already in AX and CX)
    MOVQ    AX, DX               // DX = tail
    ANDQ    CX, DX               // DX = tail & mask = idx

    // buffer[idx] = elem
    MOVQ    BUFFER_PTR(DI), R8   // R8 = buffer.ptr
    SHLQ    $3, DX               // DX = idx * 8 (sizeof(uintptr))
    ADDQ    DX, R8               // R8 = &buffer[idx]
    MOVQ    SI, (R8)             // buffer[idx] = elem

    // q.tail.StoreRelease(tail + 1)
    // On x86-64 TSO, plain MOV provides release semantics
    INCQ    AX                   // tail++
    MOVQ    AX, TAIL_OFF(DI)     // store tail

    // Return 0 (success)
    MOVQ    $0, ret+16(FP)
    RET

spsc_enq_slow:
    // Reload actual head: cachedHead = q.head.LoadAcquire()
    // On x86-64 TSO, plain MOV provides acquire semantics
    MOVQ    HEAD_OFF(DI), BX     // BX = head (fresh load)
    MOVQ    BX, CACHED_HEAD(DI)  // update cachedHead

    // Recheck: if tail - cachedHead > mask, queue is full
    MOVQ    AX, DX               // DX = tail
    SUBQ    BX, DX               // DX = tail - cachedHead
    CMPQ    DX, CX               // compare with mask
    JBE     spsc_enq_store       // if <= mask, proceed to store

    // Queue is full
    MOVQ    $1, ret+16(FP)
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
    MOVQ    q+0(FP), DI          // DI = q (struct pointer)

    // head := q.head.LoadRelaxed()
    MOVQ    HEAD_OFF(DI), AX     // AX = head

    // Load cachedTail and mask
    MOVQ    CACHED_TAIL(DI), BX  // BX = cachedTail
    MOVQ    MASK_OFF(DI), CX     // CX = mask

    // Fast path: if head >= cachedTail, need to reload tail
    CMPQ    AX, BX               // compare head with cachedTail
    JAE     spsc_deq_slow        // if head >= cachedTail, slow path

spsc_deq_load:
    // idx = head & mask
    MOVQ    AX, DX               // DX = head
    ANDQ    CX, DX               // DX = head & mask = idx

    // elem = buffer[idx]
    MOVQ    BUFFER_PTR(DI), R8   // R8 = buffer.ptr
    SHLQ    $3, DX               // DX = idx * 8 (sizeof(uintptr))
    ADDQ    DX, R8               // R8 = &buffer[idx]
    MOVQ    (R8), SI             // SI = buffer[idx] = elem

    // q.head.StoreRelease(head + 1)
    INCQ    AX                   // head++
    MOVQ    AX, HEAD_OFF(DI)     // store head

    // Return elem, 0 (success)
    MOVQ    SI, elem+8(FP)
    MOVQ    $0, err+16(FP)
    RET

spsc_deq_slow:
    // Reload actual tail: cachedTail = q.tail.LoadAcquire()
    MOVQ    TAIL_OFF(DI), BX     // BX = tail (fresh load)
    MOVQ    BX, CACHED_TAIL(DI)  // update cachedTail

    // Recheck: if head >= cachedTail, queue is empty
    CMPQ    AX, BX               // compare head with cachedTail
    JB      spsc_deq_load        // if head < cachedTail, proceed to load

    // Queue is empty
    MOVQ    $0, elem+8(FP)
    MOVQ    $1, err+16(FP)
    RET
