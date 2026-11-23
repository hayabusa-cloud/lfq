// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq_test

import (
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/lfq"
)

// sess.New uses four slots per data queue.
const sessionQueueCapacity = 4

func BenchmarkSPSCSessionHandoff(b *testing.B) {
	var message any = true

	b.Run("Queue", func(b *testing.B) {
		q := lfq.NewSPSC[any](sessionQueueCapacity)
		send := func() bool { return q.Enqueue(&message) == nil }
		var received any
		receive := func() bool {
			var err error
			received, err = q.Dequeue()
			return err == nil
		}
		benchmarkSessionHandoff(b, send, receive)
		if received != message {
			b.Fatalf("received %v, want %v", received, message)
		}
	})
	b.Run("Channel", func(b *testing.B) {
		ch := make(chan any, sessionQueueCapacity)
		send := func() bool {
			select {
			case ch <- message:
				return true
			default:
				return false
			}
		}
		var received any
		receive := func() bool {
			select {
			case received = <-ch:
				return true
			default:
				return false
			}
		}
		benchmarkSessionHandoff(b, send, receive)
		if received != message {
			b.Fatalf("received %v, want %v", received, message)
		}
	})
}

// One iteration ends when the consumer has received one message. Both owners
// use the same backoff policy that sess uses for synchronous operations.
func benchmarkSessionHandoff(b *testing.B, send, receive func() bool) {
	transfer := func(try func() bool) int64 {
		var waits int64
		for range b.N {
			var backoff iox.Backoff
			for !try() {
				waits++
				backoff.Wait()
			}
		}
		return waits
	}

	start := make(chan struct{})
	sent := make(chan int64)
	go func() {
		<-start
		sent <- transfer(send)
	}()

	b.ResetTimer()
	close(start)
	waits := transfer(receive)
	waits += <-sent
	b.StopTimer()
	b.ReportMetric(float64(waits)/float64(b.N), "waits/op")
}

func BenchmarkSPSCSessionFullRetry(b *testing.B) {
	q := lfq.NewSPSC[any](sessionQueueCapacity)
	var message any = true
	for range sessionQueueCapacity {
		if err := q.Enqueue(&message); err != nil {
			b.Fatal(err)
		}
	}

	// Model an event loop servicing the peer before retrying the pending send.
	for b.Loop() {
		if err := q.Enqueue(&message); err != lfq.ErrWouldBlock {
			b.Fatalf("enqueue on full queue returned %v", err)
		}
		if value, err := q.Dequeue(); err != nil || value != message {
			b.Fatalf("dequeue returned (%v, %v), want (%v, nil)", value, err, message)
		}
		if err := q.Enqueue(&message); err != nil {
			b.Fatal(err)
		}
	}
}
