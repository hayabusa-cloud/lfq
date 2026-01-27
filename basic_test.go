// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"testing"
	"unsafe"

	"code.hybscloud.com/lfq"
)

// =============================================================================
// Generic Queues - Basic Operations
// =============================================================================

// TestSPSCBasic tests basic SPSC (Single Producer, Single Consumer) operations.
// SPSC provides wait-free operations for both enqueue and dequeue.
func TestSPSCBasic(t *testing.T) {
	q := lfq.NewSPSC[int](3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	// Enqueue to capacity
	for i := range 4 {
		v := i + 100
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	// Full queue returns ErrWouldBlock
	v := 999
	if err := q.Enqueue(&v); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Dequeue in FIFO order
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	// Empty queue returns ErrWouldBlock
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestMPSCBasic tests basic MPSC (Multiple Producer, Single Consumer) operations.
// MPSC provides lock-free enqueue and wait-free dequeue.
func TestMPSCBasic(t *testing.T) {
	q := lfq.NewMPSC[int](3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	for i := range 4 {
		v := i + 100
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	v := 999
	if err := q.Enqueue(&v); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestSPMCBasic tests basic SPMC (Single Producer, Multiple Consumer) operations.
// SPMC provides wait-free enqueue and lock-free dequeue.
func TestSPMCBasic(t *testing.T) {
	q := lfq.NewSPMC[int](3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	for i := range 4 {
		v := i + 100
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	v := 999
	if err := q.Enqueue(&v); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestMPMCBasic tests basic MPMC (Multiple Producer, Multiple Consumer) operations.
// MPMC provides lock-free operations for both enqueue and dequeue.
func TestMPMCBasic(t *testing.T) {
	q := lfq.NewMPMC[int](3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	for i := range 4 {
		v := i + 100
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	v := 999
	if err := q.Enqueue(&v); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// =============================================================================
// Compact Queues - Basic Operations (63-bit values)
// =============================================================================

// TestMPMCCompactBasic tests basic MPMC Compact operations.
// Compact queues store 63-bit values packed with a round marker for ABA safety.
func TestMPMCCompactBasic(t *testing.T) {
	q := lfq.NewMPMCCompactIndirect(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestMPSCCompactBasic tests basic MPSC Compact operations.
func TestMPSCCompactBasic(t *testing.T) {
	q := lfq.NewMPSCCompactIndirect(4)

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}
}

// TestSPMCCompactBasic tests basic SPMC Compact operations.
func TestSPMCCompactBasic(t *testing.T) {
	q := lfq.NewSPMCCompactIndirect(4)

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}
}

// =============================================================================
// 128-bit DWCAS Queues - Basic Operations
// =============================================================================

// TestMPMCIndirectBasic tests basic MPMC Indirect (128-bit DWCAS) operations.
func TestMPMCIndirectBasic(t *testing.T) {
	q := lfq.NewMPMCIndirect(4)

	// Empty dequeue
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}
}

// TestMPMCPtrBasic tests basic MPMC Ptr (128-bit DWCAS) operations.
func TestMPMCPtrBasic(t *testing.T) {
	q := lfq.NewMPMCPtr(4)

	// Empty dequeue
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	vals := []int{100, 200, 300, 400}
	for i := range vals {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	extra := 999
	if err := q.Enqueue(unsafe.Pointer(&extra)); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Verify FIFO order and pointer identity
	for i := range vals {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if ptr != unsafe.Pointer(&vals[i]) {
			t.Fatalf("Dequeue(%d): pointer mismatch", i)
		}
	}
}

// TestMPSCIndirectBasic tests basic MPSC Indirect (128-bit DWCAS) operations.
func TestMPSCIndirectBasic(t *testing.T) {
	q := lfq.NewMPSCIndirect(4)

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}
}

// TestSPMCIndirectBasic tests basic SPMC Indirect (128-bit DWCAS) operations.
func TestSPMCIndirectBasic(t *testing.T) {
	q := lfq.NewSPMCIndirect(4)

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("empty dequeue: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}
}

// =============================================================================
// Wrap-Around Tests - Verify index wrap-around behavior
// =============================================================================

// TestSPSCWrapAround tests SPSC wrap-around with multiple fill/drain cycles.
func TestSPSCWrapAround(t *testing.T) {
	q := lfq.NewSPSC[int](4)

	for round := range 10 {
		for i := range 4 {
			v := round*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}

		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := round*100 + i
			if val != expected {
				t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
			}
		}
	}
}

// TestMPMCWrapAround tests MPMC wrap-around with multiple fill/drain cycles.
func TestMPMCWrapAround(t *testing.T) {
	q := lfq.NewMPMC[int](4)

	for round := range 10 {
		for i := range 4 {
			v := round*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}

		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := round*100 + i
			if val != expected {
				t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
			}
		}
	}
}

// TestCompactWrapAround tests Compact queue wrap-around behavior.
func TestCompactWrapAround(t *testing.T) {
	q := lfq.NewMPMCCompactIndirect(4)

	for round := range 10 {
		for i := range 4 {
			val := uintptr(round*100 + i)
			if err := q.Enqueue(val); err != nil {
				t.Fatalf("round %d enqueue %d: %v", round, i, err)
			}
		}

		for i := range 4 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("round %d dequeue %d: %v", round, i, err)
			}
			expected := uintptr(round*100 + i)
			if val != expected {
				t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
			}
		}
	}
}

// Test128BitWrapAround tests 128-bit DWCAS queue wrap-around behavior.
func Test128BitWrapAround(t *testing.T) {
	tests := []struct {
		name string
		newQ func() interface {
			Enqueue(uintptr) error
			Dequeue() (uintptr, error)
		}
	}{
		{"MPMCIndirect", func() interface {
			Enqueue(uintptr) error
			Dequeue() (uintptr, error)
		} {
			return lfq.NewMPMCIndirect(4)
		}},
		{"MPSCIndirect", func() interface {
			Enqueue(uintptr) error
			Dequeue() (uintptr, error)
		} {
			return lfq.NewMPSCIndirect(4)
		}},
		{"SPMCIndirect", func() interface {
			Enqueue(uintptr) error
			Dequeue() (uintptr, error)
		} {
			return lfq.NewSPMCIndirect(4)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.newQ()

			for round := range 10 {
				for i := range 4 {
					val := uintptr(round*100 + i)
					if err := q.Enqueue(val); err != nil {
						t.Fatalf("round %d enqueue %d: %v", round, i, err)
					}
				}

				for i := range 4 {
					val, err := q.Dequeue()
					if err != nil {
						t.Fatalf("round %d dequeue %d: %v", round, i, err)
					}
					expected := uintptr(round*100 + i)
					if val != expected {
						t.Fatalf("round %d dequeue %d: got %d, want %d", round, i, val, expected)
					}
				}
			}
		})
	}
}

// =============================================================================
// Edge Cases - Zero values, nil pointers
// =============================================================================

// TestZeroValue tests that zero is a valid value for all queue types.
func TestZeroValue(t *testing.T) {
	t.Run("Generic", func(t *testing.T) {
		q := lfq.NewMPMC[int](4)
		v := 0
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("enqueue 0: %v", err)
		}
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if val != 0 {
			t.Fatalf("got %d, want 0", val)
		}
	})

	t.Run("Compact", func(t *testing.T) {
		q := lfq.NewMPMCCompactIndirect(4)
		if err := q.Enqueue(0); err != nil {
			t.Fatalf("enqueue 0: %v", err)
		}
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if val != 0 {
			t.Fatalf("got %d, want 0", val)
		}
	})

	t.Run("Indirect128", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(4)
		if err := q.Enqueue(0); err != nil {
			t.Fatalf("enqueue 0: %v", err)
		}
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if val != 0 {
			t.Fatalf("got %d, want 0", val)
		}
	})
}

// TestNilPointer tests that nil is a valid pointer value.
func TestNilPointer(t *testing.T) {
	q := lfq.NewMPMCPtr(4)

	if err := q.Enqueue(nil); err != nil {
		t.Fatalf("enqueue nil: %v", err)
	}

	ptr, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if ptr != nil {
		t.Fatalf("got %v, want nil", ptr)
	}
}

// =============================================================================
// Capacity Tests
// =============================================================================

// TestCapacityRounding tests that capacity is rounded up to next power of 2.
func TestCapacityRounding(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{7, 8},
		{8, 8},
		{9, 16},
		{100, 128},
		{1000, 1024},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			q := lfq.NewMPMC[int](tt.input)
			if q.Cap() != tt.expected {
				t.Fatalf("NewMPMC(%d).Cap() = %d, want %d", tt.input, q.Cap(), tt.expected)
			}
		})
	}
}

// TestPanicOnSmallCapacity tests that capacity < 2 causes panic.
func TestPanicOnSmallCapacity(t *testing.T) {
	tests := []struct {
		name   string
		create func()
	}{
		{"SPSC", func() { lfq.NewSPSC[int](1) }},
		{"MPSC", func() { lfq.NewMPSC[int](1) }},
		{"SPMC", func() { lfq.NewSPMC[int](1) }},
		{"MPMC", func() { lfq.NewMPMC[int](1) }},
		{"MPMCCompact", func() { lfq.NewMPMCCompactIndirect(1) }},
		{"MPSCCompact", func() { lfq.NewMPSCCompactIndirect(1) }},
		{"SPMCCompact", func() { lfq.NewSPMCCompactIndirect(1) }},
		{"MPMCIndirect", func() { lfq.NewMPMCIndirect(1) }},
		{"MPSCIndirect", func() { lfq.NewMPSCIndirect(1) }},
		{"SPMCIndirect", func() { lfq.NewSPMCIndirect(1) }},
		{"MPMCPtr", func() { lfq.NewMPMCPtr(1) }},
		{"MPSCPtr", func() { lfq.NewMPSCPtr(1) }},
		{"SPMCPtr", func() { lfq.NewSPMCPtr(1) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for capacity < 2")
				}
			}()
			tt.create()
		})
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

func TestQueueIndirectInterface(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.NewMPMCIndirect(8)
	var _ lfq.QueueIndirect = lfq.NewMPSCIndirect(8)
	var _ lfq.QueueIndirect = lfq.NewSPMCIndirect(8)
	var _ lfq.QueueIndirect = lfq.NewMPMCCompactIndirect(8)
	var _ lfq.QueueIndirect = lfq.NewMPSCCompactIndirect(8)
	var _ lfq.QueueIndirect = lfq.NewSPMCCompactIndirect(8)
}

func TestQueuePtrInterface(t *testing.T) {
	var _ lfq.QueuePtr = lfq.NewMPMCPtr(8)
	var _ lfq.QueuePtr = lfq.NewMPSCPtr(8)
	var _ lfq.QueuePtr = lfq.NewSPMCPtr(8)
}
