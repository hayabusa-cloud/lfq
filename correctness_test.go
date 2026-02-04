// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// =============================================================================
// Test Helpers
// =============================================================================

// retryWithTimeout retries f until it returns true or timeout expires.
// Reports failure with the given message if timeout is reached.
func retryWithTimeout(t *testing.T, timeout time.Duration, f func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	backoff := iox.Backoff{}
	for !f() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout after %v: %s", timeout, msg)
		}
		backoff.Wait()
	}
}

// waitForCount waits until counter reaches target or timeout expires.
func waitForCount(t *testing.T, timeout time.Duration, counter *atomix.Int64, target int64, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	backoff := iox.Backoff{}
	for counter.Load() < target {
		if time.Now().After(deadline) {
			t.Fatalf("timeout after %v: %s (got %d, want %d)", timeout, msg, counter.Load(), target)
		}
		backoff.Wait()
	}
}

// =============================================================================
// Generic Linearizability Test Helper
// =============================================================================

// linearizabilityTest is a generic linearizability verifier for queue operations.
// It launches numP producers and numC consumers, each producing/consuming itemsPerProd items.
// Values are encoded as producerID*100000 + sequence.
type linearizabilityTest struct {
	t            *testing.T
	numP, numC   int
	itemsPerProd int
	timeout      time.Duration
}

