// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/lfq"
	"code.hybscloud.com/spin"
)

// =============================================================================
// SPSC Baselines (Critical for overhead comparison)
// =============================================================================

func BenchmarkSPSC_SingleOp(b *testing.B) {
	q := lfq.NewSPSC[int](1024)

	b.ResetTimer()
	for i := range b.N {
		v := i
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkSPSCIndirect_SingleOp(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkSPSCPtr_SingleOp(b *testing.B) {
	q := lfq.NewSPSCPtr(1024)
	val := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(unsafe.Pointer(&val))
		q.Dequeue()
	}
}

// =============================================================================
// MPMC Benchmarks
// =============================================================================

func BenchmarkMPMC_SingleOp(b *testing.B) {
	q := lfq.NewMPMC[int](1024)

	b.ResetTimer()
	for i := range b.N {
		v := i
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkMPMCIndirect_SingleOp(b *testing.B) {
	q := lfq.NewMPMCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkMPMCPtr_SingleOp(b *testing.B) {
	q := lfq.NewMPMCPtr(1024)
	val := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(unsafe.Pointer(&val))
		q.Dequeue()
	}
}

func BenchmarkMPMCCompactIndirect_SingleOp(b *testing.B) {
	q := lfq.NewMPMCCompactIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkMPMCIndirect_Parallel(b *testing.B) {
	q := lfq.NewMPMCIndirect(4096)
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first to be ready for producers)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for p := range numProducers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			sw := spin.Wait{}
			base := uintptr(id * opsPerProducer)
			for i := range opsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}(p)
	}

	// Wait for all producers to finish
	producerWg.Wait()
	// Signal consumers to drain and exit
	close(done)
	consumerWg.Wait()
}

func BenchmarkMPMCPtr_Parallel(b *testing.B) {
	q := lfq.NewMPMCPtr(4096)
	val := 42
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first to be ready for producers)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for range numProducers {
		producerWg.Add(1)
		go func() {
			defer producerWg.Done()
			sw := spin.Wait{}
			for range opsPerProducer {
				for q.Enqueue(unsafe.Pointer(&val)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}()
	}

	// Wait for all producers to finish
	producerWg.Wait()
	// Signal consumers to drain and exit
	close(done)
	consumerWg.Wait()
}

// =============================================================================
// MPSC Benchmarks
// =============================================================================

func BenchmarkMPSC_SingleOp(b *testing.B) {
	q := lfq.NewMPSC[int](1024)

	b.ResetTimer()
	for i := range b.N {
		v := i
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkMPSCIndirect_SingleOp(b *testing.B) {
	q := lfq.NewMPSCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkMPSCPtr_SingleOp(b *testing.B) {
	q := lfq.NewMPSCPtr(1024)
	val := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(unsafe.Pointer(&val))
		q.Dequeue()
	}
}

func BenchmarkMPSCCompactIndirect_SingleOp(b *testing.B) {
	q := lfq.NewMPSCCompactIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkMPSCIndirect_ParallelProducers(b *testing.B) {
	q := lfq.NewMPSCIndirect(4096)
	var consumed atomix.Int64

	// Single consumer in background
	done := make(chan struct{})
	go func() {
		sw := spin.Wait{}
		for {
			select {
			case <-done:
				// Drain remaining
				for {
					if _, err := q.Dequeue(); err != nil {
						return
					}
					consumed.Add(1)
				}
			default:
				if _, err := q.Dequeue(); err == nil {
					consumed.Add(1)
					sw.Reset()
				} else {
					sw.Once()
				}
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		sw := spin.Wait{}
		i := uintptr(0)
		for pb.Next() {
			for q.Enqueue(i) != nil {
				sw.Once()
			}
			sw.Reset()
			i++
		}
	})
	b.StopTimer()
	close(done)
}

// =============================================================================
// SPMC Benchmarks
// =============================================================================

func BenchmarkSPMC_SingleOp(b *testing.B) {
	q := lfq.NewSPMC[int](1024)

	b.ResetTimer()
	for i := range b.N {
		v := i
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkSPMCIndirect_SingleOp(b *testing.B) {
	q := lfq.NewSPMCIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkSPMCPtr_SingleOp(b *testing.B) {
	q := lfq.NewSPMCPtr(1024)
	val := 42

	b.ResetTimer()
	for range b.N {
		q.Enqueue(unsafe.Pointer(&val))
		q.Dequeue()
	}
}

func BenchmarkSPMCCompactIndirect_SingleOp(b *testing.B) {
	q := lfq.NewSPMCCompactIndirect(1024)

	b.ResetTimer()
	for i := range b.N {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkSPMCIndirect_ParallelConsumers(b *testing.B) {
	q := lfq.NewSPMCIndirect(4096)
	var produced atomix.Int64

	// Single producer in background
	done := make(chan struct{})
	go func() {
		sw := spin.Wait{}
		i := uintptr(0)
		for {
			select {
			case <-done:
				return
			default:
				if q.Enqueue(i) == nil {
					produced.Add(1)
					i++
					sw.Reset()
				} else {
					sw.Once()
				}
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		sw := spin.Wait{}
		for pb.Next() {
			for {
				if _, err := q.Dequeue(); err == nil {
					sw.Reset()
					break
				}
				sw.Once()
			}
		}
	})
	b.StopTimer()
	close(done)
}

// =============================================================================
// Capacity Variants (16, 64, 256, 1024, 4096, 8192)
// =============================================================================

func BenchmarkMPMCIndirect_Capacity(b *testing.B) {
	capacities := []int{16, 64, 256, 1024, 4096, 8192}

	for _, cap := range capacities {
		b.Run(fmt.Sprintf("Cap%d", cap), func(b *testing.B) {
			q := lfq.NewMPMCIndirect(cap)
			b.ResetTimer()
			for i := range b.N {
				q.Enqueue(uintptr(i))
				q.Dequeue()
			}
		})
	}
}

func BenchmarkMPMCCompactIndirect_Capacity(b *testing.B) {
	capacities := []int{16, 64, 256, 1024, 4096, 8192}

	for _, cap := range capacities {
		b.Run(fmt.Sprintf("Cap%d", cap), func(b *testing.B) {
			q := lfq.NewMPMCCompactIndirect(cap)
			b.ResetTimer()
			for i := range b.N {
				q.Enqueue(uintptr(i))
				q.Dequeue()
			}
		})
	}
}

func BenchmarkSPSCIndirect_Capacity(b *testing.B) {
	capacities := []int{16, 64, 256, 1024, 4096, 8192}

	for _, cap := range capacities {
		b.Run(fmt.Sprintf("Cap%d", cap), func(b *testing.B) {
			q := lfq.NewSPSCIndirect(cap)
			b.ResetTimer()
			for i := range b.N {
				q.Enqueue(uintptr(i))
				q.Dequeue()
			}
		})
	}
}

// =============================================================================
// Contention Level Variants (2, 4, 8, 16 workers)
// =============================================================================

func BenchmarkMPMC_ContentionLevels(b *testing.B) {
	workerCounts := []int{2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Workers%d", workers), func(b *testing.B) {
			q := lfq.NewMPMCIndirect(1024)
			numProducers := workers / 2
			numConsumers := workers - numProducers
			if numProducers < 1 {
				numProducers = 1
			}
			if numConsumers < 1 {
				numConsumers = 1
			}

			opsPerProducer := b.N / numProducers
			if opsPerProducer < 1 {
				opsPerProducer = 1
			}

			b.ResetTimer()

			var producerWg sync.WaitGroup
			var consumerWg sync.WaitGroup

			// Consumers (start first)
			done := make(chan struct{})
			for range numConsumers {
				consumerWg.Add(1)
				go func() {
					defer consumerWg.Done()
					sw := spin.Wait{}
					for {
						select {
						case <-done:
							for {
								if _, err := q.Dequeue(); err != nil {
									return
								}
							}
						default:
							if _, err := q.Dequeue(); err == nil {
								sw.Reset()
							} else {
								sw.Once()
							}
						}
					}
				}()
			}

			// Producers
			for p := range numProducers {
				producerWg.Add(1)
				go func(id int) {
					defer producerWg.Done()
					sw := spin.Wait{}
					base := uintptr(id * opsPerProducer)
					for i := range opsPerProducer {
						for q.Enqueue(base+uintptr(i)) != nil {
							sw.Once()
						}
						sw.Reset()
					}
				}(p)
			}

			producerWg.Wait()
			close(done)
			consumerWg.Wait()
		})
	}
}

func BenchmarkMPMCCompactIndirect_ContentionLevels(b *testing.B) {
	workerCounts := []int{2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Workers%d", workers), func(b *testing.B) {
			q := lfq.NewMPMCCompactIndirect(1024)
			numProducers := workers / 2
			numConsumers := workers - numProducers
			if numProducers < 1 {
				numProducers = 1
			}
			if numConsumers < 1 {
				numConsumers = 1
			}

			opsPerProducer := b.N / numProducers
			if opsPerProducer < 1 {
				opsPerProducer = 1
			}

			b.ResetTimer()

			var producerWg sync.WaitGroup
			var consumerWg sync.WaitGroup

			// Consumers (start first)
			done := make(chan struct{})
			for range numConsumers {
				consumerWg.Add(1)
				go func() {
					defer consumerWg.Done()
					sw := spin.Wait{}
					for {
						select {
						case <-done:
							for {
								if _, err := q.Dequeue(); err != nil {
									return
								}
							}
						default:
							if _, err := q.Dequeue(); err == nil {
								sw.Reset()
							} else {
								sw.Once()
							}
						}
					}
				}()
			}

			// Producers
			for p := range numProducers {
				producerWg.Add(1)
				go func(id int) {
					defer producerWg.Done()
					sw := spin.Wait{}
					base := uintptr(id * opsPerProducer)
					for i := range opsPerProducer {
						for q.Enqueue(base+uintptr(i)) != nil {
							sw.Once()
						}
						sw.Reset()
					}
				}(p)
			}

			producerWg.Wait()
			close(done)
			consumerWg.Wait()
		})
	}
}

func BenchmarkMPSC_ContentionLevels(b *testing.B) {
	workerCounts := []int{2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Producers%d", workers), func(b *testing.B) {
			q := lfq.NewMPSCIndirect(1024)
			opsPerWorker := b.N / workers
			if opsPerWorker < 1 {
				opsPerWorker = 1
			}

			// Single consumer
			done := make(chan struct{})
			go func() {
				sw := spin.Wait{}
				for {
					select {
					case <-done:
						for {
							if _, err := q.Dequeue(); err != nil {
								return
							}
						}
					default:
						if _, err := q.Dequeue(); err == nil {
							sw.Reset()
						} else {
							sw.Once()
						}
					}
				}
			}()

			b.ResetTimer()

			var wg sync.WaitGroup
			for w := range workers {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					sw := spin.Wait{}
					base := uintptr(id * opsPerWorker)
					for i := range opsPerWorker {
						for q.Enqueue(base+uintptr(i)) != nil {
							sw.Once()
						}
						sw.Reset()
					}
				}(w)
			}
			wg.Wait()
			b.StopTimer()
			close(done)
		})
	}
}

func BenchmarkSPMC_ContentionLevels(b *testing.B) {
	workerCounts := []int{2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Consumers%d", workers), func(b *testing.B) {
			q := lfq.NewSPMCIndirect(1024)
			opsPerWorker := b.N / workers
			if opsPerWorker < 1 {
				opsPerWorker = 1
			}

			// Single producer
			done := make(chan struct{})
			go func() {
				sw := spin.Wait{}
				i := uintptr(0)
				for {
					select {
					case <-done:
						return
					default:
						if q.Enqueue(i) == nil {
							i++
							sw.Reset()
						} else {
							sw.Once()
						}
					}
				}
			}()

			b.ResetTimer()

			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sw := spin.Wait{}
					for range opsPerWorker {
						for {
							if _, err := q.Dequeue(); err == nil {
								sw.Reset()
								break
							}
							sw.Once()
						}
					}
				}()
			}
			wg.Wait()
			b.StopTimer()
			close(done)
		})
	}
}

// =============================================================================
// Batch Operations
// =============================================================================

func BenchmarkMPMCIndirect_Batch(b *testing.B) {
	batchSizes := []int{1, 4, 8, 16}

	for _, batch := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batch), func(b *testing.B) {
			q := lfq.NewMPMCIndirect(4096)
			ops := b.N / batch
			if ops < 1 {
				ops = 1
			}

			b.ResetTimer()
			for range ops {
				// Enqueue batch
				sw := spin.Wait{}
				for j := range batch {
					for q.Enqueue(uintptr(j)) != nil {
						sw.Once()
					}
					sw.Reset()
				}
				// Dequeue batch
				for range batch {
					for {
						if _, err := q.Dequeue(); err == nil {
							sw.Reset()
							break
						}
						sw.Once()
					}
				}
			}
		})
	}
}

