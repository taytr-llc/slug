# Set 44 helper integration proposal — authenticated historical snapshots

**Status:** proposed engineering integration, awaiting the joint core/helper byte-contract freeze. No runtime or oracle is changed by this document.
**Date:** November 17, 2026.
**Decision owner:** root, with independent core/reference review. D76/D77 are user decisions recorded in `slug/EVENT_LOCK_POLICY.md` and spec §7, decision-only commit `b7985449768989cd808897c9ba9a432dbdd11729`. Choices below implement a bounded candidate foundation and do not add product policy.

## Scope and authority

D76 fixes the event at Saturday noon Pacific, including roster/assignment facts, eligibility and condition. Later retirement, roster changes or future ownership changes must not rewrite that event. D77 retains the eventual selected seed, waits without replacement or automatic cancellation, and reaches an incident deadline at the following Saturday lock. Neither decision supplies a seed-selection or finality protocol.

Root has selected **late authenticated capture per participant** for Set44. Each participant is one of the two existing public fixture gyms. Capture may happen after noon but reconstructs the state at noon from retained history. The event remains incomplete until both fixed participants are captured. This is not an unattended/offline league service and does not claim a timer executed precisely at noon.

Do not fabricate `gymstable.Session`, refresh a token inside a task, or add an all-powerful background account capability. New captures use the original captured View session and exact account/enrollment binding. Reset, logout and expiry interrupt the task; a new login may start a new explicitly correlated capture or read an existing immutable result. They never authorize a previous in-flight task. Existing EntryManage/RosterManage actions and D72 restrictions remain unchanged. Held current sessions may capture/read via View because this does not transfer assets or rewrite roster/intake.

Candidate assignment is the only implemented custody fact. Record the exact immutable `gymstable.Assignment`, its pinned account/gym and source digest. Describe it as a candidate assignment snapshot, not transferable ownership or an invented historical transfer journal. A future ownership authority must have its own pinned rules/version and cannot reinterpret these candidate records.

## Historical boundary and source fencing

The core contract must define noon as `C = Event.CutoffMS` and consistently select committed mutations with `CommittedAtMS < C`; equality belongs to the following side of the boundary. This matches existing intake closure at equality. Preserve the pre-write recorded timestamp convention already used by stable/intake journals: filesystem ACK time is not a second eligibility time.

Retained committed stable receipts contain complete ten-slot states, immutable operation order and timestamps. Reconstruct the last committed roster state strictly before C, not today's slots, the roster at entry-save time, or the roster at capture time. Prepared/abandoned operations do not change historical slots. Repeated no-change roster operations do not create a roster revision. Entry selection is the last committed receipt for the date before C; a withdrawal freezes an empty selection. Missing/uninitialized participant evidence is not silently equated with an authenticated empty participant.

Drain the entry and stable operation gates before sealing their durable clock high waters to at least C. That prevents later authorized writes from acquiring a pre-C timestamp. Historical reconstruction must not temporarily set either runtime clock backward. Condition uses exactly C, with original birth and the existing zero-XP/zero-training candidate semantics; actual capture/observation time is separately retained evidence. An entry saved while eligible may have become retired or unrostered before C: preserve its submitted identity and the explicit cutoff eligibility outcome rather than silently dropping or replacing it. Class/static identity mismatch is corruption, not a selectable alternate class.

The proposed core lock order is event work gate → entry gate → stable gate → original account View lease → stable mutex → entry mutex → event mutex. Verify at most five source records serially before taking the final account lease. Source verification must compare exact retained assignment/source-record digests. Recheck history fences and original session inside the final lease. All stable/entry observation writes precede the event-store callback; no foreign source write follows an event commit. No callback may reenter any held store/account/controller gate.

Historical snapshot input and digest must exclude late capture timestamps, current generation, post-cutoff operation tails and present-day roster values. Keep those observations in separate capture evidence. The core owns final exact canonical order, preimage domains, participant order and schema limits. The helper never accepts claimant receipts, snapshots, a supplied cutoff, supplied condition, supplied participant pin or an arbitrary history cursor as capture authority.

## Shared parent store and child lifetime

