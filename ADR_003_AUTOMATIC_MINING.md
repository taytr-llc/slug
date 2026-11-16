# ADR-003: Broadcast block wins before prospect admission

**Status:** Accepted for set 21 implementation  
**Date:** November 16, 2026  
**Decider:** Mike approved D69; implementation details follow existing D44/D68.

## Context

Broadcasting a winning block changes the current parent before its fighter can
be admitted. Delaying propagation until signing and admission finish would tie
chain progress to the off-chain authority. The two deliveries cannot be atomic.

## Decision

Synchronize found public work, then broadcast block-target wins immediately,
before waiting for a gym signature or prospect admission. Treat block and
prospect thresholds independently. When the parent has advanced, permit a
prospect only if its exact winning block and original parent are observed on
the canonical chain, with a matching raw header. A submission acknowledgment
alone is insufficient. The authority checks the original five-minute expiry
after proof verification and the node observation; there is no grace period.

Opt into D69 with explicitly initialized registry schema 4 and authority/client
protocol v2. Preserve schema 3/v1 behavior. Store the first eligibility
observation atomically with proof, authorization, job and birth. Historical
recovery returns that receipt unchanged and does not reassert canonicality.

The automatic worker owns one hash loop and one explicitly initialized LevelDB
outbox. Synchronize scan intent before hashing and found evidence before
transport. Persist the exact public authorization before admission. Separate
block and prospect delivery state, retain uncertain outcomes, and recover with
identical evidence. Bound retained records and bytes; pause when full. Private
wallet keys remain behind an in-memory signing adapter.

## Alternatives and consequences

Admission before broadcast delays propagation and makes the authority part of
the miner's block delivery path. Accepting all stale shares weakens freshness.
The chosen exception is limited to a block already accepted on the canonical
chain; it does not alter consensus, difficulty or report-only checkpoints.

Canonicality remains an observation that can change after the final RPC. Birth
and historical evidence do not change with it. Crashes may repeat unfinished
hashing or delivery, but cannot renew the original job. A full outbox needs an
explicit capacity increase; automatic retention deletion is deferred.

Wallet/login, tested full seed export, desktop lifecycle/UI and production
policy calibration remain separate work. Fixture mining and public receipt
export are not substitutes for the D48 wallet requirement.
