// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"sync"
	"testing"
	"time"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// ptrOf returns unsafe.Pointer to v.
func ptrOf[T any](v *T) unsafe.Pointer {
	return unsafe.Pointer(v)
}

// =============================================================================
// Seq Variant Stress Tests
//
// Seq variants use CAS-based per-slot sequence numbers for ABA safety.
// They have n physical slots (half the memory of FAA-based variants).
// Unlike FAA variants, Seq variants do NOT have:
//   - Drain() method (no threshold mechanism)
//   - Threshold exhaustion behavior
// =============================================================================

// =============================================================================
// MPMCSeq Stress Tests
// =============================================================================

// TestMPMCSeqStressConcurrent tests MPMCSeq under high concurrent load.
// Multiple producers and consumers compete for limited capacity.
func TestMPMCSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 8
		numConsumers = 8
		itemsPerProd = 10000
		timeout      = 10 * time.Second
	)

	q := lfq.NewMPMCSeq[int](64)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Producers: each produces unique values (id*itemsPerProd + seq)
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v := id*itemsPerProd + i
				for q.Enqueue(&v) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
					backoff.Wait()
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Consumers: track seen values
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < int64(expectedTotal) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v >= 0 && v < expectedTotal {
						seen[v].Add(1)
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					// Check if all produced and nothing left
					if produced.Load() == int64(expectedTotal) && consumed.Load() == int64(expectedTotal) {
						return
					}
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), expectedTotal)
	}

	// All produced items must be consumed (no loss)
	if got := consumed.Load(); got != int64(expectedTotal) {
		t.Errorf("consumed %d, want %d", got, expectedTotal)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestMPMCSeqStressLinearizability verifies no duplicates under concurrent access.
func TestMPMCSeqStressLinearizability(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 4
		numConsumers = 4
		itemsPerProd = 5000
		timeout      = 5 * time.Second
	)

	q := lfq.NewMPMCSeq[int](32)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var consumedCount atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Producers: each produces unique values (id*100000 + seq)
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v := id*itemsPerProd + i // Use simpler encoding for indexing
				for q.Enqueue(&v) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(p)
	}

	// Consumers: track seen values
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumedCount.Load() < int64(expectedTotal) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v >= 0 && v < expectedTotal {
						seen[v].Add(1)
					}
					consumedCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Verify: no duplicates (linearizability)
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestMPMCSeqStressFillDrain tests rapid fill/drain cycles.
func TestMPMCSeqStressFillDrain(t *testing.T) {
	q := lfq.NewMPMCSeq[int](16)

	for cycle := range 5000 {
		// Fill
		for i := range 16 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
		}

		// Drain with FIFO verification
		for i := range 16 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			expected := cycle*100 + i
			if val != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, expected)
			}
		}
	}
}

// =============================================================================
// MPSCSeq Stress Tests
// =============================================================================

// TestMPSCSeqStressConcurrent tests MPSCSeq under high producer contention.
func TestMPSCSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 16
		itemsPerProd = 5000
		timeout      = 10 * time.Second
	)

	q := lfq.NewMPSCSeq[int](64)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Producers: multiple goroutines
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v := id*itemsPerProd + i
				for q.Enqueue(&v) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
					backoff.Wait()
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Single consumer (MPSC contract): track seen values
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for consumed.Load() < int64(expectedTotal) {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v, err := q.Dequeue()
			if err == nil {
				if v >= 0 && v < expectedTotal {
					seen[v].Add(1)
				}
				consumed.Add(1)
				backoff.Reset()
			} else {
				if produced.Load() == int64(expectedTotal) && consumed.Load() == int64(expectedTotal) {
					return
				}
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), expectedTotal)
	}

	if got := consumed.Load(); got != int64(expectedTotal) {
		t.Errorf("consumed %d, want %d", got, expectedTotal)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestMPSCSeqStressLinearizability verifies per-producer FIFO under contention.
func TestMPSCSeqStressLinearizability(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 8
		itemsPerProd = 2000
		timeout      = 5 * time.Second
	)

	q := lfq.NewMPSCSeq[int](32)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var consumedCount atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Producers
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v := id*itemsPerProd + i
				for q.Enqueue(&v) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
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
		for consumedCount.Load() < int64(expectedTotal) {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v, err := q.Dequeue()
			if err == nil {
				if v >= 0 && v < expectedTotal {
					seen[v].Add(1)
				}
				consumedCount.Add(1)
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	// Verify: no duplicates
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestMPSCSeqStressFillDrain tests rapid fill/drain cycles (single-threaded).
func TestMPSCSeqStressFillDrain(t *testing.T) {
	q := lfq.NewMPSCSeq[int](16)

	for cycle := range 5000 {
		// Fill
		for i := range 16 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
		}

		// Drain with FIFO verification
		for i := range 16 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			expected := cycle*100 + i
			if val != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, expected)
			}
		}
	}
}

