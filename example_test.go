// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build !race

// This file contains examples that use atomix concurrency primitives.
// These trigger false positives with Go's race detector because atomix
// atomic operations appear as regular memory accesses to the detector.
// The examples are correct; they're excluded from race testing.

package lfq_test

import (
	"fmt"
	"slices"
	"sync"
	"unsafe"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// ExampleNewSPSC demonstrates a basic SPSC queue for pipeline stages.
func ExampleNewSPSC() {
	// Create a single-producer single-consumer queue
	q := lfq.NewSPSC[int](8)

	// Producer sends 5 values
	for i := 1; i <= 5; i++ {
		v := i * 10
		q.Enqueue(&v)
	}

	// Consumer receives values
	for range 5 {
		v, _ := q.Dequeue()
		fmt.Println(v)
	}

	// Output:
	// 10
	// 20
	// 30
	// 40
	// 50
}

// ExampleNewMPMC demonstrates a multi-producer multi-consumer queue.
func ExampleNewMPMC() {
	q := lfq.NewMPMC[string](16)

	// Producers
	var wg sync.WaitGroup
	for p := range 3 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			msg := fmt.Sprintf("msg from producer %d", id)
			for q.Enqueue(&msg) != nil {
				backoff.Wait()
			}
		}(p)
	}

	// Wait for producers then consume
	wg.Wait()

	for {
		msg, err := q.Dequeue()
		if err != nil {
			break
		}
		fmt.Println(msg)
	}

	// Unordered output:
	// msg from producer 0
	// msg from producer 1
	// msg from producer 2
}

// ExampleBuild demonstrates the builder API for automatic algorithm selection.
func ExampleBuild() {
	// SPSC - both constraints
	spsc := lfq.Build[int](lfq.New(64).SingleProducer().SingleConsumer())

	// MPSC - only single consumer constraint
	mpsc := lfq.Build[int](lfq.New(64).SingleConsumer())

	// SPMC - only single producer constraint
	spmc := lfq.Build[int](lfq.New(64).SingleProducer())

	// MPMC - no constraints
	mpmc := lfq.Build[int](lfq.New(64))

	fmt.Println("SPSC capacity:", spsc.Cap())
	fmt.Println("MPSC capacity:", mpsc.Cap())
	fmt.Println("SPMC capacity:", spmc.Cap())
	fmt.Println("MPMC capacity:", mpmc.Cap())

	// Output:
	// SPSC capacity: 64
	// MPSC capacity: 64
	// SPMC capacity: 64
	// MPMC capacity: 64
}

// ExampleNewMPMCIndirect demonstrates pool index passing.
func ExampleNewMPMCIndirect() {
	// Simulate a buffer pool
	bufferPool := make([][]byte, 4)
	for i := range bufferPool {
		bufferPool[i] = make([]byte, 1024)
	}

	// Queue passes indices, not buffers
	q := lfq.NewMPMCIndirect(8)

	// Producer "allocates" from pool by passing index
	for i := range len(bufferPool) {
		q.Enqueue(uintptr(i))
	}

	// Consumer retrieves indices and accesses pool
	for range len(bufferPool) {
		idx, _ := q.Dequeue()
		buf := bufferPool[idx]
		fmt.Printf("Got buffer %d with len %d\n", idx, len(buf))
	}

	// Output:
	// Got buffer 0 with len 1024
	// Got buffer 1 with len 1024
	// Got buffer 2 with len 1024
	// Got buffer 3 with len 1024
}

// ExampleNewMPMCPtr demonstrates zero-copy pointer passing.
func ExampleNewMPMCPtr() {
	type Message struct {
		ID   int
		Data string
	}

	q := lfq.NewMPMCPtr(8)

	// Producer creates and enqueues messages
	messages := []*Message{
		{ID: 1, Data: "hello"},
		{ID: 2, Data: "world"},
	}

	for msg := range slices.Values(messages) {
		q.Enqueue(unsafe.Pointer(msg))
	}

	// Consumer receives pointers directly - no copy
	for {
		ptr, err := q.Dequeue()
		if err != nil {
			break
		}
		msg := (*Message)(ptr)
		fmt.Printf("Message %d: %s\n", msg.ID, msg.Data)
	}

	// Output:
	// Message 1: hello
	// Message 2: world
}

// ExampleIsWouldBlock demonstrates error handling patterns.
func ExampleIsWouldBlock() {
	q := lfq.NewSPSC[int](2) // Cap()=2

	// Fill the queue
	one, two := 1, 2
	q.Enqueue(&one)
	q.Enqueue(&two)

	// Queue is full
	five := 5
	err := q.Enqueue(&five)
	if lfq.IsWouldBlock(err) {
		fmt.Println("Queue full - applying backpressure")
	}

	// Drain the queue
	q.Dequeue()
	q.Dequeue()

	// Queue is empty
	_, err = q.Dequeue()
	if lfq.IsWouldBlock(err) {
		fmt.Println("Queue empty - no data available")
	}

	// Output:
	// Queue full - applying backpressure
	// Queue empty - no data available
}

