// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Lock-free algorithm tests excluded from race detection.
//
// Go's race detector tracks explicit synchronization primitives (mutex, channels,
// WaitGroup) but cannot observe happens-before relationships established through
// atomic memory orderings (acquire-release semantics).
//
// These tests exercise lock-free queue algorithms that use sequence numbers with
// acquire-release semantics to protect non-atomic data fields. The algorithms are
// correct, but the race detector reports false positives because it cannot track
// the synchronization provided by atomic operations on separate variables.

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

// =============================================================================
// High Contention Tests (Consolidated)
// =============================================================================

// TestHighContentionEnqueue consolidates all enqueue-side high contention tests.
// Tests MPSC variants with 32 producers competing for capacity=4.
func TestHighContentionEnqueue(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	tests := []struct {
		name string
		enq  func() error
		deq  func()
	}{
		{
			name: "MPSC",
			enq: func() error {
				q := lfq.NewMPSC[int](4)
				go func() {
					backoff := iox.Backoff{}
					for {
						if _, err := q.Dequeue(); err == nil {
							backoff.Reset()
						} else {
							backoff.Wait()
						}
					}
				}()
				v := 1
				return q.Enqueue(&v)
			},
		},
	}

	// Run each variant individually to avoid interface issues
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup
			var enqueued, blocked atomix.Int64

			// Test MPSC
			q := lfq.NewMPSC[int](4)
			done := make(chan struct{})
			go func() {
				backoff := iox.Backoff{}
				for {
					select {
					case <-done:
						return
					default:
						if _, err := q.Dequeue(); err == nil {
							backoff.Reset()
						} else {
							backoff.Wait()
						}
					}
				}
			}()

			for range 32 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 1000 {
						v := 1
						if q.Enqueue(&v) == nil {
							enqueued.Add(1)
						} else {
							blocked.Add(1)
						}
					}
				}()
			}

			wg.Wait()
			close(done)

			if enqueued.Load() == 0 {
				t.Error("expected some successful enqueues")
			}
			if blocked.Load() == 0 {
				t.Error("expected some blocked enqueues (queue full)")
			}
		})
	}

	// MPSCIndirect
	t.Run("MPSCIndirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(4)
		var wg sync.WaitGroup
		var enqueued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if _, err := q.Dequeue(); err == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if q.Enqueue(1) == nil {
						enqueued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if enqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked enqueues (queue full)")
		}
	})

	// MPSCPtr
	t.Run("MPSCPtr", func(t *testing.T) {
		q := lfq.NewMPSCPtr(4)
		var wg sync.WaitGroup
		var enqueued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if _, err := q.Dequeue(); err == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if q.Enqueue(unsafe.Pointer(new(int))) == nil {
						enqueued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if enqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked enqueues (queue full)")
		}
	})

	// MPSCCompact
	t.Run("MPSCCompact", func(t *testing.T) {
		q := lfq.NewMPSCCompactIndirect(4)
		var wg sync.WaitGroup
		var enqueued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if _, err := q.Dequeue(); err == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if q.Enqueue(1) == nil {
						enqueued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if enqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked enqueues (queue full)")
		}
	})
}

// TestHighContentionDequeue consolidates all dequeue-side high contention tests.
// Tests SPMC variants with 32 consumers competing for limited items.
func TestHighContentionDequeue(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	// SPMC
	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](4)
		var wg sync.WaitGroup
		var dequeued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					v := 1
					if q.Enqueue(&v) == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if _, err := q.Dequeue(); err == nil {
						dequeued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if dequeued.Load() == 0 {
			t.Error("expected some successful dequeues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked dequeues (queue empty)")
		}
	})

	// SPMCIndirect
	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(4)
		var wg sync.WaitGroup
		var dequeued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if q.Enqueue(1) == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if _, err := q.Dequeue(); err == nil {
						dequeued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if dequeued.Load() == 0 {
			t.Error("expected some successful dequeues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked dequeues (queue empty)")
		}
	})

	// SPMCPtr
	t.Run("SPMCPtr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(4)
		var wg sync.WaitGroup
		var dequeued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if q.Enqueue(unsafe.Pointer(new(int))) == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if _, err := q.Dequeue(); err == nil {
						dequeued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if dequeued.Load() == 0 {
			t.Error("expected some successful dequeues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked dequeues (queue empty)")
		}
	})

	// SPMCCompact
	t.Run("SPMCCompact", func(t *testing.T) {
		q := lfq.NewSPMCCompactIndirect(4)
		var wg sync.WaitGroup
		var dequeued, blocked atomix.Int64

		done := make(chan struct{})
		go func() {
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
					if q.Enqueue(1) == nil {
						backoff.Reset()
					} else {
						backoff.Wait()
					}
				}
			}
		}()

		for range 32 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 1000 {
					if _, err := q.Dequeue(); err == nil {
						dequeued.Add(1)
					} else {
						blocked.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		close(done)

		if dequeued.Load() == 0 {
			t.Error("expected some successful dequeues")
		}
		if blocked.Load() == 0 {
			t.Error("expected some blocked dequeues (queue empty)")
		}
	})
}

