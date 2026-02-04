// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// Builder API Tests (Consolidated)
// =============================================================================

// TestBuilderAPI tests all Builder combinations in a table-driven fashion.
func TestBuilderAPI(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (cap int, enq func() error, deq func() (any, error))
		wantCap int
	}{
		{
			name: "GenericSPSC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.BuildSPSC[int](lfq.New(7).SingleProducer().SingleConsumer())
				return q.Cap(), func() error { v := 42; return q.Enqueue(&v) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "IndirectSPSC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).SingleProducer().SingleConsumer().BuildIndirectSPSC()
				return q.Cap(), func() error { return q.Enqueue(42) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "IndirectMPSC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).SingleConsumer().BuildIndirect()
				return q.Cap(), func() error { return q.Enqueue(42) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "IndirectSPMC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).SingleProducer().BuildIndirect()
				return q.Cap(), func() error { return q.Enqueue(42) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "IndirectMPMC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).BuildIndirect()
				return q.Cap(), func() error { return q.Enqueue(42) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "PtrMPSC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).SingleConsumer().BuildPtr()
				val := 42
				return q.Cap(), func() error { return q.Enqueue(unsafe.Pointer(&val)) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "PtrSPMC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).SingleProducer().BuildPtr()
				val := 42
				return q.Cap(), func() error { return q.Enqueue(unsafe.Pointer(&val)) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
		{
			name: "PtrMPMC",
			build: func() (int, func() error, func() (any, error)) {
				q := lfq.New(7).BuildPtr()
				val := 42
				return q.Cap(), func() error { return q.Enqueue(unsafe.Pointer(&val)) }, func() (any, error) { return q.Dequeue() }
			},
			wantCap: 8,
		},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, func(t *testing.T) {
			cap, enq, deq := tt.build()
			if cap != tt.wantCap {
				t.Fatalf("Cap: got %d, want %d", cap, tt.wantCap)
			}
			if err := enq(); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			v, err := deq()
			if err != nil {
				t.Fatalf("Dequeue: %v", err)
			}
			if v == nil {
				t.Fatal("Dequeue returned nil")
			}
		})
	}
}

// =============================================================================
// Full and Empty Edge Cases (Consolidated)
// =============================================================================

// TestFullEmpty tests full/empty boundary behavior for all queue types.
func TestFullEmpty(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"MPSCIndirect", testFullEmptyMPSCIndirect},
		{"MPSCPtr", testFullEmptyMPSCPtr},
		{"SPMCIndirect", testFullEmptySPMCIndirect},
		{"SPMCPtr", testFullEmptySPMCPtr},
		{"MPMCIndirect", testFullEmptyMPMCIndirect},
		{"MPMCPtr", testFullEmptyMPMCPtr},
		{"MPMCCompactIndirect", testFullEmptyMPMCCompact},
		{"MPSCCompactIndirect", testFullEmptyMPSCCompact},
		{"SPMCCompactIndirect", testFullEmptySPMCCompact},
	}

	for tt := range slices.Values(tests) {
		t.Run(tt.name, tt.run)
	}
}

func testFullEmptyMPSCIndirect(t *testing.T) {
	q := lfq.NewMPSCIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptyMPSCPtr(t *testing.T) {
	q := lfq.NewMPSCPtr(4)
	vals := make([]int, 5)
	testFullEmptyPtr(t, func(i int) error { return q.Enqueue(unsafe.Pointer(&vals[i])) }, q.Dequeue, vals)
}

func testFullEmptySPMCIndirect(t *testing.T) {
	q := lfq.NewSPMCIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptySPMCPtr(t *testing.T) {
	q := lfq.NewSPMCPtr(4)
	vals := make([]int, 5)
	testFullEmptyPtr(t, func(i int) error { return q.Enqueue(unsafe.Pointer(&vals[i])) }, q.Dequeue, vals)
}

func testFullEmptyMPMCIndirect(t *testing.T) {
	q := lfq.NewMPMCIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptyMPMCPtr(t *testing.T) {
	q := lfq.NewMPMCPtr(4)
	vals := make([]int, 5)
	testFullEmptyPtr(t, func(i int) error { return q.Enqueue(unsafe.Pointer(&vals[i])) }, q.Dequeue, vals)
}

func testFullEmptyMPMCCompact(t *testing.T) {
	q := lfq.NewMPMCCompactIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptyMPSCCompact(t *testing.T) {
	q := lfq.NewMPSCCompactIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptySPMCCompact(t *testing.T) {
	q := lfq.NewSPMCCompactIndirect(4)
	testFullEmptyIndirect(t, q.Enqueue, q.Dequeue)
}

func testFullEmptyIndirect(t *testing.T, enq func(uintptr) error, deq func() (uintptr, error)) {
	t.Helper()
	// Fill
	for i := range 4 {
		if err := enq(uintptr(i)); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}
	// Full
	if err := enq(99); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}
	// Drain
	for i := range 4 {
		v, err := deq()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		if v != uintptr(i) {
			t.Fatalf("Dequeue: got %d, want %d", v, i)
		}
	}
	// Empty
	_, err := deq()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

func testFullEmptyPtr(t *testing.T, enq func(int) error, deq func() (unsafe.Pointer, error), vals []int) {
	t.Helper()
	for i := range vals {
		vals[i] = i + 1
	}
	// Fill
	for i := range 4 {
		if err := enq(i); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}
	// Full
	if err := enq(4); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}
	// Drain
	for i := range 4 {
		v, err := deq()
		if err != nil {
			t.Fatalf("Dequeue(%d): %v", i, err)
		}
		got := *(*int)(v)
		if got != vals[i] {
			t.Fatalf("Dequeue: got %d, want %d", got, vals[i])
		}
	}
	// Empty
	_, err := deq()
	if !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// =============================================================================
// Panic Tests (Consolidated)
// =============================================================================

// TestPanicOnSmallCapacityAllTypes tests that all queue constructors panic for capacity < 2.
func TestPanicOnSmallCapacityAllTypes(t *testing.T) {
	constructors := []struct {
		name string
		fn   func()
	}{
		{"Builder_New", func() { lfq.New(1) }},
		{"MPSC", func() { lfq.NewMPSC[int](1) }},
		{"SPMC", func() { lfq.NewSPMC[int](1) }},
		{"MPMC", func() { lfq.NewMPMC[int](1) }},
		{"SPSCIndirect_Zero", func() { lfq.NewSPSCIndirect(0) }},
		{"SPSCPtr_Negative", func() { lfq.NewSPSCPtr(-1) }},
		{"MPSCIndirect", func() { lfq.NewMPSCIndirect(1) }},
		{"MPSCPtr", func() { lfq.NewMPSCPtr(1) }},
		{"SPMCIndirect", func() { lfq.NewSPMCIndirect(1) }},
		{"SPMCPtr", func() { lfq.NewSPMCPtr(1) }},
		{"MPMCIndirect", func() { lfq.NewMPMCIndirect(1) }},
		{"MPMCPtr", func() { lfq.NewMPMCPtr(1) }},
		{"MPMCCompactIndirect", func() { lfq.NewMPMCCompactIndirect(1) }},
		{"MPSCCompactIndirect", func() { lfq.NewMPSCCompactIndirect(1) }},
		{"SPMCCompactIndirect", func() { lfq.NewSPMCCompactIndirect(1) }},
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

// TestPanicBuildSPSC tests that BuildSPSC panics without proper constraints.
func TestPanicBuildSPSC(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{"NoConstraints", func() { lfq.BuildSPSC[int](lfq.New(8)) }},
		{"OnlySP", func() { lfq.BuildSPSC[int](lfq.New(8).SingleProducer()) }},
		{"OnlySC", func() { lfq.BuildSPSC[int](lfq.New(8).SingleConsumer()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.build()
		})
	}
}

// TestPanicBuildIndirectSPSC tests that BuildIndirectSPSC panics without proper constraints.
func TestPanicBuildIndirectSPSC(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{"NoConstraints", func() { lfq.New(8).BuildIndirectSPSC() }},
		{"OnlySP", func() { lfq.New(8).SingleProducer().BuildIndirectSPSC() }},
		{"OnlySC", func() { lfq.New(8).SingleConsumer().BuildIndirectSPSC() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.build()
		})
	}
}

// TestPanicBuildMPSC tests that BuildMPSC panics without proper constraints.
func TestPanicBuildMPSC(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{"NoConstraints", func() { lfq.BuildMPSC[int](lfq.New(8)) }},
		{"WithSingleProducer", func() { lfq.BuildMPSC[int](lfq.New(8).SingleProducer()) }},
		{"WithBothConstraints", func() { lfq.BuildMPSC[int](lfq.New(8).SingleProducer().SingleConsumer()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.build()
		})
	}
}

// TestPanicBuildSPMC tests that BuildSPMC panics without proper constraints.
func TestPanicBuildSPMC(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{"NoConstraints", func() { lfq.BuildSPMC[int](lfq.New(8)) }},
		{"WithSingleConsumer", func() { lfq.BuildSPMC[int](lfq.New(8).SingleConsumer()) }},
		{"WithBothConstraints", func() { lfq.BuildSPMC[int](lfq.New(8).SingleProducer().SingleConsumer()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.build()
		})
	}
}

// TestPanicBuildMPMC tests that BuildMPMC panics with constraints.
func TestPanicBuildMPMC(t *testing.T) {
	tests := []struct {
		name  string
		build func()
	}{
		{"WithSingleProducer", func() { lfq.BuildMPMC[int](lfq.New(8).SingleProducer()) }},
		{"WithSingleConsumer", func() { lfq.BuildMPMC[int](lfq.New(8).SingleConsumer()) }},
		{"WithBothConstraints", func() { lfq.BuildMPMC[int](lfq.New(8).SingleProducer().SingleConsumer()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tt.build()
		})
	}
}

// TestBuildIndirectAllBranches tests all BuildIndirect switch branches.
func TestBuildIndirectAllBranches(t *testing.T) {
	t.Run("CompactSPMC", func(t *testing.T) {
		q := lfq.New(8).SingleProducer().Compact().BuildIndirect()
		if err := q.Enqueue(42); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		v, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		if v != 42 {
			t.Fatalf("got %d, want 42", v)
		}
	})

	t.Run("CompactMPSC", func(t *testing.T) {
		q := lfq.New(8).SingleConsumer().Compact().BuildIndirect()
		if err := q.Enqueue(42); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		v, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		if v != 42 {
			t.Fatalf("got %d, want 42", v)
		}
	})
}

// TestTypedBuildersWithoutCompact tests BuildMPSC, BuildSPMC, BuildMPMC without Compact flag.
func TestTypedBuildersWithoutCompact(t *testing.T) {
	t.Run("BuildMPSC", func(t *testing.T) {
		q := lfq.BuildMPSC[int](lfq.New(8).SingleConsumer())
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

	t.Run("BuildSPMC", func(t *testing.T) {
		q := lfq.BuildSPMC[int](lfq.New(8).SingleProducer())
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

	t.Run("BuildMPMC", func(t *testing.T) {
		q := lfq.BuildMPMC[int](lfq.New(8))
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

	t.Run("BuildPtrSPSC", func(t *testing.T) {
		q := lfq.New(8).SingleProducer().SingleConsumer().BuildPtrSPSC()
		v := 42
		if err := q.Enqueue(unsafe.Pointer(&v)); err != nil {
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

// TestPanicCompact63Bit tests that Compact queues panic on 63-bit values.
func TestPanicCompact63Bit(t *testing.T) {
	constructors := []struct {
		name string
		fn   func()
	}{
		{"MPMCCompact", func() { lfq.NewMPMCCompactIndirect(4).Enqueue(1 << 63) }},
		{"MPSCCompact", func() { lfq.NewMPSCCompactIndirect(4).Enqueue(1 << 63) }},
		{"SPMCCompact", func() { lfq.NewSPMCCompactIndirect(4).Enqueue(1 << 63) }},
	}

	for c := range slices.Values(constructors) {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic for 63-bit value")
				}
			}()
			c.fn()
		})
	}
}

// =============================================================================
// Capacity Rounding Tests
// =============================================================================

// TestRoundToPow2 tests that capacity is rounded up to next power of 2.
func TestRoundToPow2(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		// Powers of 2 remain unchanged
		{2, 2},
		{4, 4},
		{8, 8},
		{16, 16},
		{32, 32},
		{64, 64},
		{128, 128},
		{256, 256},
		{512, 512},
		{1024, 1024},
		// Non-powers round up to next power of 2
		{3, 4},
		{5, 8},
		{6, 8},
		{7, 8},
		{9, 16},
		{15, 16},
		{17, 32},
		{31, 32},
		{33, 64},
		{100, 128},
		{200, 256},
		{500, 512},
		{1000, 1024},
	}

	for tc := range slices.Values(tests) {
		t.Run("", func(t *testing.T) {
			q := lfq.NewSPSC[int](tc.input)
			if q.Cap() != tc.expected {
				t.Errorf("NewSPSC(%d).Cap() = %d, want %d", tc.input, q.Cap(), tc.expected)
			}
		})
	}
}

// TestCapacityTwoBoundary tests minimum capacity boundary.
// With original semantics: NewSPSC(2) → Cap()=2
func TestCapacityTwoBoundary(t *testing.T) {
	q := lfq.NewSPSC[int](2)
	if q.Cap() != 2 {
		t.Fatalf("Cap: got %d, want 2", q.Cap())
	}

	// Fill
	for i := range 2 {
		v := i
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue(%d): %v", i, err)
		}
	}

	// Full
	v := 99
	if err := q.Enqueue(&v); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
	}

	// Drain
	for i := range 2 {
		elem, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		if elem != i {
			t.Fatalf("Dequeue: got %d, want %d", elem, i)
		}
	}

	// Empty
	if _, err := q.Dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
		t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
	}
}

// =============================================================================
// Concurrent Stress Tests
// =============================================================================

func TestMPSCConcurrentProducers(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPSC[int](1024)
	numProducers := 8
	itemsPerProducer := 100

	var wg sync.WaitGroup
	wg.Add(numProducers)

	for p := range numProducers {
		go func(id int) {
			defer wg.Done()
			for i := range itemsPerProducer {
				v := id*1000 + i
				for {
					err := q.Enqueue(&v)
					if err == nil {
						break
					}
					if errors.Is(err, lfq.ErrWouldBlock) {
						continue
					}
					t.Errorf("unexpected error: %v", err)
					return
				}
			}
		}(p)
	}

	wg.Wait()

	// Drain all items
	count := 0
	for {
		_, err := q.Dequeue()
		if errors.Is(err, lfq.ErrWouldBlock) {
			break
		}
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		count++
	}

	expected := numProducers * itemsPerProducer
	if count != expected {
		t.Fatalf("count: got %d, want %d", count, expected)
	}
}

func TestSPMCConcurrentConsumers(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewSPMC[int](1024)
	numItems := 800
	numConsumers := 8

	// Producer: fill queue first
	for i := range numItems {
		v := i
		if err := q.Enqueue(&v); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	var wg sync.WaitGroup
	var consumed sync.Map
	wg.Add(numConsumers)

	for c := range numConsumers {
		go func(id int) {
			defer wg.Done()
			for {
				elem, err := q.Dequeue()
				if errors.Is(err, lfq.ErrWouldBlock) {
					return
				}
				if err != nil {
					t.Errorf("Dequeue: %v", err)
					return
				}
				consumed.Store(elem, true)
			}
		}(c)
	}

	wg.Wait()

	// Verify all items consumed
	count := 0
	consumed.Range(func(_, _ any) bool {
		count++
		return true
	})

	if count != numItems {
		t.Fatalf("consumed: got %d, want %d", count, numItems)
	}
}

func TestMPMCConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(256)
	numProducers := 4
	numConsumers := 4
	itemsPerProducer := 200

	var wg sync.WaitGroup
	var produced, consumedCount sync.Map

	// Producers
	wg.Add(numProducers)
	for p := range numProducers {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProducer {
				v := uintptr(id*10000 + i + 1)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				produced.Store(v, true)
				backoff.Reset()
			}
		}(p)
	}

	// Consumers
	wg.Add(numConsumers)
	for range numConsumers {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for {
				elem, err := q.Dequeue()
				if err == nil {
					consumedCount.Store(elem, true)
					backoff.Reset()
				} else {
					// Check if producers done
					prodCount := 0
					produced.Range(func(_, _ any) bool {
						prodCount++
						return true
					})
					if prodCount >= numProducers*itemsPerProducer {
						return
					}
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Count consumed
	count := 0
	consumedCount.Range(func(_, _ any) bool {
		count++
		return true
	})

	expected := numProducers * itemsPerProducer
	if count != expected {
		t.Fatalf("consumed: got %d, want %d", count, expected)
	}
}

// =============================================================================
// Wraparound Tests
// =============================================================================

func TestSPSCWraparound(t *testing.T) {
	q := lfq.NewSPSC[int](4)

	// Multiple cycles through the buffer
	for cycle := range 10 {
		// Enqueue 4 items
		for i := range 4 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		// Dequeue 4 items
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

func TestSPSCPtrBasic(t *testing.T) {
	q := lfq.NewSPSCPtr(4)

	// Test Cap()
	if q.Cap() != 4 {
		t.Fatalf("Cap: got %d, want 4", q.Cap())
	}

	// Test basic enqueue/dequeue
	val1 := 42
	val2 := 100
	if err := q.Enqueue(unsafe.Pointer(&val1)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := q.Enqueue(unsafe.Pointer(&val2)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	ptr1, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if *(*int)(ptr1) != 42 {
		t.Fatalf("got %d, want 42", *(*int)(ptr1))
	}

	ptr2, err := q.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if *(*int)(ptr2) != 100 {
		t.Fatalf("got %d, want 100", *(*int)(ptr2))
	}

	// Test empty dequeue
	_, err = q.Dequeue()
	if err != lfq.ErrWouldBlock {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}
}

func TestSPSCPtrWraparound(t *testing.T) {
	q := lfq.NewSPSCPtr(4)
	values := make([]int, 4)

	// Multiple cycles through the buffer
	for cycle := range 10 {
		// Enqueue 4 items
		for i := range 4 {
			values[i] = cycle*100 + i
			if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}
		// Dequeue 4 items
		for i := range 4 {
			ptr, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue: %v", cycle, err)
			}
			expected := cycle*100 + i
			if *(*int)(ptr) != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, *(*int)(ptr), expected)
			}
		}
	}
}

func TestSPSCPtrFull(t *testing.T) {
	q := lfq.NewSPSCPtr(4)
	values := make([]int, 5)

	// Fill queue
	for i := range 4 {
		values[i] = i
		if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Should fail when full
	values[4] = 99
	if err := q.Enqueue(unsafe.Pointer(&values[4])); err != lfq.ErrWouldBlock {
		t.Fatalf("expected ErrWouldBlock, got %v", err)
	}

	// Dequeue one and try again
	q.Dequeue()
	if err := q.Enqueue(unsafe.Pointer(&values[4])); err != nil {
		t.Fatalf("Enqueue after dequeue: %v", err)
	}
}

func TestMPMCWraparound(t *testing.T) {
	q := lfq.NewMPMC[int](4)

	// Multiple cycles through the buffer
	for cycle := range 10 {
		// Enqueue 4 items
		for i := range 4 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue: %v", cycle, err)
			}
		}

		// Dequeue 4 items
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

func TestCompactWraparound(t *testing.T) {
	q := lfq.NewMPMCCompactIndirect(4)

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
// MPMC Retry Path Coverage
// =============================================================================

func TestMPMCEnqueueRetryPath(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(16)
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

func TestMPMCDequeueRetryPath(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(16)
	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(2)

	// Producer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range iterations {
			v := uintptr(i + 1)
			for q.Enqueue(v) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Consumer
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		consumed := 0
		for consumed < iterations {
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

// =============================================================================
// High-Contention Stress Tests for Retry Paths
// =============================================================================

func TestMPSCHighContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPSC[int](16) // Small capacity increases contention
	const numProducers = 8
	const itemsPerProducer = 200

	var wg sync.WaitGroup
	var produced atomix.Int64

	// Many producers competing for few slots
	wg.Add(numProducers)
	for p := range numProducers {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProducer {
				v := id*10000 + i
				for {
					err := q.Enqueue(&v)
					if err == nil {
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}(p)
	}

	// Single consumer draining
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				_, err := q.Dequeue()
				if err == nil {
					produced.Add(1)
				}
			}
		}
	}()

	wg.Wait()

	// Drain remaining
	for {
		_, err := q.Dequeue()
		if errors.Is(err, lfq.ErrWouldBlock) {
			break
		}
		if err == nil {
			produced.Add(1)
		}
	}
	close(done)

	expected := int64(numProducers * itemsPerProducer)
	if produced.Load() != expected {
		t.Fatalf("produced: got %d, want %d", produced.Load(), expected)
	}
}

func TestSPMCHighContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewSPMCCompactIndirect(64) // Compact queue for performance
	const numConsumers = 8
	const totalItems = 2000

	var consumed atomix.Int64
	var wg sync.WaitGroup

	// Single producer
	go func() {
		backoff := iox.Backoff{}
		for i := range totalItems {
			for {
				if q.Enqueue(uintptr(i)) == nil {
					backoff.Reset()
					break
				}
				backoff.Wait()
			}
		}
	}()

	// Many consumers competing
	wg.Add(numConsumers)
	for range numConsumers {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < totalItems {
				_, err := q.Dequeue()
				if err == nil {
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if consumed.Load() != totalItems {
		t.Fatalf("consumed: got %d, want %d", consumed.Load(), totalItems)
	}
}

func TestMPMCHighContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(64) // Compact queue for performance
	const numWorkers = 8
	const opsPerWorker = 200

	var wg sync.WaitGroup
	var totalOps atomix.Int64

	// Each worker does both enqueue and dequeue
	wg.Add(numWorkers)
	for w := range numWorkers {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range opsPerWorker {
				v := uintptr(id*10000 + i)
				// Try enqueue
				for {
					if q.Enqueue(v) == nil {
						totalOps.Add(1)
						backoff.Reset()
						break
					}
					// Dequeue to make room
					if _, err := q.Dequeue(); err == nil {
						totalOps.Add(1)
					}
					backoff.Wait()
				}
			}
		}(w)
	}

	wg.Wait()

	// Drain remaining
	for {
		if _, err := q.Dequeue(); errors.Is(err, lfq.ErrWouldBlock) {
			break
		}
	}

	if totalOps.Load() < int64(numWorkers*opsPerWorker) {
		t.Fatalf("totalOps: got %d, want >= %d", totalOps.Load(), numWorkers*opsPerWorker)
	}
}

func TestMPMCBurstPattern(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(16)
	const bursts = 8
	const burstSize = 6

	backoff := iox.Backoff{}
	for b := range bursts {
		base := b * 100
		// Burst enqueue
		for i := range burstSize {
			v := uintptr(base + i)
			for {
				if q.Enqueue(v) == nil {
					backoff.Reset()
					break
				}
				if _, err := q.Dequeue(); err == nil {
					backoff.Reset()
					break
				}
				backoff.Wait()
			}
		}
		// Drain after burst
		for {
			if _, err := q.Dequeue(); errors.Is(err, lfq.ErrWouldBlock) {
				break
			}
		}
	}
}

// =============================================================================
// Coverage: Stale Slot Path (MPSC)
// =============================================================================

// TestCoverageStaleSlotMPSC exercises the stale-slot return path in MPSC Enqueue.
// Multiple producers pass the early fullness check simultaneously via FAA,
// causing some to claim positions beyond available capacity where
// slotCycle < expectedCycle. These producers return ErrWouldBlock through the
// stale-slot guard rather than the early fullness check.
func TestCoverageStaleSlotMPSC(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("Generic", func(t *testing.T) {
		for range 500 {
			q := lfq.NewMPSC[int](2)
			v := 1
			q.Enqueue(&v)

			const P = 16
			var wg sync.WaitGroup
			wg.Add(P)
			start := make(chan struct{})

			for range P {
				go func() {
					defer wg.Done()
					val := 42
					<-start
					q.Enqueue(&val)
				}()
			}
			close(start)
			wg.Wait()
		}
	})

	t.Run("Indirect", func(t *testing.T) {
		for range 500 {
			q := lfq.NewMPSCIndirect(2)
			q.Enqueue(1)

			const P = 16
			var wg sync.WaitGroup
			wg.Add(P)
			start := make(chan struct{})

			for range P {
				go func() {
					defer wg.Done()
					<-start
					q.Enqueue(42)
				}()
			}
			close(start)
			wg.Wait()
		}
	})

	t.Run("Ptr", func(t *testing.T) {
		for range 500 {
			q := lfq.NewMPSCPtr(2)
			val := 1
			q.Enqueue(unsafe.Pointer(&val))

			const P = 16
			var wg sync.WaitGroup
			wg.Add(P)
			start := make(chan struct{})

			for range P {
				go func() {
					defer wg.Done()
					v := 42
					<-start
					q.Enqueue(unsafe.Pointer(&v))
				}()
			}
			close(start)
			wg.Wait()
		}
	})
}

// =============================================================================
// Coverage: emptyFlag Path (MPMC Compact)
// =============================================================================

// TestCoverageEmptyFlagMPMCCompact exercises the transient emptyFlag path in
// MPMCCompactIndirect.Dequeue. With a small capacity and many concurrent
// workers, consumers occasionally read a slot still marked with emptyFlag
// from a previous round (producer allocated via tail advance but has not yet
// written the value). This covers mpmc_compact.go:116-118.
func TestCoverageEmptyFlagMPMCCompact(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	q := lfq.NewMPMCCompactIndirect(4)
	const workers = 8
	const opsPerWorker = 500

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range opsPerWorker {
				v := uintptr(i + 1)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				backoff.Reset()
				for {
					_, err := q.Dequeue()
					if err == nil {
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}()
	}
	wg.Wait()
}

// =============================================================================
// Coverage: Drain + Threshold Path (SPMC)
// =============================================================================

// TestCoverageDrainThreshold exercises the threshold exhaustion path in SPMC
// Dequeue when draining is active. With many consumers racing on a small queue
// while the producer enqueues, consumers encounter stale slots that decrement
// the threshold counter. After Drain() is called, consumers hitting
// threshold <= 0 && !draining evaluate to false and continue past the guard,
// covering the draining=true branch of the threshold check.
func TestCoverageDrainThreshold(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("Generic", func(t *testing.T) {
		q := lfq.NewSPMC[int](8)
		const totalItems = 200
		const numConsumers = 8

		var consumed atomix.Int64
		var wg sync.WaitGroup

		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for consumed.Load() < totalItems {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}

		backoff := iox.Backoff{}
		for i := range totalItems {
			v := i
			for q.Enqueue(&v) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
		q.Drain()

		wg.Wait()

		if consumed.Load() != totalItems {
			t.Fatalf("consumed: got %d, want %d", consumed.Load(), totalItems)
		}
	})

	t.Run("Indirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(8)
		const totalItems = 200
		const numConsumers = 8

		var consumed atomix.Int64
		var wg sync.WaitGroup

		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for consumed.Load() < totalItems {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}

		backoff := iox.Backoff{}
		for i := range totalItems {
			for q.Enqueue(uintptr(i+1)) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
		q.Drain()

		wg.Wait()

		if consumed.Load() != totalItems {
			t.Fatalf("consumed: got %d, want %d", consumed.Load(), totalItems)
		}
	})

	t.Run("Ptr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(8)
		const totalItems = 200
		const numConsumers = 8
		vals := make([]int, totalItems)

		var consumed atomix.Int64
		var drained atomix.Bool
		var wg sync.WaitGroup

		wg.Add(numConsumers)
		for range numConsumers {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				emptyCount := 0
				for consumed.Load() < totalItems {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						emptyCount = 0
						backoff.Reset()
					} else {
						if drained.Load() {
							emptyCount++
							if emptyCount > 100 {
								return
							}
						}
						backoff.Wait()
					}
				}
			}()
		}

		backoff := iox.Backoff{}
		for i := range totalItems {
			vals[i] = i + 1
			for q.Enqueue(unsafe.Pointer(&vals[i])) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
		q.Drain()
		drained.Store(true)

		wg.Wait()

		if consumed.Load() < totalItems-int64(numConsumers) {
			t.Fatalf("consumed: got %d, want at least %d", consumed.Load(), totalItems-numConsumers)
		}
	})

	// DrainEarly sub-tests: call Drain() BEFORE starting consumers so
	// consumers always see draining=true in the threshold check.

	t.Run("GenericDrainEarly", func(t *testing.T) {
		q := lfq.NewSPMC[int](8)
		for i := range 8 {
			v := i + 1
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}
		q.Drain()

		var consumed atomix.Int64
		var wg sync.WaitGroup
		wg.Add(8)
		for range 8 {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for consumed.Load() < 8 {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}
		wg.Wait()
		if consumed.Load() != 8 {
			t.Fatalf("consumed: got %d, want 8", consumed.Load())
		}
	})

	t.Run("IndirectDrainEarly", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(8)
		for i := range 8 {
			if err := q.Enqueue(uintptr(i + 1)); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}
		q.Drain()

		var consumed atomix.Int64
		var wg sync.WaitGroup
		wg.Add(8)
		for range 8 {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for consumed.Load() < 8 {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}
		wg.Wait()
		if consumed.Load() != 8 {
			t.Fatalf("consumed: got %d, want 8", consumed.Load())
		}
	})

	t.Run("PtrDrainEarly", func(t *testing.T) {
		q := lfq.NewSPMCPtr(8)
		vals := make([]int, 8)
		for i := range 8 {
			vals[i] = i + 1
			if err := q.Enqueue(unsafe.Pointer(&vals[i])); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}
		q.Drain()

		var consumed atomix.Int64
		var wg sync.WaitGroup
		wg.Add(8)
		for range 8 {
			go func() {
				defer wg.Done()
				backoff := iox.Backoff{}
				for consumed.Load() < 8 {
					_, err := q.Dequeue()
					if err == nil {
						consumed.Add(1)
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}()
		}
		wg.Wait()
		if consumed.Load() != 8 {
			t.Fatalf("consumed: got %d, want 8", consumed.Load())
		}
	})
}

// =============================================================================
// Coverage: MPSC Drain (API completeness)
// =============================================================================

// TestCoverageMPSCDrain exercises the Drain() method on MPSC queues.
// MPSC Drain is a hint for graceful shutdown — it signals that no more
// enqueues will occur. The method itself just sets a flag; the coverage
// ensures the API is exercised.
func TestCoverageMPSCDrain(t *testing.T) {
	t.Run("Generic", func(t *testing.T) {
		q := lfq.NewMPSC[int](4)
		v := 42
		q.Enqueue(&v)
		q.Drain()
		// Drain doesn't affect dequeue behavior for MPSC
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue after Drain: %v", err)
		}
		if got != 42 {
			t.Fatalf("got %d, want 42", got)
		}
	})

	t.Run("Indirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(4)
		q.Enqueue(42)
		q.Drain()
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue after Drain: %v", err)
		}
		if got != 42 {
			t.Fatalf("got %d, want 42", got)
		}
	})

	t.Run("Ptr", func(t *testing.T) {
		q := lfq.NewMPSCPtr(4)
		v := 42
		q.Enqueue(unsafe.Pointer(&v))
		q.Drain()
		got, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue after Drain: %v", err)
		}
		if got != unsafe.Pointer(&v) {
			t.Fatalf("got %v, want %v", got, unsafe.Pointer(&v))
		}
	})
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkSPSCEnqueueDequeue(b *testing.B) {
	q := lfq.NewSPSC[int](1024)
	v := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkMPMCEnqueueDequeue(b *testing.B) {
	q := lfq.NewMPMC[int](1024)
	v := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkMPMCParallel(b *testing.B) {
	q := lfq.NewMPMC[int](1024)
	v := 42

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(&v)
			q.Dequeue()
		}
	})
}

// Latency comparison benchmarks
func BenchmarkLatency(b *testing.B) {
	benchmarks := []struct {
		name string
		newQ func() interface{ Enqueue(*int) error }
		deq  func(q any) (int, error)
	}{
		{"SPSC", func() interface{ Enqueue(*int) error } { return lfq.NewSPSC[int](64) }, func(q any) (int, error) { return q.(*lfq.SPSC[int]).Dequeue() }},
		{"MPSC", func() interface{ Enqueue(*int) error } { return lfq.NewMPSC[int](64) }, func(q any) (int, error) { return q.(*lfq.MPSC[int]).Dequeue() }},
		{"SPMC", func() interface{ Enqueue(*int) error } { return lfq.NewSPMC[int](64) }, func(q any) (int, error) { return q.(*lfq.SPMC[int]).Dequeue() }},
		{"MPMC", func() interface{ Enqueue(*int) error } { return lfq.NewMPMC[int](64) }, func(q any) (int, error) { return q.(*lfq.MPMC[int]).Dequeue() }},
	}

	for bm := range slices.Values(benchmarks) {
		b.Run(bm.name, func(b *testing.B) {
			q := bm.newQ()
			v := 42
			b.ResetTimer()
			for range b.N {
				q.Enqueue(&v)
				bm.deq(q)
			}
		})
	}
}

// Batch operation benchmarks
func BenchmarkBatch(b *testing.B) {
	benchmarks := []struct {
		name  string
		batch int
	}{
		{"Batch1", 1},
		{"Batch10", 10},
		{"Batch100", 100},
	}

	for bm := range slices.Values(benchmarks) {
		b.Run("SPSC_"+bm.name, func(b *testing.B) {
			q := lfq.NewSPSC[int](1024)
			v := 42
			b.ResetTimer()
			for range b.N {
				for range bm.batch {
					q.Enqueue(&v)
				}
				for range bm.batch {
					q.Dequeue()
				}
			}
		})
		b.Run("MPMC_"+bm.name, func(b *testing.B) {
			q := lfq.NewMPMC[int](1024)
			v := 42
			b.ResetTimer()
			for range b.N {
				for range bm.batch {
					q.Enqueue(&v)
				}
				for range bm.batch {
					q.Dequeue()
				}
			}
		})
	}
}

// Capacity scaling benchmarks
func BenchmarkMPMCCapacity(b *testing.B) {
	capacities := []int{64, 256, 1024, 4096, 16384}

	for cap := range slices.Values(capacities) {
		b.Run(fmt.Sprintf("Cap%d", cap), func(b *testing.B) {
			q := lfq.NewMPMC[int](cap)
			v := 42
			b.ResetTimer()
			for range b.N {
				q.Enqueue(&v)
				q.Dequeue()
			}
		})
	}
}

// Throughput benchmarks
func BenchmarkThroughput(b *testing.B) {
	benchmarks := []struct {
		name string
		newQ func() (enq func(*int) error, deq func() (int, error))
	}{
		{"SPSC", func() (func(*int) error, func() (int, error)) {
			q := lfq.NewSPSC[int](4096)
			return q.Enqueue, q.Dequeue
		}},
		{"MPMC", func() (func(*int) error, func() (int, error)) {
			q := lfq.NewMPMC[int](4096)
			return q.Enqueue, q.Dequeue
		}},
	}

	for bm := range slices.Values(benchmarks) {
		b.Run(bm.name, func(b *testing.B) {
			enq, deq := bm.newQ()
			done := make(chan struct{})

			// Consumer goroutine
			go func() {
				for {
					select {
					case <-done:
						return
					default:
						deq()
					}
				}
			}()

			b.ResetTimer()
			v := 42
			for range b.N {
				for enq(&v) != nil {
				}
			}
			b.StopTimer()
			close(done)
		})
	}
}

// Indirect/Ptr benchmarks
func BenchmarkIndirectPtrVariants(b *testing.B) {
	b.Run("SPSCIndirect", func(b *testing.B) {
		q := lfq.NewSPSCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})
	b.Run("SPSCPtr", func(b *testing.B) {
		q := lfq.NewSPSCPtr(1024)
		val := 42
		b.ResetTimer()
		for range b.N {
			q.Enqueue(unsafe.Pointer(&val))
			q.Dequeue()
		}
	})
	b.Run("MPSCIndirect", func(b *testing.B) {
		q := lfq.NewMPSCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})
	b.Run("MPSCPtr", func(b *testing.B) {
		q := lfq.NewMPSCPtr(1024)
		val := 42
		b.ResetTimer()
		for range b.N {
			q.Enqueue(unsafe.Pointer(&val))
			q.Dequeue()
		}
	})
	b.Run("SPMCIndirect", func(b *testing.B) {
		q := lfq.NewSPMCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})
	b.Run("SPMCPtr", func(b *testing.B) {
		q := lfq.NewSPMCPtr(1024)
		val := 42
		b.ResetTimer()
		for range b.N {
			q.Enqueue(unsafe.Pointer(&val))
			q.Dequeue()
		}
	})
	b.Run("MPMCIndirect", func(b *testing.B) {
		q := lfq.NewMPMCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})
	b.Run("MPMCPtr", func(b *testing.B) {
		q := lfq.NewMPMCPtr(1024)
		val := 42
		b.ResetTimer()
		for range b.N {
			q.Enqueue(unsafe.Pointer(&val))
			q.Dequeue()
		}
	})
}
