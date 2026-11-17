# Set42 draft: trusted pair acquisition and unrecorded fight preview

Status: engineering draft for root and independent review; not a frozen contract or permission to implement. Stage baseline is `bbce6106c95eaa9284db38cdebd50c754c8ec775` in `set42-fight-stage`. The engine equations/DTOs are a separate coordinated draft. No new runtime or expected outputs are created by this document. The engine DTO seam below tracks the current reviewed draft; both documents must be frozen together before code or normative fixtures.

## 1. Existing product and authority boundary

Use the existing Candidate Pilot bundle, current `--candidate-manifests --candidate-stable` activation, LocalManifestPilot account-root v2, stable schema1 and the same retained fixture wallets. Add no demo namespace, migration, provider, signer, admission route, paid action or generic scheduler. Existing Manifest v1 and Stable v1 frames, parsers, APIs, fixtures, job kinds and limits remain byte/behavior compatible.

Visible label: **Fight preview — unrecorded**. Select two distinct full prospect IDs from the currently assigned gym. Candidates may be off-roster or retired; this slice adds no entry/active-career restriction. Both immutable manifest classes must match. Current authenticated `View` sessions, including D72-held sessions, may preview; mining tokens, old sessions/generations, recovery-required or unenrolled accounts may not. No RosterManage capability is needed for a read.

Neither execution nor replay changes an assignment, consumed checkpoint, roster slot/revision, durable operation history, permanent/provisional XP, training, wallet, damage, fight record, ranking or title. Existing clock high-water, `LastAsOfMS` and `ObservedSourceCount` observations may be synchronized. This is therefore an authenticated observation with possible metadata writes, not a filesystem-read-only operation. Uncertain storage still poisons/refuses as set41 requires. Observation changes do not invalidate roster CAS.

Predictable public seeds provide reproducibility only. They do not implement D12 unpredictability, future block acquisition, official fairness, anti-grinding, exhibition records or professional results. D65 calibration and D74/GUI gates remain required before release.

## 2. One concrete trusted pair read

Proposed new gymstable API, preserving every existing method:

```go
type PreviewPair struct {
    Version uint32 `json:"version"`
    Status Status `json:"status"`
    AsOfMS int64 `json:"as_of_ms"`
    Fighters [2]Item `json:"fighters"`
}
func (s *Store) ReadPreviewPair(ctx context.Context, session Session,
    ids [2]string) (PreviewPair, error)
```

`Version=1`. IDs are exact lowercase64hex and distinct. `ReadPreviewPair` sorts the two complete selections by full prospect ID ascending before acquisition/output, fixing A/B corners independently of click order; there is no caller-provided receipt, manifest, condition, time, seed, engine selector, bonus or result. IDs are never truncated or case-normalized. The wire parser requires A<B; the typed native Begin method validates strings then sorts the pair before sending. UI selection order is not a seed selector. `Item` has its existing public assignment/manifest/condition and transient private owned record; no new public receipt input API is introduced. No `PreviewPair` parser authenticates assignment.

Algorithm and reuse boundary:

1. Validate pair shape cheaply; enter existing production-profile guard, context-aware stable gate, lifetime cancellation and original `View` lease. An already canceled request starts no source read.
2. Use existing concrete pinned source and `sourceInfo` rollback/count checks. Resolve **both** IDs through consumed stable assignment/index rows before any proof. Missing IDs return existing core `gymstable.ErrNotFound` (`stable_not_found`), mapped to `preview_not_found` only in this new transport family, with no catch-up, fallback namespace, receipt import or chosen substitute. A source backlog does not block an already-consumed pair and does not implicitly sync it.
3. Capture checkpoint, roster revision and both assignments while this one store operation owns the gate. Call existing `readSource` once per ID, serially; compare the complete immutable assignment including source-record digest. Each call verifies normal work, authorization, full static binding and exact source pin. Stop after the first failure/cancellation. Source count growth cannot silently change the selected pair.
4. Compare the two regenerated immutable manifest class fields. A mismatch returns new core sentinel `gymstable.ErrClassMismatch = errors.New("preview_class_mismatch")`; return no partial item. Only the Preview-family mapper recognizes this new sentinel. Old Stable errors/codecs/vocabulary remain unchanged. Never trust a browser summary or infer class from current weight/condition.
5. Reuse the `publishRead` ordering: reacquire the original `View` lease, check exact account enrollment, generation and view, compare unchanged captured assignment/checkpoint/roster revision, observe one valid monotonic `now`, require it at least both birth and accepted-at times, and project **both** conditions with that same `now` and `ProXP=0`. Persist the existing observation metadata, final-recheck the original lease/context and return the complete owned pair only. `Pair.AsOfMS == Pair.Status.LastAsOfMS == both Condition.AsOfMS`.
6. No source hashing or engine execution under account lease or stable metadata mutex. A source call is not cancellable inside Argon: join its one current proof, then check cancellation before the second. `Close` drains the tracked operation before closing stable, then its borrowed source. No timeout goroutine may escape the gate.