Keep the current Candidate Pilot bundle, account root and frontend. Add a shared candidate event component beneath that root; do not create another demo namespace. Its participant list is fixed to the two exact fixture entry pins, in the core's canonical order. Do not discover participants from an untrusted directory scan or silently add/remove one after the event-store pin is created.

The account parent owns the event store for the lifetime of the helper. The logged-in child borrows that exact handle for a tracked call; the event store never retains an entry/stable/manifest source pointer between calls. Capture passes the concrete current child's `*gymentry.Store` under its lane lifetime gate. Matching a string StoreID is insufficient: the core validates the borrowed source's complete pin. Logout first detaches the view, then cancels/joins child work outside parent/account locks. Only after child borrowing is drained may the parent close the event store. Account switching reuses the same event store and a newly authorized child, never an old session or closed source pointer.

If both fixture installations are not ready, existing account/mining/stable/entry actions remain usable and EventSnapshot reports `setup_incomplete`. Do not create an inactive participant's wallet, log it in, sign on its behalf or open its private recovery state to satisfy event readiness. A user's later legitimate fixture login can complete the existing installation, then event setup retries against the complete fixed pins. A validated existing two-fixture v3 root can create the coordinator during NewEvents before any live child or login. Creation still requires actual time strictly before the desired cutoff; no backdated coordinator or late participant addition is permitted.

This prerequisite is real, not an unnecessary login check: AccountID is randomly allocated by gymaccount.Login and source AuthorityID by registry.CreateManifestAuthority. Deterministic hashes of a full pin cannot derive those missing inputs from fixture names. Root explicitly chose the complete established pins; this version does not introduce partial participant declarations, preallocated identities or a late-binding policy.

Use explicit recoverable parent installation state rather than silently extending the v3 JSON decoder. Proposed new `NewEvents` route upgrades the same validated root to v4 and preserves every stable/entry install while installing one shared event component. Old `NewEntries`/`NewStable`/`NewManifest` reject v4. New-route writers must preserve all v4 fields when the second fixture is installed. Cheap-PoW refusal and existing-root validation occur before new directories/marker writes. An established ready event component that is missing must never be recreated. A prepared component can resume only absent, real empty, exact recognized initialization intent, or exactly matching initialized-empty state through the core's explicit ResumePrepared seam; unknown partial/corrupt evidence is retained/refused.

Avoid duplicating two complete entry pins in an already near-limit parent marker: derive the immutable event pin from the existing validated child pins and compiled selectors. The root extension retains `event_schema`, `event_phase`, `event_store_id` in that order; the StoreID already commits the complete event Pin, so a second pin hash is redundant. The event component marker retains the full Pin. Appendix B fixes the phase and size rules, subject to a formula-only maximum-shape check before freeze. Parent and child validators must use the same exact v4 representation; no generic unknown-field tolerance.

## Typed helper and native surface

The following helper declarations align with the gymevent core draft. They become normative only in the joint freeze; no runtime implementation or corpus generation is authorized by this proposal alone.

```go
type EventControllerAPI interface {
    EntryControllerAPI
    EventSnapshot() EventSnapshot
    EventStatus(context.Context, string, string) (gymevent.EventStatus, error)
    EventRecord(context.Context, string, string) (gymevent.SnapshotReceipt, error)
    EventHistory(context.Context, string, string, uint32) (gymevent.HistoryPage, error)
    BeginEventCapture(context.Context, string, string) (EventJob, error)
    EventJob(context.Context, string, string) (EventJob, error)
    CancelEventJob(context.Context, string, string) (EventJob, error)
}
func NewEvents(pilot.Config) (*accountpilot.StableController, error)
func ServeEvents(context.Context, EventControllerAPI, io.Reader, io.Writer) error
```

Arguments after context are original `view_id`, then canonical Saturday or exact job ID, except History's canonical exclusive cursor and limit. `BeginEventCapture` derives its participant exclusively from that original current child; no participant argument exists. Status/Record/History must correlate the event-store pin derived from the already authenticated root/stable/entry pins, not bless a helper string on first sight. An authenticated fixed participant may read aggregate **public candidate snapshot facts** for both fixtures; no other account's session, account-recovery state, keys or private receipt enters those DTOs.

