# Set47 retained candidate bout helper/native/UI contract — draft

This external engineering draft accompanies the forthcoming Set47 `gymbout` core contract and [scope recommendation](/Users/taytrrip/slug/set47-scope-recommendation.md). It does not authorize runtime edits, expected-output generation, branch changes, packaging or release. Set46 sources and its running acceptance evidence remain immutable. Exact core field tables and formula proof must be bound before joint freeze; the integration choices below are proposed concrete requirements, not new league policy.

## 1. One explicit saved candidate transcript

Continue the existing two-fixture card route in the same Candidate app/root/helper process. An authenticated participant selects the canonical Saturday and an existing booked full BoutID. This is the only run input. The concrete card source determines CardID, opposing ascending-ID corners, class, original owners, immutable noon eligibility/condition, canonical manifests and fixed `Bout.Seed`. Use unchanged `fighterbout` v1, `slug-fighter-bout-candidate-v1`, standard three rounds, explicit seed, cutoff AsOfMS, and exactly zero ProXP/TrainingSkill. No caller pair, manifest, source JSON, seed, clock, bonuses or result is accepted.

One synced result is retained per booked BoutID. Creation time is the actual trusted observation and is distinct from event cutoff. Exact retry returns the first immutable transcript and creation bytes with a separate current source overlay. A source conflict blocks a new transcript; it does not rewrite an existing transcript, choose a different seed, rerun a bout or cancel an event. Later retirement/roster/ownership changes do not replace the noon inputs. The existing C1 profile and all paired/unmatched counts/reasons remain unchanged. A saved candidate win/draw/finish awards no professional or exhibition record, XP, training, title, prize, transaction or official result authority. No new engine, tuning, featured/title mode or calibration exception is introduced.

Use either pinned fixture's captured original held-compatible `gymaccount.View`; this is non-spending work. Do not require/grant SeedPublish, borrow a node/publisher or substitute a fresh session after cancellation. A mining token, wrong account, stale generation, expired/revoked view or detached child cannot authorize work. The core remains responsible for the exact source recipe, signatures already retained in source evidence, clock bounds, simulation, durable insert and replay validation.

## 2. Additive parent v7 and concrete lifetime

Proposed route `accountpilot.NewBouts(context.Context, pilot.Config)` extends the existing Cards route. Append `bout_schema` (uint32 exactly1), `bout_phase` and `bout_store_id`, in that order, to the exact parent v6 marker; set version7. Retain the16KiB marker bound only after a maximum-shape proof. Preserve all prior fields/order and all account, stable, entry, event, seed and card identities/creation times. All six older constructors continue to refuse v7.

The component is `<root>/bouts`, initialized only from the actual ready parent CardStore. Phases are `waiting_source|prepared|ready`. Waiting has an empty store ID and no bouts path; it may coexist with a ready CardStore in the crash window before bout-prepared is synced. Existing controls remain functional while waiting. Derive `gymbout.CandidatePin(cardInfo.Pin)` and ID with source CreatedAtMS equal the concrete card creation time; no live node, finalized event, second live child or before-cutoff restriction is required to initialize this journal. Do not backdate any earlier source component.

Validate the required old root/components and cheap profile before mutation/lock creation, then reread under the existing instance lock. Before v6→v7 publication, validate any required ready event, seed and card components via required-open paths and require the new bouts path absent. No missing ready source is initialized during upgrade validation. Persist prepared before touching the new directory. Use exact core Create/Open/ResumePrepared with the concrete CardStore and expected Pin; prepared recovery accepts only core-recognized absent/real-empty/marker/init/initialized-empty states, with no saved bout records and exact source creation identity. Unknown partials stay retained/refused. Ready always Open; never recreate a missing/partial ready journal. Preserve original created time when prepared recovery finds an exact initialized-empty component. Sync ready only after successful durable initialization. Private checkpoints: `bout-prepared`, `bout-created`, `bout-ready`.

Every v7-only mutation path through earlier stable/entry/event/seed/card installation preserves the new suffix; never serialize v7 with a v6 or older writer. The child unwraps the exact card/seed/event/entry/stable base through a private new-route opt-in. Existing earlier routes/decoders remain exact. Parent owns the shared bout journal across gym switches. Child borrows it once, after AttachCards and before publishing its ready original view; concrete source entry/stable handles are borrowed only through the existing lane/lifetime. Nil attachment is permitted only for exact waiting_source.

