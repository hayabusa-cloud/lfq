// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

package asm_test

import (
	"reflect"
	"testing"
	"unsafe"

	"code.hybscloud.com/lfq"
	"code.hybscloud.com/lfq/internal/asm"
)

func TestSPSCIndirectLayout(t *testing.T) {
	typ := reflect.TypeOf(lfq.SPSCIndirect{})

	checkOffset := func(name string, want uintptr) {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		if field.Offset != want {
			t.Fatalf("%s offset: got %d, want %d", name, field.Offset, want)
		}
	}

	checkOffset("head", 64)
	checkOffset("cachedTail", 136)
	checkOffset("tail", 208)
	checkOffset("cachedHead", 280)
	checkOffset("buffer", 352)
	checkOffset("mask", 376)

	if typ.Size() != 384 {
		t.Fatalf("SPSCIndirect size: got %d, want 384", typ.Size())
	}
}

func TestSPSCEnqueueDequeueAsm(t *testing.T) {
	q := lfq.NewSPSCIndirect(8)

	// Test enqueue
	for i := range 8 {
		ret := asm.SPSCEnqueue(uintptr(unsafe.Pointer(q)), uintptr(i))
		if ret != 0 {
			t.Fatalf("Enqueue(%d): got %d, want 0", i, ret)
		}
	}

	// Queue should be full
	ret := asm.SPSCEnqueue(uintptr(unsafe.Pointer(q)), 99)
	if ret != 1 {
		t.Fatalf("Enqueue on full: got %d, want 1", ret)
	}

	// Test dequeue
	for i := range 8 {
		elem, err := asm.SPSCDequeue(uintptr(unsafe.Pointer(q)))
		if err != 0 {
			t.Fatalf("Dequeue: got err %d, want 0", err)
		}
		if elem != uintptr(i) {
			t.Fatalf("Dequeue: got %d, want %d", elem, i)
		}
	}

	// Queue should be empty
	_, err := asm.SPSCDequeue(uintptr(unsafe.Pointer(q)))
	if err != 1 {
		t.Fatalf("Dequeue on empty: got err %d, want 1", err)
	}
}

func TestSPSCWraparoundAsm(t *testing.T) {
	q := lfq.NewSPSCIndirect(4)

	// Multiple rounds of fill/drain
	for round := range 100 {
		// Fill
		for i := range 4 {
			v := uintptr(round*100 + i)
			ret := asm.SPSCEnqueue(uintptr(unsafe.Pointer(q)), v)
			if ret != 0 {
				t.Fatalf("round %d: Enqueue(%d): got %d", round, i, ret)
			}
		}

		// Drain
		for i := range 4 {
			elem, err := asm.SPSCDequeue(uintptr(unsafe.Pointer(q)))
			if err != 0 {
				t.Fatalf("round %d: Dequeue: got err %d", round, err)
			}
			expected := uintptr(round*100 + i)
			if elem != expected {
				t.Fatalf("round %d: got %d, want %d", round, elem, expected)
			}
		}
	}
}
