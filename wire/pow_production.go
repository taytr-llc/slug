//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

// TestPoW reports whether this build uses the incompatible, reduced-cost
// proof-of-work profile for isolated tests. It cannot be changed at runtime.
const TestPoW = false

// powArgon2Memory is the production memory cost in KiB: 256 MiB.
const powArgon2Memory = 256 * 1024