Proposed constructors:

```go
// accountpilot
func NewBouts(context.Context, pilot.Config) (*StableController, error)
// pilot
func NewBoutManifestAccount(Config, StableAccountConfig) (*StableController, error)
func (*StableController) AttachBouts(context.Context, *gymbout.Store) error
// slugtestclock files only; exact shared immutable clock, no CLI/env/DTO setter.
func NewBoutsForTestClock(context.Context, pilot.Config, func() time.Time) (*StableController, error)
func NewBoutManifestAccountForTestClock(Config, StableAccountConfig, func() time.Time) (*StableController, error)
```

The new parent constructor receives the original helper signal/lifetime context and derives a25-second startup admission/audit budget, leaving a5-second engineering margin inside the unchanged30-second first native request. Pass that context through every existing API that accepts one, including the fully cancellable gymbout engine audit. Before and after each bounded context-free legacy synchronous open/validator, check the original budget and join/close the call normally; existing gymstable/registry/account opens and the earlier stable-upgrade validator do not acquire a fictitious cancellation API. Never detach such a call behind a timeout goroutine or claim its return is forcibly bounded by the new context. Cancellation may wait for the one already-entered legacy call; no later step starts. Timed-out/cancelled startup closes all acquired handles/locks and publishes no controller. Old constructors and transport deadlines stay unchanged. The256-row capacity gate measures complete new-route startup, including legacy joins/cleanup as applicable, and requires uncontended successful startup<30seconds. An actual failure of that measured bound is a pre-release engineering failure requiring explicit review, not permission to silently widen timeouts.

Logout/reset/expiry invalidate presentation, fence work, cancel/join the actual operation outside account/parent/store locks and detach the child; retain parent journals. Only parent Close closes shared bout→card→seed→event→account after child drainage and releases the root lock. Stop drains active work without revoking the authenticated view; stopped metadata access remains possible through the existing tracked pair lifecycle. Preserve the shared-store caller-cancellation versus lifetime-closure distinction. No orphan timeout goroutine, additional scheduler, duplicate source handle or automatic run loop.

## 3. Closed Bout v1 API and shapes

Exact proposed controller surface, aligned with core draft c72ee57237bdda90c8c1c940d649196180e8d6a8ecf067fc789285e2fad29edc:

```go
const MaxBoutRequestSize = 8192
const MaxBoutResponseSize = 262144 // including LF; existing scanner unchanged
const MaxBoutJobSize = 253952     // 248KiB
const MaxBoutMetadataSize = 73728 // 72KiB, formula proof pending
const MaxBoutPortraitBytes = 65536 // two JSON-encoded SVG strings <=32768 each
const MaxBoutCacheSize = 393216   // 384KiB aggregate, proof pending

type BoutControllerAPI interface {
    CardControllerAPI
    BoutSnapshot() BoutSnapshot
    BoutCardStatus(context.Context, string, string) (gymbout.CardStatus, error)
    BoutStatus(context.Context, string, string, string) (gymbout.Status, error)
    BoutRecord(context.Context, string, string, string) (gymbout.ReadResult, error)
    BeginBoutRun(context.Context, string, string, string) (BoutJob, error)
    BoutJob(context.Context, string, string) (BoutJob, error)
    CancelBoutJob(context.Context, string, string) (BoutJob, error)
}
func ServeBouts(context.Context, BoutControllerAPI, io.Reader, io.Writer) error
func MarshalBoutRequest(BoutRequest) ([]byte, error)
func ParseBoutRequest([]byte) (BoutRequest, error)
func MarshalBoutResponse(BoutResponse) ([]byte, error)
func ParseBoutResponse([]byte) (BoutResponse, error)
```

Arguments after context are original ViewID, then Saturday and booked BoutID; Job/Cancel take ViewID+JobID. CardStatus has one common authorized card/source overlay and at most five saved/not-run metadata rows, no transcript. Existing CardHistory supplies event navigation; no new global transcript history or five-result response is added. Core's256 saved-bout storage cap is an explicit local capacity, no eviction or new league ownership rule.

The following fields are all required with snake_case JSON tags, in listed canonical order. No omitempty or generic maps. Unused strings are empty, unused numbers0, inactive pointers null. IDs/digests lowercase64-hex, request IDs1..2^53−1, timestamps bounded as existing core.

