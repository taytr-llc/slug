# Set46 candidate card helper contract — review draft

This is a prospective engineering contract paired with [the core draft](/Users/taytrrip/slug/set46-card-core-contract-draft.md). No implementation, expected-output generation, release or GUI work is authorized by this draft. It uses the approved two-fixture candidate scope: zero professional records by the explicit candidate basis, same class, inclusive half-to-double rating band, and only the widen-to-five requirement waived. It makes no general matchmaking, production ownership, fight-result or shipping-policy decision.

## 1. Scope and unchanged authority

Use the existing Candidate Pilot root, bundle, page, account signer and serial work lane. Add a closed Card v1 family without changing old Manifest/Stable/Preview/Entry/Event/Seed request/response grammar, error vocabularies, scanner caps or persisted records. The card is an immutable projection of a concrete retained Set45 source with historical finality; its current source-conflict overlay is separate. No card action obtains a publisher, invokes node RPC, signs, mines, selects another seed, runs a bout, changes a roster or increments a professional record/XP. An above-target depth loss or offline node alone does not erase historical finality. Source conflict forbids a new card; an existing card stays inspectable with `usable=false`.

Either pinned fixture uses its captured original held-compatible `gymaccount.View` session. `SeedPublish` is not required or granted. A mining token, revoked/expired session, different fixture/account, stale generation or substituted current login cannot authorize this work. Preserve the existing schema1 fixture account/recovery lifecycle; no provider/vault migration or transfer authority is inferred.

Core owns the exact source, rules, eligibility, display name, reason priority, counts, Plan, identifiers and seed recipes. Every source member is retained in the card, including an ineligible entrant. The UI shows total/eligible/paired/unmatched fighter counts, global bout count, both per-gym counts, and each fighter's result/reason. Zero records mean `candidate_zero_record_no_results_authority`, not an inference from XP0. DisplayName is the existing deterministic name derived once by core. Card/bout seeds are future-use identifiers; this two-fixture profile has at most one opposing entrant per class and performs no random opponent draw. No claim of a general record authority, official booking or unbiased public randomness is added.

## 2. Parent v6 installation and lifetime

`accountpilot.NewCards(pilot.Config)` opens the existing root in a new explicit route. Append, in order, `card_schema` (uint32, exactly1), `card_phase` (string) and `card_store_id` (string) to the exact v5 marker. Set parent `version=6`; preserve every prior field, field order, stable/entry install row, EventStore and SeedStore identity. Keep the existing16KiB parent-marker limit, subject to the pre-freeze formula proof. New routes recognize validated v1–v5 upgrade paths and existing v6; all five older constructors refuse v6. Do not teach old decoders to ignore new fields.

Phases are `waiting_source`, `prepared`, `ready`. Waiting has empty CardStoreID and no `<root>/cards` component; it means card initialization has not acquired its ready source yet. It may coexist with a ready SeedStore in the recoverable crash window after seed-ready is synced and before card-prepared is synced. Ordinary mining/entry/event/seed controls remain usable while card setup waits. Once the actual SeedStore is ready, obtain bounded parent-only Info and derive `gymcard.CandidatePin(seedInfo.Pin)`, then `gymcard.ID`; require exact source StoreID and immutable source CreatedAtMS. The ensureCards step persists prepared before any card-directory mutation, including when resuming that seed-ready/card-waiting window. This can happen before login without a node or second live child. Source history need not yet contain a finalized event. Card initialization has **no created-before-cutoff rule**: core records actual installation time and the established seed creation time. Existing event/seed restrictions are unchanged.

