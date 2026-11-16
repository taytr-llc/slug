// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package prospectwork constructs full payable SLUG block candidates and
// matching job-bound prospect proofs. It preserves the node's transaction set,
// checks byte commitments and resource limits, and never hashes a header or
// broadcasts work. Fees, contextual sigops and transaction spend validity
// still require a trusted node's UTXO view and proposal validation. A candidate
// is not qualifying work, evidence of job issuance, birth or an accepted block.
package prospectwork