At most two serial production proof evaluations per pair call; one source Info call; no Inventory call or full stable scan. Nonselected old records retain set41's lazy integrity checking, not an unsupported all-inventory audit claim. Pair canonical public size is at most 24,576 bytes (each Item ≤8,192); exactly two elements, no nulls. A failed/uncertain metadata write yields no pair, even if clock observations may have committed. Such uncertainty does not mean a fight or roster mutation occurred.

## 3. Engine seam and exact preview binding

This section proposes integration-owned recipe bytes and uses `fighterbout` declarations in `set42-fight-contract-draft.md`: Version1, EngineID `slug-fighter-bout-candidate-v1`, standard three-round explicit-seed Input, and the concrete Result. The coordinated model draft is currently SHA256 `abb5b54edd921855b81dc5b99ef29b2ac94cadc9296dcd2981af4b57ad25040f` (reviewed API seam; no simulations/goldens). Both documents still require joint freeze. There will be no `map[string]any`, raw-JSON transcript escape hatch or arbitrary engine callback in the public API.

Canonical recipe DTO order:

```go
type PreviewFighterBinding struct {
    ProspectID string `json:"prospect_id"`
    SourceRecordDigest string `json:"source_record_digest"`
    ManifestDigest string `json:"manifest_digest"`
    ConditionDigest string `json:"condition_digest"`
}
type PreviewRecipe struct {
    Version uint32 `json:"version"`
    EngineVersion uint32 `json:"engine_version"`
    EngineID string `json:"engine_id"`
    Network uint32 `json:"network"`
    Genesis string `json:"genesis"`
    StoreID string `json:"store_id"`
    SourceAuthorityID string `json:"source_authority_id"`
    DescriptorID string `json:"descriptor_id"`
    AsOfMS int64 `json:"as_of_ms"`
    Rounds uint32 `json:"rounds"`
    Training [2]uint32 `json:"training"`
    Fighters [2]PreviewFighterBinding `json:"fighters"`
}
```

Version1, exact compiled engine tuple, rounds3 and training `[0,0]` only on this helper route. Network/genesis/source/descriptor come from Pair.Status.Pin, and StoreID from Pair.Status.StoreID, never UI. Both bindings use the verified pair in ascending full-ID A/B order. Condition v1 deliberately has no canonical codec/digest API: this integration defines the following new **preview-only** encoding without changing that contract. Encode the existing Snapshot fields in their declared Go order with attributes Power/Speed/Chin/Stamina/Skill, compact ASCII JSON, no whitespace/newline/omission, minimal decimal integers; bound to4,096 bytes and require equality with the already validated exact projection. `ConditionDigest = SHA256(ASCII("SLUG fight preview condition\0v1\0") || LE32(byte_length) || bytes)`, rendered lowercasehex. This hash authenticates no timestamp or XP source. Each manifest digest is the retained verified assignment digest. StoreID already pins account authority/account identity independently of reset generation. Session/view ID, roster membership/revision and current source count are not seed inputs; current view authorization is instead enforced by the outer job. Retained source digest already includes the whole original authority record/birth.

Recipe canonical JSON is compact ASCII, declared field order, decimal integers, no escapes for canonical ID/engine strings, no omitted/defaulted fields, no newline; max4,096 bytes. Proposed seed bytes:

`SHA256(ASCII "SLUG fight preview\0v1\0" with actual NUL bytes || LE32(len(recipeJSON)) || recipeJSON)`.

The seed string is lowercase64hex. It is the recipe digest; do not add a self-referential digest field to the recipe. Selecting the same canonical pair at a later trusted as-of may change the recipe/seed. Reversing click order gives the same canonical pair at an identical as-of; no seed reroll from corner order is permitted. No corner-neutrality claim is inferred. Statistical corner-bias testing remains the engine calibration gate.

