// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package registry durably stores immutable, gym-bound prospect work evidence.
// Receipts remain conditional on the supplied context. They do not establish
// ownership, canonicality, discovery time, or fighter birth. Explicit schema-2
// registries additionally verify proof-specific authorization by the key that
// derives the gym ID; schema-1 evidence retains its unauthenticated meaning.
// Explicit schema-3 stores issue fixed five-minute jobs and record immutable
// D68 birth at issuance, under a trusted authority clock. They do not prove the
// exact instant of mining; historical retries never reset birth or eligibility.
//
// Each dedicated registry belongs to one network and genesis. Its append-only
// API reserves prospect IDs permanently and orders receipts with consecutive
// local sequence numbers. Duplicate submissions return the original evidence;
// no clock, update, delete, or chain-status operation exists.
//
// Synchronous atomic writes precede acknowledgment. After an uncertain write
// failure the handle must be closed and reopened before it can be used again.
// Stored evidence is reverified before reads emit it. These checks assume
// trusted local storage: they do not authenticate chain policy, prevent an
// administrator from rolling back the directory, or provide a signed archive.
package registry
