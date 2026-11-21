# Fresh daily indexed settlement: proposed contract and code map

**Source-only proposal for root selection; no authoritative fixture, implementation, runtime, or readiness claim.** This replaces the earlier unselected two-to-three-batch append proposal as the proposed ledger scope. No existing V1 root is migrated, reinterpreted, reset or repinned. The selected source prerequisite is `source-v2-stage/CONTRACT.md`; the round prerequisite is `daily-round-contract.md`. Both are finite fresh profiles, not indefinite service.

## 1. Selection and concrete ownership

Use the existing ordered `poolsettle.Config` fields unchanged:

```text
{"version":2,"change_script":<native script>,"destinations":[<DestinationV1>],"max_rounds":N,"max_batches":N}
```

Root has selected positive immutable lifetime limits `MaxRounds<=16384`, `MaxBatches<=32767`, with the existing sorted unique 1–256 destinations and change-script exclusions. These are real lifetime ceilings, never page sizes or counters reset after payout. Smaller selections remain legal; admission must reserve against them. Require `MaxRounds<=original SourceV2.MaxSubmissions` and the selected daily round capacity to cover the chosen credit count. The wallet/runtime's selected enrollment limit remains separate; permitting a codec shape does not enroll more miners.

Legacy `Create/Open/OpenAudit`, `ConfigBindingID`, `CheckBackendBinding`, `CheckSigningRequest`, scheduler binding and service `BindingID` remain strict V1. New APIs explicitly select daily mode:

```go
CreateDaily(ctx, path, ConfigV2, *poolround.Store, Backend) (*Store, error)
OpenDaily(ctx, path, ConfigV2, *poolround.Store, Backend) (*Store, error)
OpenDailyAudit(ctx, path, ConfigV2, *poolround.Store) (*Store, error)
DailyConfigBindingID(Config) (string, error)
(*Store).CheckDailyBackendBinding(ctx, Backend, Config, poolshare.Config) error
CaptureDailySigningCheck(ctx, *Store, Backend, Config, poolshare.Config,
    []Coin, []byte) (*DailySigningCheck, error)
(*DailySigningCheck).Check(ctx) error
(*DailySigningCheck).Recheck(ctx, []Coin, []byte) error
```

All constructors authenticate the original concrete daily round owner, full SourceV2 and exact payout script before readiness. Same `*Store`, one comparable backend pointer and one scheduler remain attached during the lifetime; no per-page owners or callbacks supply monetary authority. Construction context governs initial audit, not a replacement owner lifetime. Failed cleanup returns the original retained owner; no missing-child repair or fallback to V1.

Selected identity separation:

- `DailyConfigBindingID`: SHA256(`"SLUG operator pool settlement config\x00v2\x00" || LE32(len(canonical ConfigV2 JSON)) || JSON`), lowercase hex; reject invalid config and bytes above65536. Existing V1 implementation/domain remains exact.
- `poolshare.DailyConfigID`: the separately selected source API hashes `"SLUG pool share daily source\x00v2\x00" || canonical ConfigV2 JSON`, with **no** length prefix or LF. Do not duplicate this algorithm in settlement or loosen `poolservice.BindingID`.
- Store/marker keep the full selected config/source as well as their recomputed IDs. These IDs correlate configuration, not a unique physical store or spending authorization. No dependency from poolsettle back to poolservice is introduced.
- Fresh daily wallet binding uses these IDs in the unchanged schema16 guard field grammar and original derivation. A separate daily backend bind selects the daily node profile and original Store once. No old wallet guard is overwritten.

## 2. Financial and working-set bounds

Preserve Round.Version1, its exact credit digest domain, 4096-share allocation window and 60-descendant confirmation semantics. SourceV2's global accepted tail is at most16384; its append order does not reset at a daily payout or storage boundary. For complete disjoint round windows, total miner allocation leaves are at most accepted shares `N`; each credited round adds one operator leaf, so original claim leaves are at most `N+R<=32768`. Spending removes or partially debits those claims; it does not create new origin/gym identities. The active frontier therefore has at most16384 origins and32768 claim leaves, below the selected daily node's65536 aggregate claim/origin bounds. Pending input and projected change are never counted as simultaneously spendable. Coinbase aliases remain a funding conflict/duplicate outpoint, not additional available money.

