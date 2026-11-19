# Set51 draft: authenticated finalized fighter history

Status: engineering direction accepted by root; closed successor for independent review, not yet frozen, implemented or reference-qualified. The original f63 draft is preserved under `set51-history-contract-attempt1/`. Root selected the separate capture job, newest-first offsets, capture-relative applied XP and invalidation on Stop/reopen; size/retention targets below supersede the impossible provisional736KiB reservation. Source basis: clean Set50 topic `slug-career-preview` at `89b12447bd2bba220fd956156f12cacaa4f6eafb`, followed only by the separately reviewed normal-package test correction. Exact inspected source hashes accompany this draft. No shipping runtime edits or new expected vectors were made for this proposal.

## 1. Scope and preserved authority

Add **CareerHistory v1** to the installed v9 `--career` route: an explicit, authenticated, bounded history of already effective standard-bout awards for one consumed fighter in the signed-in gym. Show original result identity, opponent IDs, Saturday, method, first-finality effective time, W/L/D and base-point contribution, earned XP and its capped contribution. Held View, retired and off-roster reads are allowed. This is neither a standings table nor opponent-quality points, title status, matchmaking, ownership transfer, new awards or proof of whole-league completeness.

D62/D78 and the current CareerPolicyID remain unchanged: only the first retained own signed Result Finality plus60 descendants produces the exactly-once effective event; later reported conflict does not remove it. Standard earned XP is1, total XP caps at15, and record/base points continue after that cap. Preserve original Result `awards:none` publication rules and the separately activated Career layer. Legacy finalized v1 results gain no retrospective award. C1 guards, activation, all old schemas/commands/event bytes/digests/reference files and engine-v1 stay byte-identical. No new DB key, ledger, store/parent version, node/miner or automatic Result action is needed.

## 2. One trusted capture, then immutable pages

Add a narrow opaque capture operation inside `internal/careerstate`, available through the installed `careerconsumer.Guard`; do not expose the reader's raw State or a generic importer. Suggested signatures:

```go
// Internal trusted-reader API; private fields and no JSON constructor.
func (r *Reader) CaptureHistory(ctx context.Context, prospectID string,
    asOfMS int64) (*History, error)
func (h *History) Snapshot() Snapshot
func (h *History) PrefixCount() uint32
func (h *History) EventCount() uint32
func (h *History) Rows(after, limit uint32) ([]HistoryRow, error)
// gymstable; exact owner/source/original session remain private in the handle.
func (s *Store) PrepareCareerHistory(ctx context.Context, session Session,
    prospectID string) (CareerHistoryCapture, *CareerHistoryRead, error)
func (s *Store) ReadCareerHistory(ctx context.Context, session Session,
    captured *CareerHistoryRead, after, limit uint32) (CareerHistoryPage, error)
func (r *CareerHistoryRead) RetainedBytes() int
```

`CaptureHistory` obtains **one owned atomic State** from the existing concrete Result owner, validates the activation/scope/index/clock and derives the unchanged Current snapshot for exactly `[prospectID]`. Prefix selection is `EffectiveAtMS <= asOfMS`, in `(EffectiveAtMS,AwardID)` ascending order. Every prefix event, including unrelated fighters, contributes to the existing CareerDigest and PrefixCount. Matching rows and totals come from this same owned State, never a separate `Snapshot()` read. Snapshot.Mode is Current and its sole requested ID is exact. No cutoff fence or new Result observation is written.

Stable preparation uses the existing operation gate and original View lease; requires the installed Career binding and exact concrete registry source, resolves one already consumed assignment, then verifies **at most one normal retained admission proof** through `readSource`. This expensive source verification and inherited manifest work occur outside account/store/cache locks under the existing30-second asynchronous work budget. No catch-up, other-fighter proof, additional history-specific condition projection, portrait generation, engine simulation or chain RPC is required. The existing `publishRead` path still performs its inherited `projectItems` condition projection; this contract does not bypass or remove it.

At the existing `publishRead` boundary, recheck original authority, unchanged selected assignment/checkpoint/revision/source identity and sampled count; sample one real monotonic Stable time; capture history through the installed Guard, whose Scope must include this Stable StoreID. This only takes the Result owner's short index mutex, never its action gate or source traversal. Validate prospective capture/rows/caps before exposing it. Only inherited observation clock/count/LastAsOf writes may occur. Preserve their sync/poison/cancellation/unknown ordering; any failed public operation returns no capture/page/partial rows. Join uncancellable in-flight source verification before returning; the deadline is not a hard wall interruption guarantee.