```go
type BoutRequest struct {
    ID uint64
    Command string
    ViewID string
    Saturday string
    BoutID string
    JobID string
}
type BoutResponse struct {
    ID uint64
    OK bool
    View *ManifestViewPin
    StoreID string
    Snapshot *BoutSnapshot
    CardStatus *gymbout.CardStatus
    Status *gymbout.Status
    Record *gymbout.ReadResult
    Job *BoutJob
    Error string
}
type BoutSnapshot struct {
    Version uint32
    State string
    Error string
    View *ManifestViewPin
    StoreID string
    Pin *gymbout.Pin
    SourceCreatedAtMS int64
    CreatedAtMS int64
    ActiveJobID string
    QueuedJobID string
    LastJobID string
}
type BoutJob struct {
    Version uint32
    JobID string
    Kind string
    State string
    View ManifestViewPin
    StoreID string
    CardID string
    Saturday string
    BoutID string
    Error string
    Result *gymbout.RunResult
}
```

Commands/used fields: bout_snapshot uses no view/date/bout/job; bout_card_status uses ViewID+Saturday; bout_status/bout_record/bout_run use ViewID+Saturday+BoutID; bout_job/bout_cancel use ViewID+JobID. Every other field is its required zero value. Unknown commands remain invalid; old families route through their own unchanged parsers/error unions.

Snapshot Version1 states: ready, busy, setup_incomplete, unavailable, failed. Ready/busy have exact current View+derived full Pin/StoreID and both creation times; source creation<=journal creation. Busy reflects any foreign/global lane occupancy but IDs identify Bout work only. At most one active/queued Bout ID is nonempty, and it requires busy; active/queued/last cannot alias. Ready may retain last only. Setup-incomplete retains current View, null Pin, empty store/job IDs, zero times and bout_setup_incomplete. Unavailable has null View/Pin, all IDs empty, times0 and bout_unavailable. Failed retains previously validated current authority with only bout_unavailable/bout_corrupt/bout_clock/bout_source_changed, no active/queued IDs; without current view it becomes unavailable. No chain freshness or publisher availability field is invented.

Job Kind is run; states queued/running/completed/failed/cancelled. Accepted Run ACK is exactly the queued copy. Queued/running have no result/error; completed carries one RunResult and empty error; failed/cancelled have null result and fixed error. Cancelled uses bout_cancelled. Durable uncertainty is failed/outcome_unknown, never cancelled. Terminal canonical bytes, including completion-time overlay/observation, are immutable. Known running→queued is refused; polls may skip running. Accepted Begin discards prior terminal; a rejected/unpublished Begin preserves it. Snapshot-discovered IDs may establish an unknown tuple only after exact current-view Job correlation.

Native adds LaunchBouts and StableBridge.BoutRequest on the same pipe. Exact Gym methods return the complete typed response: BoutSnapshot(), BoutCardStatus(viewID,saturday), BoutStatus(viewID,saturday,boutID), BoutRecord(viewID,saturday,boutID), BeginBoutRun(viewID,saturday,boutID), BoutJob(viewID,jobID), CancelBoutJob(viewID,jobID). Add candidate-bouts requiring the entire manifests/stable/entries/events/seeds/cards chain; Candidate0.4.7/build47 selects it. Preserve existing old defaults/modes, detailed app icon, letterless menubar, bundle identity and lifecycle controls.

## 4. Authority, bounded parsing and immutable evidence

Bootstrap Manifest→Stable→Entry→Event→Seed→Card using private serialization-gate-held requests, then derive full expected BoutPin/StoreID from the established CardPin. SourceCreatedAtMS must equal that CardSnapshot's immutable creation time. Setup/unavailable follows the established upstream state; a reply never blesses a new arbitrary pin. Pin/creation identity remains fixed over the helper lifetime, including later fixture logins. Every nonblank response carries the current original full View and BoutStoreID. Historical source owners and author observations are separate from the current read authorization.