The existing fraction grammar remains exact, reduced and bounded4096 digits. This source count proof does not prove a small constant RAM footprint: partially debited fractions can acquire larger denominators. Retain scalar/count bounds before allocations, use bounded serialized pages and streaming commitments, and measure peak working memory with the actual selected bounds. **Do not reintroduce the old129MiB whole-JSON request limit or invent a new all-history JSON blob as a shortcut.** Active claims may be materialized where the existing planner/node API requires them; historical batch bodies must not be retained. Any new working-memory limit needs an independently proven bound and must not silently drop claims.

Daily transaction construction/signing uses the selected `pooltxshape.NativeP2WPKH` profile: up to1464 inputs and256 payees, conservative weight<=400000 and actual raw/weight checks, with existing exact output/fee/sequence/sighash rules. All ready inputs form the candidate. D84 defers the entire candidate on shape/provenance capacity, preserves balances, does not occupy its day, and never selects a subset or consolidates funds. Its error/report grammar must be explicit; a storage/source integrity error is never relabeled as capacity.

## 3. Proposed exact disk grammar

Use a fresh root containing a canonical marker `SLUG-POOL-SETTLEMENT.json` and `db/`. Marker field order:

```text
{"application":"slug-pool-settlement-daily","version":2,"config":<ConfigV2>,"source":<SourceV2>,"config_id":"...","source_id":"..."}
```

Compact canonical JSON, no final LF; ASCII strings, exact integer spellings, no unknown/duplicate/reordered fields. Marker/head bound128KiB. There is no V1 adoption field, JournalID, operator secret, or implicit source discovery.

`head` is one canonical value, ordered:

```text
{"application":"slug-pool-settlement-daily","version":2,"config_id":"...","source_id":"...","revision":N,"credits":N,"batches":N,"events":N,"logical_bytes":N,"pending_batch":N,"last_day":"...","last_clock_ms":N}
```

Counts/sequence fields are checked uint64; `events=credits+batches`, all first sequences are1,0 denotes no pending/empty tail. Revision advances once per successful DB commit and cannot wrap. Day/clock retain current UTC/high-water semantics. Selected counts remain below their fixed finite maxima; revisions do not reset. No persisted ready/audited flag grants authority.

Allowed DB keys (prefixes are ASCII, numeric suffixes exactly big-endian uint64; displayed hashes are their existing64-character lowercase hex strings):

| Key | Exact value / role |
|---|---|
| `head` | Header above |
| `report` | Existing closed Report grammar; latest dated observation only |
| `c/` + BE64 credit sequence | Existing complete Credit value, immutable |
| `cr/` + round hash | BE64 credit sequence; exactly-once original round lookup |
| `b/` + BE64 batch sequence | Immutable DailyBatchBody below |
| `bs/` + BE64 batch sequence | `{"state":"prepared|broadcast|confirmed"}`; mutable delivery only |
| `bt/` + txid | BE64 batch sequence; immutable unique signed tx |
| `bd/` + ten-byte YYYY-MM-DD | BE64 batch sequence; immutable occupied day |
| `o/` + txid + BE32 output index | `{"kind":"credit|change","sequence":N}`; unique original funding object |
| `spent/` + txid + BE32 input index | BE64 consuming batch; never removed/reused |
| `e/` + BE64 event sequence | `{"kind":"credit|batch","sequence":N}`; immutable original insertion order |

Every credit and batch occurs exactly once in `e`; each per-kind sequence is ascending. Credits imported in a commit precede its newly prepared batch. A new root appends to the frontier, while a batch's change prepends, so interleaved replay reproduces V1 ordering without preloading all historical credits. `o` includes only nonzero credit coins and nonnil change; zero-reward credits still retain `c/cr/e`. All lifetime input/outpoint duplicates are checked against original indexes, including already spent coins. There is no generic arbitrary metadata namespace.

DailyBatchBody field order:

```text
{"version":2,"sequence":N,"day":"...","created_at_ms":N,
 "inputs":[{"txid":"...","index":N}],
 "payments":[<PaymentV1>],"fee_atoms":N,
 "change":null|{"index":N,"atoms":N,"script":"...","claim_count":N,"claims_digest":"..."},
 "txid":"...","raw":"<exact lowerhex signed transaction>"}
```