Use a separate closed Event v1 wire family, preserving all Manifest/Stable/Preview/Entry v1 frames and caps. Proposed commands are `event_snapshot`, `event_status`, `event_record`, `event_history`, `event_capture`, `event_job`, `event_cancel`. Each response carries a current View and event StoreID; historical capture author/evidence remains historical. Only Snapshot has the exact unavailable-view/null exception. Fixed-order typed re-encoding, decoded-key duplicate rejection, exact required/unused fields and bounded array/depth checks follow the existing private codecs. Appendix A fixes the separate helper response union and enums; the core contract owns its nested DTOs.

Keep request ≤8KiB, response ≤128KiB within the existing 256KiB framing ceiling, and History ≤8 metadata summaries. A full event is at most two participants × five submitted fighters; never put eight full events in History. Public snapshot DTOs contain no raw Argon proof/coinbase or private source receipt. Snapshot/job/status parsing must not call Argon, SVG rendering or bout simulation. Native checking uses bounded shape, canonical digests and exact captured tuples; authoritative history/proof work stays in the helper/core.

Every read releases copied data only after a final original View recheck. Native tracks current full view/store/event/job/participant correlation, including discovered jobs and delayed cancellation. A returned original capture is not permission to mutate under its old author. A successful event query under a new login is wrapped in that new current query view.

## Lane, scheduling and cache

Add the event family explicitly to the existing single-active/single-queued lane. Replace negative family tests with an exact family discriminator wherever needed; event IDs never appear in old-family snapshots. Permit at most one active-or-queued event task. Capture is async because it may verify five normal-PoW proofs; metadata and polling remain bounded and do not wait behind source work. Queue ACK publication fencing must prevent a reported rejected/busy job from later starting.

At most the currently selected closed Saturday can be offered for capture. A normal UI may request capture automatically only when authoritative status has observed_at_ms>=cutoff_ms, coordinator_created_at_ms<cutoff_ms, and this original view's participant is missing, with a once-per-view/event guard and explicit failed/unknown reconciliation. Do not sweep all past dates, switch the selected Saturday, or create a second scheduler. No browser timer decides cutoff/eligibility. Open events expose pending status; successful production-package tests conducted before the real cutoff must not pretend to seal them. Closed-path testing uses private test-only authority clocks and genuine normal-work fixtures, with no runtime clock flag.

Retain one terminal EventJob and a small selected-event summary. Reserve a fixed aggregate event budget ≤128KiB **before** publishing a Begin ACK, charge it in every manifest/stable/preview/entry cache admission in both directions, and include native/frontend retained event data in their existing 1MiB totals. A detailed completed record should have one owned representation rather than duplicate whole snapshots in job and status caches. Do not enlarge existing shared limits to conceal duplication. Clear view-bound presentation on logout/reset/expiry/Close; persistent capture remains in the event store. Cancel/Stop joins the current unavoidable Argon call and prevents later proof/commit dispatch; it never rolls back a completed capture.

## Durable outcome and recovery

Capture identity is deterministic from event + fixed participant, not an arbitrary retry-created mutation ID. The core defines idempotent existing-capture replay and frozen original capture evidence. If the event write or final original-session/public-response publication is uncertain, expose `outcome_unknown` and reconcile the exact event/participant through current authorized Record/Status. Do not declare cancellation after a known commit, generate another event to retry, erase the prior participant, or substitute the newer roster. An `outcome_unknown` from an observation-only Status/Record/History call is clock-observation uncertainty in that command context; it does not imply a roster, capture or seed mutation. The UI must preserve this distinction without inventing another durable operation ID.

For poisoning, close/reopen only the owned affected handle after all tracked borrowing drains, in reverse lifetime order. In-memory completion is not a substitute for reading the committed record. Partial event capture is durable and visible as incomplete; the second participant's error cannot discard the first. The final root/snapshot digest appears only when the complete fixed participant set has passed validation and committed atomically with its completion marker. Repeated polling/reopen returns those exact historical bytes.

## Seed/deadline presentation