Request IDs strictly increase through prerequisite/final requests; recheck cancellation before final dispatch. Correlate date, CardID, booked BoutID, current authority/view, selected InputID/result/record digests and command family. Retain at most three compact active/queued/last tuples `{job_id,kind,state,card_id,saturday,bout_id,terminal_digest}` referencing the one authority. InputID/ResultDigest/RecordDigest are learned only from the first fully shape/digest/correlation-checked completed Job or Read for that exact tuple; later replies cannot replace them. No Run prefetch invents an input or result before core source preparation. Any late response must pass full view plus selection epoch; an old terminal completion overlay never overwrites a newer explicit conflict/unavailable/source-changed observation. View loss/helper failure clears all presentation/correlation and portrait tuples, never the durable journal.

Response cap includes LF; unchanged256KiB scanner. Decode bounded ASCII JSON with duplicate decoded-key rejection, exact required/unknown fields, explicit nullable forms, exact fixed-array lengths and preallocation checks. Integers reject floats/exponents/negative-zero/Boolean coercion. Canonical hashes use fixed-order typed encoding after permitted whitespace/key-order/escaped-name parsing. Parsers never demand raw embedded subtree bytes are canonical. Root object depth1; provisional maximum16 must be proven after exact source nesting. The exact allowed paths are in Appendix A below; no suffix-based height exception or generic arbitrary array is allowed.

Private transport/native checks verify bounded source/input/result shape, identities, core recipe bindings and canonical digests, plus cheap engine consistency (score/final-state and finish/tie/winner relationships). Do not call public engine/core marshal/validate/digest APIs that re-simulate, regenerate source conditions/manifests/names, verify source proofs or render portraits. The unchanged preview pure shape/hash checker may be factored internally only with old behavior/tests preserved; no public “verified” bypass. Core performs its full deterministic engine audit once per saved row on Open and derives/simulates an explicit new Run. Transcript-local retained-row digest/identity checks serve reads/replay without engine execution; the separate concrete card/source validation cost is described below. Input/Result occur once each; no duplicate full CardSource or second transcript in any response.

## 5. Lane, cache and uncertainty

Use the same explicit-family active1/queued1 lane; add Bout to all dispatch, expiry/cancel, family Snapshot and retained-result paths. At most one Bout job active or queued. Run's30-second timeout includes queue time. The new-run engine/derivation executes in core outside source/account commit locks; helper publication uses only its private bounded shape/canonical-digest checks, never public gymbout.ValidateRunResult/MarshalRunResult/RecordDigest or fighterbout.ValidateResult/MarshalResult. This remains true on replayed Run, so copying the prior Card full-result validation call is forbidden; final original View/current immutable source fence and synced insert follow core order. A new relevant source conflict before commit prevents a new result. No implicit Run on login, navigation, polling, retry, restart or recovery.

Before accepting Begin reserve the complete bounded new-family budget; queue unpublished, then final same-original-view ACK fence. On definite failed publication unlink/cancel the exact task and restore prior cache/reservation. Once dispatched/possibly written, cancellation, consumer validation/size failure, lost ACK or final-view failure is outcome_unknown with no success payload. Reconcile by explicit saved Record/Status under a fresh authorized view, never by blindly resimulating. A known existing record Run replay does not simulate and never replaces its first bytes. Ordinary refusal/busy/cancel is not local corruption; corruption/poison/closed owned dependencies stop the existing mining dependency and retain evidence.

Concrete source work is not falsely free: the unchanged Set46 card read/audit path validates the selected card and seed/event evidence and derives its Plan outside account leases, which can regenerate bounded source manifests, names and cutoff conditions. A new WithBoutView preserves that selected-card semantic audit once per invocation, then privately copies and canonical-hashes at most5 compact refs from the already validated owned card. It must not call public ValidateBoutSource/BoutSourceDigest separately for every ref or regenerate engine Input just to build the metadata list. No new persistent or caller-supplied trusted-source index bypass is introduced. Source validation never runs the engine, verifies Argon work, renders SVG, signs or performs RPC; its measured cost is included in the explicit250ms gate. The no-regeneration/no-source-work claim is absolute for cached Snapshot/Job and private transport/native parsing, and for transcript-local index/row handling; it is not an absolute claim about the existing semantic source audits reached by explicit reads.

Explicit CardStatus/Status/Record use the same cancellable250ms metadata lane and original View; a foreign active lane yields busy. They may persist source/local clock/incident observations exactly as core specifies, so post-dispatch read uncertainty means uncertain observation, not a fight/award/transaction. Cached Snapshot/Job only copy bounded immutable retained objects under fresh original authorization and final child/view check: no core call, clock advance, condition generation, proof, SVG or simulation. Source closure and caller cancellation remain separate.