Input references are the exact ordered subsequence of authenticated live funding, never wallet-selected coins. Replay retrieves their complete current claims from its frontier. It runs the original plan, binds change.TxID to body.TxID, compares every stored payment/fee/change field, validates the canonical signed raw and all signatures, then applies the transition. `claims_digest` is proposed SHA256(`"SLUG pool daily change claims\x00v2\x00" || canonical ordered Claims JSON`), with no length prefix/LF. Compute it incrementally; `claim_count` must equal the derived count. That digest is a corruption/binding check, not stand-alone financial authority. It avoids storing another full copy of accumulated claim provenance in every historical batch. Exact prior pending inputs/claims remain reconstructible from original preceding events; its raw is stored once and never regenerated for retry.

Proposed encoded bounds: Credit128KiB; batch body2MiB; event/object refs and delivery state256bytes each; report8MiB; individual index keys/values have their exact fixed shapes. Iterators process one bounded row at a time; public/internal page materialization is at most8 rows and a checked byte budget. No whole-history `[]Batch` read/export is permitted. Legacy history DTOs remain strict; a new bounded history page API is a separate explicit consumer, never truncation of V1 history.

**Proposed global logical journal bound:8GiB**, including all permanent row/index bytes, with a fixed128KiB head reserve and8MiB report reserve; count current key+value lengths for all other rows, reserving the full delivery-state256bytes per batch so later observation cannot consume new intake capacity. The exact accounting formula is part of the codec/reference freeze. LevelDB files/compaction and physical disk use are additional, not covered by this logical figure.

The bound must be independently proved before intake integration: `R<=16384`, successful `B<=min(MaxBatches,2R−1)`, total batch inputs<=R+B, <=256 payments per batch, original credit claim leaves<=N+R, and no copied historical claim lists. A deliberately loose budget of256bytes/payment, fixed metadata/index overhead, the existing native transaction formula and hex expansion is below8GiB at these maxima; the producer must enumerate the full proposed row grammar rather than accept this estimate. If the precise proof fails, revise the proposed bound before vectors/code—never silently waive reservation at runtime. Smaller selected round/batch limits and current usage still constrain admission; source fullness/D80 obligations are not solved by this disk cap alone.

## 4. Complete audit and current-view integrity

On OpenDaily, and before any operation publishes financial state, take a consistent original DB snapshot under the Store gate. Enumerate **every** namespace and reject missing/extra keys, malformed suffixes, counters/bytes mismatch, gaps, reused or misdirected indexes, wrong config IDs, impossible pending/day/report state, or any unknown key. Stream the event sequence into `settlementReplay`; resolve its indexed rows, supply authenticated count/duplicate facts, and reconstruct each immutable batch from its input references. Recompute the exact change commitment and signed-byte validity. Compare all inverse indexes and selected counts. Full source Walk must authenticate every old credit against its original round digest and capture every new confirmed record; a newer-tail cursor alone cannot skip an old round's first confirmation. Source errors return no partial imports or successful projection.

Maintain at most the active frontier, per-gym projection, fixed metadata and bounded row/page buffers. Paid/reserved totals derive from immutable history plus current audited delivery states. A cached live projection is never trusted merely because its header revision matches; current stored rows/indexes must be verified against the same-process authenticated content, or replayed again. The private directory model does not promise protection against an arbitrary hostile local writer or a complete valid-directory rollback; original selected path/descriptor/owner fences still apply.

**Performance prerequisite requiring explicit implementation:** `AccountingPage` currently rehashes each consumed nonce with normal Argon (`poolshare/accounting.go:96`, `store.go:452`), while `poolschedule/run.go:183` grants each settlement call30seconds. Full repeat verification of a721+ receipt prefix is not proved to fit. Daily source capture must either retain bounded original-owner audit progress or use a same-process cache of fully verified exact immutable work/receipt bytes, checking current complete rows/indexes/source before reuse. New or changed bytes still receive full normal verification. A cache rebuilt solely from an unauthenticated persisted digest is forbidden. Cold-open audit remains cancelable and fully completes before writable readiness. No blanket deadline enlargement, hidden retry, lost source fence, or partial signing is authorized by this design. This prerequisite and node latency must be qualified at daily scale, not assumed from small fixtures.

## 5. Original operation and commit ordering

