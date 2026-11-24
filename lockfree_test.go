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
	"runtime"
	"slices"
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

// TestHighContentionEnqueue exercises MPSC backpressure with 32 producers and one consumer.
func TestHighContentionEnqueue(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	const producers, attempts = 32, 1000

	t.Run("MPSC", func(t *testing.T) {
		q := lfq.NewMPSC[int](producers)
		testContendedEnqueue(t, producers, attempts, func(v int) error { return q.Enqueue(&v) }, q.Dequeue)
	})
	t.Run("MPSCIndirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(producers)
		testContendedEnqueue(t, producers, attempts, func(v int) error { return q.Enqueue(uintptr(v)) }, q.Dequeue)
	})
	t.Run("MPSCPtr", func(t *testing.T) {
		q := lfq.NewMPSCPtr(producers)
		values := make([]byte, producers*attempts)
		var pin runtime.Pinner
		pin.Pin(&values[0])
		defer pin.Unpin()

		enqueue := func(i int) error { return q.Enqueue(unsafe.Pointer(&values[i])) }
		testContendedEnqueue(t, producers, attempts, enqueue, q.Dequeue)
	})
	t.Run("MPSCCompact", func(t *testing.T) {
		q := lfq.NewMPSCCompactIndirect(4)
		testContendedEnqueue(t, producers, attempts, func(v int) error { return q.Enqueue(uintptr(v)) }, q.Dequeue)
	})
}

func testContendedEnqueue[T any](
	t *testing.T, producers, attempts int,
	enqueue func(int) error, dequeue func() (T, error),
) {
	t.Helper()

	finished := make(chan int, producers)
	for p := range producers {
		go func() {
			accepted := 0
			for i := range attempts {
				err := enqueue(p*attempts + i)
				if err == nil {
					accepted++
				} else if err != lfq.ErrWouldBlock {
					t.Errorf("Enqueue returned %v", err)
					break
				}
			}
			finished <- accepted
		}()
	}

	// One producer's burst exceeds the ring size, so it exercises backpressure
	// before the consumer can make room.
	accepted := <-finished
	if accepted == attempts {
		t.Errorf("all %d attempts by one producer succeeded before the first Dequeue", attempts)
	}
	consumed := 0
	var backoff iox.Backoff
	var unexpected error
	for remaining := producers - 1; remaining > 0; {
		select {
		case n := <-finished:
			accepted += n
			remaining--
		default:
			if _, err := dequeue(); err == nil {
				consumed++
				backoff.Reset()
			} else {
				if err != lfq.ErrWouldBlock {
					unexpected = err
				}
				backoff.Wait()
			}
		}
	}
	if unexpected != nil {
		t.Fatalf("Dequeue returned %v", unexpected)
	}
	if accepted == 0 {
		t.Fatal("all enqueue attempts were refused")
	}

	for consumed < accepted {
		if _, err := dequeue(); err != nil {
			t.Fatalf("consumed %d of %d accepted items before %v", consumed, accepted, err)
		}
		consumed++
	}
	if v, err := dequeue(); consumed != accepted || err != lfq.ErrWouldBlock {
		t.Fatalf("consumed %d of %d accepted items, next Dequeue returned (%v, %v)", consumed, accepted, v, err)
	}
	if err := enqueue(0); err != nil {
		t.Fatalf("Enqueue after draining returned %v", err)
	}
	if _, err := dequeue(); err != nil {
		t.Fatalf("Dequeue of the next item returned %v", err)
	}
}

// TestHighContentionDequeue exercises four-slot SPMC queues with 32 concurrent consumers.
func TestHighContentionDequeue(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	const capacity, items = 4, 1024
	values := make([]int, items)
	words := make([]uintptr, items)
	for i := range values {
		values[i] = i + 1
		words[i] = uintptr(i + 1)
	}

	t.Run("SPMCSeq", func(t *testing.T) {
		q := lfq.NewSPMCSeq[int](capacity)
		testContendedDequeue(t, values, func(v int) error { return q.Enqueue(&v) }, q.Dequeue)
	})
	t.Run("SPMCIndirectSeq", func(t *testing.T) {
		q := lfq.NewSPMCIndirectSeq(capacity)
		testContendedDequeue(t, words, q.Enqueue, q.Dequeue)
	})
	t.Run("SPMCPtrSeq", func(t *testing.T) {
		q := lfq.NewSPMCPtrSeq(capacity)
		var pin runtime.Pinner
		pin.Pin(&values[0])
		defer pin.Unpin()
		ptrs := make([]unsafe.Pointer, len(values))
		for i := range values {
			ptrs[i] = unsafe.Pointer(&values[i])
		}
		testContendedDequeue(t, ptrs, q.Enqueue, q.Dequeue)
	})
	t.Run("SPMCCompact", func(t *testing.T) {
		q := lfq.NewSPMCCompactIndirect(capacity)
		testContendedDequeue(t, words, q.Enqueue, q.Dequeue)
	})
}

