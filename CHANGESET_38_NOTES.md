# Change set 38 — serve known headers on an idle chain

Branch: `slug-idle-headers`, stacked on set 37 implementation
`fb4c0caf0a6bbad3741a30fe74ad34c67da1459e`.

## Problem and change

The unchanged Phase-A height 61 block timestamp is 1794799795. At
2026-11-17 03:29:55 UTC it became older than 24 hours. The inherited
`OnGetHeaders` early return then suppressed every response because the node
was no longer considered current. Fresh peers stalled at genesis and eventually
disconnected, although the complete existing chain remained available.

Remove only this header-serving freshness gate. Normal locator queries retain
`LocateHeaders`' connected best-chain selection and maximum 2,000-header reply.
The existing empty-locator/known-stop lookup can return a single indexed header,
including a header without a connected block body; that behavior is preserved.
A response does not assert full synchronization or validated bodies for that
special lookup. The existing aggregate peer queue is not newly bounded by this
repair; per-message bounds are not an aggregate DoS guarantee.

## Deliberately unchanged

Chain and sync currentness, stale-template refusal, transaction relay, compact
filter gates, consensus, proof verification, report-only checkpoints and all
stored chain data remain unchanged. No public block was mined, no private key
was accessed, and the Railway data volume is retained.

Candidate manifest service/worker integration moves to set 39. The current Mac
GUI remains the set 32 fixture pilot; visual/screen-reader acceptance and the
joint D74 design review remain open.

## Validation

The old frozen binary reproduced the failure on two isolated nodes using an
already verified copy of public height 61, with a tip age over24 hours. A fresh
peer advertised the correct seed height but remained at zero blocks/headers.
Both nodes exited cleanly and all listeners closed. Independent read-only
P2P `getblocks`/`getdata` confirmed the live seed's 61-block inventory and exact
recorded tip with normal Argon verification.

The old runtime failed all eight real-peer regression requests. With the fix,
normal, race and tagged tests pass, including exact connected-history and
header-only lookup behavior, ordinary stale-template refusal, and the existing
inventory-selection suite. All ten workspace modules build; vet, formatting
and whitespace checks pass. Frozen stage and integrated source hashes match.

A new node using the fixed normal-PoW binary downloaded all 61 existing blocks
from an isolated peer and retained the same connection for 110 seconds,
exceeding the former 90-second stall timeout. Both nodes still refused ordinary
mining templates; no new block was generated. Both nodes exited zero and all
three listeners closed. The first harness attempt had a five-second peer-RPC
timeout during 12.8 seconds of serial Argon header validation; that failed
attempt is archived, and the successful run uses a bounded 30-second RPC timeout
and waits for sync before querying peer state. Runtime code did not change.

Pre-promotion smoke uses a repaired isolated peer with the exact verified
public history, because the current live seed has the known serving failure.
The required public-peer smoke and a full 15-minute live observation follow the
repair deployment. Live tip identity is checked immediately before promotion
using existing read-only P2P getblocks/getdata with normal Argon verification.
The original failed set 37 observer is preserved; no failed run is relabeled.
The post-repair observation will verify the combined 37–38 engineering tree.

Evidence: workspace set38-idle-header-checks.json, set38-checks.json,
set38-idle-sync-acceptance.json, independent review, and deployment artifacts.