// =============================================================================
// SPMCSeq Stress Tests
// =============================================================================

// TestSPMCSeqStressConcurrent tests SPMCSeq under high consumer contention.
func TestSPMCSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numConsumers = 16
		itemCount    = 80000
		timeout      = 10 * time.Second
	)

	q := lfq.NewSPMCSeq[int](64)
	seen := make([]atomix.Int32, itemCount)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Single producer (SPMC contract)
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range itemCount {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v := i
			for q.Enqueue(&v) != nil {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				backoff.Wait()
			}
			produced.Add(1)
			backoff.Reset()
		}
	}()

	// Multiple consumers: track seen values
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < itemCount {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v >= 0 && v < itemCount {
						seen[v].Add(1)
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					if produced.Load() == itemCount && consumed.Load() == int64(itemCount) {
						return
					}
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), itemCount)
	}

	if got := consumed.Load(); got != int64(itemCount) {
		t.Errorf("consumed %d, want %d", got, itemCount)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range itemCount {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestSPMCSeqStressLinearizability verifies no duplicates under consumer contention.
func TestSPMCSeqStressLinearizability(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numConsumers = 8
		itemCount    = 20000
		timeout      = 5 * time.Second
	)

	q := lfq.NewSPMCSeq[int](32)
	seen := make([]atomix.Int32, itemCount)

	var wg sync.WaitGroup
	var consumedCount atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Single producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range itemCount {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v := i
			for q.Enqueue(&v) != nil {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Multiple consumers
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumedCount.Load() < itemCount {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v >= 0 && v < itemCount {
						seen[v].Add(1)
					}
					consumedCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Verify: no duplicates
	var duplicates int
	for i := range itemCount {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestSPMCSeqStressFillDrain tests rapid fill/drain cycles (single-threaded).
func TestSPMCSeqStressFillDrain(t *testing.T) {
	q := lfq.NewSPMCSeq[int](16)

	for cycle := range 5000 {
		// Fill
		for i := range 16 {
			v := cycle*100 + i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("cycle %d: Enqueue(%d): %v", cycle, i, err)
			}
		}

		// Drain with FIFO verification
		for i := range 16 {
			val, err := q.Dequeue()
			if err != nil {
				t.Fatalf("cycle %d: Dequeue(%d): %v", cycle, i, err)
			}
			expected := cycle*100 + i
			if val != expected {
				t.Fatalf("cycle %d: got %d, want %d", cycle, val, expected)
			}
		}
	}
}

// =============================================================================
// Seq Ptr Variant Stress Tests (128-bit CAS)
// =============================================================================

// TestMPMCPtrSeqStressConcurrent tests MPMCPtrSeq under high concurrent load.
func TestMPMCPtrSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 8
		numConsumers = 8
		itemsPerProd = 5000
		timeout      = 10 * time.Second
	)

	q := lfq.NewMPMCPtrSeq(64)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Pre-allocate values to avoid GC pressure
	values := make([]int, expectedTotal)
	for i := range values {
		values[i] = i
	}

	// Producers
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			base := id * itemsPerProd
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				for q.Enqueue(ptrOf(&values[base+i])) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
					backoff.Wait()
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Consumers: track seen values
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < int64(expectedTotal) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				ptr, err := q.Dequeue()
				if err == nil {
					if ptr != nil {
						v := *(*int)(ptr)
						if v >= 0 && v < expectedTotal {
							seen[v].Add(1)
						}
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					if produced.Load() == int64(expectedTotal) && consumed.Load() == int64(expectedTotal) {
						return
					}
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), expectedTotal)
	}

	if got := consumed.Load(); got != int64(expectedTotal) {
		t.Errorf("consumed %d, want %d", got, expectedTotal)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestMPSCPtrSeqStressConcurrent tests MPSCPtrSeq under high producer contention.
func TestMPSCPtrSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 16
		itemsPerProd = 2500
		timeout      = 10 * time.Second
	)

	q := lfq.NewMPSCPtrSeq(64)
	expectedTotal := numProducers * itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Pre-allocate
	values := make([]int, expectedTotal)
	for i := range values {
		values[i] = i
	}

	// Producers
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			base := id * itemsPerProd
			for i := range itemsPerProd {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				for q.Enqueue(ptrOf(&values[base+i])) != nil {
					if time.Now().After(deadline) {
						timedOut.Store(true)
						return
					}
					backoff.Wait()
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Single consumer: track seen values
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for consumed.Load() < int64(expectedTotal) {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			ptr, err := q.Dequeue()
			if err == nil {
				if ptr != nil {
					v := *(*int)(ptr)
					if v >= 0 && v < expectedTotal {
						seen[v].Add(1)
					}
				}
				consumed.Add(1)
				backoff.Reset()
			} else {
				if produced.Load() == int64(expectedTotal) && consumed.Load() == int64(expectedTotal) {
					return
				}
				backoff.Wait()
			}
		}
	}()

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), expectedTotal)
	}

	if got := consumed.Load(); got != int64(expectedTotal) {
		t.Errorf("consumed %d, want %d", got, expectedTotal)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range expectedTotal {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}

// TestSPMCPtrSeqStressConcurrent tests SPMCPtrSeq under high consumer contention.
func TestSPMCPtrSeqStressConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: CAS-based algorithm uses cross-variable memory ordering")
	}

	const (
		numConsumers = 16
		itemCount    = 40000
		timeout      = 10 * time.Second
	)

	q := lfq.NewSPMCPtrSeq(64)
	seen := make([]atomix.Int32, itemCount)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	var timedOut atomix.Bool
	deadline := time.Now().Add(timeout)

	// Pre-allocate
	values := make([]int, itemCount)
	for i := range values {
		values[i] = i
	}

	// Single producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := range itemCount {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			for q.Enqueue(ptrOf(&values[i])) != nil {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				backoff.Wait()
			}
			produced.Add(1)
			backoff.Reset()
		}
	}()

	// Multiple consumers: track seen values
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < int64(itemCount) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				ptr, err := q.Dequeue()
				if err == nil {
					if ptr != nil {
						v := *(*int)(ptr)
						if v >= 0 && v < itemCount {
							seen[v].Add(1)
						}
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					if produced.Load() == int64(itemCount) && consumed.Load() == int64(itemCount) {
						return
					}
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Logf("timeout: produced=%d, consumed=%d/%d", produced.Load(), consumed.Load(), itemCount)
	}

	if got := consumed.Load(); got != int64(itemCount) {
		t.Errorf("consumed %d, want %d", got, itemCount)
	}

	// Verify: no duplicates
	var duplicates int
	for i := range itemCount {
		if count := seen[i].Load(); count > 1 {
			duplicates++
		}
	}
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates", duplicates)
	}
}