The core's proposed `SeedAuthority = "unavailable"` is a fixed truthful selector for this foundation. It may expose `IncidentDeadlineMS` as the following Saturday noon from the **same pinned TZif and civil calendar**, not `C + 168h`. DST weeks can span167 or169 hours. The final supported Saturday must have an explicitly defined following-noon deadline even if that next date is outside the intake date range; calendar failure cannot silently substitute a duration.

No DTO accepts a seed hash, height, finalized flag or caller-supplied incident time. No node checkpoint changes or chain veto follow a candidate deadline. Authority-observed time may tell the UI that the candidate waiting threshold was reached, with a clear seed/finality-integration-unavailable label. It must not announce a measured public-chain incident or a selected/finalized seed. D77's eventual real incident requires the later pinned seed/finality authority and does not permit fallback randomness, seed replacement or auto-cancellation now.

## Acceptance and remaining contract work

Required tests cover: exact C−1/C/C+1 history boundaries; same-millisecond receipt order; proof/clock/gate races across C; retirement exactly at C; roster removal before versus after C; reset/logout/final-publication races; both capture orders; one participant committed while the other fails; replay across new login; clock rollback and post-noon backdating refusal; genuinely missing versus authenticated empty participant; poisoned write/reopen/lost ACK; real process kill at each new installation and capture boundary; fixed participant/cross-account/cross-store/source substitution; shared lane all directions; reciprocal cache reservations; old-route/old-frame preservation; spring/fall following-Saturday deadlines; and no proof/SVG/engine on repeated polls.

The current core draft resolves no-entry versus withdrawal, strict history/fence coverage, replay, fixed calendar deadline, coordinator time and exact nested DTOs. The maximum-shape check `set44-helper-shape-bounds.py/.json` is formula-only using the deliberately oversized core constructors, not a reference outcome corpus. It checks every wrapper plus LF, root marker and retained snapshot/job/status against the unchanged caps. Joint review must bind the final exact core/helper/tagged-test declarations and these source hashes before implementation or independent expected-output generation. No further player policy is being proposed here.


## Appendix A — exact helper wire proposal

These are new types in `desktop/pilot`. Fields below appear in this exact order with the literal JSON names in parentheses. Every field is required; pointer fields encode null when inactive. Nothing is added to an old frame. Uint counters are bounded by 2^53−1 as well as their declared type; IDs use canonical lowercase64-hex, optional IDs use empty string, and dates use the core Saturday grammar.

```go
const MaxEventRequestSize = 8192
const MaxEventResponseSize = 131072
const MaxEventCacheSize = 131072

type EventRequest struct {
    ID uint64                  // id, 1..2^53-1
    Command string             // command
    ViewID string              // view_id
    Saturday string            // saturday
    AfterEventID string        // after_event_id
    Limit uint32               // limit
    JobID string               // job_id
}
type EventResponse struct {
    ID uint64                       // id
    OK bool                         // ok
    View *ManifestViewPin           // view
    StoreID string                  // store_id
    Snapshot *EventSnapshot         // snapshot
    Status *gymevent.EventStatus    // status
    Record *gymevent.SnapshotReceipt // record
    History *gymevent.HistoryPage   // history
    Job *EventJob                   // job
    Error string                    // error
}
type EventSnapshot struct {
    Version uint32              // version, 1
    State string                // state
    Error string                // error
    View *ManifestViewPin       // view
    StoreID string              // store_id
    Pin *gymevent.Pin           // pin
    CoordinatorCreatedAtMS int64 // coordinator_created_at_ms
    ParticipantID string        // participant_id, this original view's GymID
    ActiveJobID string          // active_job_id, only this family
    QueuedJobID string          // queued_job_id, only this family
    LastJobID string            // last_job_id, only this family
}
type EventJob struct {
    Version uint32                 // version, 1
    JobID string                   // job_id
    Kind string                    // kind, capture
    State string                   // state
    View ManifestViewPin           // view, original accepted job view
    StoreID string                 // store_id
    EventID string                 // event_id
    Saturday string                // saturday
    ParticipantID string           // participant_id
    Error string                   // error
    Result *gymevent.CaptureResult // result
}

func MarshalEventRequest(EventRequest) ([]byte,error)
func ParseEventRequest([]byte) (EventRequest,error)
func MarshalEventResponse(EventResponse) ([]byte,error)
func ParseEventResponse([]byte) (EventResponse,error)
```

