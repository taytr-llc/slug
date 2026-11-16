// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package gymauth authenticates a gym key's authorization of one exact prospect
// admission. A gym ID is derived from a canonical secp256k1 public key and chain
// identity. The admission signature binds policy and the complete proof bytes.
//
// Authorization is separate from work verification, trusted node context,
// freshness, ownership transfers, wallet/account enrollment, and birth time.
// Anyone may relay the same authorization; the prospect registry still supplies
// durable deduplication. Client applications can sign automatically without a
// manual prospect-signing step. This package performs no key storage or I/O.
package gymauth
