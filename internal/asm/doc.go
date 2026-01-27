// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package asm provides architecture-specific helpers for hot paths.
//
// Layout contract:
// The SPSCIndirect offsets used by assembly must match the Go struct
// layout. The expected offsets are verified by tests on supported
// architectures.
package asm