The opaque handle binds exact Stable owner/source pointers, original session Generation/ViewID, activation, immutable assignment, captured source Status and detached history bytes. Zero, foreign, reconstructed-from-JSON or reopened-owner handles refuse. `ReadCareerHistory` rechecks the same source health/identity before locks, allows only monotonic unrelated source-count growth, and under the original lease requires exact assignment/checkpoint/roster revision, owner and activation. Reader liveness checks must discard any newer prefix. New awards—including equal-millisecond events—never change an existing capture. A new explicit capture is required to see them. No second proof or Current-prefix reconstruction is performed for pagination. Reuse the narrow Reader Activation/Scope/lifetime getters rather than calling Snapshot solely to rehash the current index.

## 3. Additive public types, ordering and contribution semantics

Proposed new `gymstable` DTOs, declaration order is JSON order; required snake_case fields, inherited tags unchanged. No field is added to an old DTO.

```go
type CareerHistoryCapture struct {
    Version uint32
    Status Status
    Activation careerstate.Activation
    CareerDigest string
    AsOfMS int64
    Assignment Assignment
    PrefixCount uint32
    EventCount uint32
    Totals fightercareer.Totals
    HistoryID string
}
type CareerHistoryRow struct {
    Ordinal uint32
    Event careerstate.Event
    Earned fightercareer.Delta
    XPBefore uint32
    XPApplied uint32
    TotalsAfter fightercareer.Totals
}
type CareerHistoryPage struct {
    Version uint32
    Capture CareerHistoryCapture
    After uint32
    Next uint32
    HasMore bool
    Items []CareerHistoryRow
}
```

Version=1. `Status.LastAsOfMS == AsOfMS`; exact source pin/gym/consumed Assignment and activation validate under their inherited rules. AsOfMS is server-sampled, positive/safe and no earlier than activation, birth or accepted time. Each row's matching Corner must equal the selected ProspectID and original pinned AccountID/GymID; the opponent is the other original Corner. Every retained Event retains its existing canonical bytes and all source/result/finality identity fields. No name/current-owner/current-conflict field is invented from an Event.

Filter matching events while folding **chronologically from zero**, not from the start of each page. Map the original winner to `a`, `b` or `draw`, call the unchanged `fightercareer.Award(...,"standard")`, select the matching corner's Delta, and Apply it to the prior matching Totals. `Earned.XP=1`; `XPBefore=prior.XP`, `XPApplied=TotalsAfter.XP-XPBefore` is0 or1. Ordinal is the matching chronological position1..EventCount. Final Totals equals the same captured Snapshot's selected totals; W+L+D=EventCount<=PrefixCount<=256. Earned points are finish14/decision10/draw4/loss0, with no quality bonus.

**Applied XP is capture-relative explanatory accounting, not a separately persisted irreversible allocation to an Event.** A later same-time event may sort before another event in a future capture and change which row explains the fifteenth applied XP. Original events and aggregate cap remain unchanged. Label captured rows/as-of accordingly.

Proposed presentation order is newest-first, the reverse of matching canonical order. `After` is a count already skipped in that immutable display sequence, not an AwardID/time cursor. Limit is1..8; After is0..EventCount. len(Items)=min(Limit,EventCount-After), Ordinal[i]=EventCount-After-i, Next=After+len(Items), HasMore=(Next<EventCount). A nonterminal success never returns an empty or artificially short page. At the end return nonnull empty Items, Next=After, HasMore=false. After beyond end, limit0/>8 or overflow refuse. Different pages repeat the exact Capture. There is no fabricated row for an unawarded fighter. Unrelated prefix events affect CareerDigest/PrefixCount, never EventCount or page boundaries.

Use existing compact Go-compatible canonical JSON `J`, lowercase64-hex and actual NUL domains. A proposed display/correlation identity, not new award authority:

```
HistoryID = SHA256("SLUG candidate fighter history\0v1\0" ||
    RAW(Status.StoreID) || RAW(Activation.ActivationID) || RAW(CareerDigest) ||
    LP(J(Assignment)) || LE64(AsOfMS) || LE32(PrefixCount) ||
    LE32(EventCount) || LP(J(Totals)))
```