Construct each `fighterbout.FighterInput` from verified ProspectID, Manifest, Assignment.BornAtMS, ProXP0 and TrainingSkill0. Call `NewInput(Pair.AsOfMS, seed, a, b)`: it sets Version1/EngineID/Mode standard/SeedKind explicit and sorts whole corners. Require its A/B order to match the pair. It internally reprojects condition at the same birth/as-of; compare those deterministic projections to both pair Conditions before simulation and never add XP twice. `fighterbout.InputID(input)` binds its whole canonical input; no copied full Input need appear in the public wrapper because it is exactly reconstructed from pair+seed. Use `Simulate(ctx,input)` and the frozen full validation/digest API before publication. Pure simulation and full result validation run outside all account/database/cache locks within this one tracked lane task.

Proposed helper result (nested fields in this order): `PreviewResult{version, pair, recipe, seed, engine_result, engine_result_digest}`. `engine_result` has concrete Go type `fighterbout.Result`, not raw JSON; it does not repeat either fighter input. Its InputID must equal the reconstructed canonical InputID. `engine_result_digest` is the engine's lowercase SHA256 of actual-NUL domain `SLUG fighter bout result\0v1\0` followed directly by canonical Result bytes (no length prefix, matching the pure engine contract). Before first publication use `fighterbout.Digest(ctx,input,result)` which fully validates/re-simulates as specified; cache that checked output. This is one explicit Simulate plus Digest's one authoritative re-simulation; do not invoke additional re-simulating codecs to marshal the transport. Native/helper polling codecs must NOT call the engine's ParseResult/ValidateResult/MarshalResult/Digest APIs because those re-simulate. Their private transport validator checks exact closed field shapes/counts/scalar bounds, canonical bytes, InputID and raw digest bindings only, trusting the authenticated helper channel as computation source. It is not an exported “verified result” constructor or an official result-import API; math equality is established at helper execution and independent acceptance. Final helper publication checks recipe/input/seed/result cross-bindings and whole size, then reacquires the **original** View lease and exact current parent-child/view check. No replacement token is fetched. Engine completion after logout/reset/expiry produces no result under another view. The nested pair keeps the exact captured as-of/revisions; later clock/count observations do not relabel it as current.

## 4. Existing serial lane, tracked work and caching

Add a private preview task family to set41's lane, not a second proof goroutine or a new StableJob kind. Aggregate capacity stays **one active operation plus one explicit queued operation across all families**, with automatic catch-up coalesced as before. At most one preview may be active OR queued: another Begin returns busy without changing the existing job/result, even when the other aggregate slot is free. A preview can run while mining at the same synchronous worker cycle boundary or while stopped on the same stopped loop. Explicit preview may cancel an automatic catch-up at its next safe proof boundary; it may not preempt another explicit task. Expired queued work is fenced before dispatch. A successful Begin needs the same final View recheck/publication fence as existing Begin calls. In that final short critical section it publishes the new queued task and clears the prior terminal preview; preflight or failed authorization/publication-fence checks preserve the prior terminal and leave no runnable new task. A pipe write failure after this internal acceptance is transport-uncertain and does not promise that the old result survived.

Use the existing 30-second task deadline from accepted Begin, including queue time, linked to original account/view and lane lifecycle. ACK/cached metadata acquisition is genuinely context-cancellable within250ms; it does not hash, run the engine, wait on lane-held proof, open another source or start a detached timer worker. CPU/Argon cancellation joins before the lane can take another task.

`Stop` cancels queued/active previews, joins the current unavoidable proof/engine step and leaves an already completed authorized replay usable. Logout/reset/detach/helper EOF/Close clear every preview selection/result, cancel/join and prevent late publication before source close. Caller Cancel is nonblocking: queued→cancelled immediately; running cancellation returns its current state until polling observes terminal cancelled. Cancel cannot erase a previously completed result. Fatal owned source/stable corruption/poison/close halts mining with existing cycle-unavailable semantics; ordinary unknown IDs/class mismatch/cancel do not.

Maintain a **separate latest terminal preview job/result**, active/queued metadata and no preview history. Existing `StableJob`, StableSnapshot active/queued/last IDs and old job lookup expose only stable-family tasks; never return a preview ID that old StableJob polling must decode. Stable operations arriving during a preview respect aggregate busy/capacity even though old cached status remains readable. New preview metadata exposes preview-family IDs. No unacknowledged stable mutation result is overwritten to make room for a preview.

