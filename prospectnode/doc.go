// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package prospectnode obtains prospect verification context from an explicitly
// configured, authenticated production SLUG mainnet RPC endpoint. Policy is
// supplied separately by the operator; proofs cannot select the work target.
//
// A consistent RPC observation and a fresh admission check are not a transaction
// with the chain. A reorganization can occur after the final observation. The
// package does not establish ownership, discovery time, finality, or full-block
// validity, and preserves the node's peer and synchronization requirements.
package prospectnode