LP is LE32 length plus bytes. Repeated captures with identical facts may share HistoryID; their process-local jobs/sessions remain distinct. A filtered page cannot recompute the full CareerDigest without unrelated events: it is a trusted sealed-source correlation commitment, **not** a standalone exported proof of completeness. Strict validation still checks every included Event identity, matching corner, arithmetic, ordinal, widths/counts and enclosing binding that is locally checkable. Unknown/missing/duplicate fields, wrong null/array shapes, aliases, floats/exponents/unsafe integers, invalid UTF-8 and trailing JSON refuse. No parser or HistoryID creates a trusted handle.

## 4. Exact helper/wire arms

Typed methods are `CareerHistorySnapshot(ctx)`, `BeginCareerHistory(ctx,viewID,prospectID)`, `CareerHistoryJob(ctx,viewID,jobID)`, `CancelCareerHistory(ctx,viewID,jobID)` and `CareerHistoryPage(ctx,viewID,jobID,after,limit)`. The Snapshot method takes context so one500ms budget can include parent and child checks. Dispatch the closed `career_history_` prefix before broad `career_`; earlier routes do not accept it.

New helper types use the following field order and lower_snake_case tags. **All fields are required, including empty strings/zero scalars/null arms; no omitempty.** This avoids optional-zero ambiguity for the first page. Inherited full View remains its exact type/schema, bounded1024B.

```go
type CareerHistoryRequest struct {
    ID uint64; Version uint32; Command string
    ViewID string; ProspectID string; JobID string
    After uint32; Limit uint32
}
type CareerHistorySnapshot struct {
    Version uint32; State string; Error string
    View *ManifestViewPin; StoreID string
    Activation *careerstate.Activation
    ActiveJobID string; QueuedJobID string; LastJobID string
}
type CareerHistoryJob struct {
    Version uint32; JobID string; State string; View ManifestViewPin
    StoreID string; ActivationID string; ProspectID string; Error string
    Capture *gymstable.CareerHistoryCapture
}
type CareerHistoryPageResult struct {
    View ManifestViewPin; StoreID string; JobID string
    Page gymstable.CareerHistoryPage
}
type CareerHistoryResponse struct {
    ID uint64; Version uint32; OK bool
    Snapshot *CareerHistorySnapshot; Job *CareerHistoryJob
    Page *CareerHistoryPageResult; Error string
}
```

Every version=1; ID is an integer1..9007199254740991. It is an exchange correlation ID, not job/capture authority. Required identity strings are lowercase64-hex; empty is allowed only in the named arms below. Job IDs use fresh32-byte random identifiers under the existing process-local job mechanism; equal deterministic HistoryIDs do not merge jobs. Requests contain exactly the eight declared fields:

| Command | Required nonempty/meaningful values | Other selector fields |
| --- | --- | --- |
| career_history_snapshot | none | ViewID/ProspectID/JobID empty, After/Limit0 |
| career_history_begin | ViewID, ProspectID | JobID empty, After/Limit0 |
| career_history_job / career_history_cancel | ViewID, JobID | ProspectID empty, After/Limit0 |
| career_history_page | ViewID, JobID, After0..256, Limit1..8 | ProspectID empty |

Core additionally checks After<=captured EventCount. No caller AsOfMS, cursor Event, activation, source Pin or prior totals is accepted. Canonical JSON follows §3, including inherited scalar widths, counted LF and strict nested arrays. Request decoder depth1; response max12 containers. All new parsers reject unknown/missing/duplicate fields and wrong union payloads before controller dispatch.

Every successful response has Error empty and **exactly one** nonnull arm: snapshot command→Snapshot, begin/job/cancel→Job, page→PageResult. Other arms are null. A refusal has OK=false, all three arms null, and exactly one nonempty code. Local predispatch refusal uses Version1/ID1 when no exchange ID was allocated; dispatched replies must echo the actual request ID.

Job grammar: queued/running→Capture null, Error empty; done→nonnull Capture, Error empty; cancelled→Capture null, Error exactly `history_cancelled`; failed→Capture null, any terminal error below except `history_cancelled`. Metadata with Capture null is<=2048B. All states require the exact full View/StoreID/ActivationID/ProspectID/JobID tuple. Done Capture repeats its StoreID/ActivationID/selected Assignment identity and its full validated Status binds the original View authority. Terminal Job bytes are immutable; cancelled/failed cannot become done on later polling. Begin success is queued only. Cancel may return an already completed immutable terminal, or the same running job while cancellation joins; it does not claim work has stopped before joining.