The shared retained-cache ceiling remains1MiB, counting manifest/receipt/SVG cache, protected fixed page, exclusive export slot, set41's128KiB last-stable-result plus8KiB metadata, and the new preview allowance. Reserve up to256KiB **before accepting** a preview; while reserved, lazy SVG/export writes include it in the same check. Replacing an older preview reserves the larger of old retained size and new allowance, never two permanently retained results. An accepted Begin discards the previous terminal replay in the same final publication critical section; later failure/cancellation has no promise of recovering the previous result. Preflight and failed internal ACK-publication-fence checks preserve it. There is exactly one latest terminal job, with no hidden successful-result history. Never silently evict protected page/export evidence to run a preview; if reservation cannot fit, refuse `preview_capacity` before proof and retain existing state. Ordinary no-preview behavior retains its previous cache budget. Final publication atomically swaps the one result under the short cache lock, releases reservation, and charges actual retained bytes; all transient private receipts are discarded, not retained a second time in public jobs. Working source/engine memory is separately bounded and must not leak into the cache.

The preview does not replace the selected inventory page, Follow Recent selection, appearance receipt cache or export slot. UI may retain the two selected full IDs across paging. It sends only those IDs; Begin always refetches both from the trusted source. Rendering a replay is local and adds no proof/engine call.

## 5. Closed Preview v1 wire family and methods

New typed family; old structs/parsers are unmodified. Proposed interface extends the existing controller through a separate interface, not by adding methods to old mocks/interfaces:

```go
type PreviewControllerAPI interface {
    StableControllerAPI
    PreviewSnapshot() PreviewSnapshot
    BeginFightPreview(context.Context, string, string, string) (PreviewJob, error)
    FightPreviewJob(context.Context, string, string) (PreviewJob, error)
    CancelFightPreview(context.Context, string, string) (PreviewJob, error)
}
```

The parent delegates only to the exact current stable child. Each child method uses the immutable original login lease. Every cached snapshot/job/result copy occurs under that lease and final current view/child recheck, then releases locks before transport writing. No old-view result is rewrapped in a current view.

Exact new pilot DTO declarations (ordinary `omitempty` applies only to command/response selection, never nested required fields):

```go
type PreviewRequest struct {
    ID uint64 `json:"id"`
    Version uint32 `json:"version"`
    Command string `json:"command"`
    ViewID string `json:"view_id,omitempty"`
    FighterAID string `json:"fighter_a_id,omitempty"`
    FighterBID string `json:"fighter_b_id,omitempty"`
    JobID string `json:"job_id,omitempty"`
}
type PreviewResponse struct {
    ID uint64 `json:"id"`
    Version uint32 `json:"version"`
    OK bool `json:"ok"`
    Snapshot *PreviewSnapshot `json:"snapshot,omitempty"`
    Job *PreviewJob `json:"job,omitempty"`
    Error string `json:"error,omitempty"`
}
type PreviewSnapshot struct {
    Version uint32 `json:"version"`
    State string `json:"state"`
    Error string `json:"error"`
    View *ManifestViewPin `json:"view"`
    StoreID string `json:"store_id"`
    EngineVersion uint32 `json:"engine_version"`
    EngineID string `json:"engine_id"`
    ActiveJobID string `json:"active_job_id"`
    QueuedJobID string `json:"queued_job_id"`
    LastJobID string `json:"last_job_id"`
}
type PreviewJob struct {
    Version uint32 `json:"version"`
    JobID string `json:"job_id"`
    State string `json:"state"`
    View ManifestViewPin `json:"view"`
    StoreID string `json:"store_id"`
    EngineVersion uint32 `json:"engine_version"`
    EngineID string `json:"engine_id"`
    FighterAID string `json:"fighter_a_id"`
    FighterBID string `json:"fighter_b_id"`
    Error string `json:"error"`
    Result *PreviewResult `json:"result"`
}
type PreviewResult struct {
    Version uint32 `json:"version"`
    Pair gymstable.PreviewPair `json:"pair"`
    Recipe PreviewRecipe `json:"recipe"`
    Seed string `json:"seed"`
    EngineResult fighterbout.Result `json:"engine_result"`
    EngineResultDigest string `json:"engine_result_digest"`
}
func MarshalPreviewRequest(PreviewRequest) ([]byte,error)
func ParsePreviewRequest([]byte) (PreviewRequest,error)
func MarshalPreviewResponse(PreviewResponse) ([]byte,error)
func ParsePreviewResponse([]byte) (PreviewResponse,error)
```