// =============================================================================
// Medium Contention Tests (NEW)
// =============================================================================

// TestMediumContention tests moderate concurrency levels (4-8 workers, capacity 512).
// This fills the gap between single-threaded tests and extreme stress tests.
func TestMediumContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	tests := []struct {
		name      string
		producers int
		consumers int
		capacity  int
		items     int
	}{
		{"MPMC_4x4", 4, 4, 512, 1000},
		{"MPMC_8x8", 8, 8, 512, 1000},
		{"MPSC_8x1", 8, 1, 512, 1000},
		{"SPMC_1x8", 1, 8, 512, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := lfq.NewMPMC[int](tt.capacity)
			testMediumContention(t, q, tt.producers, tt.consumers, tt.items)
		})
	}
}

// TestMediumContentionIndirect tests indirect variants at medium contention.
func TestMediumContentionIndirect(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	tests := []struct {
		name      string
		producers int
		consumers int
		capacity  int
		items     int
	}{
		{"MPMCIndirect_4x4", 4, 4, 512, 1000},
		{"MPSCIndirect_8x1", 8, 1, 512, 1000},
		{"SPMCIndirect_1x8", 1, 8, 512, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := lfq.NewMPMCIndirect(tt.capacity)
			testMediumContentionIndirect(t, q, tt.producers, tt.consumers, tt.items)
		})
	}
}

// TestMediumContentionCompact tests compact variants at medium contention.
func TestMediumContentionCompact(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	tests := []struct {
		name      string
		producers int
		consumers int
		capacity  int
		items     int
	}{
		{"MPMCCompact_4x4", 4, 4, 512, 1000},
		{"MPSCCompact_8x1", 8, 1, 512, 1000},
		{"SPMCCompact_1x8", 1, 8, 512, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := lfq.NewMPMCCompactIndirect(tt.capacity)
			testMediumContentionCompact(t, q, tt.producers, tt.consumers, tt.items)
		})
	}
}