1. Acquire original Store action gate, check normal profile/lifetime, fully audit current snapshot and old source, then sample monotonic clock. Reobserve all old payout transactions as today; an old confirmed payout loss holds new signing. Stream bodies one at a time rather than loading all of them.
2. Recover original pending A **before new credit import, new capacity checks, fees or signatures**. An unknown transaction may only rebroadcast exact retained raw after the unchanged original source/funding checks. Preserve reservations on every uncertainty/conflict. Observation commits update only `bs`, head clock/revision/pending and report. Never reconstruct a new transaction from its economic fields for recovery.
3. Stage all new confirmed credits/indexes/events after a successful complete source walk. New credits and their exact conditional frontier remain private until the final sync. Use a bounded pending write batch; count/claim-source bounds admit a checked aggregate import budget, not a per-row maximum multiplied blindly. Refuse the entire import on overflow/read/cancel/source mismatch. No incremental partial page credit commits are exposed.
4. Derive all ready funding, plan the whole candidate and use the daily shape/provenance profile. A deferral can atomically publish its authenticated credits/clock/report but creates no batch/day/spent rows. On a due successful candidate, mint the original synchronous daily capability, quote/sign once, repeat full current source/funding/lifetime checks, and sample the actual preparation day after signing as today.
5. **One synced LevelDB batch** writes new credits and all indexes/events, immutable body, initial `bs=prepared`, day/tx/outpoint/spent indexes, pending head/day/clock/revision and report before any Broadcast. Retain signed raw when a write outcome is uncertain; poison the owner, preserve its handles and require exact reopen reconciliation. A response/output loss never permits resigning, releasing the day or advancing reserves. Broadcast occurs once; subsequent calls reconcile exact A.

The distinct daily private capability owns config/source/profile, original Store/backend/parent context, exact inputs/unsigned/payments/fee/change and an incrementally computed body commitment. `CaptureDailySigningCheck` performs the complete match once and returns an opaque original-capability handle. `Check` checks only original active lifetime; `Recheck` repeats full input/body equality before disclosure. The capability is invalidated synchronously immediately on Sign return. No public constructor, callback signer, reusable offline token or Store gate reentry is added. Mode comes from the selected owner, never counts, decoded response fields or a caller boolean.

Use the existing Store lifecycle/action serialization and joined Close, augmented with daily retained snapshot/iterator cleanup. Daily constructor cleanup errors retain the exact partial Store. No helper goroutine may outlive its admitted operation; cancellation releases iterators/snapshots and joins workers before Close releases DB/files. Failures remain sticky where close ownership is uncertain.

Scheduler/CLI/runtime require explicit daily constructors/binding that select this same Store and a separately domain-framed daily scheduler binding. Keep V1 scheduling JSON/IDs strict and untouched. Scheduler is an attempt cursor, not another ledger. Existing RunDaily/RecoverPending can dispatch by the private immutable Store mode after explicit construction; legacy callers cannot obtain a V2 owner through V1 Create/Open/Bind.

## 6. Code seams and proof before readiness

Root package external implementation ownership is feasible in a bounded staged family: `daily_config.go` (validator/ID); `daily_store.go` (fresh create/open/marker/closed codecs); `daily_audit.go` (namespace/event replay); `daily_operations.go` (original recovery and atomic daily commits); `daily_signing_request.go` (distinct active capability); narrow mode branches in Store lifecycle/RunDaily/recovery and selected pure plan/transaction/replay leaves. Keep original V1 functions/wrappers; use the selected shape helper, never widen old globals. Root owns source/round implementation; wallet agent owns explicit schema16 backend/manager consumers.

Before any new expected bytes: root selects this grammar, global-byte formula, claims digest and bounded source-audit mechanism. Freeze a standalone producer/input catalog using published configs, existing independent signed transaction fixtures, literal event recipes and independently calculated fractions. No product-generated values become expected vectors.

Required integrated proof is **two daily-scale generations**, not a two-to-three-batch-only test: >720 authenticated credit rounds and a721-input payout (including old change), accumulated old provenance beyond256, whole candidate paid once, next-day residual exact, original pending A byte-identical through restart/lost acknowledgment, original old issued-job late winner credited once without replacing A. Cross old256 credit/input and100000 raw boundaries plus several indexed pages; test current old-origin orphan holds and post-confirmation payout conflict retention. Exercise prepare-before/after-sync faults, bad late source/index/claims digest, cancellation/full-audit failure with zero partial imports, wrong daily owner/profile/capability, cheap refusal, and V1 byte/refusal parity. Include actual standard-node acceptance through the dedicated wallet, measured bounded memory/latency, and D84 near/over-weight whole-deferral without Sign/day consumption.

Finite maxima, public authority's original signed limits, pathological no-winner windows, physical disk exhaustion and an oversized ever-growing D84 backlog remain real boundaries. This proposal changes storage and engineering profiles, not fees, finality, minimums, payout selection, reserves, consolidation, source custody or incident resolution policy.