Snapshot grammar is exact:

* unavailable: View/Activation null, StoreID and all job IDs empty; Error is one of `history_unavailable`, `career_inactive`, `history_clock`, `history_corrupt`.
* installed states require valid full View, StoreID and Activation. idle has all IDs/Error empty. busy has exactly one ActiveJobID or QueuedJobID, LastJobID/Error empty. ready has only LastJobID and Error empty. failed has only LastJobID and a terminal error excluding `history_cancelled`; cancelled has only LastJobID and Error=`history_cancelled`.

There is at most one active-or-queued History job; other families' IDs never appear in this Snapshot. A previously retained terminal may be hidden while the current History job is busy. Snapshot is metadata, never permission to manufacture a capture. Metadata/admission/page deadline exhaustion (including an earlier caller deadline) or admission contention is outer `busy`, not a fabricated unavailable/completed state, provided no already known observation-unknown, fatal dependency or original-view authorization error takes precedence. Explicit caller cancellation maps `history_cancelled`; expiration of the admitted30-second task is also `history_cancelled`. This contextual distinction overrides the general cancellation mapping below and never extends a shorter deadline.

Closed outer-error union: `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `outcome_unknown`, `career_inactive`, `history_unavailable`, `history_corrupt`, `history_clock`, `history_capacity`, `history_not_found`, `history_source_changed`, `history_cancelled`, `history_observation_unknown`. Terminal errors are this union excluding `outcome_unknown` and `busy` (the latter is admission/metadata refusal); failed additionally excludes `history_cancelled`.

Map known Stable invalid/auth/forbidden/stale directly; not-found→history_not_found; dependency identity/revision/assignment mismatch→history_source_changed; capacity→history_capacity; clock→history_clock; corrupt→history_corrupt; closed/poisoned→history_unavailable; inactive Career→career_inactive; unexpected Cutoff-unfenced in this Current-only path→history_corrupt. Known explicit context/Stable/Career cancellation and capture-task timeout map history_cancelled (metadata/admission/page deadlines follow the contextual busy rule above), except owner closure learned before successful linearization maps unavailable. Opaque foreign errors are not reclassified as cancellation; closed default mapping is unavailable. No old error union changes.

**Unknown precedence:** any known Stable observation `ErrOutcomeUnknown` takes precedence over cancellation/Close and maps `history_observation_unknown`, with no Capture/page. It is a synchronized-observation uncertainty, not an award or job-import permission. A lost/malformed/wrong-family/correlation-failing reply after dispatch maps outer `outcome_unknown`; it is not stored as a terminal Job error. A known successful observation Sync followed by inherited `withLease`/`publishRead` cancellation may return history_cancelled with no payload, as today; do not relabel every post-Sync cancellation as unknown or expose the prospective capture. Once the outer original-authority publication has completed successfully, later cancellation must not retroactively rewrite its immutable terminal. Known unknown/poison/corruption cannot be downgraded to a harmless cancelled terminal. Learned fatal owner failure clears this family's pageable handle and latches refusal; a later successful lookup cannot repair the failed dependency.

## 5. Lane, selection, cache and native correlation

Use the existing global work lane. Begin refuses if any job is queued, a History job is active, Stop is draining, or the original parent is not signed_in/nonbusy/current. One History job may queue behind existing eligible non-History work; it never preempts explicit work. Existing automatic-work cancellation/pause rules apply only through the existing scheduler, without starting mining. There is no second History worker or active+queued History pair.

The500ms admission subcontext bounds only admission checks; the accepted task has its own at-most30s context linked to the original caller/lifetime and any earlier deadline, not a subcontext cancelled merely when Begin returns. Under the original View admission guard, reserve the family allowance, create the exact task object and install it as an **unpublished** queue entry. Neither polling nor the worker may see/run it yet. Recheck the original child/session and then, only if that same task still owns the queue, publish it and retire the former History pageable selection/last terminal. Before publication, a denied/expired/cancelled admission preserves the former capture. Rollback restores prior reservation/selection only while `queued==task` and the original owner is still live; late cancellation cannot restore bytes over a successor, clear its handle, or resurrect a closed child. Capacity refusal has no selection side effect. A lost Begin ACK does not undo an admitted job: reconcile via Snapshot/exact discovered Job, never automatically repeat Begin.

Only the exact active task may publish its finished Capture/handle or terminal failure. Check cancellation before and after proof, before capture, before final lease and before final task publication. No account/Stable/cache mutex covers source proof. Final publication retains the original parent full View and actual account lease; fresh login credentials cannot repair it. Parent logout/reset/expiry/child replacement drains work before owner close. Stop joins work and invalidates the pageable handle and History selection; subsequent pages/old jobs return history_not_found when the same View survives, stale_view after identity loss. A terminal retained elsewhere in UI stays labelled historical/captured. New sessions or store reopen require a new explicit job.

Store the opaque capture as **one canonical matching-row byte buffer**, exact byte offsets and one canonical Capture header; do not retain the full global Event index, a duplicate decoded row graph, or all page copies. Compute each row once from the captured chronological fold, then lay it out in display order. The temporary State/index used during capture is released before retaining the result. Public returns are detached and cannot mutate owner bytes. No full history export/import is introduced.

Caps after closed scalar analysis:

| Object/work | Limit |
| --- | --- |
| Capture work | one serial normal source proof, existing30s context |
| Authority index | existing256 events and2048B old Event cap unchanged |
| New-context Event / row |1344 /1536 canonical bytes |
| Capture header |8192B; Status<=4096, Assignment<=2048, Activation<=512, Totals<=128 |
| Retained opaque capture |416KiB, including canonical header+row buffer+257 four-byte offsets+4096 owner allowance |
| Page |8rows,32768 canonical bytes |
| Job / null-Capture Job / Snapshot |12288 /2048 /4096 canonical bytes |
| Helper History allowance |512KiB: capture416+terminal12+metadata16+optional retained page32=476KiB plus36KiB margin |
| Native+web History cache |one page32768 plus compact metadata16384, charged to each existing shared cache |
| New request / response |1024 /36864 bytes including LF; raw encoder/parser limits are cap−1 |
| All metadata/page calls |one500ms context encompassing parent and original-account pre/post checks; shorter caller deadline preserved |

Closed relaxed maxima are Event1330B, Earned55B, Totals64B and row1529B, using fixed-width ASCII identities, safe16-digit timestamp,10-byte date, longest allowed method `double_corner_stoppage`, count<=256 and points<=3584. The relaxation deliberately combines incompatible maxima and is an upper bound, not a valid fixture. Enforcing row<=1536 yields array<=393473B; header8192+array+offset1028+owner4096=406789B<416KiB. See the independent-review candidate arithmetic report. No new Event cap changes the existing stored Event grammar/bytes.

Stable's current136KiB base reserve plus selected Career200KiB plus History512KiB is848KiB, leaving176KiB for the actual existing receipts/manifests/SVG/export/other-family payloads. Keep all those bytes in every admission/cache-growth calculation. This permits the ordinary selected-page route with bounded actual payloads; it does not promise all families' independent maxima fit simultaneously. If the exact retained sum exceeds1MiB, refuse before admission without silent eviction or truncation. Do not drop the selected Career page merely to start history. Independent tests must cover an ordinary eight-fighter selected page and maximum History, plus genuine capacity refusal when other retained payloads consume the remainder.

These are **retained-cache charges, not a total process-heap bound**. One normal Argon source verification has its inherited transient cost; temporary validated index copies, decoded eight-row pages, concurrent response marshal buffers and Go allocation overhead are measured separately. They must remain bounded by the stated job/page/index limits and be released, but are not falsely claimed inside1MiB. No full-history serialization is sent over the pipe. Parent startup25s/first-native30s and all old-family budgets remain unchanged. Race verifies correctness separately from normal latency.

Native discovery first establishes the original Stable full View/pin/store and Result incarnation from existing snapshots; its installed activation must match ResultStoreID/creation/source-creation and the learned Career activation. Begin binds the exact request prospect/original authority. At most3 compact job/discovery tuples are retained. Job lookup for an unknown ID is allowed only if the latest same-authority Snapshot named it; it must carry that exact family/view/store/activation. Every learned terminal has a private canonical whole-Job commitment, so done→running or changed terminal refuses even without a separately supplied digest.

Page requests require an already correlated done Job with Capture and the currently selected job/owner. Response must equal its **entire Capture**, not merely HistoryID (which deliberately omits mutable Status fields), plus exact full View/StoreID/JobID/After; length==min(requested Limit,Capture.EventCount-requested After), each Ordinal[i]==Capture.EventCount-After-i, exact Next/HasMore and matching count. Independent typed Page parsing checks all self-contained bounds (including no empty nonterminal page); the request-aware core/native boundary additionally enforces the exact requested Limit relation. Row validity includes the existing Event/Award identity and original corner, outcome/delta, XP/record/point equations and adjacent running-total consistency. A middle page's opening totals are trusted captured-source output, not a standalone proof that omitted prior events are complete. Capture.Totals equals TotalsAfter only for a returned row with Ordinal==Capture.EventCount (the globally newest row), not the first row of an arbitrary later page. No engine, source proof, signature/chain traversal or Current recomputation occurs in transport.

A new explicit selection supersedes older selection intent. Delayed old terminal/page responses may be validated but cannot replace a newer selected capture or erase learned commitments. For the selected capture retain a fixed256-slot binary SHA256 commitment array keyed by chronological Ordinal plus a256-bit seen bitmap:8192+32B, charged within the16KiB metadata allowance alongside at most3 compact job tuples, current authority and whole-Capture/terminal commitments. On every returned row compute SHA256(J(row)); first observation records it, every later observation of that Ordinal must match. This catches changed same-job pages across different offsets/limits and revisits, without an arbitrary page-key map or full-history payload retention. A fresh selected capture resets this fixed row-commitment array only after its full original-authority correlation succeeds. Reauthorize every requested page. There is no cache-hit authorization shortcut. Clear all native History authority/page metadata on original View loss or learned fatal dependency failure; never overwrite a fresher shared Stable cache with captured old Status.

## 6. Result links, UI and evidence

A row opens the **existing explicit Result read** by Saturday/BoutID in the same original session. Before displaying its current overlay/proof action, match ResultKey, SourceRecordDigest (the retained Bout record digest, not mutable Result row digest), InputID, ResultDigest, original corners/outcome and first-FinalityDigest to the selected event; verify the same installed Result incarnation from Activation. A later mutable Result RecordDigest may change. The link is selection data, never a substitute source/importer. Failure/missing current detail leaves retained history labelled captured and detail unavailable, without silently mutating/removing an award. Reading history never starts Check/Finalize/export or chain connection automatically.

Show “Finalized history”, “Base points (not ranking)” and earned/applied XP distinctly. Provisional outcomes remain in the existing result/bout surfaces and never enter this finalized table. Current conflict/source usability is a separately fetched and timestamped Result overlay; absent a fresh query, say current status not checked rather than “healthy”. Do not fetch opponent proofs merely to invent names or contemporary ownership.

Before code, freeze/review this contract and independent Python inputs/expected pages: zero/one/max256 matching and unrelated events, equal-time insertion/reordered capture, all methods, XP14→15→15, page boundaries/newest-order/cursor forgery, full-prefix digest with unrelated changes, immutable Event bytes, job/full-View/activation correlation and invalid fields/caps. Preserve all old corpora. Runtime gates include original held/stale/reset/close, replacement/cancellation/unknown-sync, unrelated source append versus regression, no repeated proof/engine/RPC on pages, cap accounting, no C1 weakening, and retained awards after conflict. Existing actual59/60/61 evidence remains separately scoped; new actual/native and normal-package acceptance must exercise the new history link/page boundary without claiming a GUI review or league rankings.

## 7. Review and minimal reference boundary

Root accepted the explicit capture job, newest-first offsets, capture-relative applied XP and Stop/reopen invalidation. The revised512KiB family allowance, closed new wire shapes and error/lifetime mappings above now await independent source/formula review; no gameplay judgment is outstanding.

Freeze only the needed public inputs: this contract; D62/D78 and Career accounting/consumer/pure-arithmetic definitions; old `careerstate` Event/Activation/Snapshot equation and independently generated Career arithmetic/reference cases; exact existing Stable Assignment/Status/Pin and ManifestView grammar; selected lifecycle/session/error source definitions as documentary inputs. Preserve their original reference bytes. A finite independent Python producer can derive synthetic labelled Career events, ordered folds, captures/pages/IDs and wire/model controls without importing or copying fighterbout/preview/portrait engines, Argon generators, full chain corpora, private journals or Go outputs. Source verification remains a separately measured real-account/normal-PoW integration gate, not a synthetic corpus claim.

Independent reference review must distinguish locally checkable row arithmetic and correlation from sealed-authority/completeness, include the size relaxation as arithmetic rather than a valid positive vector, and preserve every failing candidate. No reference generation or repository implementation starts until root freezes the reviewed contract.
