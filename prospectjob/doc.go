// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package prospectjob defines immutable five-minute mining-job assertions.
// A job binds a public challenge, gym key, complete proof context and issuance
// interval into a reproducible ID. Rule version 1 assigns the career anchor to
// issuance and permits new admissions only before the exclusive expiry.
//
// New and ParseJob establish byte consistency, not issuer authenticity, clock
// correctness, chain canonicality, work validity, or registered ownership.
// A trusted issuer must generate the unpredictable challenge, persist the job
// before returning it, and verify admission against that immutable stored job.
// This package does not read clocks, generate challenges, hold keys, contact a
// node, or maintain storage. It never retroactively assigns jobs to v1 work.
package prospectjob
