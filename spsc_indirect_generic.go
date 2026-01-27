// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build !amd64 && !arm64 && !riscv64 && !loong64

package lfq

import "unsafe"

// Enqueue adds an element (producer only).
func (q *SPSCIndirect) Enqueue(elem uintptr) error {
	tail := q.tail.LoadRelaxed()

	if tail-q.cachedHead > q.mask {
		q.cachedHead = q.head.LoadAcquire()
		if tail-q.cachedHead > q.mask {
			return ErrWouldBlock
		}
	}

	// Bounds check eliminated: tail&mask is always < len(buffer)
	// because mask = len(buffer)-1 and x&mask <= mask
	*(*uintptr)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(q.buffer)), int(tail&q.mask)*ptrSize)) = elem
	q.tail.StoreRelease(tail + 1)
	return nil
}

// Dequeue removes and returns an element (consumer only).
func (q *SPSCIndirect) Dequeue() (uintptr, error) {
	head := q.head.LoadRelaxed()

	if head >= q.cachedTail {
		q.cachedTail = q.tail.LoadAcquire()
		if head >= q.cachedTail {
			return 0, ErrWouldBlock
		}
	}

	// Bounds check eliminated: head&mask is always < len(buffer)
	elem := *(*uintptr)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(q.buffer)), int(head&q.mask)*ptrSize))
	q.head.StoreRelease(head + 1)
	return elem, nil
}
