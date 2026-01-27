// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build riscv64

package asm

// SPSCEnqueue performs an optimized SPSC enqueue operation.
// It combines the cached index check, buffer store, and tail update
// in a tight instruction sequence without Go runtime preemption points.
//
// Parameters (passed via registers per Go ABI):
//   - q: pointer to SPSCIndirect struct
//   - elem: the uintptr value to enqueue
//
// Returns:
//   - 0 on success
//   - 1 if queue is full (ErrWouldBlock)
//
// The struct layout is (verified via reflection):
//   - offset 0:   pad (64 bytes cache line isolation)
//   - offset 64:  head (atomix.Uint64 - 8 bytes)
//   - offset 72:  pad (64 bytes)
//   - offset 136: cachedTail (8 bytes, non-atomic)
//   - offset 144: pad (64 bytes)
//   - offset 208: tail (atomix.Uint64 - 8 bytes)
//   - offset 216: pad (64 bytes)
//   - offset 280: cachedHead (8 bytes, non-atomic)
//   - offset 288: pad (64 bytes)
//   - offset 352: buffer (slice header: ptr, len, cap = 24 bytes)
//   - offset 376: mask (8 bytes)
//   - Total size: 384 bytes
//
// Memory ordering: Uses FENCE instructions for acquire/release semantics.
// FENCE R,RW provides load-acquire, FENCE RW,W provides store-release.
//
//go:nosplit
//go:noescape
func SPSCEnqueue(q uintptr, elem uintptr) int

// SPSCDequeue performs an optimized SPSC dequeue operation.
// Combines cached index check, buffer load, and head update.
//
// Returns:
//   - elem: the dequeued value (valid only if err == 0)
//   - err:  0 on success, 1 if queue is empty (ErrWouldBlock)
//
//go:nosplit
//go:noescape
func SPSCDequeue(q uintptr) (elem uintptr, err int)