func BenchmarkMPMCCompactIndirect_Batch(b *testing.B) {
	batchSizes := []int{1, 4, 8, 16}

	for _, batch := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batch), func(b *testing.B) {
			q := lfq.NewMPMCCompactIndirect(4096)
			ops := b.N / batch
			if ops < 1 {
				ops = 1
			}

			b.ResetTimer()
			for range ops {
				// Enqueue batch
				sw := spin.Wait{}
				for j := range batch {
					for q.Enqueue(uintptr(j)) != nil {
						sw.Once()
					}
					sw.Reset()
				}
				// Dequeue batch
				for range batch {
					for {
						if _, err := q.Dequeue(); err == nil {
							sw.Reset()
							break
						}
						sw.Once()
					}
				}
			}
		})
	}
}

func BenchmarkSPSCIndirect_Batch(b *testing.B) {
	batchSizes := []int{1, 4, 8, 16}

	for _, batch := range batchSizes {
		b.Run(fmt.Sprintf("Batch%d", batch), func(b *testing.B) {
			q := lfq.NewSPSCIndirect(4096)
			ops := b.N / batch
			if ops < 1 {
				ops = 1
			}

			b.ResetTimer()
			for range ops {
				// Enqueue batch
				for j := range batch {
					q.Enqueue(uintptr(j))
				}
				// Dequeue batch
				for range batch {
					q.Dequeue()
				}
			}
		})
	}
}