The recipe/condition digest helper names are `PreviewSeed(PreviewRecipe) (string,error)` and `PreviewConditionDigest(fightercondition.Snapshot) (string,error)`. PreviewSeed checks exact supported selectors, times, sorted distinct IDs and hash shapes before hashing. PreviewConditionDigest regenerates DNA from the supplied canonical seed and compares the entire Project result using its birth/as-of/XP before hashing. These pure checks validate deterministic contents, not authority. Constructing a preview still requires the private trusted pair path. Any public DTO or codec result carries no assignment/proof authority by itself.

Request DTO scalar fields/order: `id:uint64, version:uint32, command:string, view_id?:string, fighter_a_id?:string, fighter_b_id?:string, job_id?:string`. Version must1. Exact command shapes:

| Command | Required fields after id/version/command | Success member |
| --- | --- | --- |
| `preview_snapshot` | none | snapshot |
| `preview_begin` | view_id,fighter_a_id,fighter_b_id | job |
| `preview_job` | view_id,job_id | job |
| `preview_cancel` | view_id,job_id | job |

Response envelope order: `id,version,ok`, then exactly one `snapshot` or `job` on success; exactly `error` on failure. No simultaneous payload or error. New parsers have explicit command/family correlation at native dispatch; raw union dispatch is only command selection, not permissive decoding. Proposed codecs: `MarshalPreviewRequest`, `ParsePreviewRequest`, `MarshalPreviewResponse`, `ParsePreviewResponse`; signatures match existing typed codec conventions. `ServePreview(ctx,in,out,PreviewControllerAPI)` routes exact old Manifest/Stable parsers plus this family. Keep `ServeStable` available unchanged; helper's stable-enabled branch selects the new superset server, with no new data mode/flag.

`PreviewSnapshot` field order: `version,state,error,view,store_id,engine_version,engine_id,active_job_id,queued_job_id,last_job_id`. States `unavailable|idle|busy|ready|failed|cancelled`; error empty for unavailable/idle/busy/ready, a fixed failure code for failed, and `preview_cancelled` for cancelled. An unavailable/signed-out snapshot is exactly `{version:1,state:"unavailable",error:"",view:null,store_id:"",engine_version:0,engine_id:"",active_job_id:"",queued_job_id:"",last_job_id:""}` (JSON requires quoted field names). No other null, omitted field, stale engine pin or job handle is permitted. This blank form is also used when current authorization cannot be established; it leaks no previous identity. An authorized local failure instead uses state failed, its original non-null view/pins and the fixed error. Otherwise view is full original ManifestViewPin, store and exact compiled engine tuple are present. Busy means a preview is active/queued, ready means latest successful result retained with no preview active; idle has no terminal job. Failed/cancelled correspond to the latest terminal state. A terminal preview may be inspected by its last_job_id. Snapshot is bounded cached metadata; no transcript.

`PreviewJob` field order: `version,job_id,state,view,store_id,engine_version,engine_id,fighter_a_id,fighter_b_id,error,result`. States `queued|running|done|failed|cancelled`. All identities fixed for the life of a job. Result must be null unless done, done requires complete result and empty error; queued/running have empty error. Failed/cancelled require fixed code; no partial transcript. A done job never becomes queued/running or acquires a different seed/input. For initial ACK the job is queued; tracked task ID is a random32-byte handle unrelated to deterministic fight seed. Only one terminal preview is retained; an evicted/unknown ID returns `preview_not_found`. Snapshot exposes last_job_id for lost ACK discovery within the same live view; polling never itself recomputes a lost result.

Fixed error vocabulary: existing `invalid_request,unauthorized,forbidden,stale_view,busy,stable_corrupt,stable_clock,stable_conflict,stable_sync_required,outcome_unknown`, plus `preview_not_found,preview_class_mismatch,preview_capacity,preview_cancelled,preview_unavailable,preview_observation_unknown`. No raw Go/node/database error strings. Map core ErrOutcomeUnknown arising during pair observation to terminal state failed/error `preview_observation_unknown`, no result; record that only clock/count observations may have synchronized, then halt the poisoned local dependency. Subsequent closed/poisoned storage is `preview_unavailable` until normal reopen. Do not enter the roster reconciliation flow or expose an operation ID. Generic `outcome_unknown` in this family is reserved for post-dispatch transport uncertainty: an in-memory preview/observation may have occurred, never a durable fight or roster mutation. No durable preview receipt is invented.

