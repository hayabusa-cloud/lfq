// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"errors"
	"slices"
	"testing"
	"unsafe"

	"code.hybscloud.com/lfq"
)

// =============================================================================
// Cross-Queue Consistency Tests
//
// These tests verify that all queue variants (generic, compact, indirect, ptr)
// behave identically for the same operation sequence. This ensures the different
// implementations are interchangeable at the semantic level.
// =============================================================================

// queueOps defines a generic interface for testing queue operations.
type queueOps struct {
	name    string
	cap     func() int
	enqueue func(int) error
	dequeue func() (int, error)
	isFull  func() bool
	isEmpty func() bool
}

// =============================================================================
// MPSC Consistency
// =============================================================================

// TestMPSCConsistency verifies all MPSC variants behave identically.
func TestMPSCConsistency(t *testing.T) {
	const capacity = 8

	// Create all MPSC variants
	genericQ := lfq.NewMPSC[int](capacity)
	seqQ := lfq.NewMPSCSeq[int](capacity)
	indirectQ := lfq.NewMPSCIndirect(capacity)
	compactQ := lfq.NewMPSCCompactIndirect(capacity)
	ptrQ := lfq.NewMPSCPtr(capacity)

	// Pre-allocate values for pointer queue
	ptrVals := make([]int, capacity+1)

	queues := []queueOps{
		{
			name:    "MPSC[int]",
			cap:     genericQ.Cap,
			enqueue: func(v int) error { return genericQ.Enqueue(&v) },
			dequeue: func() (int, error) { return genericQ.Dequeue() },
		},
		{
			name:    "MPSCSeq[int]",
			cap:     seqQ.Cap,
			enqueue: func(v int) error { return seqQ.Enqueue(&v) },
			dequeue: func() (int, error) { return seqQ.Dequeue() },
		},
		{
			name:    "MPSCIndirect",
			cap:     indirectQ.Cap,
			enqueue: func(v int) error { return indirectQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := indirectQ.Dequeue(); return int(u), e },
		},
		{
			name:    "MPSCCompactIndirect",
			cap:     compactQ.Cap,
			enqueue: func(v int) error { return compactQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := compactQ.Dequeue(); return int(u), e },
		},
		{
			name: "MPSCPtr",
			cap:  ptrQ.Cap,
			enqueue: func(v int) error {
				ptrVals[v%len(ptrVals)] = v
				return ptrQ.Enqueue(unsafe.Pointer(&ptrVals[v%len(ptrVals)]))
			},
			dequeue: func() (int, error) {
				p, e := ptrQ.Dequeue()
				if e != nil {
					return 0, e
				}
				return *(*int)(p), nil
			},
		},
	}

	runConsistencyTests(t, queues, capacity)
}

// =============================================================================
// SPMC Consistency
// =============================================================================

// TestSPMCConsistency verifies all SPMC variants behave identically.
func TestSPMCConsistency(t *testing.T) {
	const capacity = 8

	genericQ := lfq.NewSPMC[int](capacity)
	seqQ := lfq.NewSPMCSeq[int](capacity)
	indirectQ := lfq.NewSPMCIndirect(capacity)
	compactQ := lfq.NewSPMCCompactIndirect(capacity)
	ptrQ := lfq.NewSPMCPtr(capacity)

	ptrVals := make([]int, capacity+1)

	queues := []queueOps{
		{
			name:    "SPMC[int]",
			cap:     genericQ.Cap,
			enqueue: func(v int) error { return genericQ.Enqueue(&v) },
			dequeue: func() (int, error) { return genericQ.Dequeue() },
		},
		{
			name:    "SPMCSeq[int]",
			cap:     seqQ.Cap,
			enqueue: func(v int) error { return seqQ.Enqueue(&v) },
			dequeue: func() (int, error) { return seqQ.Dequeue() },
		},
		{
			name:    "SPMCIndirect",
			cap:     indirectQ.Cap,
			enqueue: func(v int) error { return indirectQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := indirectQ.Dequeue(); return int(u), e },
		},
		{
			name:    "SPMCCompactIndirect",
			cap:     compactQ.Cap,
			enqueue: func(v int) error { return compactQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := compactQ.Dequeue(); return int(u), e },
		},
		{
			name: "SPMCPtr",
			cap:  ptrQ.Cap,
			enqueue: func(v int) error {
				ptrVals[v%len(ptrVals)] = v
				return ptrQ.Enqueue(unsafe.Pointer(&ptrVals[v%len(ptrVals)]))
			},
			dequeue: func() (int, error) {
				p, e := ptrQ.Dequeue()
				if e != nil {
					return 0, e
				}
				return *(*int)(p), nil
			},
		},
	}

	runConsistencyTests(t, queues, capacity)
}