Validate the exact existing root/required managed components and cheap-PoW profile before mutation or instance-lock creation, then reread under the retained instance lock. Follow the established protected-ancestor, real-directory, no-link and opened-file identity rules. Before publishing a v5→v6 upgrade, validate any existing ready EventStore and SeedStore through their required-open path and require the new cards path absent; do not initialize a missing source as a side effect of validating the old root. Persist parent prepared intent before constructing the child journal. Use only core Create/Open/ResumePrepared at `<root>/cards` with the concrete parent SeedStore and exact expected Pin. Prepared may resume only the core's recognized absent/real-empty/marker/init/initialized-empty states, with event_count0 and unchanged source creation identity; unknown partial state remains retained/refused. Ready uses Open only, never recreates a missing/partial journal. Preserve exact CreatedAtMS when resuming an initialized-empty store. Sync parent ready after successful child initialization. Private installer checkpoints are `card-prepared`, `card-created`, `card-ready`.

All new-route v6 writers, including stable/entry second-fixture completion and event/seed readiness transitions, preserve the appended card fields and every prior field. In particular installSeedComponents/ensureSeedEvents/ensureSeeds gain v6-only reader/writer paths; never serialize a v6 root through writeSeedMarker. The new child validates the card root and its exact v5 seed/v4 event/v3 entry/stable base fields through a private cardEnabled opt-in, preserving older route version refusals. A second fixture cannot drop an already established shared journal. Existing v1–v5 update code remains behaviorally unchanged outside the new route. Do not silently promote unknown earlier initializing state or relax the existing recognized READY recovery rules.

Exact constructors/attachment:

```go
// accountpilot
func NewCards(pilot.Config) (*StableController, error)
// pilot
func NewCardManifestAccount(Config, StableAccountConfig) (*StableController, error)
func (s *StableController) AttachCards(context.Context, *gymcard.Store) error
// Only in files guarded by slugtestclock; no CLI/env/DTO clock entry point.
// accountpilot
func NewCardsForTestClock(pilot.Config, func() time.Time) (*StableController, error)
// pilot
func NewCardManifestAccountForTestClock(Config, StableAccountConfig, func() time.Time) (*StableController, error)
```

AttachCards occurs exactly once after AttachEvents/AttachSeeds and before the parent publishes the ready original view. Nil is valid only for exact waiting_source. Otherwise compare concrete Info, full derived Pin, StoreID, source creation time and actual card creation time to the validated v6 installation. No mutable setter, fetch-current-token closure or public authorization Boolean is introduced. Tagged private immutable factories supply the same owned clock to account/fixture-authentication and all relevant stores, including reopen; normal constructors continue to use time.Now. A nil clock fails, and slugtestpow fails before filesystem effects even when combined with slugtestclock.

The parent owns the shared card journal across fixture switches. The child borrows it, never closes it, and borrows concrete entry/stable handles only while the existing lane owns them; no source handle is retained across Start/Stop replacement. Keep the Set45 shared-source closure-versus-caller-cancellation fix. On logout/reset/expiry/Close: invalidate current view, reject/fence new work, cancel and join queued/active work outside parent/account/store locks, and drain/detach the child. Logout/reset/expiry retain the parent journals for a later fixture/session; **only parent Close** then closes shared card→seed→event→account stores and parent lock. Normal Stop cancels/joins lane work and preserves authenticated cached evidence as existing families do; the stopped entry/stable pair remains available or is reestablished through the existing tracked lifecycle before bounded explicit reads. No lazy orphan goroutine or second scheduler.

## 3. Exact helper and native API

```go
const MaxCardRequestSize = 8192
const MaxCardResponseSize = 262144
const MaxCardCacheSize = 294912 // 288KiB
const MaxCardJobSize = 245760   // 240KiB
const MaxCardMetadataSize = 49152 // 48KiB

type CardControllerAPI interface {
    SeedControllerAPI
    CardSnapshot() CardSnapshot
    CardStatus(context.Context, string, string) (gymcard.Status, error)
    CardRecord(context.Context, string, string) (gymcard.ReadResult, error)
    CardHistory(context.Context, string, string, uint32) (gymcard.HistoryPage, error)
    BeginCardBuild(context.Context, string, string) (CardJob, error)
    CardJob(context.Context, string, string) (CardJob, error)
    CancelCardJob(context.Context, string, string) (CardJob, error)
}
func ServeCards(context.Context, CardControllerAPI, io.Reader, io.Writer) error
func MarshalCardRequest(CardRequest) ([]byte, error)
func ParseCardRequest([]byte) (CardRequest, error)
func MarshalCardResponse(CardResponse) ([]byte, error)
func ParseCardResponse([]byte) (CardResponse, error)
```

