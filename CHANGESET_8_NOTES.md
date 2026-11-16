# Change set 8: PoW security review and availability fixes

Branch `slug-pow-security`, stacked on the set 7 implementation (`6b3e9e90`).
Set 7 authority enrollment is still pending. This set reviews the requested
PoW surfaces and fixes the demonstrated availability paths; it is not a
claim that every release/operational risk is resolved.

## Review evidence

The original findings and complete required-area review are preserved in
`docs/security/pow-swap-audit.md`, with a separate stored-index review and
classified/raw caller inventories. Those documents describe the pre-fix
baseline; the dispositions below describe this branch.

Reviewed: production BlockHash/block.Hash consumers, target comparison,
header-first sync, orphan/request/peer bounds, startup's stored-key trust,
and parameter selection. No attacker-selectable Argon2 memory, time, lanes,
or input length was found. Txids, merkle roots, sighashes, and checksums
remain separate SHA256 constructions. Target arithmetic is bounded by the
compact format; its exponent cannot allocate attacker-sized Argon2 arenas.

A bounded pre-fix measurement on this Mac (Go 1.27.4, arm64) computed three
genesis hashes: 216.695 ms/hash and 268,437,834 allocated bytes/hash. The
attack estimates in the audit are extrapolations, not load tests or traffic
sent to the seed.

## Findings and fixes

1. **Concurrent unauthenticated peer hashing — fixed for the identified
   receive/debug paths.** OnBlock used to hash before the serialized queue.
   A syntactically decoded empty block needed no valid PoW to start a
   256 MiB allocation on each peer goroutine. At 117 inbound slots, the
   theoretical active arenas alone reached 29.25 GiB. Hashing and inventory
   marking now happen in netsync's serialized block handler. Debug summaries
   use existing header fields and never hash incoming or outgoing blocks.
   Peer block-receive completion backpressure is preserved.

2. **Known-header replay and long batch stalls — mitigated with bounded
   digest reuse, backpressure, and scheduling.** An admitted batch previously
   processed 2000 hashes before servicing other work; one peer could queue
   many such messages. A 4096-entry FIFO now memoizes only exact serialized
   80-byte headers to locally computed digest values. It does not cache
   validity, trust caller hashes, skip target/context checks, or change
   Argon2 parameters. Locks cover lookup/insertion only; independent mining
   attempts still run concurrently.

   QueueHeaders now waits for completion, peer disconnect, or shutdown.
   The manager processes at most eight headers per turn and round-robins
   retained responses with ordinary messages and stall checks. It preserves
   full 2000-header replies (getheaders has no smaller-count request).
   Continuations never enqueue into the consumer's own input channel.
   Only an actual best-header change refreshes sync progress; known-header
   replay cannot keep resetting the stall clock. Current ancestor/invalid
   status checks still run on digest-cache hits.

   Final review also found that another peer's short response could trigger
   an IBD continuation while the selected sync peer's response was still
   partially processed. Only completion of the selected peer's own response
   can now request its next batch.

3. **Migration failure swallowed during startup — fixed.** initChainState
   now propagates migrateBlockIndex errors instead of reporting success
   with incomplete state. A real-ffldb transaction-fault regression covers
   this path without PoW grinding.

4. **Stored-index safety explanation overstated validation — corrected.**
   Persisted identities are computed locally, but side branches and failed
   connections may be stored before full block connection. Validation status
   is separate. Startup trusts the local database as it already trusts UTXOs
   and the best-tip pointer; it does not authenticate imported snapshots.
   No remote arbitrary index-key injection path was identified. Mandatory
   restart rehashing was not restored.

5. **Miner telemetry assumed SHA256d — corrected.** Each nonce now counts
   one Argon2 attempt instead of two SHA256 operations. No mining result or
   target comparison changed.

## Verification

- All ten modules build; gofmt and diff checks are clean.
- Complete netsync suite passed uncached (11.324 s), including the existing
  sync/stall-recovery state-machine tests and new turn/backpressure tests.
- Full peer suite passed (0.393 s), plus root (0.690 s), mining (0.311 s),
  wire (5.528 s), chaincfg (1.542 s), btcutil packages, rpcclient (3.811 s),
  and the offline checkpoint tool (0.316 s).
- Targeted blockchain migration, PoW vector, subsidy, retarget, regtest,
  and signed-checkpoint tests passed (1.985 s). The production proof vector
  remains unchanged; all header-field mutations are also checked against
  independent direct Argon2 computation in wire tests.
- Cache tests cover all canonical key bytes, eviction, copied keys/values,
  concurrent access, and cheap warm hits. A warm hit measured 128 bytes/op,
  rather than a 256 MiB arena. Peer summary regression uses an empty invalid
  block and checks allocation far below an Argon2 arena; its benchmark was
  364 bytes/op and about 476 ns/op on this Mac.
- New header tests cover partial response yield, no premature block fetch,
  known-header replay without false progress, cancellation before hashing,
  completion backpressure, and shutdown during enqueue/wait.
- A real connected peer over net.Pipe completes a full 2000-header response
  while ordinary queries keep filling the input queue; queries are observed
  between the first and last chain advancements. Disconnected batches are
  released before validation. Combined cache/header/queue/scheduler race
  checks passed (wire 1.324 s, netsync 22.037 s).
- Required isolated smoke passed: correct version/genesis, two blocks mined,
  exactly 10 SLUG subsidy, seed subversion verified, convergence to height 3
  and tip `42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`,
  correct report-only/unconfigured-authority RPC state, and clean stop.

## Residual risks and limits

- Unique headers and replay sets larger than the cache still cost real
  hashing. Fair turns preserve service opportunities; they are not a
  per-peer CPU quota or a promise of bounded overall sync time. The accepted
  header index is not globally size-capped, and work-backed side-branch
  growth remains relevant at Phase A's easy difficulty. These sustained
  workloads were source-reviewed but not stress-tested.
- Serializing P2P hashing closes the identified remote concurrency path.
  It is not a global process memory cap: operator-selected miner workers,
  authenticated RPC rescans/verifychain, GC retention, stored bodies, and
  other node state still consume memory. The seed's private RPC boundary
  and deployment resource settings remain important operational controls.
- The orphan pool's 100-block cap can retain roughly 381.5 MiB of raw
  bodies before decoded-object overhead. It bounds count, not all memory.
- Malformed legacy migration rows/links can still panic or loop, and
  additional stored key/header/height/work consistency checks are possible.
  These are documented local-corruption/import recoverability issues, not
  demonstrated network injection paths. `verifychain` is not a complete
  integrity audit of a hostile snapshot. No disk-format changes were made.
- Argon2 cryptographic design, difficulty economics, full UTXO migration
  history, LevelDB internals, and hostile snapshot tooling were not audited
  comprehensively. The four handover-mandated review areas were examined;
  this does not imply a whole-project security certification.
- Full blockchain and remaining integration execution remains set 9.
  No master merge, genesis change, seed deploy, or production-launch
  approval is part of this review.
