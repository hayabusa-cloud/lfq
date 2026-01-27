// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

import "code.hybscloud.com/iox"

// ErrWouldBlock indicates the operation cannot proceed immediately.
//
// For Enqueue: the queue is full (backpressure)
// For Dequeue: the queue is empty (no data available)
//
// ErrWouldBlock is a control flow signal, not a failure. The caller should
// retry the operation later (with backoff or yield) rather than propagating
// the error.
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
