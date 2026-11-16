// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package prospect verifies production work committed to one gym identifier
// against a separately supplied context. It authenticates neither that
// context's chain/policy assertions nor control of the named gym. A verified
// record establishes no ownership, discovery time, fighter birth, registration,
// pool acceptance, or validity of the unprovided block transactions.
//
// The fixed versioned format commits the chain, DNA version, prospect target,
// and gym before hashing. Its index-zero Merkle path proves that commitment
// against the header; it cannot detect mutations hidden in sibling subtrees.
// The independent prospect target is explicit, with no production default.
//
// Verification rejects the slugtestpow build profile, bounds parsing work,
// and serializes this package's production Argon calls. Callers must still
// bound admission and queued requests. The hash cannot be cancelled after it
// starts, and this package does not impose a global node resource quota.
package prospect