// =============================================================================
// MPMC Consistency
// =============================================================================

// TestMPMCConsistency verifies all MPMC variants behave identically.
func TestMPMCConsistency(t *testing.T) {
	const capacity = 8

	genericQ := lfq.NewMPMC[int](capacity)
	seqQ := lfq.NewMPMCSeq[int](capacity)
	indirectQ := lfq.NewMPMCIndirect(capacity)
	compactQ := lfq.NewMPMCCompactIndirect(capacity)
	ptrQ := lfq.NewMPMCPtr(capacity)

	ptrVals := make([]int, capacity+1)

	queues := []queueOps{
		{
			name:    "MPMC[int]",
			cap:     genericQ.Cap,
			enqueue: func(v int) error { return genericQ.Enqueue(&v) },
			dequeue: func() (int, error) { return genericQ.Dequeue() },
		},
		{
			name:    "MPMCSeq[int]",
			cap:     seqQ.Cap,
			enqueue: func(v int) error { return seqQ.Enqueue(&v) },
			dequeue: func() (int, error) { return seqQ.Dequeue() },
		},
		{
			name:    "MPMCIndirect",
			cap:     indirectQ.Cap,
			enqueue: func(v int) error { return indirectQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := indirectQ.Dequeue(); return int(u), e },
		},
		{
			name:    "MPMCCompactIndirect",
			cap:     compactQ.Cap,
			enqueue: func(v int) error { return compactQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := compactQ.Dequeue(); return int(u), e },
		},
		{
			name: "MPMCPtr",
			cap:  ptrQ.Cap,
			enqueue: func(v int) error {
				ptrVals[v%len(ptrVals)] = v
				return ptrQ.Enqueue(unsafe.Pointer(&ptrVals[v%len(ptrVals)]))
			},
			dequeue: func() (int, error) {
				p, e := ptrQ.Dequeue()
				if e != nil {
					return 0, e
				}
				return *(*int)(p), nil
			},
		},
	}

	runConsistencyTests(t, queues, capacity)
}

// =============================================================================
// SPSC Consistency
// =============================================================================

// TestSPSCConsistency verifies all SPSC variants behave identically.
func TestSPSCConsistency(t *testing.T) {
	const capacity = 8

	genericQ := lfq.NewSPSC[int](capacity)
	indirectQ := lfq.NewSPSCIndirect(capacity)
	ptrQ := lfq.NewSPSCPtr(capacity)

	ptrVals := make([]int, capacity+1)

	queues := []queueOps{
		{
			name:    "SPSC[int]",
			cap:     genericQ.Cap,
			enqueue: func(v int) error { return genericQ.Enqueue(&v) },
			dequeue: func() (int, error) { return genericQ.Dequeue() },
		},
		{
			name:    "SPSCIndirect",
			cap:     indirectQ.Cap,
			enqueue: func(v int) error { return indirectQ.Enqueue(uintptr(v)) },
			dequeue: func() (int, error) { u, e := indirectQ.Dequeue(); return int(u), e },
		},
		{
			name: "SPSCPtr",
			cap:  ptrQ.Cap,
			enqueue: func(v int) error {
				ptrVals[v%len(ptrVals)] = v
				return ptrQ.Enqueue(unsafe.Pointer(&ptrVals[v%len(ptrVals)]))
			},
			dequeue: func() (int, error) {
				p, e := ptrQ.Dequeue()
				if e != nil {
					return 0, e
				}
				return *(*int)(p), nil
			},
		},
	}

	runConsistencyTests(t, queues, capacity)
}

// =============================================================================
// Consistency Test Implementation
// =============================================================================