After context, arguments are original ViewID then canonical Saturday or JobID. History uses original ViewID, exclusive CardID cursor, limit. All fields below are required and JSON tags are the displayed snake_case names in listed canonical order. Inactive pointers encode null, unused strings empty, unused numbers0. IDs/digests are lowercase64-hex; timestamps and integer precision follow core. There are no map/generic payloads.

```go
type CardRequest struct {
    ID uint64              // id, 1..2^53-1
    Command string         // command
    ViewID string          // view_id
    Saturday string        // saturday
    AfterCardID string     // after_card_id
    Limit uint32           // limit
    JobID string           // job_id
}
type CardResponse struct {
    ID uint64                     // id
    OK bool                       // ok
    View *ManifestViewPin         // view
    StoreID string                // store_id
    Snapshot *CardSnapshot        // snapshot
    Status *gymcard.Status        // status
    Record *gymcard.ReadResult    // record
    History *gymcard.HistoryPage  // history
    Job *CardJob                  // job
    Error string                  // error
}
type CardSnapshot struct {
    Version uint32            // version,1
    State string              // state
    Error string              // error
    View *ManifestViewPin     // view
    StoreID string            // store_id
    Pin *gymcard.Pin          // pin
    SourceCreatedAtMS int64   // source_created_at_ms
    CreatedAtMS int64         // created_at_ms
    ActiveJobID string       // active_job_id
    QueuedJobID string       // queued_job_id
    LastJobID string         // last_job_id
}
type CardJob struct {
    Version uint32               // version,1
    JobID string                 // job_id
    Kind string                  // kind
    State string                 // state
    View ManifestViewPin         // view
    StoreID string               // store_id
    CardID string                // card_id
    Saturday string              // saturday
    Error string                 // error
    Result *gymcard.BuildResult  // result
}
```

| Command | Nonzero fields beyond ID/Command | Sole success payload |
| --- | --- | --- |
| card_snapshot | none | snapshot |
| card_status, card_record | view_id, saturday | status, record respectively |
| card_history | view_id, limit1..8; after_card_id empty or fullID | history |
| card_build | view_id, saturday | job |
| card_job, card_cancel | view_id, job_id | job |

All unused request fields are exactly empty/zero. Success has empty Error and exactly one payload. All nonblank success envelopes carry the current original full View and established CardStoreID; payload identities must match it. The sole null-view success is unavailable Snapshot. Setup-incomplete Snapshot has current View but empty outer StoreID. Failure has OK=false, one closed error code, all payload/View pointers null and empty StoreID. Never serialize internal paths or raw error strings.

Snapshot.State is `ready|busy|setup_incomplete|unavailable|failed`. Ready/busy contain full View/Pin/StoreID, positive SourceCreatedAtMS and CreatedAtMS with CreatedAtMS>=SourceCreatedAtMS, and empty Error. Busy means any family occupies the lane; active/queued/last IDs name only Card jobs. At most one of active_job_id/queued_job_id is nonempty and a nonempty value requires busy; all nonempty active/queued/last IDs are distinct. Ready has no active/queued IDs but may retain last. Setup-incomplete has current View, no Pin, all IDs empty, both times0, Error=`card_setup_incomplete`. Unavailable has Version1, State unavailable, Error=`card_unavailable`, null View/Pin, all IDs empty, both times0. Failed retains a previously validated full current view/authority/times and uses only card_unavailable/card_corrupt/card_clock/card_source_changed; it has no active/queued IDs, and without current view it instead returns unavailable. No node-state field or chain freshness promise is added.