The native Gym names match the controller methods, minus context: `EventSnapshot`, `EventStatus(viewID,saturday)`, `EventRecord(viewID,saturday)`, `EventHistory(viewID,afterEventID,limit)`, `BeginEventCapture(viewID,saturday)`, `EventJob(viewID,jobID)`, `CancelEventJob(viewID,jobID)`. Each native method returns the complete EventResponse, never a trusted bare core object. No export/import/seed/clock API is added.

Request command matrix: `event_snapshot` has all argument fields empty/zero; `event_status`, `event_record`, `event_capture` require only ViewID+Saturday; `event_history` requires ViewID, optional AfterEventID, Limit1..8; `event_job`, `event_cancel` require ViewID+JobID. Unused fields must be empty/zero. Every successful response has exactly the payload corresponding to its command and Error empty; every failed response has all payload pointers null, View null, StoreID empty and one allowed error. Snapshot success is the only outer envelope allowed a null View/empty StoreID: those equal its nested snapshot fields. Setup-incomplete Snapshot has a current View but empty StoreID. Other successful replies require the exact nonnull current View and nonempty StoreID. Discovery never turns a historical captured participant into current authorization.

Snapshot states are `unavailable`, `setup_incomplete`, `ready`, `busy`, `failed`. Unavailable is exactly Version1, Error=`event_unavailable`, View=null, Pin=null, all IDs empty and CoordinatorCreatedAtMS0. Setup-incomplete is Version1, Error=`event_setup_incomplete`, current View, own published ParticipantID, Pin=null, StoreID/job IDs empty and CoordinatorCreatedAtMS0. Ready/busy/failed have full validated Pin, derived StoreID, own ParticipantID, positive coordinator time, and current View. Ready/busy Error is empty. Busy means the shared lane contains any active or queued work, even when both Event family job IDs are empty. Failed has a bounded sanitized storage/authentication error, never `outcome_unknown` as a summary of an unrelated mutation. Job states are `queued`, `running`, `completed`, `failed`, `cancelled`: queued/running have empty Error/null Result; completed has empty Error/non-null Result; failed has allowed Error/null Result; cancelled has Error=`event_cancelled`/null Result. Result Status event/store and Capture participant must exactly match the job. Once terminal, the entire EventJob including its Result, status observation time and canonical bytes is immutable across polls. A new replay job can obtain fresh Status.observed_at_ms while returning exact old capture bytes; it cannot rewrite the preceding retained job. Completed replay may retain older evidence, never an older outer View.

Allowed wire errors are exactly the core's public categories (`invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `event_unavailable`, `event_corrupt`, `event_capacity`, `event_clock`, `event_calendar`, `event_not_due`, `event_created_too_late`, `event_incomplete`, `event_conflict`, `event_not_found`, `event_cancelled`, `outcome_unknown`) plus `event_setup_incomplete`. Closed/profile refusals map to event_unavailable; poisoned writes map conservatively according to known commit/observation state. Never expose raw database paths/errors. A completed durable Capture followed by any validation, account or publication failure produces failed/outcome_unknown. A capture transport failure after dispatch also yields outcome_unknown; pre-dispatch busy/refusal stays definite. Canceling a job does not prove a capture absent. A lost/malformed discovered job frame is conservatively unknown because its only possible kind is Capture.

Native establishes context by fetching current Manifest/Stable/Entry snapshots through its existing serialization gate, then EventSnapshot. The current participant's complete entry pin must equal the already established Entry/Stable pin; its account/gym/source must match the full ManifestView. The other participant must be the other compiled published fixture under that same account authority/network/genesis and compiled selectors. The parent-owned helper, through its exact validated v4 root, is the authority for the other installed account/source's random IDs; the browser is not. Native freezes this complete Event Pin/StoreID for the helper lifetime and refuses a changed Pin. Reset/account switch clears presentation/correlation and reauthorizes the current participant without adopting a changed coordinator. Restart establishes a new helper connection against the same validated disk root; missing established state is refused by the helper. No standalone arbitrary StoreID becomes a trusted pin.