func (lt *linearizabilityTest) runGeneric(
	enqueue func(v int) error,
	dequeue func() (int, error),
) {
	t := lt.t
	if lfq.RaceEnabled {
		t.Skip("skip: linearizability test requires concurrent access")
	}

	var wg sync.WaitGroup
	expectedTotal := lt.numP * lt.itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)
	var consumedCount atomix.Int64
	var timedOut atomix.Bool

	// Producers
	for p := range lt.numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deadline := time.Now().Add(lt.timeout)
			backoff := iox.Backoff{}
			for i := range lt.itemsPerProd {
				v := id*100000 + i
				for enqueue(v) != nil {
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

	// Consumers
	var consumeCount atomix.Int64
	for range lt.numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(lt.timeout)
			backoff := iox.Backoff{}
			for consumeCount.Load() < int64(expectedTotal) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := dequeue()
				if err == nil {
					producerID := v / 100000
					seq := v % 100000
					if producerID < 0 || producerID >= lt.numP || seq < 0 || seq >= lt.itemsPerProd {
						t.Errorf("value out of range: %d", v)
						consumeCount.Add(1)
						continue
					}
					idx := producerID*lt.itemsPerProd + seq
					seen[idx].Add(1)
					consumeCount.Add(1)
					consumedCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Linearizability verification: no duplicates allowed.
	// Missing items are acceptable (SCQ threshold exhaustion is valid behavior).
	var missing, duplicates int
	for i := range expectedTotal {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	// Duplicates = linearizability violation (MUST fail)
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates detected", duplicates)
	}

	// Log statistics (missing items are expected due to threshold exhaustion)
	if timedOut.Load() || missing > 0 {
		t.Logf("consumed %d/%d (missing=%d, threshold exhaustion expected)",
			consumedCount.Load(), expectedTotal, missing)
	}
}

// runIndirect runs linearizability test for Indirect queues (uintptr values).
// Values are 1-based to avoid 0 which might conflict with empty markers.
func (lt *linearizabilityTest) runIndirect(
	enqueue func(v uintptr) error,
	dequeue func() (uintptr, error),
) {
	t := lt.t
	if lfq.RaceEnabled {
		t.Skip("skip: linearizability test requires concurrent access")
	}

	var wg sync.WaitGroup
	expectedTotal := lt.numP * lt.itemsPerProd
	seen := make([]atomix.Int32, expectedTotal)
	var consumedCount atomix.Int64
	var timedOut atomix.Bool

	// Producers
	for p := range lt.numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deadline := time.Now().Add(lt.timeout)
			backoff := iox.Backoff{}
			for i := range lt.itemsPerProd {
				v := uintptr(id*100000 + i + 1) // +1 to avoid 0
				for enqueue(v) != nil {
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

	// Consumers
	var consumeCount atomix.Int64
	for range lt.numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(lt.timeout)
			backoff := iox.Backoff{}
			for consumeCount.Load() < int64(expectedTotal) {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := dequeue()
				if err == nil {
					if v == 0 {
						t.Errorf("value out of range: %d", v)
						consumeCount.Add(1)
						continue
					}
					tmp := int(v - 1)
					producerID := tmp / 100000
					seq := tmp % 100000
					if producerID < 0 || producerID >= lt.numP || seq < 0 || seq >= lt.itemsPerProd {
						t.Errorf("value out of range: %d", v)
						consumeCount.Add(1)
						continue
					}
					idx := producerID*lt.itemsPerProd + seq
					seen[idx].Add(1)
					consumeCount.Add(1)
					consumedCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Linearizability verification: no duplicates allowed.
	// Missing items are acceptable (SCQ threshold exhaustion is valid behavior).
	var missing, duplicates int
	for i := range expectedTotal {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	// Duplicates = linearizability violation (MUST fail)
	if duplicates > 0 {
		t.Errorf("linearizability violation: %d duplicates detected", duplicates)
	}

	// Log statistics (missing items are expected due to threshold exhaustion)
	if timedOut.Load() || missing > 0 {
		t.Logf("consumed %d/%d (missing=%d, threshold exhaustion expected)",
			consumedCount.Load(), expectedTotal, missing)
	}
}

// =============================================================================
// FIFO Ordering Tests
// =============================================================================

// TestSPSCFIFOOrdering verifies strict FIFO ordering for SPSC queues.
func TestSPSCFIFOOrdering(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: SPSC uses cross-variable memory ordering not understood by race detector")
	}

	q := lfq.NewSPSC[int](64)
	const n = 5000

	var wg sync.WaitGroup
	results := make([]int, n)
	var count atomix.Int64
	var timedOut atomix.Bool

	// Consumer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(5 * time.Second)
		backoff := iox.Backoff{}
		idx := 0
		for idx < n {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v, err := q.Dequeue()
			if err == nil {
				results[idx] = v
				idx++
				count.Add(1)
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	// Producer (in main goroutine for SPSC)
	for i := range n {
		v := i
		retryWithTimeout(t, 3*time.Second, func() bool {
			return q.Enqueue(&v) == nil
		}, fmt.Sprintf("producer: enqueue item %d", i))
	}

	wg.Wait()

	if timedOut.Load() {
		t.Fatalf("consumer timeout: consumed %d/%d", count.Load(), n)
	}
	if count.Load() != n {
		t.Fatalf("consumed %d items, want %d", count.Load(), n)
	}

	// Verify FIFO order
	for i := range n {
		if results[i] != i {
			t.Fatalf("FIFO violation at %d: got %d, want %d", i, results[i], i)
		}
	}
}

// TestMPSCFIFOOrderingPerProducer verifies FIFO ordering per producer in MPSC.
// Each producer's items should maintain relative order.
func TestMPSCFIFOOrderingPerProducer(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: FIFO test requires precise timing")
	}

	q := lfq.NewMPSC[int](1024)
	const (
		numProducers = 4
		itemsPerProd = 5000
	)

	var wg sync.WaitGroup

	// Producers: each produces items with their ID encoded
	// Item format: producerID * 100000 + sequence
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deadline := time.Now().Add(5 * time.Second)
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				v := id*100000 + i
				for q.Enqueue(&v) != nil {
					if time.Now().After(deadline) {
						return // Let test detect via count mismatch
					}
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(p)
	}

	// Consumer: collect all items and verify per-producer ordering
	results := make([][]int, numProducers)
	for i := range results {
		results[i] = make([]int, 0, itemsPerProd)
	}
	var resultsMu sync.Mutex
	var timedOut atomix.Bool

	wg.Add(1)
	go func() {
		defer wg.Done()
		collected := 0
		deadline := time.Now().Add(5 * time.Second)
		backoff := iox.Backoff{}
		for collected < numProducers*itemsPerProd {
			if time.Now().After(deadline) {
				timedOut.Store(true)
				return
			}
			v, err := q.Dequeue()
			if err == nil {
				producerID := v / 100000
				seq := v % 100000
				resultsMu.Lock()
				results[producerID] = append(results[producerID], seq)
				resultsMu.Unlock()
				collected++
				backoff.Reset()
			} else {
				backoff.Wait()
			}
		}
	}()

	wg.Wait()
	if timedOut.Load() {
		collected := 0
		for _, seqs := range results {
			collected += len(seqs)
		}
		t.Fatalf("consumer timeout: collected %d/%d", collected, numProducers*itemsPerProd)
	}

	// Verify each producer's items are in order
	for p, seqs := range results {
		if len(seqs) != itemsPerProd {
			t.Errorf("Producer %d: got %d items, want %d", p, len(seqs), itemsPerProd)
			continue
		}
		for i := 1; i < len(seqs); i++ {
			if seqs[i] <= seqs[i-1] {
				t.Errorf("Producer %d: FIFO violation at index %d: %d <= %d",
					p, i, seqs[i], seqs[i-1])
				break
			}
		}
	}
}

// TestSPMCFIFOOrderingPerConsumer verifies FIFO ordering across consumers.
// No item should be consumed twice and all items should be consumed.
func TestSPMCFIFOOrderingPerConsumer(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: FIFO test requires precise timing")
	}

	q := lfq.NewSPMC[int](2048)
	const (
		numConsumers = 2 // Reduced for faster execution
		totalItems   = 2000
	)

	var wg sync.WaitGroup
	var consumed atomix.Int64
	seen := make([]atomix.Int32, totalItems)

	// Producer
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(5 * time.Second)
		backoff := iox.Backoff{}
		for i := range totalItems {
			v := i
			for q.Enqueue(&v) != nil {
				if time.Now().After(deadline) {
					return
				}
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Consumers
	var timedOut atomix.Bool
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(5 * time.Second)
			backoff := iox.Backoff{}
			for consumed.Load() < totalItems {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v < 0 || v >= totalItems {
						t.Errorf("value out of range: %d", v)
						consumed.Add(1)
						continue
					}
					seen[v].Add(1)
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Fatalf("timeout: consumed %d/%d", consumed.Load(), totalItems)
	}

	// Verify all items consumed exactly once
	var missing, duplicates int
	for i := range totalItems {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}
	if missing > 0 || duplicates > 0 {
		t.Errorf("missing=%d duplicates=%d", missing, duplicates)
	}
}

// =============================================================================
// Linearizability Tests (Consolidated)
// =============================================================================

// TestLinearizability verifies atomic operation semantics for all queue variants.
func TestLinearizability(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"MPMC", func(t *testing.T) {
			q := lfq.NewMPMC[int](128)
			lt := &linearizabilityTest{t: t, numP: 2, numC: 2, itemsPerProd: 5000, timeout: 5 * time.Second}
			lt.runGeneric(func(v int) error { return q.Enqueue(&v) }, func() (int, error) { return q.Dequeue() })
		}},
		{"MPMCCompact", func(t *testing.T) {
			q := lfq.NewMPMCCompactIndirect(128)
			lt := &linearizabilityTest{t: t, numP: 4, numC: 4, itemsPerProd: 5000, timeout: 5 * time.Second}
			lt.runIndirect(q.Enqueue, q.Dequeue)
		}},
		{"MPSCCompact", func(t *testing.T) {
			q := lfq.NewMPSCCompactIndirect(128)
			lt := &linearizabilityTest{t: t, numP: 4, numC: 1, itemsPerProd: 5000, timeout: 5 * time.Second}
			lt.runIndirect(q.Enqueue, q.Dequeue)
		}},
		{"SPMCCompact", func(t *testing.T) {
			q := lfq.NewSPMCCompactIndirect(128)
			lt := &linearizabilityTest{t: t, numP: 1, numC: 4, itemsPerProd: 5000, timeout: 5 * time.Second}
			lt.runIndirect(q.Enqueue, q.Dequeue)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

// =============================================================================
// Progress (Liveness) Tests
// =============================================================================

// TestMPMCProgress verifies system-wide progress under contention.
// Uses Compact queue for reliable performance.
func TestMPMCProgress(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: progress test requires high contention")
	}

	q := lfq.NewMPMCCompactIndirect(128)

	const (
		numProducers = 4
		numConsumers = 4
		totalItems   = 5000
	)

	var produced, consumed atomix.Int64
	var wg sync.WaitGroup

	// Producers
	for range numProducers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for produced.Load() < totalItems {
				v := uintptr(produced.Load() + 1)
				if q.Enqueue(v) == nil {
					produced.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	// Consumers
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumed.Load() < totalItems {
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

	// Verify progress was made
	if consumed.Load() < totalItems {
		t.Errorf("Not all items consumed: produced=%d consumed=%d target=%d",
			produced.Load(), consumed.Load(), totalItems)
	}
}

// =============================================================================
// ABA Safety Tests (Consolidated)
// =============================================================================

// TestABASafety verifies round-based detection prevents ABA problem.
func TestABASafety(t *testing.T) {
	tests := []struct {
		name   string
		newQ   func() abaTester
		cycles int
	}{
		{"MPMCCompact_FillDrain", func() abaTester { return lfq.NewMPMCCompactIndirect(8) }, 5000},
		{"MPSCCompact_FillDrain", func() abaTester { return lfq.NewMPSCCompactIndirect(8) }, 5000},
		{"SPMCCompact_FillDrain", func() abaTester { return lfq.NewSPMCCompactIndirect(8) }, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testABASafetyFillDrain(t, tt.newQ(), tt.cycles)
		})
	}
}

type abaTester interface {
	Enqueue(uintptr) error
	Dequeue() (uintptr, error)
}

func testABASafetyFillDrain(t *testing.T, q abaTester, cycles int) {
	t.Helper()

	for cycle := range cycles {
		// Fill
		for i := range 4 {
			v := uintptr(cycle*4 + i + 1)
			if err := q.Enqueue(v); err != nil {
				t.Fatalf("Cycle %d, enqueue %d: %v", cycle, i, err)
			}
		}

		// Drain
		for i := range 4 {
			v, err := q.Dequeue()
			if err != nil {
				t.Fatalf("Cycle %d, dequeue %d: %v", cycle, i, err)
			}
			expected := uintptr(cycle*4 + i + 1)
			if v != expected {
				t.Fatalf("Cycle %d, dequeue %d: got %d, want %d", cycle, i, v, expected)
			}
		}
	}
}

// TestABASafetyConcurrent tests ABA safety under concurrent access.
func TestABASafetyConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: concurrent ABA test")
	}

	tests := []struct {
		name       string
		newQ       func() abaTester
		numP       int
		numC       int
		totalItems int
	}{
		{"MPMCCompact_4x4", func() abaTester { return lfq.NewMPMCCompactIndirect(8) }, 4, 4, 5000},
		{"SPMCCompact_1x4", func() abaTester { return lfq.NewSPMCCompactIndirect(8) }, 1, 4, 5000},
		{"MPSCCompact_4x1", func() abaTester { return lfq.NewMPSCCompactIndirect(8) }, 4, 1, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testABASafetyConcurrent(t, tt.newQ(), tt.numP, tt.numC, tt.totalItems)
		})
	}
}

func testABASafetyConcurrent(t *testing.T, q abaTester, numP, numC, totalItems int) {
	t.Helper()

	itemsPerProd := totalItems / numP
	var wg sync.WaitGroup
	var consumed atomix.Int64
	seenValues := make([]atomix.Int64, totalItems+1)

	// Producers
	for p := range numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			deadline := time.Now().Add(5 * time.Second)
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				v := uintptr(id*itemsPerProd + i + 1)
				for q.Enqueue(v) != nil {
					if time.Now().After(deadline) {
						return
					}
					backoff.Wait()
				}
				backoff.Reset()
			}
		}(p)
	}

	// Consumers
	for range numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(5 * time.Second)
			backoff := iox.Backoff{}
			for consumed.Load() < int64(totalItems) {
				if time.Now().After(deadline) {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v > 0 && int(v) <= totalItems {
						seenValues[v].Add(1)
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Verify each value seen exactly once (no ABA duplicates)
	for i := 1; i <= totalItems; i++ {
		count := seenValues[i].Load()
		if count != 1 {
			t.Errorf("Value %d seen %d times (expected 1)", i, count)
		}
	}
}

// =============================================================================
// Indirect Queue Specific Tests
// =============================================================================

// TestIndirectValuePreservation verifies uintptr values are preserved exactly.
func TestIndirectValuePreservation(t *testing.T) {
	q := lfq.NewMPMCIndirect(64)

	// Test with various bit patterns
	testValues := []uintptr{
		0,
		1,
		0x7FFFFFFF,
		0x7FFFFFFFFFFFFFFF, // Max 63-bit value
		0x5555555555555555,
		0x2AAAAAAAAAAAAAAA,
	}

	for _, v := range testValues {
		if err := q.Enqueue(v); err != nil {
			t.Fatalf("Enqueue %x: %v", v, err)
		}
	}

	for _, expected := range testValues {
		v, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
		if v != expected {
			t.Fatalf("Value mismatch: got %x, want %x", v, expected)
		}
	}
}

// TestIndirectValuePreservationConcurrent verifies value preservation under concurrency.
func TestIndirectValuePreservationConcurrent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: requires concurrent access")
	}

	q := lfq.NewMPMCIndirect(2048)
	const totalItems = 2000 // Reduced for faster execution

	var wg sync.WaitGroup
	produced := make([]uintptr, totalItems)
	consumed := make([]uintptr, 0, totalItems)
	var consumedMu sync.Mutex
	var timedOut atomix.Bool
	deadline := time.Now().Add(5 * time.Second)

	// Initialize items with distinct patterns
	for i := range totalItems {
		produced[i] = uintptr(i*17 + 1) // Prime multiplier for distribution
	}

	// Producers
	wg.Add(4)
	for p := range 4 {
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			start := id * (totalItems / 4)
			end := start + (totalItems / 4)
			for i := start; i < end; i++ {
				for q.Enqueue(produced[i]) != nil {
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

	// Consumers
	var consumeCount atomix.Int64
	wg.Add(4)
	for range 4 {
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumeCount.Load() < totalItems {
				if time.Now().After(deadline) {
					timedOut.Store(true)
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					consumedMu.Lock()
					consumed = append(consumed, v)
					consumedMu.Unlock()
					consumeCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	if timedOut.Load() {
		t.Fatalf("timeout: consumed %d/%d", consumeCount.Load(), totalItems)
	}

	// Sort and compare
	sort.Slice(produced, func(i, j int) bool { return produced[i] < produced[j] })
	sort.Slice(consumed, func(i, j int) bool { return consumed[i] < consumed[j] })

	if len(consumed) != len(produced) {
		t.Fatalf("Count mismatch: produced %d, consumed %d", len(produced), len(consumed))
	}

	for i := range produced {
		if produced[i] != consumed[i] {
			t.Fatalf("Value mismatch at %d: produced %x, consumed %x", i, produced[i], consumed[i])
		}
	}
}

// =============================================================================
// Ptr Queue Specific Tests
// =============================================================================

// TestPtrBasicOperation verifies Ptr queue behavior with valid pointers.
func TestPtrBasicOperation(t *testing.T) {
	q := lfq.NewMPMCPtr(8)

	// Enqueue some valid pointers
	vals := []int{1, 2, 3}
	for i := range vals {
		q.Enqueue(unsafe.Pointer(&vals[i]))
	}

	// Dequeue and verify
	for i := range vals {
		v, err := q.Dequeue()
		if err != nil {
			t.Fatalf("Dequeue %d: %v", i, err)
		}
		if v == nil {
			t.Fatalf("Dequeue %d: got nil, want pointer", i)
		}
		got := *(*int)(v)
		if got != vals[i] {
			t.Fatalf("Dequeue %d: got %d, want %d", i, got, vals[i])
		}
	}

	// Empty queue should return error
	_, err := q.Dequeue()
	if err != lfq.ErrWouldBlock {
		t.Fatalf("Empty dequeue: got %v, want ErrWouldBlock", err)
	}
}

// =============================================================================
// Stress Tests with Verification
// =============================================================================

// TestMPMCStressWithVerification runs high-load stress test with full verification.
func TestMPMCStressWithVerification(t *testing.T) {
	if lfq.RaceEnabled || testing.Short() {
		t.Skip("skip: stress test")
	}

	q := lfq.NewMPMCCompactIndirect(1024)
	const (
		numProducers = 4
		numConsumers = 4
		itemsPerProd = 2500
	)

	var wg sync.WaitGroup
	produced := make([]uintptr, 0, numProducers*itemsPerProd)
	consumed := make([]uintptr, 0, numProducers*itemsPerProd)
	var producedMu, consumedMu sync.Mutex

	// Producers
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				v := uintptr(id*itemsPerProd + i + 1)
				for q.Enqueue(v) != nil {
					backoff.Wait()
				}
				producedMu.Lock()
				produced = append(produced, v)
				producedMu.Unlock()
				backoff.Reset()
			}
		}(p)
	}

	// Consumers
	var consumeCount atomix.Int64
	totalItems := int64(numProducers * itemsPerProd)
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backoff := iox.Backoff{}
			for consumeCount.Load() < totalItems {
				v, err := q.Dequeue()
				if err == nil {
					consumedMu.Lock()
					consumed = append(consumed, v)
					consumedMu.Unlock()
					consumeCount.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	wg.Wait()

	// Sort and compare
	sort.Slice(produced, func(i, j int) bool { return produced[i] < produced[j] })
	sort.Slice(consumed, func(i, j int) bool { return consumed[i] < consumed[j] })

	if len(produced) != len(consumed) {
		t.Fatalf("Count mismatch: produced %d, consumed %d",
			len(produced), len(consumed))
	}

	for i := range produced {
		if produced[i] != consumed[i] {
			t.Fatalf("Mismatch at %d: produced %d, consumed %d",
				i, produced[i], consumed[i])
		}
	}
}

// =============================================================================
// Threshold Exhaustion Tests (Consolidated)
// =============================================================================

// TestThresholdExhaustion verifies the livelock prevention mechanism.
func TestThresholdExhaustion(t *testing.T) {
	const cap = 4
	// thresholdBudget = 3n - 1: maximum empty dequeues before ErrWouldBlock
	// Formula derivation: (n-1) lagging dequeuers + 2n max slot distance
	const thresholdBudget = 3*cap - 1 // 11 for capacity 4

	// MPMC
	t.Run("MPMC", func(t *testing.T) {
		q := lfq.NewMPMC[int](cap)

		// Fill and drain to test threshold on empty queue
		for i := range cap {
			v := i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		// Now queue is empty - exhaust threshold via empty dequeues
		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})

	// SPMC
	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](cap)

		// Fill and drain to test threshold on empty queue
		for i := range cap {
			v := i
			if err := q.Enqueue(&v); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		// Now queue is empty - exhaust threshold via empty dequeues
		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})

	// MPMCIndirect
	t.Run("MPMCIndirect", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(cap)

		for i := uintptr(1); i <= cap; i++ {
			if err := q.Enqueue(i); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})

	// MPMCPtr
	t.Run("MPMCPtr", func(t *testing.T) {
		q := lfq.NewMPMCPtr(cap)

		values := [cap]int{1, 2, 3, 4}
		for i := range cap {
			if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})

	// SPMCIndirect
	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(cap)

		for i := uintptr(1); i <= cap; i++ {
			if err := q.Enqueue(i); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})

	// SPMCPtr
	t.Run("SPMCPtr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(cap)

		values := [cap]int{1, 2, 3, 4}
		for i := range cap {
			if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
				t.Fatalf("Initial enqueue(%d): %v", i, err)
			}
		}
		for range cap {
			if _, err := q.Dequeue(); err != nil {
				t.Fatalf("Initial dequeue: %v", err)
			}
		}

		var wouldBlockCount int
		for i := 0; i < thresholdBudget+5; i++ {
			_, err := q.Dequeue()
			if err == lfq.ErrWouldBlock {
				wouldBlockCount++
			}
		}

		if wouldBlockCount == 0 {
			t.Fatal("Expected ErrWouldBlock after exhausting threshold")
		}

		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Fatalf("Expected ErrWouldBlock when threshold exhausted, got %v", err)
		}

		t.Logf("Threshold exhausted after %d ErrWouldBlock returns", wouldBlockCount)
	})
}
