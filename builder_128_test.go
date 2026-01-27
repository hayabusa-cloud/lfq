// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"testing"

	"code.hybscloud.com/lfq"
)

// =============================================================================
// Builder Default Algorithm Tests (FAA-based 128-bit is default for Indirect/Ptr)
// =============================================================================

func TestBuilderDefaultIndirect(t *testing.T) {
	// MPMC Indirect uses FAA-based 128-bit by default
	q := lfq.New(7).BuildIndirect()
	if q.Cap() != 8 {
		t.Fatalf("MPMC Indirect Cap: got %d, want 8", q.Cap())
	}

	// Test basic operation
	if err := q.Enqueue(42); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if val != 42 {
		t.Fatalf("got %d, want 42", val)
	}
}

func TestBuilderDefaultIndirectMPSC(t *testing.T) {
	// MPSC Indirect (SingleConsumer) uses FAA-based 128-bit by default
	q := lfq.New(7).SingleConsumer().BuildIndirect()
	if q.Cap() != 8 {
		t.Fatalf("MPSC Indirect Cap: got %d, want 8", q.Cap())
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if val != uintptr(i) {
			t.Fatalf("got %d, want %d", val, i)
		}
	}
}

func TestBuilderDefaultIndirectSPMC(t *testing.T) {
	// SPMC Indirect (SingleProducer) uses FAA-based 128-bit by default
	q := lfq.New(7).SingleProducer().BuildIndirect()
	if q.Cap() != 8 {
		t.Fatalf("SPMC Indirect Cap: got %d, want 8", q.Cap())
	}

	for i := range 4 {
		if err := q.Enqueue(uintptr(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	for i := range 4 {
		val, err := q.Dequeue()
		if err != nil {
			t.Fatalf("dequeue %d: %v", i, err)
		}
		if val != uintptr(i) {
			t.Fatalf("got %d, want %d", val, i)
		}
	}
}

func TestBuilderDefaultIndirectSPSCUsesLamport(t *testing.T) {
	// SPSC uses Lamport (already optimal), not FAA-based 128-bit
	q := lfq.New(7).SingleProducer().SingleConsumer().BuildIndirect()
	if q.Cap() != 8 {
		t.Fatalf("SPSC Cap: got %d, want 8", q.Cap())
	}

	// Should still work (uses Lamport)
	if err := q.Enqueue(100); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if val != 100 {
		t.Fatalf("got %d, want 100", val)
	}
}

func TestBuilderDefaultPtr(t *testing.T) {
	// MPMC Ptr uses FAA-based 128-bit by default
	q := lfq.New(7).BuildPtr()
	if q.Cap() != 8 {
		t.Fatalf("MPMC Ptr Cap: got %d, want 8", q.Cap())
	}
}

func TestBuilderDefaultPtrMPSC(t *testing.T) {
	q := lfq.New(7).SingleConsumer().BuildPtr()
	if q.Cap() != 8 {
		t.Fatalf("MPSC Ptr Cap: got %d, want 8", q.Cap())
	}
}

func TestBuilderDefaultPtrSPMC(t *testing.T) {
	q := lfq.New(7).SingleProducer().BuildPtr()
	if q.Cap() != 8 {
		t.Fatalf("SPMC Ptr Cap: got %d, want 8", q.Cap())
	}
}

// =============================================================================
// Type-Safe Builder Tests
// =============================================================================

func TestBuilderTypedBuildIndirectMPMC(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.New(7).BuildIndirectMPMC()
}

func TestBuilderTypedBuildIndirectMPSC(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.New(7).SingleConsumer().BuildIndirectMPSC()
}

func TestBuilderTypedBuildIndirectSPMC(t *testing.T) {
	var _ lfq.QueueIndirect = lfq.New(7).SingleProducer().BuildIndirectSPMC()
}

func TestBuilderTypedBuildPtrMPMC(t *testing.T) {
	var _ lfq.QueuePtr = lfq.New(7).BuildPtrMPMC()
}

func TestBuilderTypedBuildPtrMPSC(t *testing.T) {
	var _ lfq.QueuePtr = lfq.New(7).SingleConsumer().BuildPtrMPSC()
}

func TestBuilderTypedBuildPtrSPMC(t *testing.T) {
	var _ lfq.QueuePtr = lfq.New(7).SingleProducer().BuildPtrSPMC()
}

// =============================================================================
// Compact Mode Takes Precedence
// =============================================================================

func TestBuilderCompactTakesPrecedence(t *testing.T) {
	// Compact mode uses 8-byte slots (round-based), not FAA-based 128-bit
	q := lfq.New(7).Compact().BuildIndirect()
	if q.Cap() != 8 {
		t.Fatalf("Cap: got %d, want 8", q.Cap())
	}

	// Should work - uses Compact variant
	if err := q.Enqueue(1); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	val, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if val != 1 {
		t.Fatalf("got %d, want 1", val)
	}
}

// =============================================================================
// Panic Tests for Invalid Builder Configurations
// =============================================================================

func TestBuilderPanicIndirectMPSCNoConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildIndirectMPSC without SingleConsumer()")
		}
	}()
	lfq.New(7).BuildIndirectMPSC()
}

func TestBuilderPanicIndirectMPSCWrongConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildIndirectMPSC with SingleProducer()")
		}
	}()
	lfq.New(7).SingleProducer().SingleConsumer().BuildIndirectMPSC()
}

func TestBuilderPanicIndirectSPMCNoConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildIndirectSPMC without SingleProducer()")
		}
	}()
	lfq.New(7).BuildIndirectSPMC()
}

func TestBuilderPanicIndirectSPMCWrongConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildIndirectSPMC with SingleConsumer()")
		}
	}()
	lfq.New(7).SingleProducer().SingleConsumer().BuildIndirectSPMC()
}

func TestBuilderPanicIndirectMPMCWithConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildIndirectMPMC with SingleProducer()")
		}
	}()
	lfq.New(7).SingleProducer().BuildIndirectMPMC()
}

func TestBuilderPanicPtrSPSCNoConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildPtrSPSC without constraints")
		}
	}()
	lfq.New(7).BuildPtrSPSC()
}

func TestBuilderPanicPtrSPSCPartialConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildPtrSPSC with only SingleProducer()")
		}
	}()
	lfq.New(7).SingleProducer().BuildPtrSPSC()
}

func TestBuilderPanicPtrMPSCNoConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildPtrMPSC without SingleConsumer()")
		}
	}()
	lfq.New(7).BuildPtrMPSC()
}

func TestBuilderPanicPtrSPMCNoConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildPtrSPMC without SingleProducer()")
		}
	}()
	lfq.New(7).BuildPtrSPMC()
}

func TestBuilderPanicPtrMPMCWithConstraint(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for BuildPtrMPMC with SingleConsumer()")
		}
	}()
	lfq.New(7).SingleConsumer().BuildPtrMPMC()
}