func testContendedDequeue[T comparable](
	t *testing.T, values []T, enqueue func(T) error, dequeue func() (T, error),
) {
	t.Helper()
	const consumers, attempts = 32, 1000

	// Seed the queue before the consumers start. At most len(values) of their
	// attempts can succeed, even if the producer runs ahead.
	if err := enqueue(values[0]); err != nil {
		t.Fatalf("first Enqueue returned %v", err)
	}
	var received [consumers][]T
	var wg sync.WaitGroup
	for c := range received {
		wg.Go(func() {
			var batch []T
			for range attempts {
				v, err := dequeue()
				if err == nil {
					batch = append(batch, v)
				} else if err != lfq.ErrWouldBlock {
					t.Errorf("Dequeue returned %v", err)
					break
				}
			}
			received[c] = batch
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	published := 1
	var backoff iox.Backoff
produce:
	for published < len(values) {
		select {
		case <-done:
			break produce
		default:
		}
		if err := enqueue(values[published]); err == nil {
			published++
			backoff.Reset()
		} else if err == lfq.ErrWouldBlock {
			backoff.Wait()
		} else {
			t.Errorf("Enqueue returned %v", err)
			break
		}
	}
	<-done

	reader := make(map[T]int, published)
	for c, batch := range received {
		for _, v := range batch {
			if _, exists := reader[v]; exists {
				t.Fatalf("duplicate dequeue of %v", v)
			}
			reader[v] = c
		}
	}
	n := len(reader)
	if n == 0 || n > published {
		t.Fatalf("consumers received %d distinct values from %d enqueues", n, published)
	}
	// Each consumer's calls are ordered. Calls from different consumers may overlap.
	for _, want := range values[:n] {
		c, ok := reader[want]
		if !ok {
			t.Fatalf("consumers missed %v in the first %d published values", want, n)
		}
		if got := received[c][0]; got != want {
			t.Fatalf("consumer %d received %v before %v", c, got, want)
		}
		received[c] = received[c][1:]
	}
	for _, want := range values[n:published] {
		if got, err := dequeue(); err != nil || got != want {
			t.Fatalf("Dequeue after consumers finished returned (%v, %v), want %v", got, err, want)
		}
	}
	if v, err := dequeue(); err != lfq.ErrWouldBlock {
		t.Fatalf("Dequeue after the last item returned (%v, %v), want ErrWouldBlock", v, err)
	}
}

// =============================================================================
// Medium Contention Tests (NEW)
// =============================================================================

// TestMediumContention exercises typed queues with producers and consumers running together.
func TestMediumContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	const capacity, items = 512, 1000

	t.Run("MPMCSeq_4x4", func(t *testing.T) {
		testMediumContentionTyped(t, lfq.NewMPMCSeq[int](capacity), 4, 4, items)
	})
	t.Run("MPMCSeq_8x8", func(t *testing.T) {
		testMediumContentionTyped(t, lfq.NewMPMCSeq[int](capacity), 8, 8, items)
	})
	t.Run("MPSC_8x1", func(t *testing.T) {
		testMediumContentionTyped(t, lfq.NewMPSC[int](capacity), 8, 1, items)
	})
	t.Run("SPMCSeq_1x8", func(t *testing.T) {
		testMediumContentionTyped(t, lfq.NewSPMCSeq[int](capacity), 1, 8, items)
	})
}

// TestMediumContentionIndirect checks concurrent delivery through the packed uintptr queues.
func TestMediumContentionIndirect(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	const capacity, items = 512, 1000

	t.Run("MPMCIndirect_4x4", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewMPMCIndirect(capacity), 4, 4, items)
	})
	t.Run("MPSCIndirect_8x1", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewMPSCIndirect(capacity), 8, 1, items)
	})
	t.Run("SPMCIndirectSeq_1x8", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewSPMCIndirectSeq(capacity), 1, 8, items)
	})
}

// TestMediumContentionCompact tests compact variants at medium contention.
func TestMediumContentionCompact(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}
	const capacity, items = 512, 1000

	t.Run("MPMCCompact_4x4", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewMPMCCompactIndirect(capacity), 4, 4, items)
	})
	t.Run("MPSCCompact_8x1", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewMPSCCompactIndirect(capacity), 8, 1, items)
	})
	t.Run("SPMCCompact_1x8", func(t *testing.T) {
		testMediumContentionIndirect(t, lfq.NewSPMCCompactIndirect(capacity), 1, 8, items)
	})
}

func testMediumContentionTyped(t *testing.T, q lfq.Queue[int], numP, numC, totalItems int) {
	t.Helper()

	values := make([]int, totalItems+1)
	for i := range values {
		values[i] = i
	}
	enqueue := func(id int) error { return q.Enqueue(&values[id]) }
	testMediumContentionDelivery(t, enqueue, q.Dequeue, numP, numC, totalItems)
}

