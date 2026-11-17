# Change set 32 — bounded resumable candidate recovery

Branch: `slug-resumable-recovery`, stacked on set 31.
Paired wallet: `1bedbf995ebc612ce6f8432c41955ef8a6600661`; desktop pin `v0.0.0-20261117011936-1bedbf995ebc`. This set adds an explicit
wallet schema 11 for recovery beyond the previous synchronous 4096-index limit.
It retains the approved profile and file format, and supports only the two
published candidate fixtures. The desktop's current fixture login remains the
set-30 UI; this library is not yet an integrated recovery screen.

## Behavior and limits

A pending session exclusively owns the unstarted manager and transaction store.
It commits at most 128 child derivation attempts or one block per step. It can
close and reopen without a private password while preserving the exact root,
gym, imported cursors, issued cursors and last complete batch. Ordinary wallet
open refuses pending state. Unknown write outcomes poison the handle until a
valid reopen reconciles storage.

The manager distinguishes issued history from unused lookahead. Discovery in
lookahead raises issued cursors monotonically, derives more scripts in later
steps, and rescans earlier history. Reorg recovery conservatively rebuilds the
private ledger while retaining address facts. Script and transaction indexes,
credits, counters and canonical checkpoint advance atomically. Pending reopen
audits public derivation and ledger crosslinks rather than accepting counters
as sufficient evidence of recovered funds. It validates raw storage lengths
and KDF costs before early decoders, proves recorded invalid-child skips from
public derivation, and checks the full block/transaction/credit/debit/unspent
relationships before accepting pending recovery state.

The operational ceiling is 65,536 total child attempts across all eight
branches, including 20-address lookahead. Ready allocation reserves sufficient
lookahead to keep accepted highwaters restorable. Separate bounds cover retained
transactions, owned outputs, serialized transaction bytes, notification batches
and actual rescan JSON. Capacity failure does not silently drop watches or
reuse an issued address. Schema-11 spending requires coherent live sync.

The read-only node adapter verifies network, genesis and activation settings on
each physical connection, limits requests/responses and closes the actual
socket on cancellation through TCP, TLS, HTTP upgrade and WebSocket reads.
It does not auto-reconnect under a previous connection's identity check. The
scanner verifies block hash, Merkle/witness binding and canonical ancestry;
normal node consensus remains the trusted chain source.

See `RESUMABLE_RECOVERY.md` for the API and exact resource boundaries.

## Verification

Independent derivation adds 112 public payment vectors at indices 4096, 4097,
4116, 4128, 8191, 8192 and 65535 across both fixtures and all eight branches.
The original standards and candidate corpus remain unchanged. Tests cover
cancellation, malformed state, chronological lookahead discovery, reorgs,
uncertain writes, missing or contradictory ledger indexes and nine actual
subprocess termination boundaries. The complete pending-store review records
closed findings in the workspace's `set32-pending-store-review.md`.

All ten chain modules build. All six wallet modules build, pass complete normal
tests and pass vet, with captured sources unchanged. The wallet package passed
in 248.880 seconds; the root module test command took 256.152 seconds. Final
focused manager hardening and full transaction-store races passed in 8.565 and
4.315 seconds. The schema-11 loading checks preserve existing profiles; the
cheap-profile recovery refusal suite also passed.

The frozen normal-PoW acceptance passed in 186.91 seconds (package 187.762;
wrapper 193.813), with all 662 captured source hashes unchanged. It issued
actual address prefixes in 128-address manager transactions, then called real
wallet RPC for index 4096 on every branch. After funding, reopening and export,
the original database was made unavailable. File-only recovery resumed during
derivation and scanning, checked exact balance and funding history before live
sync, then signed all eight recovered inputs across four script types and
confirmed that spend plus its native change spend at height 103. The restored
scan materialized 32,937 attempts, retained 32,777 issued indices and recovered
102 transactions with 110 owned outputs. All four listeners closed and the
owned node exited normally. Earlier diagnostics and changed-source attempts
remain archived separately.

The final maximum-inventory test passed in 41.14 seconds normally and 417.68
seconds under the race detector, with all captured sources unchanged. It
registered 65,536 addresses in 512 requests, with a largest request of 8,641
bytes, then sent a 5,851,288-byte rescan covering 16,384 outpoints. The normal
run's sampled ready-watch heap peaked at 53.04 MB and was 19.41 MB after GC;
this excludes creation/open/KDF peaks and is not total application RAM. Node
RSS rose from 426.20 to 457.23 MB; RSS is not Go heap. The watch benchmark mines
nothing; the separate funded acceptance proves ledger recovery and spending.
All benchmark-owned processes were joined.

The complete final scanner normal gate passed in 87.125 seconds and its full
race gate passed in 690.130 seconds (693.350 wall), with unchanged sources.
Both runs include all nine actual process-kill boundaries. Remote wallet CI
remains in progress: run 43994714263 on the paired wallet commit above.
Release packaging,
required per-set smoke and subsequent live observation are separate gates;
none is claimed complete before its recorded result.

## Preserved boundaries

No implicit migration of schemas 8/9/10, profile/file change, real entropy,
public-chain mining, checkpoint-policy change or production issuance is added.
The fixture vault remains an immutable legacy snapshot; this work does not
claim it tracks later candidate addresses. A current export covers its local
snapshot, not future issuance or an arbitrarily stale backup.

Production profile values, operator-owned provider/KMS deployment, evolving
hosted metadata and product UI remain separate integration gates. Mike's D74
thorough joint design review is mandatory before friends/tester or public
release. Engineering seed deployment does not replace that session.