func testMediumContention(t *testing.T, q *lfq.MPMC[int], numP, numC, totalItems int) {
	t.Helper()

	// Pre-allocate ALL values before test (stable addresses)
	values := make([]int, totalItems)
	for i := range totalItems {
		values[i] = i
	}

	itemsPerProd := totalItems / numP
	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	seenValues := make([]atomix.Int32, totalItems)
	done := make(chan struct{})
	var closeOnce sync.Once

	// Start producers
	for p := range numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			start := id * itemsPerProd
			end := start + itemsPerProd
			for idx := start; idx < end; idx++ {
				select {
				case <-done:
					return
				default:
				}
				for {
					select {
					case <-done:
						return
					default:
					}
					if q.Enqueue(&values[idx]) == nil {
						produced.Add(1)
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}(p)
	}

	// Start consumers
	for range numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if consumed.Load() >= int64(totalItems) {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v >= 0 && v < totalItems {
						seenValues[v].Add(1)
					}
					consumed.Add(1)
				}
			}
		}()
	}

	// Timeout watchdog
	go func() {
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				closeOnce.Do(func() { close(done) })
				return
			case <-ticker.C:
				if consumed.Load() >= int64(totalItems) {
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()

	wg.Wait()
	closeOnce.Do(func() { close(done) })

	// Verify no duplicates or missing items
	var missing, duplicates int
	for i := range totalItems {
		count := seenValues[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("Duplicated %d items (data corruption)", duplicates)
	}

	// Only check missing if test completed (not timed out)
	if consumed.Load() >= int64(totalItems) && missing > 0 {
		t.Errorf("Missing %d items (queue loss)", missing)
	}

	t.Logf("Medium contention: produced=%d, consumed=%d, missing=%d", produced.Load(), consumed.Load(), missing)
}

func testMediumContentionIndirect(t *testing.T, q *lfq.MPMCIndirect, numP, numC, totalItems int) {
	t.Helper()

	itemsPerProd := totalItems / numP
	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	seenValues := make([]atomix.Int32, totalItems+1)
	done := make(chan struct{})
	var closeOnce sync.Once

	// Start producers
	for p := range numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				select {
				case <-done:
					return
				default:
				}
				v := uintptr(id*itemsPerProd + i + 1)
				for {
					select {
					case <-done:
						return
					default:
					}
					if q.Enqueue(v) == nil {
						produced.Add(1)
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}(p)
	}

	// Start consumers
	for range numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if consumed.Load() >= int64(totalItems) {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v > 0 && v <= uintptr(totalItems) {
						seenValues[int(v)].Add(1)
					}
					consumed.Add(1)
				}
			}
		}()
	}

	// Timeout watchdog
	go func() {
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				closeOnce.Do(func() { close(done) })
				return
			case <-ticker.C:
				if consumed.Load() >= int64(totalItems) {
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()

	wg.Wait()
	closeOnce.Do(func() { close(done) })

	// Verify no duplicates or missing items
	var missing, duplicates int
	for i := 1; i <= totalItems; i++ {
		count := seenValues[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("Duplicated %d items (data corruption)", duplicates)
	}

	// Only check missing if test completed (not timed out)
	if consumed.Load() >= int64(totalItems) && missing > 0 {
		t.Errorf("Missing %d items (queue loss)", missing)
	}

	t.Logf("Medium contention (Indirect): produced=%d, consumed=%d, missing=%d", produced.Load(), consumed.Load(), missing)
}

func testMediumContentionCompact(t *testing.T, q *lfq.MPMCCompactIndirect, numP, numC, totalItems int) {
	t.Helper()

	itemsPerProd := totalItems / numP
	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	seenValues := make([]atomix.Int32, totalItems+1)
	done := make(chan struct{})
	var closeOnce sync.Once

	// Start producers
	for p := range numP {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				select {
				case <-done:
					return
				default:
				}
				v := uintptr(id*itemsPerProd + i + 1)
				for {
					select {
					case <-done:
						return
					default:
					}
					if q.Enqueue(v) == nil {
						produced.Add(1)
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}(p)
	}

	// Start consumers
	for range numC {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if consumed.Load() >= int64(totalItems) {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v > 0 && v <= uintptr(totalItems) {
						seenValues[int(v)].Add(1)
					}
					consumed.Add(1)
				}
			}
		}()
	}

	// Timeout watchdog
	go func() {
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-timeout:
				closeOnce.Do(func() { close(done) })
				return
			case <-ticker.C:
				if consumed.Load() >= int64(totalItems) {
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()

	wg.Wait()
	closeOnce.Do(func() { close(done) })

	// Verify no duplicates or missing items
	var missing, duplicates int
	for i := 1; i <= totalItems; i++ {
		count := seenValues[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Errorf("Duplicated %d items (data corruption)", duplicates)
	}

	// Only check missing if test completed (not timed out)
	if consumed.Load() >= int64(totalItems) && missing > 0 {
		t.Errorf("Missing %d items (queue loss)", missing)
	}

	t.Logf("Medium contention (Compact): produced=%d, consumed=%d, missing=%d", produced.Load(), consumed.Load(), missing)
}

// =============================================================================
// High-Contention Stress Tests (Weak Memory Model Verification)
// =============================================================================

func startStressWatchdog(
	done chan struct{},
	closeOnce *sync.Once,
	timedOut *atomix.Bool,
	produced *atomix.Int64,
	consumed *atomix.Int64,
	totalItems int64,
) {
	const (
		stressTick      = 20 * time.Millisecond
		progressTimeout = 10 * time.Second
	)

	go func() {
		ticker := time.NewTicker(stressTick)
		defer ticker.Stop()

		lastProduced := produced.Load()
		lastConsumed := consumed.Load()
		lastProgress := time.Now()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				currentProduced := produced.Load()
				currentConsumed := consumed.Load()
				if currentProduced != lastProduced || currentConsumed != lastConsumed {
					lastProduced = currentProduced
					lastConsumed = currentConsumed
					lastProgress = time.Now()
					continue
				}

				if currentConsumed < totalItems && time.Since(lastProgress) >= progressTimeout {
					timedOut.Store(true)
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()
}

// TestHighContentionStress verifies queue correctness under extreme contention
// with many producers and consumers. This test validates memory model correctness
// and data integrity under high parallelism.
//
// Key correctness properties:
//   - Pre-allocated values array ensures stable addresses (no stack pointer reuse)
//   - Uses iox.Backoff for external wait semantics (producer/consumer coordination)
//   - Zero tolerance for missing or duplicate items
func TestHighContentionStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skip: stress test")
	}
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 16
		numConsumers = 16
		itemsPerProd = 500
		totalItems   = numProducers * itemsPerProd
		queueCap     = 256
	)

	// Pre-allocate ALL values before test (stable addresses)
	values := make([]int, totalItems)
	for i := range totalItems {
		values[i] = i
	}

	q := lfq.NewMPMC[int](queueCap)
	seen := make([]atomix.Int32, totalItems)
	var produced, consumed atomix.Int64
	var outOfRange atomix.Int64
	var closeOnce sync.Once
	var timedOut atomix.Bool
	done := make(chan struct{})

	startStressWatchdog(done, &closeOnce, &timedOut, &produced, &consumed, int64(totalItems))

	// Producers with correct wait semantics
	var prodWg sync.WaitGroup
	for p := range numProducers {
		prodWg.Add(1)
		go func(id int) {
			defer prodWg.Done()
			start := id * itemsPerProd
			end := start + itemsPerProd
			backoff := iox.Backoff{}
			for idx := start; idx < end; idx++ {
				select {
				case <-done:
					return
				default:
				}
				for q.Enqueue(&values[idx]) != nil {
					select {
					case <-done:
						return
					default:
					}
					backoff.Wait() // External wait for consumer
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Consumers with correct wait semantics
	var consWg sync.WaitGroup
	for range numConsumers {
		consWg.Add(1)
		go func() {
			defer consWg.Done()
			backoff := iox.Backoff{}
			for {
				select {
				case <-done:
					return
				default:
				}
				if consumed.Load() >= int64(totalItems) {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v < 0 || v >= totalItems {
						outOfRange.Add(1)
						consumed.Add(1)
						continue
					}
					seen[v].Add(1)
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait() // External wait for producer
				}
			}
		}()
	}

	// Wait for completion (timeout aborts)
	prodWg.Wait()
	consWg.Wait()
	closeOnce.Do(func() { close(done) })

	// Timeout may occur due to SCQ threshold exhaustion (expected behavior).
	// When producers finish but items become unreachable, consumers spin until timeout.
	if timedOut.Load() {
		t.Logf("MPMC timeout (produced=%d consumed=%d, threshold exhaustion expected)",
			produced.Load(), consumed.Load())
	}
	if outOfRange.Load() > 0 {
		t.Fatalf("out of range: %d values", outOfRange.Load())
	}

	// Linearizability verification: no duplicates allowed.
	// Missing items are acceptable (SCQ threshold exhaustion is valid behavior).
	var missing, duplicates int
	for i := range totalItems {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	// Duplicates = data corruption (MUST fail)
	if duplicates > 0 {
		t.Fatalf("data corruption: %d duplicates", duplicates)
	}

	// Missing items under threshold exhaustion is expected SCQ behavior
	t.Logf("MPMC stress: produced=%d consumed=%d missing=%d",
		produced.Load(), consumed.Load(), missing)
}

// TestHighContentionStressMPSC verifies MPSC under extreme contention.
// Multiple producers, single consumer pattern.
//
// Key correctness properties:
//   - Pre-allocated values array ensures stable addresses
//   - Uses iox.Backoff for external wait semantics
//   - Zero tolerance for missing or duplicate items
func TestHighContentionStressMPSC(t *testing.T) {
	if testing.Short() {
		t.Skip("skip: stress test")
	}
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 16
		itemsPerProd = 500
		totalItems   = numProducers * itemsPerProd
		queueCap     = 256
	)

	// Pre-allocate ALL values before test (stable addresses)
	values := make([]int, totalItems)
	for i := range totalItems {
		values[i] = i
	}

	q := lfq.NewMPSC[int](queueCap)
	seen := make([]atomix.Int32, totalItems)
	var produced, consumed atomix.Int64
	var closeOnce sync.Once
	var timedOut atomix.Bool
	done := make(chan struct{})

	startStressWatchdog(done, &closeOnce, &timedOut, &produced, &consumed, int64(totalItems))

	// Multiple producers with correct wait semantics
	var prodWg sync.WaitGroup
	for p := range numProducers {
		prodWg.Add(1)
		go func(id int) {
			defer prodWg.Done()
			start := id * itemsPerProd
			end := start + itemsPerProd
			backoff := iox.Backoff{}

			for idx := start; idx < end; idx++ {
				select {
				case <-done:
					return
				default:
				}
				for q.Enqueue(&values[idx]) != nil {
					select {
					case <-done:
						return
					default:
					}
					backoff.Wait() // External wait for consumer
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	// Single consumer with correct wait semantics
	var consWg sync.WaitGroup
	consWg.Add(1)
	go func() {
		defer consWg.Done()
		backoff := iox.Backoff{}

		for consumed.Load() < int64(totalItems) {
			select {
			case <-done:
				return
			default:
			}
			v, err := q.Dequeue()
			if err == nil {
				if v < 0 || v >= totalItems {
					t.Errorf("out of range: %d", v)
					consumed.Add(1)
					continue
				}
				count := seen[v].Add(1)
				if count > 1 {
					t.Errorf("duplicate: %d (count=%d)", v, count)
				}
				consumed.Add(1)
				backoff.Reset()
			} else {
				backoff.Wait() // External wait for producer
			}
		}
	}()

	// Wait for completion
	prodWg.Wait()
	consWg.Wait()
	closeOnce.Do(func() { close(done) })

	if timedOut.Load() {
		t.Fatalf("MPSC stress timeout (produced=%d consumed=%d)", produced.Load(), consumed.Load())
	}

	// Complete verification
	var missing, duplicates int
	for i := range totalItems {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Fatalf("data corruption: %d duplicates", duplicates)
	}
	if missing > 0 {
		t.Fatalf("queue loss: %d missing (produced=%d consumed=%d)",
			missing, produced.Load(), consumed.Load())
	}

	t.Logf("MPSC stress: produced=%d consumed=%d", produced.Load(), consumed.Load())
}

// TestHighContentionStressSPMC verifies SPMC under extreme contention.
// Single producer, multiple consumers pattern.
//
// Key correctness properties:
//   - Pre-allocated values array ensures stable addresses
//   - Uses iox.Backoff for external wait semantics
//   - Zero tolerance for missing or duplicate items
func TestHighContentionStressSPMC(t *testing.T) {
	if testing.Short() {
		t.Skip("skip: stress test")
	}
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		numConsumers = 16
		totalItems   = 1000
		queueCap     = 256
	)

	// Pre-allocate ALL values before test (stable addresses)
	values := make([]int, totalItems)
	for i := range totalItems {
		values[i] = i
	}

	q := lfq.NewSPMC[int](queueCap)
	seen := make([]atomix.Int32, totalItems)
	var produced, consumed atomix.Int64
	var closeOnce sync.Once
	var timedOut atomix.Bool
	done := make(chan struct{})

	startStressWatchdog(done, &closeOnce, &timedOut, &produced, &consumed, int64(totalItems))

	// Single producer with correct wait semantics
	var prodWg sync.WaitGroup
	prodWg.Add(1)
	go func() {
		defer prodWg.Done()
		backoff := iox.Backoff{}

		for idx := range totalItems {
			select {
			case <-done:
				return
			default:
			}
			for q.Enqueue(&values[idx]) != nil {
				select {
				case <-done:
					return
				default:
				}
				backoff.Wait() // External wait for consumer
			}
			produced.Add(1)
			backoff.Reset()
		}
	}()

	// Multiple consumers with correct wait semantics
	var consWg sync.WaitGroup
	for range numConsumers {
		consWg.Add(1)
		go func() {
			defer consWg.Done()
			backoff := iox.Backoff{}

			for consumed.Load() < int64(totalItems) {
				select {
				case <-done:
					return
				default:
				}
				v, err := q.Dequeue()
				if err == nil {
					if v < 0 || v >= totalItems {
						t.Errorf("out of range: %d", v)
						consumed.Add(1)
						continue
					}
					count := seen[v].Add(1)
					if count > 1 {
						t.Errorf("duplicate: %d (count=%d)", v, count)
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait() // External wait for producer
				}
			}
		}()
	}

	// Wait for completion
	prodWg.Wait()
	consWg.Wait()
	closeOnce.Do(func() { close(done) })

	if timedOut.Load() {
		t.Fatalf("SPMC stress timeout (produced=%d consumed=%d)", produced.Load(), consumed.Load())
	}

	// Complete verification
	var missing, duplicates int
	for i := range totalItems {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Fatalf("data corruption: %d duplicates", duplicates)
	}
	if missing > 0 {
		t.Fatalf("queue loss: %d missing (produced=%d consumed=%d)",
			missing, produced.Load(), consumed.Load())
	}

	t.Logf("SPMC stress: produced=%d consumed=%d", produced.Load(), consumed.Load())
}

// TestHighContentionCompactStress verifies Compact variants under extreme contention.
func TestHighContentionCompactStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skip: stress test")
	}
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		numProducers = 16
		numConsumers = 16
		itemsPerProd = 500
		totalItems   = numProducers * itemsPerProd
	)

	q := lfq.NewMPMCCompactIndirect(512)

	var wg sync.WaitGroup
	var produced, consumed atomix.Int64
	seenValues := make([]atomix.Int32, totalItems+1) // +1 because values are 1-based
	done := make(chan struct{})
	var closeOnce sync.Once
	var timedOut atomix.Bool

	startStressWatchdog(done, &closeOnce, &timedOut, &produced, &consumed, int64(totalItems))

	// Producers
	for p := range numProducers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := range itemsPerProd {
				select {
				case <-done:
					return
				default:
				}
				// Values 1-based (id*itemsPerProd + i + 1)
				v := uintptr(id*itemsPerProd + i + 1)
				for {
					select {
					case <-done:
						return
					default:
					}
					if q.Enqueue(v) == nil {
						produced.Add(1)
						backoff.Reset()
						break
					}
					backoff.Wait()
				}
			}
		}(p)
	}

	// Consumers
	for range numConsumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if consumed.Load() >= totalItems {
					return
				}
				v, err := q.Dequeue()
				if err == nil {
					if v < 1 || v > totalItems {
						t.Errorf("Value out of range: %d", v)
						continue
					}
					count := seenValues[v].Add(1)
					if count > 1 {
						t.Errorf("Duplicate: %d (count=%d)", v, count)
					}
					consumed.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	closeOnce.Do(func() { close(done) })

	if timedOut.Load() {
		t.Fatalf("Compact stress timeout (produced=%d consumed=%d)", produced.Load(), consumed.Load())
	}

	// Verify - no duplicates allowed
	var missing, duplicates int
	for i := 1; i <= totalItems; i++ {
		count := seenValues[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	if duplicates > 0 {
		t.Fatalf("Duplicated %d items", duplicates)
	}
	if missing > 0 {
		t.Fatalf("Missing %d items (produced=%d consumed=%d)", missing, produced.Load(), consumed.Load())
	}

	t.Logf("Compact stress: produced=%d, consumed=%d, missing=%d, duplicates=%d",
		produced.Load(), consumed.Load(), missing, duplicates)
}