At most4,096 request bytes; root object only. IDs for transport requests are1..9007199254740991. Other scalar types/ranges follow existing safe-integer/timestamp/ID rules. Duplicate decoded names, missing fields, unknown/case aliases, null nonnullable values, trailing tokens, unsafe integers, fractional/exponential numeric spellings, malformed UTF-8 and allocation-exceeding nesting/arrays are refused before materialization. Transport allows JSON whitespace/order/decoded field spelling consistently with existing closed codecs; canonical marshal emits declared order. Nested objects permit the same transport whitespace/order/escaped-key spelling; after strict duplicate/shape/value validation, digest checks use privately re-encoded fixed-order typed canonical bytes. Never demand that a raw embedded engine substring already has canonical formatting, and never call re-simulating engine MarshalResult to canonicalize a poll. This does not relax exact nested manifest/condition/engine values. Null is permitted only for unavailable Snapshot.view, non-done Job.result, and the engine's precisely specified Round.Recovery absence. Maximum nested depth12 counts root object as1, covering engine's maximum8 plus response/job/result wrapping; exact maximum-shape tests must confirm it before freeze. Strict preallocation paths: Pair.Fighters exactly2; Pair.Status.Slots exactly10; Recipe.Fighters exactly2; Recipe.Training exactly2; EngineResult.Initial/Final/Scores exactly2; EngineResult.Rounds length1..3; each Round.Ticks length1..60 and total ticks≤180; each Tick.States/Round.End/Recovery.States exactly2; Recovery.CornerStopped exactly2. Arrays must be non-null; no other array-valued path is accepted in this concrete DTO graph. Enforce lengths in token/structural preflight before slice allocation or fixed-array unmarshalling; Go's fixed-array decoder alone silently ignores excess elements and is insufficient. Engine continuity/terminal recovery shape is separately checked. A non-done job has no result subtree, not empty zero arrays.

Native methods: `Gym.PreviewSnapshot()`, `Gym.BeginFightPreview(viewID,aID,bID)`, `Gym.FightPreviewJob(viewID,jobID)`, `Gym.CancelFightPreview(viewID,jobID)`, each returning full typed PreviewResponse with sanitized failures. Startup/reconciliation order is existing ManifestSnapshot → full existing StableSnapshot → PreviewSnapshot. Native establishes the full current Manifest view and stable Pin/StoreID using the already validated existing path; PreviewSnapshot must match that exact View and StoreID and the compiled engine tuple. It never independently blesses a new helper-supplied store string. Until StableSnapshot is valid/current, preview is unavailable. View/account/reset change clears all preview pins and pending correlations, then repeats that order. It need not refetch StableSnapshot on each unchanged-view cheap poll. Native correlates every job response to the original sorted pair from its Begin request, or to the exact active/queued/last job ID advertised by this already-pinned PreviewSnapshot. Discovery may learn a pair only from that same-view, same-store, same-engine job; it cannot adopt an arbitrary job ID returned by a generic poll. Keep at most three active/queued/last tuples (fewer in normal one-in-flight operation), including request pair and immutable ID/state pins; delayed job/cancel cannot overwrite another tuple. After eviction unknown IDs fail. Frontend additionally pins a selection epoch: changing pair or account clears playback and rejects a late prior-selection result even if it still belongs to the same authenticated view. Returned Pair.Status.StoreID must equal Job.StoreID; its complete Pin must equal the established StableSnapshot pin, including account authority/account, fixture identity, source authority/descriptor and condition projector. Job.View must equal the exact established Manifest view (including origin/transport trust/generation). Pair IDs/order, shared as-of, manifest/condition digests, recipe seed and engine result digest must agree. A captured pair may have an older roster revision/as-of than a later same-view snapshot; keep it labelled as-of rather than rewriting or falsely calling it current. Deep bounded validation/digest checks are allowed; native never runs Argon or the engine to poll. Do not trust a browser-supplied result/hash. Detach/reset clears native correlation; expired or mismatched delayed responses cannot be displayed.

Transport failure after Begin dispatch may leave an in-memory task or metadata observation: return `outcome_unknown`, invalidate unsafe transport correlation and do not auto-retry as though nothing ran. A healthy same-view snapshot/job can discover the existing task. New preview after reconnect is explicitly another predictable unrecorded attempt, not exactly-once execution. Only durable roster commands retain set41 mutation semantics.

## 5.1 Native-only portraits for the checked preview pair

Root-approved narrow presentation seam: `Gym.FightPreviewPortrait(viewID, jobID, prospectID) PreviewPortraitResponse`. It adds **no helper command** and does not extend the Preview wire response union. This resolves the fact that a pair selected across pages need not be present in the old bounded ManifestPortrait cache. That appearance cache and its protected page/export evidence stay untouched.

Exact closed native-only DTOs, in declared JSON order:

