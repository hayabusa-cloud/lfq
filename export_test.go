// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfq

// StalledMPMCSeqForTest constructs the state after slot zero was published
// and consumed, with both workers paused before their cursor CAS.
func StalledMPMCSeqForTest() (*MPMCIndirectSeq, *MPMCPtrSeq) {
	const capacity = 2
	words, ptrs := NewMPMCIndirectSeq(capacity), NewMPMCPtrSeq(capacity)
	words.buffer[0].entry.StoreRelaxed(capacity, 0)
	ptrs.buffer[0].entry.StoreRelaxed(capacity, 0)
	return words, ptrs
}

// MPMCPtrWithAdmittedBurstForTest widens only the admission limit to model
// producers that all passed the capacity check before any claimed a ticket.
func MPMCPtrWithAdmittedBurstForTest(capacity int) (*MPMCPtr, func() uint64) {
	q := NewMPMCPtr(capacity)
	q.capacity = uint64(len(q.buffer) + 1)
	return q, q.tail.LoadRelaxed
}
