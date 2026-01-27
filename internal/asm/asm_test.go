// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build amd64

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
	qptr := uintptr(unsafe.Pointer(q))

	// Test enqueue
	for i := range 8 {
		ret := asm.SPSCEnqueue(qptr, uintptr(i))
		if ret != 0 {
			t.Fatalf("Enqueue(%d): got %d, want 0", i, ret)
		}
	}

	// Queue should be full
	ret := asm.SPSCEnqueue(qptr, 99)
	if ret != 1 {
		t.Fatalf("Enqueue on full: got %d, want 1", ret)
	}

	// Test dequeue
	for i := range 8 {
		elem, err := asm.SPSCDequeue(qptr)
		if err != 0 {
			t.Fatalf("Dequeue: got err %d, want 0", err)
		}
		if elem != uintptr(i) {
			t.Fatalf("Dequeue: got %d, want %d", elem, i)
		}
	}

	// Queue should be empty
	_, err := asm.SPSCDequeue(qptr)
	if err != 1 {
		t.Fatalf("Dequeue on empty: got err %d, want 1", err)
	}
}

func TestSPSCWraparoundAsm(t *testing.T) {
	q := lfq.NewSPSCIndirect(4)
	qptr := uintptr(unsafe.Pointer(q))

	// Multiple rounds of fill/drain
	for round := range 100 {
		// Fill
		for i := range 4 {
			v := uintptr(round*100 + i)
			ret := asm.SPSCEnqueue(qptr, v)
			if ret != 0 {
				t.Fatalf("round %d: Enqueue(%d): got %d", round, i, ret)
			}
		}

		// Drain
		for i := range 4 {
			elem, err := asm.SPSCDequeue(qptr)
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

func TestSPSCAsmMatchesGo(t *testing.T) {
	// Verify assembly produces same results as Go implementation
	qAsm := lfq.NewSPSCIndirect(16)
	qGo := lfq.NewSPSCIndirect(16)
	qAsmPtr := uintptr(unsafe.Pointer(qAsm))

	// Interleaved enqueue/dequeue pattern
	for i := range 1000 {
		v := uintptr(i)

		// Enqueue to both
		retAsm := asm.SPSCEnqueue(qAsmPtr, v)
		errGo := qGo.Enqueue(v)

		if (retAsm == 0) != (errGo == nil) {
			t.Fatalf("Enqueue mismatch at %d: asm=%d, go=%v", i, retAsm, errGo)
		}

		// Dequeue from both every 3rd iteration
		if i%3 == 0 {
			elemAsm, errAsm := asm.SPSCDequeue(qAsmPtr)
			elemGo, errGoDeq := qGo.Dequeue()

			if (errAsm == 0) != (errGoDeq == nil) {
				t.Fatalf("Dequeue err mismatch at %d: asm=%d, go=%v", i, errAsm, errGoDeq)
			}
			if errAsm == 0 && elemAsm != elemGo {
				t.Fatalf("Dequeue elem mismatch at %d: asm=%d, go=%d", i, elemAsm, elemGo)
			}
		}
	}
}

// Benchmark comparison: Assembly vs Go implementation

func BenchmarkSPSCIndirectGoEnqueueDequeue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkSPSCIndirectAsmEnqueueDequeue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)
	qptr := uintptr(unsafe.Pointer(q))

	b.ResetTimer()
	for i := range b.N {
		asm.SPSCEnqueue(qptr, uintptr(i))
		asm.SPSCDequeue(qptr)
	}
}

func BenchmarkSPSCIndirectGoEnqueue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		if i%1024 == 1023 {
			// Drain to avoid full
			for range 1024 {
				q.Dequeue()
			}
		}
	}
}

func BenchmarkSPSCIndirectAsmEnqueue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)
	qptr := uintptr(unsafe.Pointer(q))

	b.ResetTimer()
	for i := range b.N {
		asm.SPSCEnqueue(qptr, uintptr(i))
		if i%1024 == 1023 {
			// Drain to avoid full
			for range 1024 {
				asm.SPSCDequeue(qptr)
			}
		}
	}
}