// runConsistencyTests executes the same operation sequence on all queues.
func runConsistencyTests(t *testing.T, queues []queueOps, capacity int) {
	t.Helper()

	for q := range slices.Values(queues) {
		t.Run(q.name, func(t *testing.T) {
			// Test 1: Capacity is correct
			if got := q.cap(); got != capacity {
				t.Errorf("Cap: got %d, want %d", got, capacity)
			}

			// Test 2: Empty dequeue returns ErrWouldBlock
			if _, err := q.dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
				t.Errorf("Dequeue on empty: got %v, want ErrWouldBlock", err)
			}

			// Test 3: Fill to capacity
			for i := range capacity {
				if err := q.enqueue(i + 100); err != nil {
					t.Fatalf("Enqueue(%d): %v", i, err)
				}
			}

			// Test 4: Full enqueue returns ErrWouldBlock
			if err := q.enqueue(999); !errors.Is(err, lfq.ErrWouldBlock) {
				t.Errorf("Enqueue on full: got %v, want ErrWouldBlock", err)
			}

			// Test 5: Drain in FIFO order
			for i := range capacity {
				val, err := q.dequeue()
				if err != nil {
					t.Fatalf("Dequeue(%d): %v", i, err)
				}
				expected := i + 100
				if val != expected {
					t.Errorf("Dequeue(%d): got %d, want %d", i, val, expected)
				}
			}

			// Test 6: Empty after drain
			if _, err := q.dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
				t.Errorf("Dequeue after drain: got %v, want ErrWouldBlock", err)
			}
		})
	}
}

// =============================================================================
// Wraparound Consistency
// =============================================================================

// TestWraparoundConsistency verifies all variants handle wraparound identically.
func TestWraparoundConsistency(t *testing.T) {
	const (
		capacity = 4
		cycles   = 100
	)

	tests := []struct {
		name    string
		enqueue func(int) error
		dequeue func() (int, error)
	}{
		{
			name: "MPMC",
			enqueue: func() func(int) error {
				q := lfq.NewMPMC[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }
			}(),
			dequeue: func() func() (int, error) {
				q := lfq.NewMPMC[int](capacity)
				return q.Dequeue
			}(),
		},
		{
			name: "MPMCSeq",
			enqueue: func() func(int) error {
				q := lfq.NewMPMCSeq[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }
			}(),
			dequeue: func() func() (int, error) {
				q := lfq.NewMPMCSeq[int](capacity)
				return q.Dequeue
			}(),
		},
		{
			name: "MPMCIndirect",
			enqueue: func() func(int) error {
				q := lfq.NewMPMCIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) }
			}(),
			dequeue: func() func() (int, error) {
				q := lfq.NewMPMCIndirect(capacity)
				return func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			}(),
		},
		{
			name: "MPMCCompact",
			enqueue: func() func(int) error {
				q := lfq.NewMPMCCompactIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) }
			}(),
			dequeue: func() func() (int, error) {
				q := lfq.NewMPMCCompactIndirect(capacity)
				return func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh queue for wraparound test
			var enqueue func(int) error
			var dequeue func() (int, error)

			switch tt.name {
			case "MPMC":
				q := lfq.NewMPMC[int](capacity)
				enqueue = func(v int) error { return q.Enqueue(&v) }
				dequeue = q.Dequeue
			case "MPMCSeq":
				q := lfq.NewMPMCSeq[int](capacity)
				enqueue = func(v int) error { return q.Enqueue(&v) }
				dequeue = q.Dequeue
			case "MPMCIndirect":
				q := lfq.NewMPMCIndirect(capacity)
				enqueue = func(v int) error { return q.Enqueue(uintptr(v)) }
				dequeue = func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			case "MPMCCompact":
				q := lfq.NewMPMCCompactIndirect(capacity)
				enqueue = func(v int) error { return q.Enqueue(uintptr(v)) }
				dequeue = func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			}

			// Run wraparound cycles
			for cycle := range cycles {
				// Fill
				for i := range capacity {
					v := cycle*100 + i
					if err := enqueue(v); err != nil {
						t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
					}
				}

				// Drain with FIFO verification
				for i := range capacity {
					val, err := dequeue()
					if err != nil {
						t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
					}
					expected := cycle*100 + i
					if val != expected {
						t.Fatalf("cycle %d: got %d, want %d", cycle, val, expected)
					}
				}
			}
		})
	}
}

// =============================================================================
// Zero Value Consistency
// =============================================================================

