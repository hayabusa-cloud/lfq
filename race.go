// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

//go:build race

package lfq

// RaceEnabled is true when the race detector is active.
// Used by tests to skip concurrent tests for generic [T] queue variants,
// which trigger false positives due to cross-variable memory ordering.
const RaceEnabled = true