Job.Kind is exactly `build`; state `queued|running|completed|failed|cancelled`. A successful card_build ACK is exactly the accepted queued Job copy, even if work advances before transport serialization. Queued/running have empty Error and nil Result. Completed has empty Error and owned BuildResult with exact CardID/date/store, including its completion-time Status and Replayed flag. Failed/cancelled have nil Result and fixed error; cancelled uses card_cancelled. Known durable uncertainty is failed/outcome_unknown, never cancelled. A queued cancelled unpublished job creates no card. Known running→queued transitions are refused; polls may skip intermediate running state. Terminal bytes, including nested Status and its observation time, are immutable on all later polls.

Native bridge adds `LaunchCards(helper, dataDir, node string) (*StableBridge,error)` and `(*StableBridge).CardRequest(context.Context, pilot.CardRequest) pilot.CardResponse`, reusing the one existing helper pipe. Exact Gym methods return complete pilot.CardResponse:

```go
CardSnapshot()
CardStatus(viewID, saturday string)
CardRecord(viewID, saturday string)
CardHistory(viewID, afterCardID string, limit uint32)
BeginCardBuild(viewID, saturday string)
CardJob(viewID, jobID string)
CancelCardJob(viewID, jobID string)
```

No method accepts a seed, pin, receipt, name, professional record, plan, source JSON, key, endpoint, clock or export path. Add helper/shell `--candidate-cards`, requiring all candidate-manifests/stable/entries/events/seeds flags. Candidate0.4.6/build46 defaults to this complete flag chain; old build modes/defaults remain unchanged. Preserve approved original detailed app icon, letterless template menubar, bundle identity/title and existing Show/Start/Stop/Quit behavior. Existing Gym.Mode remains its exact manifest mode; successful CardSnapshot establishes additive card availability. The new family is not an excuse to normalize all previous frontends into a common wire union.

## 4. Closed transport and current authority

Keep request8192/response262144 caps including LF; do not widen the existing scanner. Accept permitted JSON whitespace/object order/decoded escaped key spelling, then fixed-order typed re-encode. Reject decoded-key duplicates, unknown/missing fields, null arrays, surplus fixed-array elements, floats/exponents/negative-zero, Boolean-as-number, malformed IDs/strings and allocation-depth/length overflow before materialization. Field-name length<=64, ASCII bounded text, container depth<=16 with root object depth1. All array orders remain semantic; malformed ordering is refused, not sorted by parsers.

Exact allowed array paths, where R is `record.record` or `job.result.record`:

- `snapshot.pin.source_pin.source_pin.participants`: exactly2.
- `R.source.source_receipt.input.participants`: exactly2. Under each participant: `roster.slots` and `roster.assigned_at_ms` exactly10; `members`0..5; nonnull `entry.latest_receipt.members`0..5.
- `R.source.source_receipt.captures`: exactly2.
- `R.source.anchor.merkle_branch`:0..20.
- `R.plan.gyms`: exactly2; `R.plan.entrants`:0..10; `R.plan.bouts`:0..5.
- `history.cards`:0..8.

No other arrays. Only the frozen nested `entry.latest_receipt` nullable form is permitted within a complete source receipt; new core fields are all values. Inactive top-level payloads/Snapshot authority/Job.Result follow the exact state grammar above. Let S be `status`, `record.status`, `job.result.status`, or `history.cards[*]`: only `S.source.status.scan_height` and `S.source.status.scan_tip_height` permit the inherited integer-1. Every other numeric path is nonnegative with its precise core bound; no suffix-based generic signed-number exception.

Standalone Parse/MarshalCardResponse use private bounded transport validation and canonical hashing, not full core source/Plan validators. They check exact shape/order, local identifiers, state/null grammar, count arithmetic, paired references, source and result relationships available in the payload, and safe scalar limits. They do not regenerate condition, name, manifest, SVG, Plan, record authority, signatures, chain proofs or engine results. Full core semantics run once in explicit preparation/read/Open outside the final account lease. Never call a public core marshal/digest function from the poll path if it transitively invokes ValidateRecord/DerivePlan/GenerateText/Project. Private fixed-order encoding/hash of an already shape-checked trusted payload is permissible; it is not an exported bypass asserting arbitrary source truth. Static dependency regression and self-rehashed negative fixtures cover this boundary.