// ExampleMPSC_eventAggregation demonstrates using MPSC for event aggregation.
func ExampleMPSC_eventAggregation() {
	type Event struct {
		Source string
		Value  int
	}

	q := lfq.NewMPSC[Event](64)

	// Multiple event sources (producers)
	var wg sync.WaitGroup
	var total atomix.Int64

	for source := range slices.Values([]string{"sensor-A", "sensor-B", "sensor-C"}) {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for i := 1; i <= 3; i++ {
				ev := Event{Source: name, Value: i}
				for q.Enqueue(&ev) != nil {
					backoff.Wait()
				}
				backoff.Reset()
				total.Add(1)
			}
		}(source)
	}

	// Wait for producers
	wg.Wait()

	// Single consumer aggregates all events
	var sum int
	for {
		ev, err := q.Dequeue()
		if err != nil {
			break
		}
		sum += ev.Value
	}

	fmt.Printf("Total events: %d, Sum of values: %d\n", total.Load(), sum)

	// Output:
	// Total events: 9, Sum of values: 18
}

// Example_compactMode demonstrates compact mode for memory-constrained scenarios.
func Example_compactMode() {
	// Compact Indirect: 8 bytes per slot (vs 16 bytes standard)
	// Values limited to 63 bits
	q := lfq.New(1024).Compact().BuildIndirect()

	// Use for buffer pool indices where nil/zero is not needed
	for i := uintptr(1); i <= 5; i++ {
		q.Enqueue(i) // Values must be < (1 << 63)
	}

	fmt.Printf("Queue capacity: %d\n", q.Cap())

	// Dequeue until empty (ErrWouldBlock)
	for {
		idx, err := q.Dequeue()
		if lfq.IsWouldBlock(err) {
			break
		}
		fmt.Printf("Index: %d\n", idx)
	}

	// Output:
	// Queue capacity: 1024
	// Index: 1
	// Index: 2
	// Index: 3
	// Index: 4
	// Index: 5
}

// Example_bufferPool demonstrates using Indirect queue for buffer pool management.
func Example_bufferPool() {
	const poolSize = 4
	const bufSize = 64

	// Create buffer pool
	pool := make([][]byte, poolSize)
	for i := range pool {
		pool[i] = make([]byte, bufSize)
	}

	// Free list tracks available buffer indices
	freeList := lfq.NewSPSCIndirect(poolSize)
	freeCount := poolSize

	// Initialize: all buffers are free
	for i := range poolSize {
		freeList.Enqueue(uintptr(i))
	}

	// Allocate a buffer
	allocate := func() ([]byte, uintptr, bool) {
		idx, err := freeList.Dequeue()
		if err != nil {
			return nil, 0, false // Pool exhausted
		}
		freeCount--
		return pool[idx], idx, true
	}

	// Release a buffer back to pool
	release := func(idx uintptr) {
		freeList.Enqueue(idx)
		freeCount++
	}

	// Usage demonstration
	fmt.Printf("Free buffers: %d\n", freeCount)

	buf1, idx1, ok := allocate()
	if ok {
		copy(buf1, "hello")
		fmt.Printf("Allocated buffer %d, free: %d\n", idx1, freeCount)
	}

	buf2, idx2, ok := allocate()
	if ok {
		copy(buf2, "world")
		fmt.Printf("Allocated buffer %d, free: %d\n", idx2, freeCount)
	}

	release(idx1)
	fmt.Printf("Released buffer %d, free: %d\n", idx1, freeCount)

	release(idx2)
	fmt.Printf("Released buffer %d, free: %d\n", idx2, freeCount)

	// Output:
	// Free buffers: 4
	// Allocated buffer 0, free: 3
	// Allocated buffer 1, free: 2
	// Released buffer 0, free: 3
	// Released buffer 1, free: 4
}

// Example_backpressure demonstrates handling backpressure with a full queue.
func Example_backpressure() {
	// Small queue to demonstrate backpressure
	q := lfq.NewSPSC[int](3) // Cap()=4 with new semantics

	// Fill the queue
	filled := 0
	for i := 1; i <= 10; i++ {
		v := i
		err := q.Enqueue(&v)
		if err == nil {
			filled++
		} else if lfq.IsWouldBlock(err) {
			fmt.Printf("Backpressure at item %d (queue full)\n", i)
			break
		}
	}
	fmt.Printf("Filled %d items\n", filled)

	// Drain some items to make room
	for range 2 {
		v, _ := q.Dequeue()
		fmt.Printf("Drained: %d\n", v)
	}

	// Now we can enqueue more
	v := 100
	if q.Enqueue(&v) == nil {
		fmt.Println("Enqueued 100 after draining")
	}

	// Output:
	// Backpressure at item 5 (queue full)
	// Filled 4 items
	// Drained: 1
	// Drained: 2
	// Enqueued 100 after draining
}

// Example_batchProcessing demonstrates collecting items into batches.
func Example_batchProcessing() {
	q := lfq.NewSPSC[int](64)

	// Single producer submits items sequentially
	for i := 1; i <= 9; i++ {
		v := i
		q.Enqueue(&v)
	}

	// Batch processing: collect up to batchSize items
	batchSize := 4
	batch := make([]int, 0, batchSize)
	batchNum := 0

	for {
		for len(batch) < batchSize {
			v, err := q.Dequeue()
			if err != nil {
				break
			}
			batch = append(batch, v)
		}

		if len(batch) == 0 {
			break
		}

		batchNum++
		fmt.Printf("Batch %d: %v\n", batchNum, batch)
		batch = batch[:0]
	}

	// Output:
	// Batch 1: [1 2 3 4]
	// Batch 2: [5 6 7 8]
	// Batch 3: [9]
}
