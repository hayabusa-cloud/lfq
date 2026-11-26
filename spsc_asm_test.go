// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build amd64 || arm64

package lfq_test

import (
	"testing"

	"code.hybscloud.com/lfq"
)

func TestSPSCAsmMatchesGo(t *testing.T) {
	const capacity = 16
	indirect := lfq.NewSPSCIndirect(capacity)
	typed := lfq.NewSPSC[uintptr](capacity)

	checkDequeue := func() {
		t.Helper()
		got, gotErr := indirect.Dequeue()
		want, wantErr := typed.Dequeue()
		if got != want || gotErr != wantErr {
			t.Fatalf("Dequeue() = (%d, %v), want (%d, %v)", got, gotErr, want, wantErr)
		}
	}
	for i := range 1000 {
		v := uintptr(i)
		if got, want := indirect.Enqueue(v), typed.Enqueue(&v); got != want {
			t.Fatalf("Enqueue(%d) = %v, want %v", v, got, want)
		}
		if i%3 == 0 {
			checkDequeue()
		}
	}
	// Drain any remaining values and check the empty result.
	for range capacity + 1 {
		checkDequeue()
	}
}

// Compare the public typed Go and indirect assembly queues with uintptr payloads.
// The enqueue benchmarks include the cost of draining each full batch.

func BenchmarkSPSCTypedGoEnqueueDequeue(b *testing.B) {
	q := lfq.NewSPSC[uintptr](1024)

	for i := 0; b.Loop(); i++ {
		v := uintptr(i)
		q.Enqueue(&v)
		q.Dequeue()
	}
}

func BenchmarkSPSCIndirectAsmEnqueueDequeue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)

	for i := 0; b.Loop(); i++ {
		q.Enqueue(uintptr(i))
		q.Dequeue()
	}
}

func BenchmarkSPSCTypedGoEnqueue(b *testing.B) {
	q := lfq.NewSPSC[uintptr](1024)

	for i := 0; b.Loop(); i++ {
		v := uintptr(i)
		q.Enqueue(&v)
		if i%1024 == 1023 {
			// Drain to avoid full
			for range 1024 {
				q.Dequeue()
			}
		}
	}
}

func BenchmarkSPSCIndirectAsmEnqueue(b *testing.B) {
	q := lfq.NewSPSCIndirect(1024)

	for i := 0; b.Loop(); i++ {
		q.Enqueue(uintptr(i))
		if i%1024 == 1023 {
			// Drain to avoid full
			for range 1024 {
				q.Dequeue()
			}
		}
	}
}