Native bootstrap first establishes Manifest→Stable→Entry→Event→Seed authority through the existing owned pipe and private gate-held requests. If that chain is signed out or setup-incomplete, accept only the matching unavailable/current-view setup-incomplete CardSnapshot; no arbitrary Pin is discovered from it. Current-view Card setup-incomplete also remains valid while the ready seed is awaiting card installation. A nonblank card authority instead requires an established complete Seed Pin: derive expected full CardPin using `gymcard.CandidatePin` and expected StoreID using ID, then require exact CardSnapshot Pin/StoreID and SourceCreatedAtMS equal that SeedSnapshot's immutable CreatedAtMS. Require CardCreatedAt>=source creation and retain both through this helper lifetime. A job/read response cannot establish/replace authority. Repeated snapshots and later logins to the same helper must not silently change established Pin/creation identity. Nested source receipt gyms/accounts must exactly match those pins; outer View must match the active original fixture, not historical receipt authors.

Use the native serialization gate without recursive request acquisition. Increment request IDs for all prerequisite reads and final dispatch; recheck ctx immediately before the latter. Every response matches the expected command family, request ID and requested current full View, CardID/date/cursor/limit as applicable. Track at most active/queued/last job tuples `{job_id,kind,state,card_id,saturday,terminal_digest}` referencing one current authority/view; unknown jobs are learned only from an exact same-view CardSnapshot-discovered ID. A delayed Job/Cancel must match that tuple. Completed polls must retain the identical terminal digest. View loss, helper failure, expiry or changed child clears all current presentation/correlation, never the durable store.

CardStatus and CardRecord provide fresh explicit overlays; CardHistory overlays are bound to their current view and page date identities. Treat a cached Job's Status as historical completion evidence, not a newer source observation. The UI/native selection state uses full View+CardStoreID+CardID+Saturday and response-ID/selection epoch to reject late results; a late terminal poll cannot replace a newer explicit overlay or hide its conflict. A newer authorized source_changed/unavailable/metadata-unknown error invalidates the current usability claim while an already authorized historical card may remain clearly historical in that same view. On view loss no prior-view card remains displayed. Do not derive current usability from local wall clock or mutate an immutable Record to display a conflict.

## 5. Work lane, cache and durable outcomes

Use the existing global active1/queued1 lane. A task belongs to exactly one explicit family; update family selection, queued expiry, cancellation, Snapshot IDs, final result ownership and all old predicates so a Card pointer cannot be treated as Stable/Preview/Entry/Event/Seed work. At most one Card job may be active or queued. Build has a30-second queue-inclusive derived context; no new scheduler, recursive source lock, automatic card loop or unjoined timer worker. The normal mining cycle may run the foreground task only through this lane. Navigation/login/restart/status polling never auto-Build or auto-reconcile/attest a seed.

Begin validates shape/current view, reserves capacity and queues an unpublished exact task. Only a successful final same-original-view publication fence accepts it, discards the prior terminal and exposes its new ID. Failed ACK publication must cancel/unlink that exact unpublished task and restore prior result/reservation; no possibly accepted task may later execute after a definite busy/stale refusal. At an actual dispatched/started mutation boundary, uncertainty takes precedence over a clean rejection. Cancel/Stop/logout/reset drains the actual operation and borrowed handles, outside leases/mutexes.

Reserve MaxCardCacheSize288KiB before accepted Build ACK: up to240KiB for one canonical retained Job/BuildResult or selected ReadResult representation (containing one full Record), and48KiB for all other card data. Native/frontend may share immutable Record bytes between last completed job and selected same-card read, but must not retain two full independent results; a different selected record replaces the prior result presentation. Helper retains its one terminal job; explicit read responses are transient and do not create a second helper record cache. Native job correlation retains its digest after releasing presentation bytes, not another transcript. Do not count transient wire buffers as permission for retained duplicates.