Provisional per-family384KiB partition: one canonical Job/RunResult or selected ReadResult <=248KiB, all metadata<=72KiB, at most two SVGs whose individual retained JSON-encoded string contribution is<=32KiB each (raw SVG also<=32KiB). Metadata includes one owned Pin/current View/Snapshot, one selected Status, one per-card<=5 status page, at most3compact tuples, two portrait bindings and a combined4096-byte JSON-encoded render/identity contribution. No duplicate deep-owned Pin; shared immutable references are okay, actual copies must be charged. Helper retains only one terminal result; explicit reads are transient. Native/frontend share the one selected transcript or release/replace it, not independent copies for job/read/replay. Any unneeded SVG/selection is released with replacement. Exact maximum formula may reduce these numbers before freeze, never waive the existing1MiB aggregate.

Charge boutBytes reciprocally across every old-family admission, protected/recent stable page, lazy portrait/export/receipt pin, callback and frontend retained object. Zero charge when inactive. Do not silently evict unrelated protected evidence or enlarge any old family cap. Capacity refusal occurs before acceptance and preserves previous accepted objects. A successful replacement charges its actual bytes and fixed metadata allowance, releasing unused reservation. Tests cover combined maximums, all old-family lane orderings and transactional rejection.

## 6. Native-only replay portraits and current authorization

A retained transcript embeds the two canonical input manifests; its compact source bindings retain each original Assignment's manifest/portrait digest. Reuse the existing deterministic renderer locally, outside account locks, for the latest checked selected saved transcript only. Proposed method `Gym.BoutPortrait(viewID,saturday,boutID,prospectID)` has no wire command and accepts no manifest, SVG, path or arbitrary metadata.

A successful BoutRecord or completed BoutJob establishes the selected tuple containing current full View, BoutStoreID/CardID/BoutID/date, immutable RecordDigest/InputID/ResultDigest, the two owned manifests and expected digests. No prior tuple means bout_not_found. Render at most two SVGs, each with both raw and JSON-encoded retained string bytes<=32768, verify the retained expected digest, then freshly call existing bounded BoutStatus under the same original view and require the exact existing record/identities/digests immediately before every SVG disclosure, including cache hits. A current conflict may accompany a clearly historical saved replay; it must not be hidden or turn into a new run. Expired/revoked view, missing/different result, helper failure or source identity drift refuses disclosure and invalidates the tuple.

Use one native serialization-gate-held path without recursive request locking. Clear/release the tuple on accepted new Run, selected-record replacement, view loss, expiry or helper failure. Do not touch the old helper appearance cache/protected page. Portrait metadata and SVGs fit the explicit new-family budget; frontend selection+replay images count against the same global1MiB. The native-only closed reply is separate from the wire union. All listed fields are required, canonical snake_case; inactive pointers are null, unused Error empty. Success has both Portrait and the fresh Status, so a newly observed conflict cannot be ignored by the replay UI. Error has neither payload. Cap the native-only reply48KiB including its encoded SVG, metadata and status; do not widen the wire scanner.

```go
type BoutPortrait struct {
    Version uint32
    View pilot.ManifestViewPin
    StoreID string
    CardID string
    Saturday string
    BoutID string
    ProspectID string
    RecordDigest string
    InputID string
    ResultDigest string
    DescriptorID string
    ManifestDigest string
    PortraitDigest string
    SVG string
}
type BoutPortraitResponse struct {
    Version uint32
    OK bool
    Portrait *BoutPortrait
    Status *gymbout.Status
    Error string
}
```