```go
type PreviewPortrait struct {
    Version uint32 `json:"version"`
    View pilot.ManifestViewPin `json:"view"`
    StoreID string `json:"store_id"`
    JobID string `json:"job_id"`
    ProspectID string `json:"prospect_id"`
    ResultDigest string `json:"result_digest"`
    DescriptorID string `json:"descriptor_id"`
    ManifestDigest string `json:"manifest_digest"`
    PortraitDigest string `json:"portrait_digest"`
    SVG string `json:"svg"`
}
type PreviewPortraitResponse struct {
    Version uint32 `json:"version"`
    OK bool `json:"ok"`
    Portrait *PreviewPortrait `json:"portrait,omitempty"`
    Error string `json:"error,omitempty"`
}
```

Version1 everywhere. Success has exactly version/ok:true/portrait and no error; failure exactly version/ok:false/error and no portrait. No null, unknown/extra fields or alternate caller text. ResultDigest is the completed preview's checked EngineResultDigest; DescriptorID and both appearance digests come from the corresponding verified pair assignment. Full IDs remain canonical64hex; View/StoreID exactly match the established stable/current-view pins. SVG is image-only canonical output of the existing pinned renderer, ASCII and at most32,768 bytes; full native reply is at most98,304 canonical JSON bytes. Enforce exact serialized size too, with no truncation. Existing native/frontend bounded image-only SVG validation remains required; no script/document injection, file path, renderer selector or arbitrary Manifest/SVG argument is accepted.

Ordinary successful completed `FightPreviewJob` polling establishes one owned **presentation tuple** for that latest checked job: exact current View/StoreID, job ID, engine tuple, seed/InputID/result digest, descriptor pin and two sorted prospect IDs with their verified manifests/manifest+portrait digests. Store no second full transcript, full receipt or condition copy in this native tuple. Canonical retained metadata/manifests are at most24,576 bytes; lazily retain at most two owned canonical SVGs of32,768 bytes each. Charge all native presentation bytes to this explicit88KiB ceiling (24,576+65,536=90,112 bytes), not an unbounded per-job map. A snapshot alone does not establish a tuple; the complete checked job result must have been received. Missing tuple/member returns `preview_not_found` with no rendering or fallback source/cache access.

Accessor sequence under the same native request serialization gate:

1. Validate arguments, acquire the bounded native gate and require this exact current completed presentation tuple and selected member. Capture its immutable tuple identity. Do not obtain a new view/token or adopt a tuple from caller JSON.
2. Render from that tuple's verified Manifest using the renderer selected by its compiled descriptor. Compare exact manifest and existing renderer digest to the retained assignment values; enforce canonical SVG/byte bounds. This runs outside all helper/account/database locks. A cached SVG is usable only if its stored tuple/digest match; cache hits still proceed through step3.
3. **After** rendering/cache lookup, freshly query the existing helper `FightPreviewJob(viewID,jobID)` under the captured original view. Use the private already-gate-held request path; do not recursively call a public method that reacquires the serialization gate. Require a fully valid done response with unchanged full View/StoreID/job/engine/pair/input/seed/result digest and the same requested member's manifest/portrait digests. No Argon or simulation occurs: this is the existing authenticated cached-job read with final original View recheck. An expired/revoked/missing/replaced job refuses the SVG, including a cache hit.
4. Recheck the native current tuple/view/selection binding under that same serialization gate, then publish the typed portrait reply. Release the gate before UI delivery. Native cannot use a stale cache alone as authorization; the fresh helper observation is the disclosure linearization point, with the same ordinary post-check race boundary as other authenticated reads. Frontend also checks its current selection epoch, exact view/job/prospect/result/appearance digests before creating the image.

Do not hold a helper account lease across rendering or join work under one. Concurrent Begin/Cancel/logout uses the same nonreentrant native gate and cannot swap the tuple during the accessor. A helper-side expiry/reset can still happen independently; the fresh original-view check rejects it. Cancellation/helper I/O failure returns no SVG. Clear this presentation tuple and both SVGs on any new accepted Begin, view/account loss, observed expiry/revocation, helper failure or native Close. An uncertain post-dispatch Begin also clears conservatively because the old helper job may already have been discarded. A failed preflight or failed internal Begin publication check may retain the old checked tuple only while its original job remains authorized; any later accessor still must requery it.

Frontend selection and replay images, including duplicate selected-card/replay copies or data URLs, count toward the existing1MiB aggregate appearance/replay budget; revoke object URLs on eviction/view loss and do not build an independent unlimited image map. No image is required in the already-large PreviewJob frame. Public fixture portrait construction remains a preview, not an ownership or accepted-result attestation.