The metadata partition includes one owned authoritative CardSnapshot/Pin/current View, one current selected Status, at most one selected History page of8 statuses, at most3 compact job tuples, and fixed render/identity strings whose **combined retained JSON-encoded contribution**, including escaping/quotes/keys if used, is capped4096 bytes. This is not an allowance for4096 arbitrary raw characters plus uncharged escaping. References to the same immutable authority are allowed; any actual duplicate must be charged. History has no implicit background cache in helper/native; frontend retention is charged by canonical bytes. Both metadata and result partitions use actual serialized-byte checks; limit violations return busy/capacity before accepting a new job and release replaced objects. Prove maximum wrapper, Pin, result, metadata and16KiB root shape before freeze.

Charge cardBytes reciprocally at **every** old-family cache admission, stable protected/recent page update, accepted callback, lazy portrait, receipt/export pin and native/frontend retained-object path. Keep the aggregate1MiB; do not evict an unrelated protected page or terminal record silently, enlarge old caps or assume all full-family reservations fit simultaneously. When card inactive its charge is0 and old routes remain unchanged. Concurrent rejected Begin/read cache admission preserves the previous accepted object. Once work finishes, charge its actual retained bounded bytes and reserved metadata, releasing unused reservation as existing families permit.

Explicit CardStatus/Record/History acquire the same tracked lane with a250ms cancellable context and original View. A foreign active operation yields busy; stopped state must still permit bounded source reads. Core History uses its Open-audited compact index and validates only the selected<=8 records plus their source states, not all256 full records per request. They may persist authenticated local/source clock and incident observations as core specifies; they never Build, sign or send transactions. Repeated CardSnapshot/CardJob only copy cached objects under the same original View lease and final parent-child/view recheck; no core Info/History/source call, clock advance or full semantic validation there.

New Build delegates to core with concrete parent card/seed/event plus lane-owned entry and captured original Session. Heavy pure derivation/encoding occurs outside account/source mutexes; the final source callback and card sync obey core lock order. Any failure after possible/confirmed durable Build, including public validation/size, final View failure, cancellation or lost ACK, reports outcome_unknown and reconciles by deterministic CardID/date via explicit Read/Status or reopen. No automatic repeated Build follows unknown. Existing Build replay is exact core replay, with new separate Status but unchanged Record/CreatedAt; it remains available after a retained conflict. An observation-only read uncertainty must be described as uncertain local evidence observation, not a card booking or transaction dispatch.

Fatal local corruption/poison/closed dependency stops the lane's owned mining dependency, preserves durable evidence, and uses sanitized errors; ordinary conflict/source-incomplete/busy/cancelled states do not manufacture a fatal node failure. Preserve the corrected distinction between dependency closure and caller cancellation.

## 6. Exact errors and UI scope