Native correlates at most active/queued/last Event tuples `(full View,StoreID,JobID,EventID,Saturday,ParticipantID)`. Snapshot discovery may establish a job tuple only from a matching same-view EventJob response. Calls for current old-family job IDs never probe or learn Event payloads. Captured historical content may be read under a new current View, but no old in-flight task may be resumed under that View. Every cached EventSnapshot/Job copy is released within the original View lease and final current-child/view check. Explicit core Status/Record/History obtain their own original View and final release check; do not nest an outer account lease around event→entry→stable work.

Parsers enforce a fixed maximum depth12 and decoded key length64 before materialization. Only these arrays exist: Pin.participants exactly2; stable roster slots/assigned_at_ms exactly10; snapshot participants/captures exactly2; participant members0..5; latest receipt members0..5; captured_participants0..2; History events0..8, including those exact arrays at their declared nested repetitions. No generic arrays/fields are admitted; fixed Go arrays alone are not a count check. Bound each nested core DTO using the core cap before decoding. Permit transport whitespace/key order/escaped ASCII key spellings, reject duplicate decoded keys/unknown keys/BOM/invalid UTF8/trailing values/float/exponent/-0, and hash only fixed-order typed canonical re-encoding. ParseEventResponse(raw) has no external Pin parameter: it therefore performs private closed structural/bounded checks and pin-independent canonical-byte/digest checks only. A Snapshot's explicit Pin can be structurally validated, but a non-Snapshot response must never infer a new Pin from its payload. The command-aware helper/native layer correlates against the previously established full EventSnapshot Pin. It checks EventForDate, current View/participant/store and command/job tuple before accepting any result. The parsed DTO by itself is not trusted authority evidence.

Core full semantic validation (canonical manifest/class/condition relations) runs once before helper capture/cache publication, outside the final account lease. Native may run those bounded no-proof semantics once when accepting an initial complete result or explicit Record, outside account locks and before publishing; then retain its canonical digest with the checked presentation. Repeated cached Snapshot/Job polling uses private structural checks plus same-tuple/canonical-result digest comparison, without regenerating manifest or condition. Private canonical hash encoding must not accidentally call public core Marshal/Digest functions that revalidate projections. No transport parser or poll may call source proof verification, portrait Render/Digest or bout execution. Public Parse/Marshal APIs remain codecs, not public Verified-result bypasses.

EventSnapshot/Job polls only copy retained data and reauthorize; zero source proofs and no core clock sampling. Explicit Status/Record/History are serialized metadata operations using a cancellable 250ms gate budget and existing try-lane behavior. They do not wait behind hashing and do not create a background timeout goroutine. Opening/rebinding the actual source chain occurs only under the existing owned lane lifetime and context, before any account lease. A deadline causes no subsequent core dispatch. Status may advance observation clocks; Record/History never capture a missing participant. Mutation ambiguity is reported in the command's own context.

The helper permanently charges at most MaxEventCacheSize for one latest EventJob, the pinned setup/snapshot metadata and a small selected Status. It does **not** retain the complete Record or History response: these are explicit reads with bounded transient serialization, so a124KiB Record cannot silently coexist with a second retained transcript. A successful accepted Begin discards the prior terminal job; preflight/rejected publication preserves it. Reserve the maximum new job+setup bytes before ACK, not after commit. Native/frontend retain at most one event detail representation (CaptureResult or full Record), plus bounded job/status metadata; replacement releases the prior detail. Charge all retained event bytes in their existing1MiB totals and in every reverse admission/lazy-SVG/export path. No old limit or frame scanner is widened.

## Appendix B — exact additive installation proposal

The activation flag is `--candidate-events`, requires `--candidate-manifests --candidate-stable --candidate-entries`, and changes only the explicit route selected by the existing Candidate bundle. Native title/mode/frontend stay Candidate Pilot; no new independent app/data namespace. New route uses NewEvents/ServeEvents; root owns CLI/main/native wiring. Earlier constructors retain their exact defaults and reject a v4 root rather than ignoring it.