// TestZeroValueConsistency verifies all variants correctly handle zero values.
func TestZeroValueConsistency(t *testing.T) {
	const capacity = 4

	tests := []struct {
		name    string
		enqueue func(int) error
		dequeue func() (int, error)
	}{
		{
			name: "MPMC",
			enqueue: func() func(int) error {
				q := lfq.NewMPMC[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }
			}(),
		},
		{
			name: "MPMCSeq",
			enqueue: func() func(int) error {
				q := lfq.NewMPMCSeq[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }
			}(),
		},
		{
			name: "MPMCIndirect",
			enqueue: func() func(int) error {
				q := lfq.NewMPMCIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) }
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var enqueue func(int) error
			var dequeue func() (int, error)

			switch tt.name {
			case "MPMC":
				q := lfq.NewMPMC[int](capacity)
				enqueue = func(v int) error { return q.Enqueue(&v) }
				dequeue = q.Dequeue
			case "MPMCSeq":
				q := lfq.NewMPMCSeq[int](capacity)
				enqueue = func(v int) error { return q.Enqueue(&v) }
				dequeue = q.Dequeue
			case "MPMCIndirect":
				q := lfq.NewMPMCIndirect(capacity)
				enqueue = func(v int) error { return q.Enqueue(uintptr(v)) }
				dequeue = func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			}

			// Enqueue zeros
			for range capacity {
				if err := enqueue(0); err != nil {
					t.Fatalf("Enqueue(0): %v", err)
				}
			}

			// Full
			if err := enqueue(0); !errors.Is(err, lfq.ErrWouldBlock) {
				t.Fatalf("Enqueue on full: got %v, want ErrWouldBlock", err)
			}

			// Dequeue zeros
			for i := range capacity {
				val, err := dequeue()
				if err != nil {
					t.Fatalf("Dequeue(%d): %v", i, err)
				}
				if val != 0 {
					t.Fatalf("Dequeue(%d): got %d, want 0", i, val)
				}
			}

			// Empty
			if _, err := dequeue(); !errors.Is(err, lfq.ErrWouldBlock) {
				t.Fatalf("Dequeue on empty: got %v, want ErrWouldBlock", err)
			}
		})
	}
}

// =============================================================================
// Interleaved Operations Consistency
// =============================================================================

// TestInterleavedConsistency tests interleaved enqueue/dequeue operations.
func TestInterleavedConsistency(t *testing.T) {
	const capacity = 8

	tests := []struct {
		name string
		newQ func() (func(int) error, func() (int, error))
	}{
		{
			name: "MPMC",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewMPMC[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }, q.Dequeue
			},
		},
		{
			name: "MPMCSeq",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewMPMCSeq[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }, q.Dequeue
			},
		},
		{
			name: "MPMCIndirect",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewMPMCIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) },
					func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			},
		},
		{
			name: "MPMCCompact",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewMPMCCompactIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) },
					func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			},
		},
		{
			name: "SPSC",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewSPSC[int](capacity)
				return func(v int) error { return q.Enqueue(&v) }, q.Dequeue
			},
		},
		{
			name: "SPSCIndirect",
			newQ: func() (func(int) error, func() (int, error)) {
				q := lfq.NewSPSCIndirect(capacity)
				return func(v int) error { return q.Enqueue(uintptr(v)) },
					func() (int, error) { u, e := q.Dequeue(); return int(u), e }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enqueue, dequeue := tt.newQ()

			// Pattern: enqueue 4, dequeue 4 (balanced to avoid overflow)
			var nextEnq, nextDeq int
			for round := range 1000 {
				// Enqueue 4
				for i := range 4 {
					if err := enqueue(nextEnq); err != nil {
						t.Fatalf("round %d: Enqueue(%d): %v", round, i, err)
					}
					nextEnq++
				}

				// Dequeue 4
				for i := range 4 {
					val, err := dequeue()
					if err != nil {
						t.Fatalf("round %d: Dequeue(%d): %v", round, i, err)
					}
					if val != nextDeq {
						t.Fatalf("round %d: got %d, want %d", round, val, nextDeq)
					}
					nextDeq++
				}
			}

			// Drain remaining
			for {
				val, err := dequeue()
				if errors.Is(err, lfq.ErrWouldBlock) {
					break
				}
				if err != nil {
					t.Fatalf("final drain: %v", err)
				}
				if val != nextDeq {
					t.Fatalf("final drain: got %d, want %d", val, nextDeq)
				}
				nextDeq++
			}

			// Verify all items consumed
			if nextDeq != nextEnq {
				t.Errorf("items lost: enqueued %d, dequeued %d", nextEnq, nextDeq)
			}
		})
	}
}