Portrait Version/response Version are1. Fresh Status.Saved must exist and match exact BoutID, RecordDigest, InputID, ResultDigest and selected source digest; Status.CardID/Event date/view correlation comes from the freshly authorized wire response. Source identity drift refuses; usable=false solely for a retained current conflict may disclose historical replay and returns that exact overlay. Result/image fields never establish authority. The native-only error set is invalid_request, unauthorized, forbidden, stale_view, busy, outcome_unknown, bout_unavailable, bout_not_found, bout_source_incomplete, bout_source_conflict, bout_source_changed, bout_corrupt, bout_clock, bout_calendar, bout_cancelled, bout_capacity. Preserve each applicable typed fresh-Status error; map bout_setup_incomplete and unknown internal/closed/profile failures to bout_unavailable. Render/digest mismatch maps bout_corrupt. Every error returns no SVG or Status payload. Busy, invalid_request, bout_calendar and a definite pre-dispatch bout_cancelled may preserve an otherwise checked tuple while refusing this disclosure. Clock rollback, uncertain observation, missing/changed source, corrupt/unavailable helper and authorization failures clear the portrait tuple and invalidate current usability; none may reuse a cached image to bypass that failure. Typed conflict refusal without a successful authorized saved Status also returns no image; historical conflict disclosure requires the successful exact saved Status described above. No generic image rendering authority or GUI acceptance claim.

## 7. Existing replay UI and error semantics

Use the existing Saturday card panel and local replay/scorecard controls. Keep total/paired/unmatched counts and reasons visible; an unmatched fighter has no selectable bout and no loss. Per booked bout show not-run/saved candidate transcript from the metadata page. Run is explicit and available only for a currently usable source without a saved transcript; saved opens a paused replay through Record. Held participants can use the same non-spending controls. Unknown completion offers “Check saved result”; no automatic Run. Render engine ticks/rounds, finish/winner/draw and whole-fight score from the retained result only. Local playback is presentation and never core work.

Clearly label candidate saved transcript, unchanged standard engine, original noon conditions/owner attribution and no professional/exhibition record or awards. Separate immutable result from fresh source-conflict overlay and chain observation time; metadata refresh does not claim network re-verification. A conflict keeps historical replay accessible within an authorized current view but blocks a new transcript. Current retirement does not rewrite the farewell bout's cutoff state. Do not infer official finality from the seed checkpoint or imply a result-publication certificate.

Proposed closed errors: invalid_request, unauthorized, forbidden, stale_view, busy, outcome_unknown, bout_unavailable, bout_setup_incomplete, bout_corrupt, bout_capacity, bout_clock, bout_calendar, bout_not_found, bout_source_incomplete, bout_source_conflict, bout_source_changed, bout_cancelled. Exact core errors map only within this new family. Failed Job excludes setup-incomplete; Snapshot failed uses its narrower vocabulary and never outcome_unknown. Core engine arithmetic/stream failures map bout_unavailable; malformed caller/composition DTOs invalid_request and impossible retained bytes bout_corrupt. Never expose raw diagnostics, save a partial draw or infer a recorded loss. Typed definite refusals remain definite. Lost/malformed post-dispatch Run/Cancel or unknown Snapshot-discovered Job is outcome_unknown; explicit metadata loss may be uncertain observation. Known cached poll failure is unavailable, with no assumption a new result was written.

## 8. Acceptance and remaining freeze dependencies

Freeze exact core/helper DTOs, recipe/digests, source callback ownership, parser paths and numeric limits, closed error mapping, root/install bound and all cache/frame formulas before implementation or vectors. Independently derive complete Input/InputID and full Result/digest using frozen Set42 Python plus retained Set46 card evidence; never derive expected values from new Go output. All existing engine/condition/reference bytes remain unchanged.

Test original held View, revoked/expired/mining/foreign refusal, final-source conflict between preflight/commit, exact replay without any engine call, later retirement, post-sync lost ACK/reopen, cancellation before/after possible write, consumer-publication unknown, immutable terminal poll, no source/engine/portrait work on polls, common-time<=5 status rows, full storage-capacity metadata liveness and protected cache coexistence. Parent actual SIGKILL prepared/created/ready preserves both fixtures/all earlier journals; unknown partial and missing-ready refuse. Default binaries lack all test-clock hooks/profile tags; scoped composition has no release clock flag or fake authority.

Use an explicit cold copy of the genuine retained Set46 journals/owned chain after all original owners are closed; preserve the original artifacts byte-for-byte. The existing old card has a latched target conflict and remains the negative new-Run-refusal control. Establish a legitimately later Saturday in the copy through normal original-view entry before its cutoff, immutable capture, explicit isolated publication, fixed target+60descendants/fixture attestation, and a new usable candidate card. Reuse existing genuine admitted fighters and mature operator fixture funds only if their retained eligibility/current UTXO facts permit it; otherwise report the bounded setup failure. Do not clear, roll back or relabel the old conflict, transplant result/source rows, or add a production repair/import API.