The canonical parent marker is the complete v3 field sequence unchanged except Version=4, then `event_schema uint32=1`, `event_phase string`, `event_store_id string`. The maximum remains16384 bytes including LF, pending exact maximum-shape proof. Full event Pin is derived from both existing ready entry installs sorted by GymID plus compiled Calendar/Rules; StoreID is its core ID. No separate redundant pin digest is stored. Valid phases:

- `waiting_participants`: fewer than two matching ready published-fixture entry installs, StoreID empty, shared event path absent. Existing prepared per-gym installs retain their prior recovery handling. This is setup_incomplete, not an empty participant.
- `prepared`: both matching ready entry installs, StoreID exactly derived, shared event installation admitted but not published ready. Require absent intended directory, verified real empty directory, or exact recognized matching initialization/initialized-empty state as specified by core ResumePrepared. After completion Info must report EventCount0 and no event rows. A resumed incomplete intent samples its actual final creation time; an already initialized coordinator retains original CreatedAtMS. Never delete/recreate a partial store to obtain an earlier time.
- `ready`: both matching ready entry installs and StoreID exactly derived. Require exact existing marker/db/Pin and read-only Info consistency. Missing established path is failure, never Create. Count may be0..256.

The component directory is `<account-root>/events`, within the existing protected data root. Upgrade validates the selected exact old marker and required child inventories before creating the instance lock or writing the new marker, then rereads under the parent instance lock. Existing old v1/v2→v3 validated conversion remains the basis; it may not repair/adopt an unknown child. Every current new-route child stable/entry marker reader/writer explicitly accepts/preserves v4 and all event fields. It must not rewrite to v2/v3 or let adding the second gym drop prior install entries.

When both ready pins already exist, NewEvents writes prepared→creates/syncs coordinator→closes/reopens or validates immutable Info→writes ready before any live login. When the second normal authenticated fixture installation later reaches ready, its parent-marker write atomically records that second entry install as ready AND transitions event_phase from waiting_participants to prepared with the exact derived StoreID. There must be no durable waiting_participants marker containing two ready installs and no intermediate v3 rewrite. The parent then performs the same one-time event installation before publishing event-ready attachment; it does not log in the first gym again. Failed/uncertain setup retains prepared state and fails closed for event capture. Never overwrite a ready coordinator, recompute CreatedAtMS, change participants or reset after a new account generation.

Required private crash checkpoints are `event-prepared`, `event-created`, `event-ready`; no public mutation bypass. Core Create first durably records the exact initialization Pin intent, then samples its final immutable CreatedAtMS and commits initialized metadata. Ordinary Open refuses an intent-only partial store. Only the parent's durable prepared phase can invoke ResumePrepared on the core's exact recognized state, preserving its Pin and sampling actual final creation time; a final-metadata lost ACK reopens the exact initialized-empty coordinator without changing CreatedAtMS. The parent never samples or supplies the coordinator creation time. Core's metadata Info (Pin/StoreID/CreatedAtMS/EventCount, no clock advancement/event payload) supplies the necessary prepared-empty check. Parent remains the event handle owner, child receives an immutable internal dependency at construction or attach-before-publication, and no mutable public setter replaces it. Close/detach drains child lane+all tracked borrows before parent closes event DB; callbacks never close/join under account lease.

## Appendix C — closed-helper test clock, absent from release

The meaningful closed-event helper test needs clock injection across package boundaries. Use a separate **`slugtestclock`** build tag, never `slugtestpow`; normal256MiB proof hashing and all normal-profile gates remain in effect. Use scoped constructors rather than a process-global clock or post-publication clock setter. The following exported symbols exist only in files guarded by `//go:build slugtestclock`:

