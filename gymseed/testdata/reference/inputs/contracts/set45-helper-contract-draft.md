# Set 45 helper contract draft — fixed candidate publisher

Status: engineering draft for joint core/helper review. No runtime or oracle changes are authorized by this document. Nested `gymseed` DTOs are defined only by the jointly reviewed core contract; this document fixes their helper ownership and wrappers. This draft implements [root's direction](/Users/taytrrip/slug/set45-engineering-direction.md) and the accepted [fixed zero256 attachment](/Users/taytrrip/slug/set45-publisher-integration-options.md).

## 1. Scope and authority

Use the existing Candidate Pilot root, bundle, page and serial work lane. Add a separately typed Seed v1 family for a bounded candidate declaration/publication/evidence journal. Do not extend an old Manifest/Stable/Preview/Entry/Event frame, change a retained snapshot, add fights/awards or claim official publication. In particular Set44 EventStatus.seed_authority remains its frozen `unavailable` selector; only the new Seed family supplies candidate publication facts. The UI labels these distinct layers without rewriting the old status or noon receipt. Core owns canonical declarations, carrier and transaction bytes, first-anchor scanning, A+1 target, exact-target fixture checkpoint plus 60 descendants, conflict retention, calendar deadline and all record limits.

There is exactly one publication chain: the retained zero256 fixture's established primary chain, selected by its trusted parent/child identity. Incrementing256 is never an alternate node. The parent owns the journal across logins; nodes still belong to the zero child's existing Prepare/Start/Stop lifecycle. The publisher borrows the already-running concrete owned pair for one bounded operation, then releases it. Stop/logout/reset/switch cancels and joins the borrow before node closure. An unavailable pair produces `seed_node_offline` and does not trigger Start, node bootstrap, mining or another chain.

Prepare consumes the concrete complete Set 44 receipt under the captured original participant View. It seals only the deterministic signed declaration; it performs no funding or RPC. Either fixed participant may prepare/read retained facts. Actual signing authority is the fixed candidate authority installed with the new route, not a key or verdict supplied by View or the browser.

Publish and Attest additionally require the **original zero256 fresh, unheld `gymaccount.SeedPublish` session** and the separately fixed concrete publisher capability. The new capability is unavailable to mining tokens, unenrolled/recovery-required/revoked sessions, another fixture, or a held/stale-auth session. Do not widen the wallet RPC allowlist, change its payout, access a real operator key, or debit gym funds. The only fund is the separately scoped public key1 operator fixture fund defined by core; absent/maturing funds are visible waiting states. Read-only network reconciliation and ordinary reads remain allowed under held View. Reconcile never signs, retransmits, mines or upgrades its authority.

Original-session authorization linearizes at the final durable exact-dispatch intent, with no account/store lock spanning signing or RPC. Release the lease, check cancellation immediately before transport, and preserve uncertain outcome if reset/cancellation races an already authorized dispatch. Do not promise atomic revocation of a network request that has begun. A later explicit dispatch needs a new original fresh/unheld authorization; polling and Reconcile never provide it.

Attest is an explicit fixture-only action taking a date, not a header/key/checkpoint object. It derives the exact retained target, rechecks canonical anchor-to-target ancestry on the pinned owned chain, signs the fixed published checkpoint fixture, and passes the result to core ObserveCheckpoint. It never loads a checkpoint into node consensus or a global checkpoint file. Candidate fixture signatures are visibly distinct from the production enrolled authority.

No helper command accepts authenticated-declaration or checkpoint JSON. Core may expose ObserveEquivocation for bounded authenticated evidence, but a normal helper feed exists only if the core contract supplies a concrete fixed publisher-store registry. In its absence this slice makes no global declaration-distribution or absence-of-equivocation claim; core signature/conflict tests remain separate evidence.

## 2. Parent installation and node attachment

The explicit route is `accountpilot.NewSeeds(pilot.Config)`. It extends the same root to version 5, preserving every v4 field and child install. Old NewEvents/NewEntries/NewStable/NewManifest reject v5. No old decoder gains unknown-field tolerance. Cheap-PoW refusal and exact existing-root validation precede mutation.

Append, in order, `seed_schema` (uint32, 1), `seed_phase` (string) and `seed_store_id` (string) to the v4 parent marker. The existing 16 KiB root cap remains. Phases are `waiting_source`, `prepared`, `ready`. `waiting_source` has empty StoreID and no established seed component; it means the concrete parent EventStore and validated ready zero child identity are not both available. Node offline is not an installation prerequisite or a reason to remain waiting_source. `prepared` and `ready` have a canonical StoreID binding the complete core Pin. Existing v4 event phase and seed phase are separate. A ready seed component cannot be recreated if missing.

Derive the selected zero256 child from validated existing install pins, not an untrusted directory scan, user path, current login or first-ready child. The core draft fixes floor height0/genesis inclusively. Every full Seed Pin field therefore derives from the trusted complete event Pin plus compiled fixture/rule selectors; no sampled current tip or node endpoint is an installation input. A new journal's immutable CreatedAtMS must be strictly before an affected cutoff. Its Pin/rules are synced before that actual creation-time sample and cannot be backdated. Parent prepared state uses the exact reconstructible Pin and core init-intent/ResumePrepared protocol; unknown partial state is retained/refused and initialized event-bearing state cannot be adopted as empty.

A private new-route-only dependency lets the zero child offer its live owned-node capability during validated startup/operation. Parent initialization does not need a live child: when both established event participants exist it can derive and create the seed component before login, without mining or opening a node. During explicit zero256 mining the scoped capability is available through the serial lane. The journal stores neither RPC credentials nor a live child pointer. Node ports/passwords may change across restart; chain-instance, selected child, fixed floor, publisher/funding/checkpoint authorities and rule pins may not.

Teardown order: invalidate current view and reject new work; cancel queued/active seed work; join the exact scoped node borrow; finish ordinary child/node drainage; close the parent seed journal; close event/account stores and the parent instance lock. No join occurs while holding a parent/account/store mutex. A new child never inherits an old task's session or node capability. Incrementing256 can read the parent journal but its live nodes are ineligible for Publish, Reconcile or Attest; those network actions report offline until the fixed zero pair is available.

The private pilot borrow calls exactly core §6.1 `seednode.AttachOwned(ctx, seednode.OwnerBinding{Source: trustedEventPin, Home: selectedZeroHome, Primary: seednode.OwnedProcess{Cmd: nodes.primary.cmd, Done: nodes.primary.done}, Peer: seednode.OwnedProcess{Cmd: nodes.peer.cmd, Done: nodes.peer.done}})`. It passes actual retained process handles and copies the trusted immutable source Pin, never a boolean/verdict callback, arbitrary RPC configuration or browser input. `gymseed.CandidatePin` and `seednode.ChainIdentityFor` define the shared identity. The parent verifies its selected root marker before attachment; the adapter validates exact opened config files, the actual primary-nolisten/peer-listener topology and live network profile as core specifies. This is a trusted in-process owner boundary, not cryptographic attestation against malicious Go code or a compromised host.

Register the existing zero `pilotNodes` pair privately only during the owning run and remove/revoke it before run cleanup. The Seed task obtains a tracked borrow under the existing lane/lifetime gates, creates one Publisher, closes/joins that Publisher in a defer, then releases the borrow. AttachOwned does not own or stop nodes. Do not read mutable exec.Cmd.ProcessState concurrently with Wait; done channels publish exit. Publisher identity must equal the expected Pin for every core call. Snapshot.NodeState uses only cached process registration and done-channel state, never AttachOwned/RPC; the next explicit task revalidates topology. No package caller may retain a publisher across operations.

Parent/child exact public constructors and attachment:

```go
// accountpilot
func NewSeeds(pilot.Config) (*StableController, error)
// pilot
func NewSeedManifestAccount(Config, StableAccountConfig) (*StableController, error)
func (s *StableController) AttachSeeds(context.Context, *gymseed.Store) error
// Only with build tag slugtestclock; no CLI/env/DTO clock option.
// accountpilot
func NewSeedsForTestClock(pilot.Config, func() time.Time) (*StableController, error)
// pilot
func NewSeedManifestAccountForTestClock(Config, StableAccountConfig, func() time.Time) (*StableController, error)
```

AttachSeeds is once per new child, after AttachEvents and before publication of a ready parent view. Nil is allowed only for exact waiting_source root state. Otherwise verify the concrete store Info, full CandidatePin, StoreID and immutable CreatedAt against the validated v5 root. Normal construction uses time.Now; tagged immutable factories supply the same owned clock to account/source/event/seed creation and reopen and fixed fixture authentication time, without a global setter. Parent creates/opens `<root>/seeds` using core Create/Open/ResumePrepared and its concrete owned EventStore. v5-aware new-route writers and child marker validation preserve every v4 field; existing route parsers retain their version refusal. Source, wallet, node and instance-lock lifetimes otherwise retain their old owners.

## 3. Exact helper surface

These declarations use the exact `gymseed` core DTO names. Arguments after context are original ViewID then canonical Saturday or JobID, except History's exclusive EventKey cursor and limit.

```go
const MaxSeedRequestSize = 8192
const MaxSeedResponseSize = 262144
const MaxSeedCacheSize = 262144
const MaxSeedJobSize = 245760
const MaxSeedMetadataSize = 16384

type SeedControllerAPI interface {
    EventControllerAPI
    SeedSnapshot() SeedSnapshot
    SeedStatus(context.Context, string, string) (gymseed.Status, error)
    SeedRecord(context.Context, string, string) (gymseed.Record, error)
    SeedHistory(context.Context, string, string, uint32) (gymseed.HistoryPage, error)
    BeginSeedPrepare(context.Context, string, string) (SeedJob, error)
    BeginSeedPublish(context.Context, string, string) (SeedJob, error)
    BeginSeedReconcile(context.Context, string, string) (SeedJob, error)
    BeginSeedAttest(context.Context, string, string) (SeedJob, error)
    SeedJob(context.Context, string, string) (SeedJob, error)
    CancelSeedJob(context.Context, string, string) (SeedJob, error)
}
func ServeSeeds(context.Context, SeedControllerAPI, io.Reader, io.Writer) error
func MarshalSeedRequest(SeedRequest) ([]byte, error)
func ParseSeedRequest([]byte) (SeedRequest, error)
func MarshalSeedResponse(SeedResponse) ([]byte, error)
func ParseSeedResponse([]byte) (SeedResponse, error)
```

Add native LaunchSeeds and the explicit helper/shell `--candidate-seeds` flag, requiring the existing candidate-manifests/stable/entries/events flags. The new candidate build may select that complete flag combination as its default; old build/default modes stay unchanged. Native Gym methods return complete typed SeedResponse wrappers. There is no browser RPC, outpoint, amount, signature, raw transaction, chain path, key or timestamp argument.

Every field below is required and canonical output uses the listed order. Pointer fields encode null when inactive; strings use empty string rather than omitted fields. IDs are lowercase 64-hex; optional IDs may be empty. All uint64 counters additionally fit 2^53−1. Dates, timestamps and nested DTO bounds are exactly core's closed contract.

```go
type SeedRequest struct {
    ID uint64          // id, 1..2^53-1
    Command string     // command
    ViewID string      // view_id
    Saturday string    // saturday
    AfterEventKey string // after_event_key
    Limit uint32       // limit
    JobID string       // job_id
}
type SeedResponse struct {
    ID uint64                  // id
    OK bool                    // ok
    View *ManifestViewPin      // view
    StoreID string             // store_id
    Snapshot *SeedSnapshot     // snapshot
    Status *gymseed.Status     // status
    Record *gymseed.Record     // record
    History *gymseed.HistoryPage // history
    Job *SeedJob               // job
    Error string               // error
}
type SeedSnapshot struct {
    Version uint32             // version, 1
    State string               // state
    Error string               // error
    View *ManifestViewPin      // view
    StoreID string             // store_id
    Pin *gymseed.Pin           // pin
    CreatedAtMS int64          // created_at_ms
    PublisherAccountID string  // publisher_account_id, fixed zero256 account
    NodeState string           // node_state
    ActiveJobID string         // active_job_id
    QueuedJobID string         // queued_job_id
    LastJobID string           // last_job_id
}
type SeedJob struct {
    Version uint32             // version, 1
    JobID string               // job_id
    Kind string                // kind
    State string               // state
    View ManifestViewPin       // view
    StoreID string             // store_id
    EventKey string            // event_key
    Saturday string            // saturday
    Error string               // error
    Result *gymseed.Record     // result
}
```

Command grammar:

| Command | Required nonzero request fields beyond ID/Command | Sole success payload |
| --- | --- | --- |
| seed_snapshot | none | snapshot |
| seed_status, seed_record | view_id, saturday | status, record respectively |
| seed_history | view_id, limit1..8; after_event_key empty or canonical | history |
| seed_prepare, seed_publish, seed_reconcile, seed_attest | view_id, saturday | job |
| seed_job, seed_cancel | view_id, job_id | job |

All unused request fields are exactly empty/zero. Successful responses have Error empty and exactly one payload. Except unavailable Snapshot, outer View equals the current original view and StoreID equals the established seed store. Job.View equals outer View; historical declaration/dispatch author fields are not replaced. Failure responses have OK=false, one fixed Error and all payload pointers/View null, StoreID empty. No arbitrary error text is serialized.

Snapshot.State is `ready|busy|setup_incomplete|unavailable|failed`. Ready/busy have full View, Pin, StoreID, CreatedAtMS and fixed PublisherAccountID, with Error empty. Busy describes **any-family** occupied lane; seed job IDs list only Seed work. NodeState is `ready|offline`, never inferred from the other gym's live node; readiness is cached process attachment, not an RPC polling claim. Setup-incomplete has current View, empty StoreID/PublisherAccountID/job IDs, nil Pin, CreatedAt0, NodeState offline, and Error seed_setup_incomplete. Unavailable is the only successful response with null View: Version1, unavailable/error seed_unavailable, nil Pin, all IDs empty, CreatedAt0, NodeState offline. Failed retains a previously validated full pin/current view and uses only seed_unavailable, seed_corrupt or seed_clock; absent current view instead yields unavailable. These shapes must receive positive vectors, not only rejection tests.

Job.Kind is `prepare|publish|reconcile|attest`; State is `queued|running|completed|failed|cancelled`. Queued/running have empty Error and nil Result. Completed has empty Error and a complete owned Record bound to EventKey/date/store; it may legitimately describe waiting anchor/target/finality. Failed/cancelled have nil Result and a fixed code; cancelled uses seed_cancelled. A known post-write uncertainty is failed/outcome_unknown, never cancelled. Publish with no funding is failed/seed_waiting_for_funds while the prepared durable declaration remains retrievable. A cancelled queued task changes no persistent event or transaction.

## 4. Closed parsing and native correlation

Transport accepts JSON whitespace/order/decoded key escapes as permitted by existing families, with decoded-key duplicates, unknown/missing fields, noncanonical scalars, array surplus, deep nesting and oversize refused before materialization. Canonical output is fixed-order ASCII. Request cap includes LF; response cap includes wrapper and LF and remains inside the unchanged existing 256 KiB scanner limit. The 224 KiB core Record is not permission to emit an oversized wrapper.

Standalone ParseSeedResponse performs bounded private shape and canonical digest checks, without source proofs, SVG, bout simulation, node RPC, funding/signing or full-block verification. Core semantic verification happens once during explicit work/Open, outside the final account lease. Repeated Snapshot/Job polls copy already checked bounded values under the original View and final same-child/view recheck. Metadata Status/Record/History may inspect bounded retained core state but never initiate network or refresh target/finality implicitly.

Structural preflight counts the root object as depth1 and allows at most16 container levels. Requests have no arrays. Response arrays are allowed only at the following paths (R is `record` or `job.result`): `snapshot.pin.source_pin.participants` exactly2; `R.source_receipt.input.participants` exactly2; each participant's `roster.slots` and `roster.assigned_at_ms` exactly10; each participant's `members` and nonnull `entry.latest_receipt.members` 0..5; `R.source_receipt.captures` exactly2; `R.anchor.merkle_branch` 0..20; `R.scan_restarts` 0..16; `R.conflicts` 0..8; `history.events` 0..8. No condition/checkpoint field is an array on the wire. All arrays must be nonnull and counts are checked before typed decoding; Go fixed-array decoding alone is insufficient. Compact Anchor carries the core's11 ordered scalar/branch fields and no full foreign transaction. Its txid/Merkle branch is not independently sufficient to prove the payload without the raw anchor block.

Signed numeric exceptions are path-specific: `R.cursor.height`, `R.cursor.complete_to_height`, `R.scan_restarts[*].previous.height`, `R.scan_restarts[*].previous.complete_to_height`, `status.scan_height`, `status.scan_tip_height` and the corresponding `history.events[*]` status fields use the core's signed scan-height grammar. Final structural checks still require each restart's previous.height>=0; only a previous.complete_to_height may remain -1 there. No other height, index, count, timestamp or unsigned field accepts a negative value. `Info.funding_height` is a core-only signed exception and does not occur in this helper response. Boolean tokens never satisfy numeric0/1 checks.

The complete Seed error vocabulary is `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `outcome_unknown`, `seed_unavailable`, `seed_setup_incomplete`, `seed_corrupt`, `seed_capacity`, `seed_clock`, `seed_calendar`, `seed_created_too_late`, `seed_source_incomplete`, `seed_not_found`, `seed_not_prepared`, `seed_node_offline`, `seed_waiting_for_funds`, `seed_scan_incomplete`, `seed_conflict`, `seed_target_unavailable`, `seed_not_mature`, `seed_cancelled`. No old-family error is leaked into this union. Account held/forbidden maps forbidden; unauthorized/expired maps unauthorized; mismatched current parent view maps stale_view. Core profile/closed/unavailable maps seed_unavailable, corrupt maps seed_corrupt; a poisoned store maps seed_unavailable unless the current operation is already outcome_unknown. A 250ms metadata acquisition deadline maps busy; explicit cancellation maps seed_cancelled unless durable/network uncertainty takes precedence. Unknown internal errors are seed_unavailable. Core additions require an explicit exact-draft update before freeze.

Native establishes Manifest→Stable→Entry→Event context first using the existing owned pipe gate. It calls `gymseed.CandidatePin` on that established Event pin, then requires exact equality and derived StoreID in SeedSnapshot. Native must not duplicate fixture key derivation or bless an arbitrary supplied Pin. CreatedAt is trusted local initialization evidence from the owned helper/root, not proof of public-chain timing. Retain the exact authority Pin/CreatedAt through that helper's lifetime; a later nonblank change refuses. No Record/Job response can establish or replace it.

Native correlates each accepted job to current full view, store, exact event/date and kind. It tracks only active/queued/last Seed tuples and learns an unknown kind only by querying that exact same-view Snapshot-discovered job. Delayed results and cancel responses must match the captured tuple. A discovered job's malformed/lost response is outcome_unknown until its kind is known; malformed/lost Prepare/Publish/Attest ACKs are outcome_unknown after dispatch to the helper. Read-only Reconcile uncertainty is evidence-observation uncertainty, not permission to rebroadcast. Context cancellation after a private metadata prefetch must prevent the later action frame. No recursive acquisition of the pipe gate or request-ID reuse.

Terminal job bytes and digest are immutable across repeated polls. Accepted Begin discards the prior terminal only after ACK-publication fencing succeeds. Preflight refusal or failed fencing preserves it. A subsequent accepted job may return a newer journal observation; it is a different job. On view loss/expiry/helper failure clear view/job presentation, never persisted journal evidence.

## 5. Lane, capacity and outcome semantics

Extend the existing family checks explicitly with global active1/queued1; a task belongs to exactly one family. In particular, a nil preview/entry/event pointer must not make a Seed task an old Stable task. At most one Seed job may be active or queued. No Seed IDs enter old-family snapshots, cancellation paths or terminal caches. Foreground seed work can wait behind one active foreign family but cannot create another scheduler. Queue deadlines include time waiting; use a 30-second operation deadline and stop at the core eight-block bound rather than looping through the whole chain. UI may request the next explicit bounded Reconcile after completion; no automatic Publish, Attest, replay, funding or mining.

Reserve the full 256 KiB seed budget before a Begin ACK, partitioned into MaxSeedJobSize245760 bytes for one canonical retained Job (which owns its Record once) and MaxSeedMetadataSize16384 bytes for all other retained Seed data. Snapshot/Pin, fixed errors, at most three active/queued/last correlation tuples and any cached selected Status must jointly fit the metadata partition. Retain exactly one authoritative Seed Pin in that metadata; Snapshot/correlation may reference the same immutable authority rather than deep-owning another Pin/View copy. Correlation tuples contain only job_id, kind, state, event_key, saturday and terminal_digest, with exact existing scalar bounds, and refer to the one current authority/view. Fixed presentation identity/render strings are additionally capped at4096 bytes within the metadata partition. Any duplicate retained representation must be charged and refused if it exceeds the partition.

History is a bounded response, not a retained helper/native history cache; no second whole Record is kept in status/job metadata. Charge the seed reservation reciprocally in every manifest/stable/preview/entry/event cache admission, lazy portrait and export slot path. Remain under the existing 1 MiB aggregate. Native/frontend retain one bounded Record representation and account for seed authority, correlation and rendered identity strings in their existing totals; a frontend-selected history page is charged by actual serialized bytes as well. Clear replaced owned objects; do not raise caps. Formula-only maximum-shape checks must confirm job/wrapper/root marker, metadata partition and reciprocal budgets before freeze.

Prepare/Publish/Attest can durably change state. Any error after known successful durable mutation, including public validation, size failure, final View failure or ACK loss, is outcome_unknown with exact-event reconciliation. Distinguish local raw-tx preparation from actual network dispatch and inclusion. Reconcile writes evidence observations but sends no transaction; uncertain evidence writes must not be described as a new publication. Fatal local poison/corruption stops the miner/lane dependency while preserving the retained operation and reporting context-appropriate uncertainty. Waiting funds/offline/ordinary conflict are bounded visible conditions, not reasons to erase a declaration or change chain.

Core replay semantics govern retries. A new Publish request never rebuilds retained raw bytes, selects a new UTXO, bumps a fee or silently retransmits because a status poll timed out. Exact retransmission, if core permits it, is a new explicitly authorized Publish operation with the same retained bytes. No Seed action mutates candidate assignment, roster, intake, noon condition, XP, preview state or D62 result history.

The UI distinguishes historical first Finality from `current_finality`, which means eligibility at the **last verified chain observation**, not continuously checked present truth. Display the core's separate chain-observation timestamp beside that fact. Local Status observed_at_ms/deadline updates and Snapshot/Job polling never advance that chain-verification timestamp. Snapshot.NodeState is present owned-process attachment availability and may be offline while a retained Record's last chain observation was online. Do not silently rewrite that historical observation or relabel a fresh local clock sample as fresh canonicality.

An above-target reorg can reduce observed descendants without replacing the pinned target or erasing first Finality. Funding/unanchored scan restart generations are bounded evidence recovery, not reseeding; show incomplete scanning until core reestablishes the prefix. Only core's exact pinned anchor/target or authenticated-statement conflict predicates create event conflict. Deadline escalation uses the trusted calendar/clock and core `incident_at_ms`/`escalation_due`; a browser timer or mere node outage never invents a public-chain incident. Candidate evidence remains conditional and does not award results, execute bouts, or imply unbiased randomness or a production checkpoint signer.

## 6. Required verification and joint freeze

Preserve all old routes/frames, cap edges and actual helper shutdown tests. Add normal/race and cheap-profile refusal tests for new typed grammar, independent core/transport corpus, all family queue directions, reciprocal cache admission, ACK fencing, same-view release, old pending/new login handling, runtime-only zero attachment and rejection of incrementing256 even at identical tip height.

Actual isolated normal-PoW composition must separately prove default unfunded/offline behavior, explicit fixture funding, exact raw-tx retention before dispatch, unknown ACK recovery, carrier inclusion, complete scanning, pinned target, explicit fixture Attest and 60-target-descendant boundary. Mining/funding is explicit test setup and never attributed to app bootstrap. Test reset/hold before and after dispatch intent, held read-only Reconcile, node death/cancellation/Close joins, restart samechain, and no leaked processes/listeners. Source snapshot/roster/condition and ordinary payout must remain unchanged.

If a closed-event helper acceptance needs time movement, extend only the existing `slugtestclock` scoped fixture constructors, absent normal/release builds; no runtime flag/env/DTO clock or fake public verdict. Real-clock package evidence must honestly distinguish open future-event behavior from tagged closed capture/publication composition. Keep one full D74 review at the end once major flows are ready; this draft authorizes no GUI bypass or production release.

The full Pin comes from CandidatePin and the parent prepared lifecycle uses the core's reconstructible initialization intent; no sampled activation tip or additional node setup is pending. Exact AttachOwned and source callbacks are core §6.1/§8. The helper error vocabulary, waiting_source state, family grammar, ownership and reciprocal cache partition above are complete engineering choices. Joint freeze binds final core/helper bytes, formula-only maximum-shape/budget report and independent reviews. Implementation and expected-vector generation still wait for that explicit approval; no new player policy is proposed here.
