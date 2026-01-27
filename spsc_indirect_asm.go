// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64 || riscv64 || loong64

package lfq

import (
	"unsafe"

	"code.hybscloud.com/lfq/internal/asm"
)

// Enqueue adds an element (producer only).
func (q *SPSCIndirect) Enqueue(elem uintptr) error {
	if asm.SPSCEnqueue(uintptr(unsafe.Pointer(q)), elem) != 0 {
		return ErrWouldBlock
	}
	return nil
}

// Dequeue removes and returns an element (consumer only).
func (q *SPSCIndirect) Dequeue() (uintptr, error) {
	elem, err := asm.SPSCDequeue(uintptr(unsafe.Pointer(q)))
	if err != 0 {
		return 0, ErrWouldBlock
	}
	return elem, nil
}