Accessor tests must cover cross-page pair availability without the old appearance cache, first render and cache-hit fresh authorization, stale generation with stable StoreID unchanged, helper job replaced between rendering and authorization, expiry/reset/cancel and helper EOF, wrong ID/result/appearance digest, bounded metadata/SVG/reply sizes, no nested-gate deadlock, no additional proof/engine call, and frontend late-selection refusal. Native visual/image rendering acceptance remains separate from these typed tests.

## 6. Exact frame and memory limits to reconcile with engine

Preserve `pilot.MaxResponseSize=262144` including newline and every existing scanner cap. Stable's own128KiB bound is unchanged. Proposed nested canonical bounds are: Pair24,576, Recipe4,096, engine Result196,608, PreviewResult229,376 bytes. That leaves32,767 bytes below the outer LF cap for the job/view/envelope; bound this non-result envelope to8,192. Reject on both typed preflight and exact final marshal size, with no truncated transcript/second page/remote asset fetch.

Engine owner must prove event/count/numeric/string bounds fit192KiB, with no duplicated full fighters or arbitrary narrative strings in Result. The result owns a bounded event sequence plus scorecard/final facts; presentation derives labels locally. No full dynamic slice is materialized before count and aggregate-byte checks. If the engine cannot meet this budget, redesign its typed event representation before freeze; do not widen old transport or invent chunk streaming as an implementation shortcut. The combined code must test the worst accepted result and one-byte/one-event overflow.

## 7. Focused acceptance and preservation

- Core pair tests: both existing consumed IDs, reversed selection gives the same sorted pair, equal/malformed/unknown/wrong gym/refused IDs, verified class mismatch, off-roster and retired positives, backlog without sync, source digest/assignment/index mutation, one shared as-of/XP0 and unchanged checkpoint/slots/revision/operations; only clock/count observation deltas. No return of a zero/partial pair.
- Count proof invocations: max2, sequential, none for cheap input/missing assignment refusal, and no second after first cancellation/failure. Revoke/reset/expire at entry, between proofs, final clock commit and outer publication. Assert no engine after rejected input, no next proof after cancellation, no late result, no unjoined thread.
- One lane tests: active stable+queued preview and inverse; automatic catch-up yield; failed ACK cannot run; cancellation and Stop while each proof/engine stage runs; final source closure waits; stable snapshots/jobs never contain preview payload/kind/IDs; old typed fixtures still exact.
- Cache tests: protected last page/export survive preview and arrivals; reservation includes lazy portraits/exports; capacity refusal before proof; old preview survives refused/pre-publication-failed Begin, accepted Begin clears it, and a second in-flight preview is busy; one terminal cache; all cleared on view loss. No receipt retained in public result and no preview cache exported as authoritative fight history.
- Codec/native tests: exact request shapes, explicit family dispatch, bad version/depth/null/duplicate/numbers/size, pair order or source mismatch, changed condition/recipe/seed/transcript digest, stale view/engine response, skipped/evicted job, post-dispatch uncertainty. Replay/polling changes neither hash count nor engine count.
- Actual normal-PoW helper acceptance in both published wallets: mine enough genuine receipts to obtain two same-class IDs; no edited class/birth fixture and no cross-wallet pairing. Then stop mining, preview with exact common trusted conditions, independently match recipe/transcript/digest, replay without more work, and verify retained assignments/roster/operations/XP/wallet unchanged. Exercise current held view and revoked old view, paging selection, restart-cache absence and full listener/process cleanup. A missing pair is an explicit fixture-preparation failure, not a skipped success.
- Existing set41 data/root1→2 recovery and old commands remain covered; no marker/schema bump or node changes. Visual playback, accessibility/chooser and D74 are separate actual acceptance, not claimed by DOM/helper tests. New-engine calibration is a pre-release gate independent from transport correctness.

## 8. Remaining drafting work, not new product questions

Engine owner now specifies exact `fighterbout.Input/Result`, canonical sorted corners, Version1/ID and192KiB bound; this draft incorporates them. Both drafts, preview-only condition digest, recipe framing, envelope-depth proof, native-only portrait seam and zero-shape/error rules need joint root+independent review before freeze. These are engineering compatibility choices, not requests for Mike to choose byte encodings. No source/oracle implementation starts until both documents are frozen together. A changed approved D65 rating tuple remains the one conditional product decision that actual calibration could expose.
