// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// Error Functions Tests
// =============================================================================

// TestIsSemantic tests the IsSemantic error classification function.
func TestIsSemantic(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"ErrWouldBlock", lfq.ErrWouldBlock, true},
		{"iox.ErrWouldBlock", iox.ErrWouldBlock, true},
		{"other error", errors.New("other"), false},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, func(t *testing.T) {
			if got := lfq.IsSemantic(tt.err); got != tt.want {
				t.Errorf("IsSemantic(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsNonFailure tests the IsNonFailure error classification function.
func TestIsNonFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, true},
		{"ErrWouldBlock", lfq.ErrWouldBlock, true},
		{"iox.ErrWouldBlock", iox.ErrWouldBlock, true},
		{"other error", errors.New("failure"), false},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, func(t *testing.T) {
			if got := lfq.IsNonFailure(tt.err); got != tt.want {
				t.Errorf("IsNonFailure(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Generic Seq Variants - Basic Operations
// =============================================================================

// TestMPMCSeqBasic tests basic MPMCSeq (CAS-based compact MPMC) operations.
func TestMPMCSeqBasic(t *testing.T) {
	q := lfq.NewMPMCSeq[int](3)

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

// TestMPSCSeqBasic tests basic MPSCSeq (CAS-based compact MPSC) operations.
func TestMPSCSeqBasic(t *testing.T) {
	q := lfq.NewMPSCSeq[int](3)

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

// TestSPMCSeqBasic tests basic SPMCSeq (CAS-based compact SPMC) operations.
func TestSPMCSeqBasic(t *testing.T) {
	q := lfq.NewSPMCSeq[int](3)

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
// Generic Seq Variants - Panic Tests
// =============================================================================

// TestSeqPanicOnSmallCapacity tests that Seq constructors panic for capacity < 2.
func TestSeqPanicOnSmallCapacity(t *testing.T) {
	constructors := []struct {
		name string
		fn   func()
	}{
		{"MPMCSeq_One", func() { lfq.NewMPMCSeq[int](1) }},
		{"MPMCSeq_Zero", func() { lfq.NewMPMCSeq[int](0) }},
		{"MPMCSeq_Negative", func() { lfq.NewMPMCSeq[int](-1) }},
		{"MPSCSeq_One", func() { lfq.NewMPSCSeq[int](1) }},
		{"MPSCSeq_Zero", func() { lfq.NewMPSCSeq[int](0) }},
		{"SPMCSeq_One", func() { lfq.NewSPMCSeq[int](1) }},
		{"SPMCSeq_Zero", func() { lfq.NewSPMCSeq[int](0) }},
	}

	for c := range slices.Values(constructors) {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for capacity < 2")
				}
			}()
			c.fn()
		})
	}
}

// =============================================================================
// Generic Seq Variants - Wraparound Tests
// =============================================================================

// TestMPMCSeqWraparound tests that MPMCSeq handles index wraparound correctly.
func TestMPMCSeqWraparound(t *testing.T) {
	q := lfq.NewMPMCSeq[int](4)

	for cycle := range 10 {
		for i := range 4 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := cycle*100 + i
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// TestMPSCSeqWraparound tests that MPSCSeq handles index wraparound correctly.
func TestMPSCSeqWraparound(t *testing.T) {
	q := lfq.NewMPSCSeq[int](4)

	for cycle := range 10 {
		for i := range 4 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := cycle*100 + i
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// TestSPMCSeqWraparound tests that SPMCSeq handles index wraparound correctly.
func TestSPMCSeqWraparound(t *testing.T) {
	q := lfq.NewSPMCSeq[int](4)

	for cycle := range 10 {
		for i := range 4 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := cycle*100 + i
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// =============================================================================
// 128-bit Ptr Seq Variants - Basic Operations
// =============================================================================

// TestMPMCPtrSeqBasic tests basic MPMCPtrSeq (CAS-based compact MPMC for pointers).
func TestMPMCPtrSeqBasic(t *testing.T) {
	q := lfq.NewMPMCPtrSeq(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	vals := make([]int, 5)
	for i := range vals {
		vals[i] = i + 100
	}

	// Enqueue to capacity
	for i := range 4 {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	// Full queue returns ErrWouldBlock
	if err := q.Enqueue(unsafe.Pointer(&vals[4])); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Dequeue in FIFO order
	for i := range 4 {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		got := *(*int)(ptr)
		if got != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, got, i+100)
		}
	}

	// Empty queue returns ErrWouldBlock
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestMPSCPtrSeqBasic tests basic MPSCPtrSeq (CAS-based compact MPSC for pointers).
func TestMPSCPtrSeqBasic(t *testing.T) {
	q := lfq.NewMPSCPtrSeq(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	vals := make([]int, 5)
	for i := range vals {
		vals[i] = i + 100
	}

	for i := range 4 {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(unsafe.Pointer(&vals[4])); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		got := *(*int)(ptr)
		if got != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, got, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestSPMCPtrSeqBasic tests basic SPMCPtrSeq (CAS-based compact SPMC for pointers).
func TestSPMCPtrSeqBasic(t *testing.T) {
	q := lfq.NewSPMCPtrSeq(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	vals := make([]int, 5)
	for i := range vals {
		vals[i] = i + 100
	}

	for i := range 4 {
		if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	if err := q.Enqueue(unsafe.Pointer(&vals[4])); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	for i := range 4 {
		ptr, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		got := *(*int)(ptr)
		if got != i+100 {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, got, i+100)
		}
	}

	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// =============================================================================
// 128-bit Ptr Seq Variants - Panic Tests
// =============================================================================

// TestPtrSeqPanicOnSmallCapacity tests that Ptr Seq constructors panic for capacity < 2.
func TestPtrSeqPanicOnSmallCapacity(t *testing.T) {
	constructors := []struct {
		name string
		fn   func()
	}{
		{"MPMCPtrSeq_One", func() { lfq.NewMPMCPtrSeq(1) }},
		{"MPMCPtrSeq_Zero", func() { lfq.NewMPMCPtrSeq(0) }},
		{"MPSCPtrSeq_One", func() { lfq.NewMPSCPtrSeq(1) }},
		{"MPSCPtrSeq_Zero", func() { lfq.NewMPSCPtrSeq(0) }},
		{"SPMCPtrSeq_One", func() { lfq.NewSPMCPtrSeq(1) }},
		{"SPMCPtrSeq_Zero", func() { lfq.NewSPMCPtrSeq(0) }},
	}

	for c := range slices.Values(constructors) {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for capacity < 2")
				}
			}()
			c.fn()
		})
	}
}

// =============================================================================
// 128-bit Ptr Seq Variants - Wraparound Tests
// =============================================================================

// TestMPMCPtrSeqWraparound tests that MPMCPtrSeq handles index wraparound correctly.
func TestMPMCPtrSeqWraparound(t *testing.T) {
	q := lfq.NewMPMCPtrSeq(4)

	vals := make([]int, 4)

	for cycle := range 10 {
		for i := range 4 {
			vals[i] = cycle*100 + i
			if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			ptr, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			got := *(*int)(ptr)
			expected := cycle*100 + i
			if got != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, got, expected)
			}
		}
	}
}

// TestMPSCPtrSeqWraparound tests that MPSCPtrSeq handles index wraparound correctly.
func TestMPSCPtrSeqWraparound(t *testing.T) {
	q := lfq.NewMPSCPtrSeq(4)

	vals := make([]int, 4)

	for cycle := range 10 {
		for i := range 4 {
			vals[i] = cycle*100 + i
			if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			ptr, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			got := *(*int)(ptr)
			expected := cycle*100 + i
			if got != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, got, expected)
			}
		}
	}
}

// TestSPMCPtrSeqWraparound tests that SPMCPtrSeq handles index wraparound correctly.
func TestSPMCPtrSeqWraparound(t *testing.T) {
	q := lfq.NewSPMCPtrSeq(4)

	vals := make([]int, 4)

	for cycle := range 10 {
		for i := range 4 {
			vals[i] = cycle*100 + i
			if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			ptr, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			got := *(*int)(ptr)
			expected := cycle*100 + i
			if got != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, got, expected)
			}
		}
	}
}

// =============================================================================
// Builder API with Compact - Tests
// =============================================================================

// TestBuilderCompactGeneric tests Builder with Compact() for generic queues.
func TestBuilderCompactGeneric(t *testing.T) {
	tests := []struct {
		name    string
		build   func() lfq.Queue[int]
		wantCap int
	}{
		{
			name:    "Compact_MPMC",
			build:   func() lfq.Queue[int] { return lfq.Build[int](lfq.New(7).Compact()) },
			wantCap: 8,
		},
		{
			name:    "Compact_MPSC",
			build:   func() lfq.Queue[int] { return lfq.Build[int](lfq.New(7).SingleConsumer().Compact()) },
			wantCap: 8,
		},
		{
			name:    "Compact_SPMC",
			build:   func() lfq.Queue[int] { return lfq.Build[int](lfq.New(7).SingleProducer().Compact()) },
			wantCap: 8,
		},
		{
			name:    "Compact_SPSC_ignored",
			build:   func() lfq.Queue[int] { return lfq.Build[int](lfq.New(7).SingleProducer().SingleConsumer().Compact()) },
			wantCap: 8,
		},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build()
			if q.Cap() != tt.wantCap {
				t.Fatalf("Cap: got %d, want %d", q.Cap(), tt.wantCap)
			}

			// Basic enqueue/dequeue
			v := 42
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			got, err := q.Dequeue()
			if err != nil {
				t.Fatalf("Dequeue: %v", err)
			}
			if got != 42 {
				t.Fatalf("got %d, want 42", got)
			}
		})
	}
}

// TestBuilderCompactPtr tests Builder with Compact() for Ptr queues.
func TestBuilderCompactPtr(t *testing.T) {
	tests := []struct {
		name    string
		build   func() lfq.QueuePtr
		wantCap int
	}{
		{
			name:    "Compact_MPMC_Ptr",
			build:   func() lfq.QueuePtr { return lfq.New(7).Compact().BuildPtr() },
			wantCap: 8,
		},
		{
			name:    "Compact_MPSC_Ptr",
			build:   func() lfq.QueuePtr { return lfq.New(7).SingleConsumer().Compact().BuildPtr() },
			wantCap: 8,
		},
		{
			name:    "Compact_SPMC_Ptr",
			build:   func() lfq.QueuePtr { return lfq.New(7).SingleProducer().Compact().BuildPtr() },
			wantCap: 8,
		},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, func(t *testing.T) {
			q := tt.build()
			if q.Cap() != tt.wantCap {
				t.Fatalf("Cap: got %d, want %d", q.Cap(), tt.wantCap)
			}

			val := 42
			if err := q.Enqueue(unsafe.Pointer(&val)); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			ptr, err := q.Dequeue()
			if err != nil {
				t.Fatalf("Dequeue: %v", err)
			}
			if *(*int)(ptr) != 42 {
				t.Fatalf("got %d, want 42", *(*int)(ptr))
			}
		})
	}
}

// TestBuildMPSCCompact tests BuildMPSC with Compact option.
func TestBuildMPSCCompact(t *testing.T) {
	q := lfq.BuildMPSC[int](lfq.New(8).SingleConsumer().Compact())
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	v := 100
	if err := q.Enqueue(&v); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %d, want 100", got)
	}
}

// TestBuildSPMCCompact tests BuildSPMC with Compact option.
func TestBuildSPMCCompact(t *testing.T) {
	q := lfq.BuildSPMC[int](lfq.New(8).SingleProducer().Compact())
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	v := 100
	if err := q.Enqueue(&v); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %d, want 100", got)
	}
}

// TestBuildMPMCCompact tests BuildMPMC with Compact option.
func TestBuildMPMCCompact(t *testing.T) {
	q := lfq.BuildMPMC[int](lfq.New(8).Compact())
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	v := 100
	if err := q.Enqueue(&v); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 100 {
		t.Fatalf("got %d, want 100", got)
	}
}

// =============================================================================
// Builder API with Compact - Indirect Tests
// =============================================================================

// TestBuildIndirectMPSCCompact tests BuildIndirectMPSC with Compact option.
func TestBuildIndirectMPSCCompact(t *testing.T) {
	q := lfq.New(8).SingleConsumer().Compact().BuildIndirectMPSC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	if err := q.Enqueue(42); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

// TestBuildIndirectSPMCCompact tests BuildIndirectSPMC with Compact option.
func TestBuildIndirectSPMCCompact(t *testing.T) {
	q := lfq.New(8).SingleProducer().Compact().BuildIndirectSPMC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	if err := q.Enqueue(42); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

// TestBuildIndirectMPMCCompact tests BuildIndirectMPMC with Compact option.
func TestBuildIndirectMPMCCompact(t *testing.T) {
	q := lfq.New(8).Compact().BuildIndirectMPMC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	if err := q.Enqueue(42); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if got != 42 {
		t.Fatalf("got %d, want 42", got)
	}
}

// =============================================================================
// Builder API with Compact - Ptr Builder Tests
// =============================================================================

// TestBuildPtrMPSCCompact tests BuildPtrMPSC with Compact option.
func TestBuildPtrMPSCCompact(t *testing.T) {
	q := lfq.New(8).SingleConsumer().Compact().BuildPtrMPSC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	val := 42
	if err := q.Enqueue(unsafe.Pointer(&val)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	ptr, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if *(*int)(ptr) != 42 {
		t.Fatalf("got %d, want 42", *(*int)(ptr))
	}
}

// TestBuildPtrSPMCCompact tests BuildPtrSPMC with Compact option.
func TestBuildPtrSPMCCompact(t *testing.T) {
	q := lfq.New(8).SingleProducer().Compact().BuildPtrSPMC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	val := 42
	if err := q.Enqueue(unsafe.Pointer(&val)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	ptr, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if *(*int)(ptr) != 42 {
		t.Fatalf("got %d, want 42", *(*int)(ptr))
	}
}

// TestBuildPtrMPMCCompact tests BuildPtrMPMC with Compact option.
func TestBuildPtrMPMCCompact(t *testing.T) {
	q := lfq.New(8).Compact().BuildPtrMPMC()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	val := 42
	if err := q.Enqueue(unsafe.Pointer(&val)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	ptr, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if *(*int)(ptr) != 42 {
		t.Fatalf("got %d, want 42", *(*int)(ptr))
	}
}

// =============================================================================
// Concurrent Tests for Seq Variants (without race detector)
// =============================================================================

// TestMPMCSeqConcurrent tests MPMCSeq under concurrent access.
func TestMPMCSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewMPMCSeq[int](16)
	const numGoroutines = 4
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Producers
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for j := range opsPerGoroutine {
				v := id*1000 + j + 1
				for q.Enqueue(&v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(i)
	}

	// Consumers
	for range numGoroutines {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			consumed := 0
			for consumed < opsPerGoroutine {
				_, err := q.Dequeue()
				if err == nil {
					consumed++
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()
}

// TestMPSCSeqConcurrent tests MPSCSeq under concurrent producer access.
func TestMPSCSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewMPSCSeq[int](16)
	const numProducers = 4
	const opsPerProducer = 100
	totalOps := numProducers * opsPerProducer

	var wg sync.WaitGroup
	wg.Add(numProducers + 1)

	// Producers
	for i := range numProducers {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for j := range opsPerProducer {
				v := id*1000 + j + 1
				for q.Enqueue(&v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(i)
	}

	// Single consumer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		consumed := 0
		for consumed < totalOps {
			_, err := q.Dequeue()
			if err == nil {
				consumed++
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()
}

// TestSPMCSeqConcurrent tests SPMCSeq under concurrent consumer access.
func TestSPMCSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewSPMCSeq[int](16)
	const numConsumers = 4
	const totalOps = 400

	var wg sync.WaitGroup
	wg.Add(numConsumers + 1)

	// Single producer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range totalOps {
			v := i + 1
			for q.Enqueue(&v) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Multiple consumers
	opsPerConsumer := totalOps / numConsumers
	for range numConsumers {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			consumed := 0
			for consumed < opsPerConsumer {
				_, err := q.Dequeue()
				if err == nil {
					consumed++
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()
}

// =============================================================================
// 128-bit Indirect Seq Variants - Basic Operations
// =============================================================================

// TestMPMCIndirectSeqBasic tests basic MPMCIndirectSeq (128-bit CAS-based MPMC for uintptr).
func TestMPMCIndirectSeqBasic(t *testing.T) {
	q := lfq.NewMPMCIndirectSeq(3)

	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	// Enqueue to capacity
	for i := range 4 {
		if err := q.Enqueue(uintptr(i + 100)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	// Full queue returns ErrWouldBlock
	if err := q.Enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Dequeue in FIFO order
	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if val != uintptr(i+100) {
			t.Fatalf("Dequeue(%d): got %d, want %d", i, val, i+100)
		}
	}

	// Empty queue returns ErrWouldBlock
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// TestMPSCIndirectSeqBasic tests basic MPSCIndirectSeq (128-bit CAS-based MPSC for uintptr).
func TestMPSCIndirectSeqBasic(t *testing.T) {
	q := lfq.NewMPSCIndirectSeq(3)

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

// TestSPMCIndirectSeqBasic tests basic SPMCIndirectSeq (128-bit CAS-based SPMC for uintptr).
func TestSPMCIndirectSeqBasic(t *testing.T) {
	q := lfq.NewSPMCIndirectSeq(3)

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

// =============================================================================
// 128-bit Indirect Seq Variants - Panic Tests
// =============================================================================

// TestIndirectSeqPanicOnSmallCapacity tests that Indirect Seq constructors panic for capacity < 2.
func TestIndirectSeqPanicOnSmallCapacity(t *testing.T) {
	constructors := []struct {
		name string
		fn   func()
	}{
		{"MPMCIndirectSeq_One", func() { lfq.NewMPMCIndirectSeq(1) }},
		{"MPMCIndirectSeq_Zero", func() { lfq.NewMPMCIndirectSeq(0) }},
		{"MPSCIndirectSeq_One", func() { lfq.NewMPSCIndirectSeq(1) }},
		{"MPSCIndirectSeq_Zero", func() { lfq.NewMPSCIndirectSeq(0) }},
		{"SPMCIndirectSeq_One", func() { lfq.NewSPMCIndirectSeq(1) }},
		{"SPMCIndirectSeq_Zero", func() { lfq.NewSPMCIndirectSeq(0) }},
	}

	for c := range slices.Values(constructors) {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for capacity < 2")
				}
			}()
			c.fn()
		})
	}
}

// =============================================================================
// 128-bit Indirect Seq Variants - Wraparound Tests
// =============================================================================

// TestMPMCIndirectSeqWraparound tests that MPMCIndirectSeq handles index wraparound correctly.
func TestMPMCIndirectSeqWraparound(t *testing.T) {
	q := lfq.NewMPMCIndirectSeq(4)

	for cycle := range 10 {
		for i := range 4 {
			v := uintptr(cycle*100 + i)
			if err := q.Enqueue(v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := uintptr(cycle*100 + i)
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// TestMPSCIndirectSeqWraparound tests that MPSCIndirectSeq handles index wraparound correctly.
func TestMPSCIndirectSeqWraparound(t *testing.T) {
	q := lfq.NewMPSCIndirectSeq(4)

	for cycle := range 10 {
		for i := range 4 {
			v := uintptr(cycle*100 + i)
			if err := q.Enqueue(v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := uintptr(cycle*100 + i)
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// TestSPMCIndirectSeqWraparound tests that SPMCIndirectSeq handles index wraparound correctly.
func TestSPMCIndirectSeqWraparound(t *testing.T) {
	q := lfq.NewSPMCIndirectSeq(4)

	for cycle := range 10 {
		for i := range 4 {
			v := uintptr(cycle*100 + i)
			if err := q.Enqueue(v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		for i := range 4 {
			elem, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := uintptr(cycle*100 + i)
			if elem != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, elem, expected)
			}
		}
	}
}

// =============================================================================
// 128-bit Indirect Seq Variants - Concurrent Tests
// =============================================================================

// TestMPMCIndirectSeqConcurrent tests MPMCIndirectSeq under concurrent access.
func TestMPMCIndirectSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewMPMCIndirectSeq(16)
	const numGoroutines = 4
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Producers
	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for j := range opsPerGoroutine {
				v := uintptr(id*1000 + j + 1)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(i)
	}

	// Consumers
	for range numGoroutines {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			consumed := 0
			for consumed < opsPerGoroutine {
				_, err := q.Dequeue()
				if err == nil {
					consumed++
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()
}

// TestMPSCIndirectSeqConcurrent tests MPSCIndirectSeq under concurrent producer access.
func TestMPSCIndirectSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewMPSCIndirectSeq(16)
	const numProducers = 4
	const opsPerProducer = 100
	totalOps := numProducers * opsPerProducer

	var wg sync.WaitGroup
	wg.Add(numProducers + 1)

	// Producers
	for i := range numProducers {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for j := range opsPerProducer {
				v := uintptr(id*1000 + j + 1)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(i)
	}

	// Single consumer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		consumed := 0
		for consumed < totalOps {
			_, err := q.Dequeue()
			if err == nil {
				consumed++
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()
}

// TestSPMCIndirectSeqConcurrent tests SPMCIndirectSeq under concurrent consumer access.
func TestSPMCIndirectSeqConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	q := lfq.NewSPMCIndirectSeq(16)
	const numConsumers = 4
	const totalOps = 400

	var wg sync.WaitGroup
	wg.Add(numConsumers + 1)

	// Single producer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range totalOps {
			v := uintptr(i + 1)
			for q.Enqueue(v) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Multiple consumers
	opsPerConsumer := totalOps / numConsumers
	for range numConsumers {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			consumed := 0
			for consumed < opsPerConsumer {
				_, err := q.Dequeue()
				if err == nil {
					consumed++
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()
}

// =============================================================================
// High Contention Tests for CAS Retry Paths
// =============================================================================

// TestSeqCASRetryContention exercises CAS failure paths in Seq variants.
// Uses small capacity (2) with many goroutines to force CAS retries.
func TestSeqCASRetryContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	// SPMCSeq.Dequeue CAS retry path
	t.Run("SPMCSeq_Dequeue", func(t *testing.T) {
		q := lfq.NewSPMCSeq[int](2)
		const numConsumers = 16
		const totalOps = 500

		var wg sync.WaitGroup
		var consumed atomix.Int32

		// Single producer - keeps queue fed
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range totalOps {
				v := i + 1
				for q.Enqueue(&v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}()

		// Many consumers competing for same slots - forces CAS failures
		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for {
					if consumed.Load() >= totalOps {
						return
					}
					if _, err := q.Dequeue(); err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}

		wg.Wait()
		if consumed.Load() < totalOps {
			t.Errorf("consumed %d, want >= %d", consumed.Load(), totalOps)
		}
	})

	// SPMCIndirectSeq.Dequeue CAS retry path
	t.Run("SPMCIndirectSeq_Dequeue", func(t *testing.T) {
		q := lfq.NewSPMCIndirectSeq(2)
		const numConsumers = 16
		const totalOps = 500

		var wg sync.WaitGroup
		var consumed atomix.Int32

		// Single producer
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range totalOps {
				for q.Enqueue(uintptr(i+1)) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}()

		// Many consumers
		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for {
					if consumed.Load() >= totalOps {
						return
					}
					if _, err := q.Dequeue(); err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}

		wg.Wait()
		if consumed.Load() < totalOps {
			t.Errorf("consumed %d, want >= %d", consumed.Load(), totalOps)
		}
	})

	// SPMCPtrSeq.Dequeue CAS retry path
	t.Run("SPMCPtrSeq_Dequeue", func(t *testing.T) {
		q := lfq.NewSPMCPtrSeq(2)
		const numConsumers = 16
		const totalOps = 500

		// Pre-allocate values
		values := make([]int, totalOps)
		for i := range values {
			values[i] = i + 1
		}

		var wg sync.WaitGroup
		var consumed atomix.Int32

		// Single producer
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range totalOps {
				for q.Enqueue(unsafe.Pointer(&values[i])) != nil {
					backoff.Wait()
				}
				backoff.Reset()
			}
		}()

		// Many consumers
		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for {
					if consumed.Load() >= totalOps {
						return
					}
					if _, err := q.Dequeue(); err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}

		wg.Wait()
		if consumed.Load() < totalOps {
			t.Errorf("consumed %d, want >= %d", consumed.Load(), totalOps)
		}
	})

	// MPSCIndirectSeq.Enqueue CAS retry path
	t.Run("MPSCIndirectSeq_Enqueue", func(t *testing.T) {
		q := lfq.NewMPSCIndirectSeq(2)
		const numProducers = 16
		const opsPerProducer = 50
		totalOps := numProducers * opsPerProducer

		var wg sync.WaitGroup
		var consumed atomix.Int32

		// Many producers competing for same tail slot - forces CAS failures
		wg.Add(numProducers)
		for p := range numProducers {
			go func(id int) {
				defer wg.Done()
				backoff := iox.Backoff{}
				for i := range opsPerProducer {
					v := uintptr(id*1000 + i + 1)
					for q.Enqueue(v) != nil {
						backoff.Wait()
					}
					backoff.Reset()
				}
			}(p)
		}

		// Single consumer
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for {
				if consumed.Load() >= int32(totalOps) {
					return
				}
				if _, err := q.Dequeue(); err == nil {
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()

		wg.Wait()
		if consumed.Load() < int32(totalOps) {
			t.Errorf("consumed %d, want >= %d", consumed.Load(), totalOps)
		}
	})

	// MPSCPtrSeq.Enqueue CAS retry path
	t.Run("MPSCPtrSeq_Enqueue", func(t *testing.T) {
		q := lfq.NewMPSCPtrSeq(2)
		const numProducers = 16
		const opsPerProducer = 50
		totalOps := numProducers * opsPerProducer

		// Pre-allocate values per producer
		values := make([][]int, numProducers)
		for p := range numProducers {
			values[p] = make([]int, opsPerProducer)
			for i := range opsPerProducer {
				values[p][i] = p*1000 + i + 1
			}
		}

		var wg sync.WaitGroup
		var consumed atomix.Int32

		// Many producers
		wg.Add(numProducers)
		for p := range numProducers {
			go func(id int) {
				defer wg.Done()
				backoff := iox.Backoff{}
				for i := range opsPerProducer {
					for q.Enqueue(unsafe.Pointer(&values[id][i])) != nil {
						backoff.Wait()
					}
					backoff.Reset()
				}
			}(p)
		}

		// Single consumer
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for {
				if consumed.Load() >= int32(totalOps) {
					return
				}
				if _, err := q.Dequeue(); err == nil {
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()

		wg.Wait()
		if consumed.Load() < int32(totalOps) {
			t.Errorf("consumed %d, want >= %d", consumed.Load(), totalOps)
		}
	})
}