After joint freeze, run the explicit final-source bout on the new usable card, save/read/replay through NewBouts/ServeBouts/native pipe, reopen and independently compare complete Input/Result bytes. Preserve at least one saved transcript before injecting its new source conflict, then prove it stays historical/immutable while new runs are refused. Scoped-clock setup/mining remains explicitly test-owned and normal-PoW, never a public chain. Bound the complete isolated acceptance harness20minutes and any newly accepted prospects<=32 per gym; no silent background mining or unbounded reselection. Normal-clock packaged checks remain honest if no closed finalized eligible event exists. No GUI/chooser/portrait/screen-reader or D74 acceptance follows from DOM/API tests; preserve one full end-of-flow D74 review when all major flows are ready.

Pending engineering alignment only: independently proven wrapper/cache/startup bounds, exact source callback review and native portrait reply shape proof. No unresolved general league/result/award policy is silently adopted by this candidate slice.


## Appendix A. Exact core transport nesting and relationship checks

This appendix uses the concrete corec72ee572 field tables. Let R be `record.record` or `job.result.record`. Permitted arrays only:

- `snapshot.pin.source_pin.source_pin.source_pin.participants`: exactly2 (Bout→Card→Seed→Event pin).
- `R.source.corners`: exactly2, A then B.
- `R.result.initial`, `R.result.final`, `R.result.scores`: exactly2.
- `R.result.rounds`:1..3; each `rounds[*].ticks`:1..60, total<=180.
- `R.result.rounds[*].ticks[*].states`, `R.result.rounds[*].end`: exactly2.
- Nonnull `R.result.rounds[*].recovery.states` and `corner_stopped`: exactly2.
- `card_status.bouts`:0..5, frozen class order.

No arrays are hidden in Manifest/DNA/Condition/Owner/Assignment/Event/Bout/SavedBout. The two engine corners are named Input.A/B objects. Only engine Round.Recovery is nullable inside Record; outer Snapshot/response/Job nulls follow the closed state grammar. Paths must be checked before Go fixed-array decoding, which otherwise accepts excess elements.

Signed numeric paths only: `R.input.a.manifest.portrait.shoulder_jitter` and B counterpart in[-6,6], and `R.result.initial[*].form_q` in[-100000,100000]. There are no copied seed scan-height fields or other negative sentinels. All other fields follow their exact source/engine bounds. Boolean values are allowed only at the core's explicitly Boolean fields and cannot pass as0/1. Integer-string width and path-specific string caps are frozen core/old-engine limits, never raw suffix heuristics.

Private transport validation binds Record.StoreID/CardID/BoutID to its source, Input to exact booked sorted IDs/seed/cutoff/birth/zero bonuses, InputID and Result.InputID to canonical fixed-order Input, ResultDigest to fixed-order Result and source_digest to compact BoutSource. It verifies fixed engine version/ID/mode/seed_kind, complete transcript scalar/array limits, cheap score/final-state and decision/finish consistency, and only available structural source facts. It never calls DeriveInput/Simulate/ValidateResult/public RecordDigest or source condition/manifest/Plan generation. Canonical source/record digests use their existing LP domains exactly; engine Input/Result domains have no added LP.

Status.Source.Event, CardID and times must agree with the requested source. Saved row identifies the exact booked Bout, canonical source digest and authoritative existence. Absent Saved has no result/digest/time/finish fields and usable=false; present chronology includes source finality/card creation<=saved creation<=observation. Standalone saved usable=true implies source.usable and no conflict; false does not prove why without saved immutable context. Full Read/Run checks exact RecordDigest/InputID/ResultDigest/source digest/summary/time correspondence and usable equality once exact source identity is available. CardStatus uses one common overlay/time and0..5unique canonical class-ordered booked rows; it contains no current full transcript.

The metadata cache owns only one authoritative Pin/View. Portrait manifests refer to the selected Record.Input objects; they must not be deep-copied without being charged. Releasing a selected transcript must either release portrait material or charge the separately retained two-manifest binding within metadata; never keep a hidden second input. Fresh portrait Status replaces the selected overlay only under current selection epoch and observation ordering, and never rewrites terminal Job bytes.