```go
// gymaccount: fixture schema1 only, rejects nil clock.
func CreateFixtureForTestClock(path string, config Config, clock func() time.Time) (*Store,error)
func OpenFixtureForTestClock(path string, config Config, clock func() time.Time) (*Store,error)
// gymstable:
func CreateForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store,error)
func OpenForTestClock(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time) (*Store,error)
// gymentry:
func CreateForTestClock(ctx context.Context, path string, pin Pin, source *gymstable.Store, clock func() time.Time) (*Store,error)
func OpenForTestClock(ctx context.Context, path string, pin Pin, source *gymstable.Store, clock func() time.Time) (*Store,error)
// gymevent:
func CreateForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store,error)
func OpenForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store,error)
func ResumePreparedForTestClock(ctx context.Context, path string, pin Pin, clock func() time.Time) (*Store,error)
// desktop/pilot, only the explicit event route:
func NewEventManifestAccountForTestClock(config Config, account StableAccountConfig, clock func() time.Time) (*StableController,error)
// desktop/accountpilot:
func NewEventsForTestClock(config pilot.Config, clock func() time.Time) (*StableController,error)
```

Tagged wrappers use the same private implementations, validation, locks, leases, syncs and profile checks as ordinary constructors, with the clock bound before the first timestamp-sensitive operation. They do not accept arbitrary account/source verdicts and still require the two exact public fixture identities and source pins. Normal Create/Open signatures and their time.Now defaults remain unchanged. NewEvents has a private immutable constructor-dependency bundle, and the tagged parent supplies scoped tagged store/child factory closures in that bundle. Every later child/source reopen uses those same closures; no fallback to a real-time handle in the middle of the test. Account factory wrappers remain schema1 and cannot open/create provider authority stores. Private parent credential-age calculations use the same bound clock; real context/transport timeouts remain monotonic wall-time deadlines. A jumped test clock is observed through the real account lease; no timer shortcut grants a session.

Tests drive actual parent/controller/ServeEvents and native bridge pipe paths in a test executable with published fixture stores and genuine normal-work receipts. Each test owns one synchronized clock object, passes its method before construction, advances it through C and the next-Saturday deadline, and closes all scoped handles on cleanup. Clocks may move backward only in explicit rollback-refusal tests. No global setter/reset is needed and tests cannot alter other running handles' clocks. Existing source receipts/births are unchanged. There is no runtime/helper CLI clock flag, environment backdoor, production Config clock field, supplied noon DTO, public fake session or alternative PoW model. The regular CLI never calls any tagged constructor, even if a diagnostic CLI is accidentally compiled with the tag.

Evidence labels this explicitly `normal-PoW, test-clock helper composition`; it is not the shipped bundle or real-wallclock cutoff evidence. Release build/source guards clear inherited Go tags, require no slugtestclock/slugtestpow tags, and verify tagged constructor symbols cannot be imported from an ordinary build. The ordinary binary is tested through its unchanged public entrypoint; no tagged helper is packaged. Exact package acceptance exercises real-clock setup/open/pending, too-late refusal where applicable, view/cancellation and cleanup. Real-clock closed-cutoff package coverage remains explicit until a real eligible Saturday; no fabricated passed claim.

## Appendix D — child dependency surface and method ownership

The normal child constructor is `pilot.NewEventManifestAccount(Config, StableAccountConfig) (*StableController,error)`. It selects explicit v4-aware install readers and old stable/entry behavior, with no active event handle yet. Add `(*StableController).AttachEvents(ctx context.Context, events *gymevent.Store) error`, callable once before the parent publishes the signed-in child. This is a trusted-host dependency attachment, never an IPC action: nil is permitted only for a validated waiting_participants root, otherwise the complete root-derived Pin and Info must match a ready coordinator. A second attachment or changing a live handle is refused. It neither closes nor takes ownership of the shared store. A parent installation of the second fixture can therefore complete its old components, create the shared event store and attach it without inventing a session for the first fixture. Events is unavailable to external calls until final parent attach succeeds.

The parent NewEvents route owns rootv4/install/eventhandle and delegates the Event API after checking exact current child/view. The child owns Event lane tasks/cache and borrows source→stable→entry plus the attached shared event handle. Both parent and child cached-release paths retain final original View authorization. Private dependencies and tagged constructors are implementation-only Go APIs, not browser/client authority imports. Helpers/native/frontend never accept an arbitrary event Pin, snapshot, proof verdict, source handle or clock over their public request grammar.
