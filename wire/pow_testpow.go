//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

// TestPoW reports whether this build uses the incompatible, reduced-cost
// proof-of-work profile for isolated tests. It cannot be changed at runtime.
const TestPoW = true

// powArgon2Memory is deliberately unsuitable for a public network. The explicit
// slugtestpow build tag makes block grinding affordable in isolated tests.
const powArgon2Memory = 64