// =============================================================================
// High Contention Benchmarks
// =============================================================================

func BenchmarkContention_MPMCIndirect(b *testing.B) {
	q := lfq.NewMPMCIndirect(256)
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for p := range numProducers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			sw := spin.Wait{}
			base := uintptr(id * opsPerProducer)
			for i := range opsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}(p)
	}

	producerWg.Wait()
	close(done)
	consumerWg.Wait()
}

func BenchmarkContention_MPMCCompactIndirect(b *testing.B) {
	q := lfq.NewMPMCCompactIndirect(256)
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for p := range numProducers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			sw := spin.Wait{}
			base := uintptr(id * opsPerProducer)
			for i := range opsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}(p)
	}

	producerWg.Wait()
	close(done)
	consumerWg.Wait()
}

// =============================================================================
// Throughput Benchmarks
// =============================================================================

func BenchmarkThroughput_MPMCIndirect(b *testing.B) {
	q := lfq.NewMPMCIndirect(4096)
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					// Drain remaining
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for p := range numProducers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			sw := spin.Wait{}
			base := uintptr(id * opsPerProducer)
			for i := range opsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}(p)
	}

	// Wait for producers directly
	producerWg.Wait()
	close(done)
	consumerWg.Wait()
}

func BenchmarkThroughput_MPMCCompactIndirect(b *testing.B) {
	q := lfq.NewMPMCCompactIndirect(4096)
	numProducers := runtime.GOMAXPROCS(0) / 2
	numConsumers := runtime.GOMAXPROCS(0) / 2
	if numProducers < 1 {
		numProducers = 1
	}
	if numConsumers < 1 {
		numConsumers = 1
	}

	opsPerProducer := b.N / numProducers
	if opsPerProducer < 1 {
		opsPerProducer = 1
	}

	b.ResetTimer()

	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Consumers (start first)
	done := make(chan struct{})
	for range numConsumers {
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			sw := spin.Wait{}
			for {
				select {
				case <-done:
					// Drain remaining
					for {
						if _, err := q.Dequeue(); err != nil {
							return
						}
					}
				default:
					if _, err := q.Dequeue(); err == nil {
						sw.Reset()
					} else {
						sw.Once()
					}
				}
			}
		}()
	}

	// Producers
	for p := range numProducers {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done()
			sw := spin.Wait{}
			base := uintptr(id * opsPerProducer)
			for i := range opsPerProducer {
				for q.Enqueue(base+uintptr(i)) != nil {
					sw.Once()
				}
				sw.Reset()
			}
		}(p)
	}

	// Wait for producers directly
	producerWg.Wait()
	close(done)
	consumerWg.Wait()
}

// =============================================================================
// Overhead Comparison (SPSC vs MPMC)
// =============================================================================

func BenchmarkOverhead_Comparison(b *testing.B) {
	b.Run("SPSC_Baseline", func(b *testing.B) {
		q := lfq.NewSPSCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})

	b.Run("MPSC_SingleThread", func(b *testing.B) {
		q := lfq.NewMPSCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})

	b.Run("SPMC_SingleThread", func(b *testing.B) {
		q := lfq.NewSPMCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})

	b.Run("MPMC_SingleThread", func(b *testing.B) {
		q := lfq.NewMPMCIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})

	b.Run("MPMCCompact_SingleThread", func(b *testing.B) {
		q := lfq.NewMPMCCompactIndirect(1024)
		b.ResetTimer()
		for i := range b.N {
			q.Enqueue(uintptr(i))
			q.Dequeue()
		}
	})
}
