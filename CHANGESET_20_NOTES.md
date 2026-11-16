# Change set 20: complete payable mining candidates

Branch: `slug-mining-templates`, stacked on set-19 implementation `f51f8cc3`.
Mike authorized deploying/verifying set 19 while starting set 20, then
authorized continued verified deployments toward the desktop mining pilot.
Set 19 was promoted as `56a4889d`; set 20 was built during its observation.

## What changed

- Added `prospectwork`: strict bounded complete GBT parsing, typed-input
  validation and deterministic construction of a payable job-bound candidate.
  It preserves all transactions, validates canonical encodings, recomputes
  txid/wtxid/weight/dependencies, checks fee/subsidy arithmetic and final
  consensus/template resource limits, and takes an explicit local payout.
- Constructed BIP34 coinbase height, an eight-byte extra nonce, node auxiliary
  flags, payout, SLUGPR02 commitment and optional witness commitment. Full block
  serialization retains witness; the matching prospect proof uses stripped
  coinbase bytes and the index-zero Merkle branch.
- Handled this node's raw 32-byte GBT witness commitment explicitly. It is
  recomputed independently before constructing the six-byte-prefix output.
  No guessed Bitcoin Core script representation or silent transaction omission.
- Added `ResolveTemplate` using the very same bracketed GBT result as its
  context. Existing `Resolve` behavior and call ordering remain intact.
- Added bounded, canonical, nonbroadcast `Proposal` with current-node checks
  before and after. Only this RPC permits null success; existing required-result
  checks remain unchanged. Rejection contents and credentials are not exposed.
- Added `slugprospect prepare-work-node` with explicit job, policy, RPC and
  payout inputs, optional deterministic extra nonce, client-side issuance-window
  checks and explicit unverified/unadmitted output labels.
- Recorded the boundary and follow-up work in `PROSPECT_WORK.md` and
  `ADR_002_PAYABLE_CANDIDATES.md`; corrected the build plan's wallet reference
  to the existing D48 decision.

## Deliberately unchanged and known debt

The builder does not hash, scan nonces, sign, broadcast or establish birth.
Node proposal validation skips PoW acceptance, but existing node internals can
still compute Argon identities. This remains bounded private-RPC work.
Fees and contextual signature-operation costs require the trusted node's UTXO
view; proposal checks the resulting candidate against it. A successful proposal
is an observation, not a promise of later acceptance or permanent canonicality.

The existing authority transport, 64/72 KiB limits, job/proof/authorization
formats, registry schemas, D44/D68 rules, consensus, genesis, seed startup,
volume, public P2P endpoint, private RPC, checkpoint behavior and dependencies
are unchanged. No real private key or production payout was used. The Phase A
chain remains disposable and existing CI debt remains; CI is not claimed green.

Automatic worker renewal/scanning, durable retries, hosted wallet/login with
tested full export, desktop integration, calibrated target and public hosting
remain subsequent work. Mike approved broadcast-first with a narrow canonical
winning-block exception within the original job deadline; this is to be recorded
and implemented as D69 in the following set. This candidate-only set does not
change admission rules. Complete remote
templates need a separately bounded transport contract. A full template that
cannot fit the additional job output is rejected, never silently truncated.

## Verification

- Three independently generated Python full-block vectors passed byte-for-byte:
  coinbase-only, dependent legacy transactions, and mixed witness transactions
  with odd Merkle levels. Expected payout, coinbase, branch, root, full/stripped
  bytes and weight are derived without project Go output or header PoW.
- Adversarial tests cover exact JSON keys (including case/escaped aliases),
  duplicates/nulls/unknowns/depth/size, invalid transaction encodings and hashes,
  dependency order, double spends, fee arithmetic, witness commitments, foreign
  jobs/payouts, context changes and resource overflow after coinbase creation.
- Normal and tagged affected suites passed. Consolidated race run passed:
  `prospectwork` 1.346s, `prospectnode` 3.689s, `cmd/slugprospect` 6.875s,
  `prospectservice` 7.684s, `registry` 24.294s. This includes old context and
  authority regressions, exact proposal null/error semantics, before/after
  parent/identity/time checks and cheap-profile refusal.
- All ten modules and Linux/amd64 command binaries built. Formatting, diff
  checks and affected-package vet passed. Independent reviews found and fixed
  JSON case-alias acceptance before release.
- The required normal-node smoke mined two isolated blocks at 10 SLUG subsidy,
  reported checkpoint conflict without veto, synchronized to the public seed
  at height 61, verified the chain, retained the finalized checkpoint and
  restarted cleanly. Existing authenticated birth/service recovery checks passed.
- The new CLI prepared a 240-byte payable candidate (weight 960), independently
  matched its complete bytes and proof, and obtained proposal acceptance. A
  one-satoshi overpayment with a recomputed Merkle root was rejected as
  `bad-cb-value`. The node remained at height 61: no candidate was mined or
  broadcast. All service/node test processes shut down cleanly.

Smoke run: `/private/tmp/slug-set20-smoke-38pe9bx9`.
Saved evidence: `/Users/taytrrip/slug/set20-smoke.json`.