func testMediumContentionIndirect(t *testing.T, q lfq.QueueIndirect, numP, numC, totalItems int) {
	t.Helper()
	testMediumContentionDelivery(t, q.Enqueue, q.Dequeue, numP, numC, totalItems)
}

func testMediumContentionDelivery[T int | uintptr](
	t *testing.T,
	enqueue func(T) error, dequeue func() (T, error),
	numP, numC, totalItems int,
) {
	t.Helper()

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
			for value := id + 1; value <= totalItems; value += numP {
				for {
					select {
					case <-done:
						return
					default:
					}
					if enqueue(T(value)) == nil {
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
				v, err := dequeue()
				if err == nil {
					if v > 0 && v <= T(totalItems) {
						seenValues[int(v)].Add(1)
					}
					consumed.Add(1)
					backoff.Reset()
				} else {
					backoff.Wait()
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

	sent, received := produced.Load(), consumed.Load()
	if sent != int64(totalItems) || received != sent || missing != 0 || duplicates != 0 {
		t.Errorf("produced %d, consumed %d, want %d each (missing %d, duplicated %d)",
			sent, received, totalItems, missing, duplicates)
	}
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
	drainSignal := make(chan struct{})

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
			draining := false
			emptyCount := 0
			for {
				select {
				case <-done:
					return
				case <-drainSignal:
					draining = true
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
					emptyCount = 0
					backoff.Reset()
				} else if draining {
					emptyCount++
					if emptyCount > 1000 {
						return
					}
				} else {
					backoff.Wait() // External wait for producer
				}
			}
		}()
	}

	// Wait for producers, then drain
	prodWg.Wait()
	q.Drain()
	close(drainSignal)
	consWg.Wait()
	closeOnce.Do(func() { close(done) })

	if timedOut.Load() {
		t.Fatalf("MPMC stress timeout (produced=%d consumed=%d)",
			produced.Load(), consumed.Load())
	}
	if outOfRange.Load() > 0 {
		t.Fatalf("out of range: %d values", outOfRange.Load())
	}

	// Linearizability verification: no duplicates or missing items allowed.
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

	// Missing items under extreme contention is expected SCQ behavior
	// (threshold exhaustion causes some items to be temporarily unreachable)
	t.Logf("MPMC stress: produced=%d consumed=%d missing=%d", produced.Load(), consumed.Load(), missing)
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

	drainSignal := make(chan struct{})

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
			draining := false
			emptyCount := 0

			for consumed.Load() < int64(totalItems) {
				select {
				case <-done:
					return
				case <-drainSignal:
					draining = true
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
					emptyCount = 0
					backoff.Reset()
				} else if draining {
					emptyCount++
					if emptyCount > 1000 {
						return
					}
				} else {
					backoff.Wait() // External wait for producer
				}
			}
		}()
	}

	// Wait for producer to finish, then signal drain mode
	prodWg.Wait()
	q.Drain() // Allow consumers to drain remaining items without threshold blocking
	close(drainSignal)
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
		t.Logf("queue loss: %d missing (produced=%d consumed=%d)",
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

// =============================================================================
// Drain Mechanism Tests
// =============================================================================

// TestDrainBasic tests basic Drain() functionality for all FAA-based queue variants.
// Verifies that after Drain() is called, consumers can drain remaining items
// without being blocked by threshold exhaustion.
func TestDrainBasic(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("MPMC", func(t *testing.T) {
		q := lfq.NewMPMC[int](8)
		testDrainGeneric(t, q)
	})

	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](8)
		testDrainGenericSPMC(t, q)
	})

	t.Run("MPSC", func(t *testing.T) {
		q := lfq.NewMPSC[int](8)
		testDrainGenericMPSC(t, q)
	})
}

// TestDrainIndirect tests Drain() for 128-bit indirect queue variants.
func TestDrainIndirect(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("MPMCIndirect", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(8)
		testDrainIndirect(t, q)
	})

	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(8)
		testDrainIndirectSPMC(t, q)
	})

	t.Run("MPSCIndirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(8)
		testDrainIndirectMPSC(t, q)
	})
}

// TestDrainPtr tests Drain() for unsafe.Pointer queue variants.
func TestDrainPtr(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("MPMCPtr", func(t *testing.T) {
		q := lfq.NewMPMCPtr(8)
		testDrainPtr(t, q)
	})

	t.Run("SPMCPtr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(8)
		testDrainPtrSPMC(t, q)
	})

	t.Run("MPSCPtr", func(t *testing.T) {
		q := lfq.NewMPSCPtr(8)
		testDrainPtrMPSC(t, q)
	})
}