The new family vocabulary is `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `outcome_unknown`, `card_unavailable`, `card_setup_incomplete`, `card_corrupt`, `card_capacity`, `card_clock`, `card_calendar`, `card_not_found`, `card_source_incomplete`, `card_source_conflict`, `card_source_changed`, `card_cancelled`. No old family code is added or leaked. Core source-incomplete/conflict map to the named Card equivalents; unknown internal/profile/closed errors map card_unavailable, corrupt to card_corrupt. Poison maps card_unavailable unless this invocation is already outcome_unknown. Expired/unauthenticated original lease maps unauthorized, wrong participant/capability forbidden, changed view/child stale_view. Ordinary250ms read deadline maps busy and explicit cancellation card_cancelled; any possible durable write uncertainty has priority.

Failure Job can use the vocabulary except card_setup_incomplete; Snapshot failed has its narrower list above and never outcome_unknown. Generic post-dispatch malformed/lost helper response to Build/Cancel/unknown-discovered-job is outcome_unknown. A typed definite preflight refusal is preserved. Source observation uncertainty from reads is also outcome_unknown but its command supplies the non-transaction context. Do not infer success or safe rerun from an empty status/history.

The existing page gains one candidate-card section using the exact new methods after CardSnapshot establishes mode. Show a pending/incomplete seed as an unavailable Build source rather than an empty card; show empty complete card as legitimate zero counts. Names/full IDs and class are available without portrait rendering or a general image endpoint. The panel exposes reason strings through fixed explanatory copy, total/paired/unmatched and both gym summaries; no hidden filter removes ineligible/unmatched rows. Show the pinned source/noon time, historical seed evidence and current conflict/usable overlay distinctly. Explicit Refresh is a bounded metadata read, not a seed network refresh. No fight/action that awards results is exposed. D65 remains unchanged; D76 noon data and D77 fixed seed/deadline meaning remain unchanged. General widen-to-five/record-filter/bye-priority policy is still unresolved outside this two-fixture candidate profile.

## 7. Required evidence before release

Use frozen independent core and helper/native/UI vectors; do not generate expectations from implementation. Verify exact array/depth/byte/number/null grammar, both fixtures, empty/status/error shapes, every reason/count partition, late current-view/date/cursor results, canonical hashes, immutable terminal polls and historical Record plus later conflict overlays. Add normal/race/cheap refusal plus tagged clock composition without changing production profile. Prove the polling dependency boundary excludes source validators/condition/name/Plan/portrait/signature/RPC work.

Exercise active Card/queued foreign-family and the inverse for all existing lane families, single-card backpressure, cancellation while queued/running/final callback, ACK publication failure preserving prior terminal, reciprocal cache thresholds and native pipe cancellation after prefetch. Include a populated256-row core journal with first/late/exhausted History pages and a helper late-page read under the unchanged250ms deadline; prove only selected<=8 rows get full validation and preserve honest busy results under contention. Held participant can Build/read; revoked/expired/foreign/mining-token sessions cannot. Test durable unknown before/after consumer sync and post-success public-validation failure; deterministic date/ID read/reopen must recover, never silently rematch. Old wire routes/limits and old root-v5 behavior remain exact.

Actual parent SIGKILL checkpoints prepared/created/ready must preserve both fixtures and seed/event histories; cover recognized empty/init recovery, unknown partial refusal, missing-ready no recreation, source identity/creation mismatch, old routes refusing v6 and second-fixture readiness. Normal build compile probes and binary metadata/symbol checks exclude slugtestclock/slugtestpow and public test constructor symbols.

The successful closed-source card path requires a **new genuine normal-PoW isolated scoped-clock pipeline**, not merely a synthetic matcher vector or the prior Set45 pair. The existing Set45 source has heavy versus middle entrants and can prove only two no_opponent_in_class rows. Fresh setup must collect bounded genuine candidates from both gyms until it can save at least one same-class in-band pair; use at most32 accepted candidates per gym and a20-minute overall harness deadline, with ordinary fixture mining explicitly invoked by the test. If the bound yields no pair, preserve a failed/incomplete attempt rather than claim booking coverage. Save an additional unmatched entrant in a different class when available; regardless, pure independent vectors must cover all reasons and the real card must report its exact actual partition.

Capture immutable noon source, explicitly fund/mine only on isolated owned nodes as the established Set45 harness does, retain fixed first anchor/A+1/exact checkpoint/60-descendant evidence, then Build at least one successful bout through actual NewCardsForTestClock→ServeCards/native pipe. Do not modify old evidence or authorize public mining. Test held read/replay, repeated identical Record, process restart, later retained source conflict overlay and unchanged card/seed/entries/roster/XP/wallet state. Core may be genuinely late-installed after cutoff/finality, which must not be confused with backdating the required earlier event/seed stores.

The untagged final bundle separately proves exact defaults/flag matrix, setup incomplete/ready, absent/unfinalized/future/too-late source refusals, held read and restart in actual wall-clock conditions. It makes no successful real-clock closed card claim unless an eligible actual source exists. Keep actual process/listener cleanup and before/after source/binary/public evidence hashes. No GUI/portrait review or D74 acceptance follows from API tests; preserve one full joint D74 review at the end when all major flows are ready.

Joint freeze requires exact core/helper bytes, independent maximum-shape/cache proof and reviews. All draft uncertainties are engineering review matters; this document does not expand the approved candidate player policy.
