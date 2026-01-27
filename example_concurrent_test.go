// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build !race

// This file contains examples with concurrent producer/consumer goroutines.
// These trigger false positives with Go's race detector because lock-free
// queue synchronization uses atomic sequences that the detector cannot see.
// The examples are correct; they're excluded from race testing.

package lfq_test

import (
	"fmt"
	"sync"

	"code.hybscloud.com/atomix"
	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// Example_workerPool demonstrates a worker pool pattern using MPMC.
func Example_workerPool() {
	type Job struct {
		ID     int
		Input  int
		Result int
	}

	// Job queue and results
	jobs := lfq.NewMPMC[Job](16)
	results := make([]int, 5)
	var wg sync.WaitGroup
	var completed atomix.Int32

	// Start 3 workers
	for w := range 3 {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			backoff := iox.Backoff{}
			for completed.Load() < 5 {
				job, err := jobs.Dequeue()
				if err != nil {
					backoff.Wait()
					continue
				}
				backoff.Reset()
				// Process job: square the input
				job.Result = job.Input * job.Input
				results[job.ID] = job.Result
				completed.Add(1)
			}
		}(w)
	}

	// Submit 5 jobs
	backoff := iox.Backoff{}
	for i := range 5 {
		job := Job{ID: i, Input: i + 1}
		for jobs.Enqueue(&job) != nil {
			backoff.Wait()
		}
		backoff.Reset()
	}

	wg.Wait()

	// Print results
	for i, r := range results {
		fmt.Printf("Job %d: %d² = %d\n", i, i+1, r)
	}

	// Output:
	// Job 0: 1² = 1
	// Job 1: 2² = 4
	// Job 2: 3² = 9
	// Job 3: 4² = 16
	// Job 4: 5² = 25
}

// Example_pipeline demonstrates a multi-stage pipeline using SPSC queues.
func Example_pipeline() {
	// Pipeline: Generate → Double → Print
	stage1to2 := lfq.NewSPSC[int](8) // Generate → Double
	stage2to3 := lfq.NewSPSC[int](8) // Double → Print

	var wg sync.WaitGroup
	results := make([]int, 0, 5)
	var mu sync.Mutex

	// Stage 1: Generate numbers 1-5
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for i := 1; i <= 5; i++ {
			v := i
			for stage1to2.Enqueue(&v) != nil {
				backoff.Wait()
			}
			backoff.Reset()
		}
	}()

	// Stage 2: Double each number
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoffDeq := iox.Backoff{}
		backoffEnq := iox.Backoff{}
		processed := 0
		for processed < 5 {
			v, err := stage1to2.Dequeue()
			if err != nil {
				backoffDeq.Wait()
				continue
			}
			backoffDeq.Reset()
			doubled := v * 2
			for stage2to3.Enqueue(&doubled) != nil {
				backoffEnq.Wait()
			}
			backoffEnq.Reset()
			processed++
		}
	}()

	// Stage 3: Collect results
	wg.Add(1)
	go func() {
		defer wg.Done()
		backoff := iox.Backoff{}
		for len(results) < 5 {
			v, err := stage2to3.Dequeue()
			if err != nil {
				backoff.Wait()
				continue
			}
			backoff.Reset()
			mu.Lock()
			results = append(results, v)
			mu.Unlock()
		}
	}()

	wg.Wait()

	for i, v := range results {
		fmt.Printf("Stage output %d: %d\n", i, v)
	}

	// Output:
	// Stage output 0: 2
	// Stage output 1: 4
	// Stage output 2: 6
	// Stage output 3: 8
	// Stage output 4: 10
}