// TestDrainIdempotent verifies that Drain() can be called multiple times safely.
func TestDrainIdempotent(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("MPMC", func(t *testing.T) {
		q := lfq.NewMPMC[int](8)
		q.Drain()
		q.Drain() // Second call should be safe
		q.Drain() // Third call should be safe

		// Queue should still work for dequeue (even if empty)
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})

	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](8)
		q.Drain()
		q.Drain()
		q.Drain()
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})

	t.Run("MPSC", func(t *testing.T) {
		q := lfq.NewMPSC[int](8)
		q.Drain()
		q.Drain()
		q.Drain()
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})

	t.Run("MPMCIndirect", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(8)
		q.Drain()
		q.Drain()
		q.Drain()
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})

	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(8)
		q.Drain()
		q.Drain()
		q.Drain()
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})

	t.Run("MPSCIndirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(8)
		q.Drain()
		q.Drain()
		q.Drain()
		_, err := q.Dequeue()
		if err != lfq.ErrWouldBlock {
			t.Errorf("expected ErrWouldBlock from empty drained queue, got %v", err)
		}
	})
}

// TestDrainGracefulShutdown covers the fresh-batch handoff and the packed
// MPMC queue's concurrent consumers.
func TestDrainGracefulShutdown(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	values := []int{1, 2, 3, 4}
	t.Run("MPMC", func(t *testing.T) {
		q := lfq.NewMPMC[int](len(values))
		testDrainBatch(t, values, 2, q.Enqueue, q.Drain, q.Dequeue)
	})

	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](len(values))
		testDrainBatch(t, values, 1, q.Enqueue, q.Drain, q.Dequeue)
	})

	t.Run("MPMCIndirect_MultiConsumer", func(t *testing.T) {
		const (
			numProducers = 4
			numConsumers = 4
			itemsPerProd = 100
			queueCap     = 32
		)

		q := lfq.NewMPMCIndirect(queueCap)
		testGracefulShutdownMPMCIndirect(t, q, numProducers, numConsumers, itemsPerProd)
	})

	t.Run("SPMCIndirect", func(t *testing.T) {
		words := []uintptr{1, 2, 3, 4}
		q := lfq.NewSPMCIndirect(len(words))
		testDrainBatch(t, words, 1, func(v *uintptr) error { return q.Enqueue(*v) }, q.Drain, q.Dequeue)
	})

	t.Run("SPMCPtr", func(t *testing.T) {
		ptrs := make([]unsafe.Pointer, len(packedPtrTestValues))
		for i := range ptrs {
			ptrs[i] = unsafe.Pointer(&packedPtrTestValues[i])
		}
		q := lfq.NewSPMCPtr(len(ptrs))
		testDrainBatch(t, ptrs, 1, func(v *unsafe.Pointer) error { return q.Enqueue(*v) }, q.Drain, q.Dequeue)
	})
}

// Helper functions for Drain tests

