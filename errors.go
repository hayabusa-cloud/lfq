// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import "code.hybscloud.com/iox"

// ErrWouldBlock indicates the operation cannot proceed immediately.
//
// A full queue can refuse an enqueue, and an empty queue can refuse a dequeue.
// [MPSC.Dequeue], [MPSCIndirect.Dequeue], [MPSCPtr.Dequeue], [MPSCSeq.Dequeue],
// and [MPMCSeq.Dequeue] can also return it while a producer is publishing the
// next item. [SPMCSeq.Enqueue] and [MPMCSeq.Enqueue] can return it while a
// consumer is releasing the next slot for reuse.
//
// During live polling, ErrWouldBlock signals backpressure. An event loop can
// retry on a later turn, and a synchronous caller can wait with [iox.Backoff].
//
// This is an alias for [iox.ErrWouldBlock] for ecosystem consistency.
//
// Example:
//
//	backoff := iox.Backoff{}
//	for {
//	    err := q.Enqueue(&item)
//	    if err == nil {
//	        backoff.Reset()
//	        break
//	    }
//	    if lfq.IsWouldBlock(err) {
//	        backoff.Wait()  // Adaptive backpressure
//	        continue
//	    }
//	    return err  // Unexpected error
//	}
var ErrWouldBlock = iox.ErrWouldBlock

// IsWouldBlock reports whether err indicates the operation would block.
// Delegates to [iox.IsWouldBlock] for wrapped error support.
func IsWouldBlock(err error) bool {
	return iox.IsWouldBlock(err)
}

// IsSemantic reports whether err is a control flow signal (not a failure).
// Delegates to [iox.IsSemantic].
func IsSemantic(err error) bool {
	return iox.IsSemantic(err)
}

// IsNonFailure reports whether err represents a non-failure condition.
// Returns true for nil, ErrWouldBlock, or ErrMore.
// Delegates to [iox.IsNonFailure].
func IsNonFailure(err error) bool {
	return iox.IsNonFailure(err)
}
