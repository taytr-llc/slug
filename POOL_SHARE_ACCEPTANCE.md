# Durable pool share acceptance — Set 53

Status: implemented and focused validation passed. D79 and D80 are approved; this component does
not calculate earnings, close rounds, send payments or admit fighters.

## Context and decision

The Set 52 checker verifies work but retains nothing. A pool needs original
issued work, duplicate recovery and a durable block-delivery intent before it
can safely acknowledge a share or broadcast a winning block. Use a separate
bounded LevelDB journal, the existing manifest issuer and original-session
authorization. Do not repurpose the fighter registry or miner outbox: neither
represents ordinary pool shares.

`poolshare.Create` and `Open` take explicit configuration, a concrete
`registry.ManifestStore`, a trusted node adapter and a
`prospectservice.SessionAuthorizer`. Configuration pins the authority, node
policy, pool payout address and share target, with finite work/submission/byte
limits. This initial coordinator uses a fixed configured share target. No
claimant may choose a target, payout, timestamp, hash or eligibility flag.
Use a dedicated manifest issuer whose issuance access belongs to this pool.
Its retained job count includes issuance that did not reach a pool work receipt.
Capacity exhaustion stops new work; it does not evict evidence. This bounded
candidate journal is not a production retention or archival policy.

`Issue(ctx, token)` resolves a full template, durably issues the gym-bound job,
builds the exact configured-payout candidate, checks the node proposal and
saves work before returning it. `Work(ctx, token, jobID)` recovers saved work.
`Submit(ctx, token, jobID, nonce)` recomputes Set 52 classification from that
saved work. `Lookup` with the same arguments returns the historical submission.
All operations bind to the original session and gym. No token is stored.

## Acceptance and delivery

A timely pool-target share may receive its immutable acceptance sequence and
time after a successful current-parent check. Recheck session and time at the
synchronous commit boundary. Ordinary expired or noncurrent shares do not
receive acceptance. Prospect-target qualification alone is not pool credit.

Block-winning evidence is saved before broadcast. The exact saved candidate
was built with the pinned payout; only its nonce can differ. A winning share
that lacks ordinary acceptance—including a late winner or one missing the
pool target—receives acceptance only after the trusted node observes that exact
block on the canonical chain. The observation uses the locally computed hash,
never an imported work summary. D80 does not extend fighter admission.

Submission processing, an unknown transport outcome, explicit rejection and a
canonical observation remain distinct delivery states. A successful broadcast
alone grants no canonical exception. Expiry after an ordinary acceptance does
not revoke it. Exact duplicate submissions and restart recovery preserve the
original record and never allocate another acceptance sequence.

Retain submission order separately from acceptance order, with the winning
block hash and height. A late winning share belongs to that block's eventual
round; receipt order alone cannot determine its D79 allocation. Canonical
observations are historical observations, not irreversible finality or proof
of sixty descendants, maturity or available funds.

## Durability and bounds

Use synchronous atomic batches and independent duplicate/sequence indexes.
An uncertain write poisons the handle; reopen and inspect durable evidence
before resuming. Never broadcast after an uncertain intent write. Reserve
space for later status changes so a full journal can reconcile retained work.
Persist clock high water, reject rollback and preserve records on failures.
One nonblocking operation gate bounds work; Close waits for the active call.
The cheap test PoW profile is refused. No automatic migrations or deletion.

Opening checks configuration, original issuer jobs, counts, indexes and work
integrity. Recompute work classifications before trusting saved summaries.
Local journal integrity is not a portable issuer signature or protection
against an operator rewriting an entire authority and journal consistently.

## Scope and validation

The existing fixed normal-Argon vectors supply independent work/hash/target
expectations. Test expiry, canonical-only late credit, exact payout-bound work,
restart duplicates, revocation, poisoned writes, capacity recovery and corrupt
storage. Exercise the real RPC adapter against controlled node responses.

A public pool transport, automatic miner integration, round accounting,
reorganization handling, rounding, transaction fees and settlement remain
separate work. No service starts from a seed deployment, and no wallet signing
or production keys are used by this component. D74 app organization remains
reserved for the final joint design review.