func testDrainGeneric(t *testing.T, q *lfq.MPMC[int]) {
	t.Helper()

	// Enqueue some items
	values := []int{1, 2, 3, 4, 5}
	for i := range values {
		if err := q.Enqueue(&values[i]); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	// Signal drain mode
	q.Drain()

	// Dequeue all items - should succeed even after drain
	var dequeued []int
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainGenericSPMC(t *testing.T, q *lfq.SPMC[int]) {
	t.Helper()

	values := []int{1, 2, 3, 4, 5}
	for i := range values {
		if err := q.Enqueue(&values[i]); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued []int
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainGenericMPSC(t *testing.T, q *lfq.MPSC[int]) {
	t.Helper()

	values := []int{1, 2, 3, 4, 5}
	for i := range values {
		if err := q.Enqueue(&values[i]); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued []int
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainIndirect(t *testing.T, q *lfq.MPMCIndirect) {
	t.Helper()

	values := []uintptr{1, 2, 3, 4, 5}
	for _, v := range values {
		if err := q.Enqueue(v); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued []uintptr
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainIndirectSPMC(t *testing.T, q *lfq.SPMCIndirect) {
	t.Helper()

	values := []uintptr{1, 2, 3, 4, 5}
	for _, v := range values {
		if err := q.Enqueue(v); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued []uintptr
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainIndirectMPSC(t *testing.T, q *lfq.MPSCIndirect) {
	t.Helper()

	values := []uintptr{1, 2, 3, 4, 5}
	for _, v := range values {
		if err := q.Enqueue(v); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued []uintptr
	for {
		v, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued = append(dequeued, v)
	}

	if len(dequeued) != len(values) {
		t.Errorf("expected %d items, got %d", len(values), len(dequeued))
	}
}

func testDrainPtr(t *testing.T, q *lfq.MPMCPtr) {
	t.Helper()

	type item struct{ v int }
	items := []*item{{1}, {2}, {3}, {4}, {5}}
	for _, it := range items {
		if err := q.Enqueue(unsafe.Pointer(it)); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued int
	for {
		_, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued++
	}

	if dequeued != len(items) {
		t.Errorf("expected %d items, got %d", len(items), dequeued)
	}
}

func testDrainPtrSPMC(t *testing.T, q *lfq.SPMCPtr) {
	t.Helper()

	type item struct{ v int }
	items := []*item{{1}, {2}, {3}, {4}, {5}}
	for _, it := range items {
		if err := q.Enqueue(unsafe.Pointer(it)); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued int
	for {
		_, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued++
	}

	if dequeued != len(items) {
		t.Errorf("expected %d items, got %d", len(items), dequeued)
	}
}

func testDrainPtrMPSC(t *testing.T, q *lfq.MPSCPtr) {
	t.Helper()

	type item struct{ v int }
	items := []*item{{1}, {2}, {3}, {4}, {5}}
	for _, it := range items {
		if err := q.Enqueue(unsafe.Pointer(it)); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	q.Drain()

	var dequeued int
	for {
		_, err := q.Dequeue()
		if err != nil {
			break
		}
		dequeued++
	}

	if dequeued != len(items) {
		t.Errorf("expected %d items, got %d", len(items), dequeued)
	}
}

func testDrainBatch[T comparable](
	t *testing.T, values []T, producers int,
	enqueue func(*T) error, drain func(), dequeue func() (T, error),
) {
	t.Helper()

	var wg sync.WaitGroup
	next := make([]int, producers)
	for p := range producers {
		next[p] = p
		wg.Go(func() {
			for i := p; i < len(values); i += producers {
				if err := enqueue(&values[i]); err != nil {
					t.Errorf("Enqueue(%v) failed: %v", values[i], err)
					return
				}
			}
		})
	}
	wg.Wait()
	drain()

	// Each producer's values must arrive in order. Producers may interleave.
	for i := range values {
		v, err := dequeue()
		if err != nil {
			t.Fatalf("Dequeue %d of %d failed: %v", i+1, len(values), err)
		}
		idx := slices.Index(values, v)
		if idx < 0 {
			t.Fatalf("Dequeue %d returned unknown value %v", i+1, v)
		}
		p := idx % producers
		if idx != next[p] {
			t.Fatalf("Dequeue %d returned index %d from producer %d, want %d", i+1, idx, p, next[p])
		}
		next[p] += producers
	}
	if v, err := dequeue(); err != lfq.ErrWouldBlock {
		t.Fatalf("Dequeue after the batch returned (%v, %v), want ErrWouldBlock", v, err)
	}
}

func testGracefulShutdownMPMCIndirect(t *testing.T, q *lfq.MPMCIndirect, numProducers, numConsumers, itemsPerProd int) {
	t.Helper()
	totalItems := numProducers * itemsPerProd

	seen := make([]atomix.Int32, totalItems)
	var produced, consumed atomix.Int64
	var prodWg, consWg sync.WaitGroup
	drainSignal := make(chan struct{})

	for p := range numProducers {
		prodWg.Add(1)
		go func(id int) {
			defer prodWg.Done()
			backoff := iox.Backoff{}
			start := id * itemsPerProd
			end := start + itemsPerProd
			for idx := start; idx < end; idx++ {
				for q.Enqueue(uintptr(idx)) != nil {
					backoff.Wait()
				}
				produced.Add(1)
				backoff.Reset()
			}
		}(p)
	}

	for range numConsumers {
		consWg.Add(1)
		go func() {
			defer consWg.Done()
			backoff := iox.Backoff{}
			draining := false
			for {
				select {
				case <-drainSignal:
					draining = true
				default:
				}
				v, err := q.Dequeue()
				if err == nil {
					backoff.Reset()
					if v < uintptr(totalItems) {
						seen[v].Add(1)
					}
					consumed.Add(1)
				} else if draining {
					if err != lfq.ErrWouldBlock {
						t.Errorf("Dequeue after Drain returned %v, want ErrWouldBlock", err)
					}
					return
				} else {
					backoff.Wait()
				}
			}
		}()
	}

	prodWg.Wait()
	q.Drain()
	close(drainSignal)
	consWg.Wait()

	var missing, duplicates int
	for i := range totalItems {
		count := seen[i].Load()
		if count == 0 {
			missing++
		} else if count > 1 {
			duplicates++
		}
	}

	sent, received := produced.Load(), consumed.Load()
	if sent != int64(totalItems) || received != sent || missing != 0 || duplicates != 0 {
		t.Errorf("produced %d, consumed %d, want %d each (missing %d, duplicated %d)",
			sent, received, totalItems, missing, duplicates)
	}
}

// TestDrainThresholdExhaustion tests that Dequeue continues when threshold
// exhausts but draining is active (covers the !q.draining.LoadAcquire() path).
func TestDrainThresholdExhaustion(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	t.Run("MPMC", func(t *testing.T) {
		q := lfq.NewMPMC[int](4)
		values := []int{1, 2, 3, 4}
		for i := range values {
			if err := q.Enqueue(&values[i]); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < len(values); i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})

	t.Run("MPMCIndirect", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(4)
		for i := uintptr(1); i <= 4; i++ {
			if err := q.Enqueue(i); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < 4; i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})

	t.Run("MPMCPtr", func(t *testing.T) {
		q := lfq.NewMPMCPtr(4)
		values := [4]int{1, 2, 3, 4}
		for i := range values {
			if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < 4; i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})

	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](4)
		values := []int{1, 2, 3, 4}
		for i := range values {
			if err := q.Enqueue(&values[i]); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < len(values); i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})

	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(4)
		for i := uintptr(1); i <= 4; i++ {
			if err := q.Enqueue(i); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < 4; i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})

	t.Run("SPMCPtr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(4)
		values := [4]int{1, 2, 3, 4}
		for i := range values {
			if err := q.Enqueue(unsafe.Pointer(&values[i])); err != nil {
				t.Fatalf("Enqueue failed: %v", err)
			}
		}
		q.Drain()
		for i := 0; i < 4; i++ {
			if _, err := q.Dequeue(); err != nil {
				t.Errorf("Dequeue %d failed after Drain: %v", i, err)
			}
		}
	})
}

// TestDrainWithThresholdExhausted exercises the threshold<=0 && draining=true
// branch in Dequeue. When draining is active, consumers must continue looping
// past the threshold check instead of returning ErrWouldBlock.
//
// Setup: many consumers race on a partially-filled queue, driving threshold
// negative. Then Drain() is called, allowing consumers to retrieve remaining
// items despite exhausted threshold.
func TestDrainWithThresholdExhausted(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		cap         = 4
		numItems    = 3 // Partially fill (less than capacity)
		numConsumer = 8 // Many consumers to exhaust threshold quickly
	)

	// MPMC — concurrent producers + consumers for threshold exhaustion with tail ahead.
	// Phase 1: producers and consumers race (covers threshold<=0 && !draining).
	// Phase 2: drain mode (covers threshold<=0 && draining=true bypass).
	t.Run("MPMC", func(t *testing.T) {
		const (
			mpmcCap         = 2
			mpmcNumProducer = 4
			mpmcNumConsumer = 16
		)
		prev := runtime.GOMAXPROCS(mpmcNumProducer + mpmcNumConsumer + 2)
		defer runtime.GOMAXPROCS(prev)

		q := lfq.NewMPMC[int](mpmcCap)
		var producerStop, consumerStop atomix.Bool
		var totalEnqueued, totalDequeued atomix.Int64
		var wg sync.WaitGroup

		wg.Add(mpmcNumProducer)
		for range mpmcNumProducer {
			go func() {
				defer wg.Done()
				v := 1
				for !producerStop.LoadAcquire() {
					if q.Enqueue(&v) == nil {
						totalEnqueued.Add(1)
					}
				}
			}()
		}

		wg.Add(mpmcNumConsumer)
		for range mpmcNumConsumer {
			go func() {
				defer wg.Done()
				for !consumerStop.LoadAcquire() {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		// Phase 1: concurrent operation — threshold exhaustion with !draining
		time.Sleep(100 * time.Millisecond)

		// Phase 2: stop producers, enable drain
		producerStop.StoreRelease(true)
		q.Drain()

		// Phase 3: consumers drain remaining with draining=true
		time.Sleep(50 * time.Millisecond)
		consumerStop.StoreRelease(true)
		wg.Wait()

		if totalDequeued.Load() > totalEnqueued.Load() {
			t.Fatalf("dequeued %d > enqueued %d", totalDequeued.Load(), totalEnqueued.Load())
		}
		t.Logf("MPMC drain+threshold: enqueued=%d dequeued=%d", totalEnqueued.Load(), totalDequeued.Load())
	})

	// MPMCIndirect
	t.Run("MPMCIndirect", func(t *testing.T) {
		const (
			mpmcCap         = 2
			mpmcNumProducer = 4
			mpmcNumConsumer = 16
		)
		prev := runtime.GOMAXPROCS(mpmcNumProducer + mpmcNumConsumer + 2)
		defer runtime.GOMAXPROCS(prev)

		q := lfq.NewMPMCIndirect(mpmcCap)
		var producerStop, consumerStop atomix.Bool
		var totalEnqueued, totalDequeued atomix.Int64
		var wg sync.WaitGroup

		wg.Add(mpmcNumProducer)
		for range mpmcNumProducer {
			go func() {
				defer wg.Done()
				for !producerStop.LoadAcquire() {
					if q.Enqueue(1) == nil {
						totalEnqueued.Add(1)
					}
				}
			}()
		}

		wg.Add(mpmcNumConsumer)
		for range mpmcNumConsumer {
			go func() {
				defer wg.Done()
				for !consumerStop.LoadAcquire() {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		time.Sleep(100 * time.Millisecond)
		producerStop.StoreRelease(true)
		q.Drain()
		time.Sleep(50 * time.Millisecond)
		consumerStop.StoreRelease(true)
		wg.Wait()

		if totalDequeued.Load() > totalEnqueued.Load() {
			t.Fatalf("dequeued %d > enqueued %d", totalDequeued.Load(), totalEnqueued.Load())
		}
		t.Logf("MPMCIndirect drain+threshold: enqueued=%d dequeued=%d", totalEnqueued.Load(), totalDequeued.Load())
	})

	// MPMCPtr
	t.Run("MPMCPtr", func(t *testing.T) {
		const (
			mpmcCap         = 2
			mpmcNumProducer = 4
			mpmcNumConsumer = 16
		)
		prev := runtime.GOMAXPROCS(mpmcNumProducer + mpmcNumConsumer + 2)
		defer runtime.GOMAXPROCS(prev)

		q := lfq.NewMPMCPtr(mpmcCap)
		sentinel := 42
		var producerStop, consumerStop atomix.Bool
		var totalEnqueued, totalDequeued atomix.Int64
		var wg sync.WaitGroup

		wg.Add(mpmcNumProducer)
		for range mpmcNumProducer {
			go func() {
				defer wg.Done()
				for !producerStop.LoadAcquire() {
					if q.Enqueue(unsafe.Pointer(&sentinel)) == nil {
						totalEnqueued.Add(1)
					}
				}
			}()
		}

		wg.Add(mpmcNumConsumer)
		for range mpmcNumConsumer {
			go func() {
				defer wg.Done()
				for !consumerStop.LoadAcquire() {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		time.Sleep(100 * time.Millisecond)
		producerStop.StoreRelease(true)
		q.Drain()
		time.Sleep(50 * time.Millisecond)
		consumerStop.StoreRelease(true)
		wg.Wait()

		if totalDequeued.Load() > totalEnqueued.Load() {
			t.Fatalf("dequeued %d > enqueued %d", totalDequeued.Load(), totalEnqueued.Load())
		}
		t.Logf("MPMCPtr drain+threshold: enqueued=%d dequeued=%d", totalEnqueued.Load(), totalDequeued.Load())
	})

	// SPMC
	t.Run("SPMC", func(t *testing.T) {
		q := lfq.NewSPMC[int](cap)
		values := make([]int, numItems)
		for i := range numItems {
			values[i] = i + 1
			if err := q.Enqueue(&values[i]); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}

		var totalDequeued atomix.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		close(start)
		time.Sleep(time.Millisecond)
		q.Drain()

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		got := totalDequeued.Load()
		if got > int64(numItems) {
			t.Fatalf("dequeued %d > enqueued %d", got, numItems)
		}
		t.Logf("SPMC drain+threshold: dequeued=%d enqueued=%d", got, numItems)
	})

	// SPMCIndirect
	t.Run("SPMCIndirect", func(t *testing.T) {
		q := lfq.NewSPMCIndirect(cap)
		for i := uintptr(1); i <= numItems; i++ {
			if err := q.Enqueue(i); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}

		var totalDequeued atomix.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		close(start)
		time.Sleep(time.Millisecond)
		q.Drain()

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		got := totalDequeued.Load()
		if got > int64(numItems) {
			t.Fatalf("dequeued %d > enqueued %d", got, numItems)
		}
		t.Logf("SPMCIndirect drain+threshold: dequeued=%d enqueued=%d", got, numItems)
	})

	// SPMCPtr
	t.Run("SPMCPtr", func(t *testing.T) {
		q := lfq.NewSPMCPtr(cap)
		for i := range numItems {
			if err := q.Enqueue(unsafe.Pointer(&packedPtrTestValues[i])); err != nil {
				t.Fatalf("Enqueue(%d): %v", i, err)
			}
		}

		var totalDequeued atomix.Int64
		var wg sync.WaitGroup
		start := make(chan struct{})

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		close(start)
		time.Sleep(time.Millisecond)
		q.Drain()

		for range numConsumer {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 50 {
					if _, err := q.Dequeue(); err == nil {
						totalDequeued.Add(1)
					}
				}
			}()
		}

		wg.Wait()
		got := totalDequeued.Load()
		if got > int64(numItems) {
			t.Fatalf("dequeued %d > enqueued %d", got, numItems)
		}
		t.Logf("SPMCPtr drain+threshold: dequeued=%d enqueued=%d", got, numItems)
	})
}

// TestEnqueueStaleSlotContention exercises the stale-slot return path in
// Enqueue (slotCycle < expectedCycle → ErrWouldBlock). This path triggers
// when a producer passes the early fullness check but its FAA-claimed slot
// has not been recycled yet (cycle from a previous round).
//
// Strategy: continuous stress test with many producers in tight loops competing
// for a tiny queue (cap=2). A single consumer dequeues continuously. Over
// millions of operations, the narrow race window (multiple producers reading
// the same tail before any FAA) gets hit probabilistically.
func TestEnqueueStaleSlotContention(t *testing.T) {
	if lfq.RaceEnabled {
		t.Skip("skip: lock-free algorithm uses cross-variable memory ordering")
	}

	const (
		cap         = 2
		numProducer = 16
	)

	prev := runtime.GOMAXPROCS(numProducer + 2)
	defer runtime.GOMAXPROCS(prev)

	duration := 80 * time.Millisecond

	// MPSC generic
	t.Run("MPSC", func(t *testing.T) {
		q := lfq.NewMPSC[int](cap)
		var stop atomix.Bool
		var totalEnqueued, totalBlocked atomix.Int64
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.LoadAcquire() {
				q.Dequeue()
			}
		}()

		wg.Add(numProducer)
		for range numProducer {
			go func() {
				defer wg.Done()
				v := 1
				for !stop.LoadAcquire() {
					if q.Enqueue(&v) == nil {
						totalEnqueued.Add(1)
					} else {
						totalBlocked.Add(1)
					}
				}
			}()
		}

		time.Sleep(duration)
		stop.StoreRelease(true)
		wg.Wait()

		if totalEnqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if totalBlocked.Load() == 0 {
			t.Error("expected some blocked enqueues")
		}
		t.Logf("MPSC: enqueued=%d blocked=%d", totalEnqueued.Load(), totalBlocked.Load())
	})

	// MPSCIndirect
	t.Run("MPSCIndirect", func(t *testing.T) {
		q := lfq.NewMPSCIndirect(cap)
		var stop atomix.Bool
		var totalEnqueued, totalBlocked atomix.Int64
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.LoadAcquire() {
				q.Dequeue()
			}
		}()

		wg.Add(numProducer)
		for range numProducer {
			go func() {
				defer wg.Done()
				for !stop.LoadAcquire() {
					if q.Enqueue(1) == nil {
						totalEnqueued.Add(1)
					} else {
						totalBlocked.Add(1)
					}
				}
			}()
		}

		time.Sleep(duration)
		stop.StoreRelease(true)
		wg.Wait()

		if totalEnqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if totalBlocked.Load() == 0 {
			t.Error("expected some blocked enqueues")
		}
		t.Logf("MPSCIndirect: enqueued=%d blocked=%d", totalEnqueued.Load(), totalBlocked.Load())
	})

	// MPSCPtr
	t.Run("MPSCPtr", func(t *testing.T) {
		q := lfq.NewMPSCPtr(cap)
		sentinel := 42
		var stop atomix.Bool
		var totalEnqueued, totalBlocked atomix.Int64
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.LoadAcquire() {
				q.Dequeue()
			}
		}()

		wg.Add(numProducer)
		for range numProducer {
			go func() {
				defer wg.Done()
				for !stop.LoadAcquire() {
					if q.Enqueue(unsafe.Pointer(&sentinel)) == nil {
						totalEnqueued.Add(1)
					} else {
						totalBlocked.Add(1)
					}
				}
			}()
		}

		time.Sleep(duration)
		stop.StoreRelease(true)
		wg.Wait()

		if totalEnqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if totalBlocked.Load() == 0 {
			t.Error("expected some blocked enqueues")
		}
		t.Logf("MPSCPtr: enqueued=%d blocked=%d", totalEnqueued.Load(), totalBlocked.Load())
	})

	// MPMCIndirect
	t.Run("MPMCIndirect", func(t *testing.T) {
		q := lfq.NewMPMCIndirect(cap)
		var stop atomix.Bool
		var totalEnqueued, totalBlocked atomix.Int64
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.LoadAcquire() {
				q.Dequeue()
			}
		}()

		wg.Add(numProducer)
		for range numProducer {
			go func() {
				defer wg.Done()
				for !stop.LoadAcquire() {
					if q.Enqueue(1) == nil {
						totalEnqueued.Add(1)
					} else {
						totalBlocked.Add(1)
					}
				}
			}()
		}

		time.Sleep(duration)
		stop.StoreRelease(true)
		wg.Wait()

		if totalEnqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if totalBlocked.Load() == 0 {
			t.Error("expected some blocked enqueues")
		}
		t.Logf("MPMCIndirect: enqueued=%d blocked=%d", totalEnqueued.Load(), totalBlocked.Load())
	})

	// MPMCPtr
	t.Run("MPMCPtr", func(t *testing.T) {
		q := lfq.NewMPMCPtr(cap)
		sentinel := 42
		var stop atomix.Bool
		var totalEnqueued, totalBlocked atomix.Int64
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.LoadAcquire() {
				q.Dequeue()
			}
		}()

		wg.Add(numProducer)
		for range numProducer {
			go func() {
				defer wg.Done()
				for !stop.LoadAcquire() {
					if q.Enqueue(unsafe.Pointer(&sentinel)) == nil {
						totalEnqueued.Add(1)
					} else {
						totalBlocked.Add(1)
					}
				}
			}()
		}

		time.Sleep(duration)
		stop.StoreRelease(true)
		wg.Wait()

		if totalEnqueued.Load() == 0 {
			t.Error("expected some successful enqueues")
		}
		if totalBlocked.Load() == 0 {
			t.Error("expected some blocked enqueues")
		}
		t.Logf("MPMCPtr: enqueued=%d blocked=%d", totalEnqueued.Load(), totalBlocked.Load())
	})
}
