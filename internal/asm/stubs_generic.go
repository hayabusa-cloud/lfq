// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build !amd64 && !arm64 && !riscv64 && !loong64

package asm

// SPSCEnqueue is a stub for unsupported architectures.
// Returns 1 (ErrWouldBlock) to indicate assembly is not available.
func SPSCEnqueue(q uintptr, elem uintptr) int {
	return 1 // Always fail - use Go implementation
}

// SPSCDequeue is a stub for unsupported architectures.
// Returns (0, 1) to indicate assembly is not available.
func SPSCDequeue(q uintptr) (elem uintptr, err int) {
	return 0, 1 // Always fail - use Go implementation
}
