# Set50: Career-aware, unrecorded same-gym preview

Status: accepted engineering contract, frozen before independent reference generation and runtime implementation. Root accepted the independently reviewed draft `471b28a6cf428fbf1b4fef9bfdc36218f885d299fba1265f205c77fd5f2ab630`; the review is `set50-career-preview-contract-independent-review.json` (SHA256 `5519eb8f09fbe4a6f3a84409a938a0a8082209343f7fce66e22eb1f5ef31843f`). This freeze changes only the title and status paragraph; all contract sections are byte-identical to that draft. Basis: Set49 implementation `5b590f40a3ebef9c643f05dc8daa578baca66b1c`; source-only intake `set50-preview-intake.md` (SHA256 `cf0d160736384c296022b7afe4c5e18432e45558c30c1a0445ca496db4c75c8d`). Set49 is separately deployed and verified. This is routine candidate preview engineering under existing authorization, with no new gameplay policy or product-release approval.

## 1. Scope and compatibility

The existing v9 Career route gains an explicit unrecorded preview of two consumed assignments in the same gym and immutable manifest weight class, using already-accounted Career XP. This is practice: held View sessions, off-roster fighters and retired fighters retain their existing preview allowance. It creates no bout/result/award, changes no professional record, points, XP, title, rank or training, does not alter C1 eligibility, and never backfills a noon cutoff. No new gameplay decision is needed. D74 remains open separately.

Preserve all existing Preview-v1 DTOs, commands, canonical bytes, recipe/domain, expected vectors, constructor routes and zero-XP behavior. The new family is **CareerPreview v1**, not a reinterpretation of Preview v1. Preserve fighterbout engine-v1, original DNA/birth, condition-v1/D65 and all previous reference files byte-for-byte. No new journal schema, parent version, generic source interface, public import, extra worker or mining is needed.

The current `--career`/`ServeCareers` route exposes the additive family; legacy `--result` and earlier dispatchers do not. Existing v9 install/recovery and refusal of incompatible legacy roots remain. Cached Career inventory overlays are useful for selection, but are never input authority for the preview.

## 2. Closed core types and trusted capture

New public types are in `gymstable`; declaration order below is canonical JSON order. All fields are required. Tags are lower_snake_case, with `XP` spelled `xp`, `ID` as `id`, and `MS` as `ms`. Referenced existing structs retain their exact existing field order/tags.

```go
type CareerPreviewFighter struct {
    Assignment Assignment
    Manifest   fightermanifest.Manifest
    Totals     fightercareer.Totals
    Condition  fightercondition.Snapshot
}
type CareerPreviewPair struct {
    Version      uint32
    Status       Status
    Activation   careerstate.Activation
    CareerDigest string
    AsOfMS       int64
    Fighters     [2]CareerPreviewFighter
}
type CareerPreviewRead struct { /* private fields only */ }
func (r *CareerPreviewRead) RetainedBytes() int
func (s *Store) PrepareCareerPreviewPair(ctx context.Context, session Session,
    ids [2]string) (CareerPreviewPair, *CareerPreviewRead, error)
func (s *Store) ReadCareerPreviewPair(ctx context.Context, session Session,
    captured *CareerPreviewRead) (CareerPreviewPair, error)
```

`CareerPreviewRead` is a concrete process-local immutable token with exact owner `*Store`, original Session.Generation and ViewID, and detached canonical Pair bytes. No constructor/parser accepts caller bytes or a claimed verdict. Zero, foreign-owner, foreign-kind, stale-epoch and reopened-store handles refuse. RetainedBytes is `len(canonical Pair)+512`, nil is zero. The public fighter contains no private receipt; full admitted receipts exist only during owned proof acquisition. Returned arrays, strings and structs cannot alias mutable owner state.

Pair.Version=1; Status is the unchanged Stable status/pin/identity. Pair.Status.LastAsOfMS equals Pair.AsOfMS. Fighters are strictly increasing by complete ProspectID, distinct, and both assignments have the pinned same GymID, source/descriptor, retained digest, original birth and accepted observations. Both canonical manifest classes agree. Each assignment is already consumed at its exact sequence/checkpoint; no catch-up or substitution occurs. Inherited retired/slot semantics are unchanged.

Activation is the exact attached, healthy Career reader's activation, with its fixed current PolicyID and derived ActivationID validated as specified by CAREER_ACCOUNTING. Its trusted Scope includes this Stable StoreID; matching JSON alone does not grant scope. CareerDigest is the Current snapshot digest for **exactly these sorted two IDs** at AsOfMS. The digest covers the full temporal event prefix, including unrelated fighters, per the unchanged careerstate equation. A page digest for a different selection cannot be substituted.

Totals use the existing order `(wins, losses, draws, xp, base_points)`. Under the fixed standard-only policy, `n=wins+losses+draws <=256`, `xp=min(15,n)`, `10*wins+4*draws <= base_points <=14*wins+4*draws`, and `(base_points-10*wins-4*draws)%4=0`. These are structural checks, not proof of authority. Conditions exactly equal `Project(1, original Manifest.DNA, Assignment.BornAtMS, AsOfMS, int(Totals.XP))`; no old Item is relabelled as XP-aware. All input timestamps are positive and at most 8640000000000000; AsOfMS is no earlier than both retained births/accepted times and Activation.ActivatedAtMS. A regressed/insufficient trusted clock refuses rather than accepting caller time.

Preparation follows this sequence:

1. Validate and sort the two full IDs; acquire the existing Stable operation gate and original captured `gymaccount.View` session. Require exact installed Career binding and concrete source identity. Read source Info, retain checkpoint/roster revision/source count, and resolve both existing assignments before any proof. The original session is reauthorized before each expensive source read.
2. Verify at most two normal retained admission proofs serially using the existing pinned source path, outside account/store/controller/cache locks. Compare each full retained assignment/static binding. Check cancellation/lifetime between reads; join an in-flight proof before returning. Class mismatch is a definite refusal.
3. At the bounded publication step, acquire the original account View lease and Stable mutex, recheck owner state/checkpoint/revision/count/assignments and observe one real monotonic Stable time. Through the already attached read-only Career guard, capture **one** Current snapshot for both IDs at that time. This uses only the Result owner's short index mutex, never its action gate or source traversal. Compute both conditions from that captured prefix and validate/bound the entire prospective Pair before synchronization. Recheck the original account capability and cancellation before syncing inherited observation metadata and exposing Pair/token.
4. Only the existing source-count/Stable clock/LastAsOf observation writes may occur. No new persistent key or cutoff fence is written. Preserve existing sync/poison/unknown precedence and disclose neither Pair nor token on uncertain observation synchronization. Owner closure wins over known pre-write cancellation; opaque foreign errors and existing unknown errors remain intact. Preserve the inherited `withLease`/`publishRead` post-commit context checks: cancellation after a known observation sync may return cancellation with no payload; a possibly failed or unacknowledged sync retains unknown/poison precedence. Do not promise that a nil inner callback already guarantees public success, and do not weaken cancellation checks in the existing source helpers.
5. Release every core/account/Result lock and gate before engine preparation. Hold only the existing desktop shared work-lane ownership. After simulation and deterministic validation, `ReadCareerPreviewPair` reacquires the Stable gate, checks token owner/epoch, validates fresh same-concrete source Info identity/health and count nonregression before account/store locks, requiring fresh count>=captured count. Under the original lease it checks exact selected immutable assignments, checkpoint/roster revision, owner/epoch, reader health and activation. Unrelated monotonic source-count growth is allowed for this new pair token; regression, identity change or selected assignment change refuses. It returns a detached copy of the **captured** Pair. It neither verifies proofs again nor advances/replaces the captured Pair.Status, as-of, totals, conditions or digest. The older captured Status must never overwrite a fresher shared Stable cache. Existing Career page handles retain their stricter count-equality behavior unchanged.
6. Only then may the controller's original full-View/active-job guard publish the exact result. No fresh login token can repair a revoked original request. The token is dropped on all terminal/cancel/owner-loss paths after its last required check.

The final reader-liveness check may use the existing bounded Current reader operation but must discard any newly observed prefix; it is not a second semantic capture. A later first Finality, even with the same millisecond, can extend Current. It does not silently reroll or rebase this preview. A new explicit preview captures that later prefix. Historical first-finality XP remains through later result conflict. Do not call Result.FenceCareer, Cutoff or RequireZero for practice. Result action gate → lower source traversal and Stable gate → Result action gate are never nested.

## 3. Exact deterministic recipe and encoding

New helper types, in declaration/JSON order:

```go
type CareerPreviewFighterBinding struct {
    ProspectID         string
    SourceRecordDigest string
    ManifestDigest     string
    ConditionDigest    string
    Totals             fightercareer.Totals
}
type CareerPreviewRecipe struct {
    Version           uint32
    EngineVersion     uint32
    EngineID          string
    Network           uint32
    Genesis           string
    StoreID           string
    SourceAuthorityID string
    DescriptorID      string
    ActivationID      string
    CareerDigest      string
    AsOfMS            int64
    Rounds            uint32
    Training          [2]uint32
    Fighters          [2]CareerPreviewFighterBinding
}
type CareerPreviewResult struct {
    Version            uint32
    Pair               gymstable.CareerPreviewPair
    Recipe             CareerPreviewRecipe
    Seed               string
    EngineResult       fighterbout.Result
    EngineResultDigest string
}
```

Versions are 1; EngineID is exactly `slug-fighter-bout-candidate-v1`, Rounds=3, Training=`[0,0]`. Network/genesis/descriptor are the unchanged fixed candidate chain/descriptor. StoreID/source authority match Pair.Status; ActivationID/CareerDigest/AsOfMS match Pair exactly. Bindings follow Pair's sorted fighters and exactly match their IDs, retained source-record digests, manifest digests, complete totals and condition digests. Every content digest and cryptographic identity field here is lowercase 64-hex. EngineID is the fixed symbolic string stated above, not hex; inherited symbolic selectors and identity encodings (such as ProjectorID, account profile/root public key and AuthorityOrigin) retain their exact existing grammars. Neither View/job/session identity nor roster revision is a deterministic seed selector.

For new enclosing DTOs, `J(x)` is compact UTF-8 JSON in the declared field order, Go `encoding/json.Marshal` escaping (including `&<>` and U+2028/U+2029), decimal integers, no insignificant whitespace or trailing LF, and no omitted required fields. Inherited nested DTOs use their frozen tags and order. UTF-8 must be valid; decoded duplicate keys, aliases/unknown/missing fields, null where nonnullable, negative zero, exponents/fractions/unsafe integer spellings and trailing tokens refuse. Transport may contain bounded whitespace and reordered fields; canonicalization follows typed decoding. Scalar widths remain their actual Go widths plus existing safe bounds, not a blanket 2^53 allowance. Fixed arrays are length-checked before decoding, including Fighters=2, Training=2 and Status.Slots=10. Engine arrays/counts/signed field paths retain their frozen grammar; total ticks<=180.

`LP(b) = LE32(len(b)) || b`. Domain text is ASCII with **actual NUL bytes**, not slash-zero literals:

```
ConditionDigest = lowerhex(SHA256(
    "SLUG career fight preview condition\0v1\0" || LP(J(Condition))))
Seed = lowerhex(SHA256(
    "SLUG career fight preview\0v1\0" || LP(J(CareerPreviewRecipe))))
```

No new domain alters the old Preview condition/seed functions. Even a zero-XP Career preview is explicitly a different recipe. The trusted helper first verifies semantic condition equality before using its new digest.

Construct unchanged `fighterbout.NewInput(Pair.AsOfMS, Seed, a, b)` with each FighterInput's original ProspectID/Manifest/BornAtMS, `ProXP=Totals.XP`, `TrainingSkill=0`. Never supply LeagueAttributesTenths as raw input or add XP twice. Engine InputID and EngineResultDigest retain their existing formulas/codecs. New enclosing canonical JSON must not replace the frozen engine digest encoding.

The explicit job performs `Simulate` and then the unchanged full `Digest` validation, which re-simulates. Thus two deliberate engine evaluations occur per successful new job; do not claim one. Native/helper transport validation may check closed scalar/array/transcript shape, bounded manifest/condition arithmetic, recomputed recipe/seed, canonical InputID and result-byte digest, but never call engine codecs that re-simulate, Argon, source reads or RPC. Local replay and every Snapshot/Job/portrait authorization poll run zero engine evaluations. Parsing public data does not authorize a new job or award.

## 4. Additive pipe/native/UI surface

The helper exports `CareerPreviewSnapshot`, `BeginCareerFightPreview(ctx,viewID,a,b)`, `CareerFightPreviewJob(ctx,viewID,jobID)`, and `CancelCareerFightPreview(ctx,viewID,jobID)` through the Career route. The native side exposes only matching typed methods plus `CareerFightPreviewPortrait(ctx,viewID,jobID,prospectID)`. No generic browser command, supplied seed/XP/as-of/manifest/receipt/token, export or history endpoint is added.

Request order/tags are `id uint64`, `version uint32`, `command string`, `view_id string,omitempty`, `fighter_a_id string,omitempty`, `fighter_b_id string,omitempty`, `job_id string,omitempty`. ID is 1..9007199254740991, Version=1. Exact command shapes:

| Command | Additional required fields | Prohibited optional fields | Success |
|---|---|---|---|
| `career_preview_snapshot` | none | all four | snapshot |
| `career_preview_begin` | view_id, fighter_a_id, fighter_b_id | job_id | queued job |
| `career_preview_job` | view_id, job_id | both fighter IDs | job |
| `career_preview_cancel` | view_id, job_id | both fighter IDs | job |

Wire Begin requires strictly ascending full IDs; typed native Begin sorts complete selected IDs before send. Response order is `id`, `version`, `ok`, `snapshot,omitempty`, `job,omitempty`, `error,omitempty`. Successful response has exactly its matching value and no error; failure has only id/version/ok=false/error. No partial payload accompanies refusal.

`CareerPreviewJob` fields in order: Version uint32; JobID string; State string; View ManifestViewPin; StoreID string; EngineVersion uint32; EngineID string; ActivationID string; FighterAID string; FighterBID string; Error string; Result *CareerPreviewResult. All fields are required, Result null except done. Job's activation equals the installed parent activation; done additionally binds Pair.Activation. States are queued/running/done/failed/cancelled, with exact inherited transition and cancellation rules. A completed result is immutable.

`CareerPreviewSnapshot` fields in order: Version uint32; State string; Error string; View *ManifestViewPin; StoreID string; EngineVersion uint32; EngineID string; Activation *careerstate.Activation; ActiveJobID string; QueuedJobID string; LastJobID string. All are required. Unavailable has View/Activation null, engine version0 and empty store/engine/job IDs; Error is exactly one of `""`, `preview_unavailable`, `career_inactive`, `career_unfenced`, `career_closed`, `career_poisoned`, `career_corrupt`. No other error is a valid unavailable Snapshot error; request-specific authentication/cancellation/refusal remains in the response or matching Job, not an invented whole-service failure. An installed snapshot has the exact current full View/Stable store/engine/activation. States follow the existing preview projection: idle, busy, ready, failed, cancelled, unavailable. Only this family's jobs appear in its IDs; successful selection of the other preview family evicts its old LastJobID. It cannot advertise another family's job under this type.

Closed new-family error union is the unchanged Preview-v1 union plus `career_invalid`, `career_inactive`, `career_unfenced`, `career_closed`, `career_poisoned`, `career_cancelled`, `career_corrupt`. Current normally needs no cutoff fence, but an unfenced/unattached owner is a refusal, never a zero fallback. C1 `career_candidate_ineligible` is not a practice refusal. Unknown observation synchronization maps to `preview_observation_unknown` before cancellation mapping. Possibly dispatched/lost Begin acknowledgement remains `outcome_unknown`; it must not be retried automatically. Known original-session revocation stays stale_view. No arbitrary error strings enter the wire.

The parent supplies the existing original full View and exact installed activation from trusted ready v9 wiring. Native first establishes and pins them through existing correlated Manifest/Stable and Career/Result readiness, not by trusting the first unsolicited preview result's own IDs. It recomputes the derived Stable StoreID and ActivationID and compares Result store/creation identity with already established Career authority. Waiting-source/participants is unavailable/inactive, never fallback to legacy XP0 preview; an incompatible current View is stale_view. No source constructor drops the v9 binding.

Native correlations retain at most three bounded expected tuples (active/queued/latest), with family, full View, job ID, sorted pair, activation, captured digest/as-of, seed, InputID, result digest and private SHA256(J(entire CareerPreviewResult)). The entire-result commitment includes status and complete totals: equal engine output does not permit metadata replacement. A response must match the original request or an exactly correlated same-view Snapshot job, never a newer session. Old/late terminals cannot replace a newer selected job or erase known commitments. Lost client acknowledgement cannot select an unknown result; later exact owned-job recovery remains allowed, since the server cannot know whether its ACK arrived.

The at-most-three compact native correlations and one current snapshot/authority tuple share the24576-byte native presentation-metadata allowance with the one presentation, including both manifests; count their actual serialized bytes plus existing fixed bookkeeping before admission. They are not an uncharged second cache. Only one latest presentation is retained across legacy/Career previews. It contains family/full View/store/job/engine/activation/digest/seed/input/result/captured-result commitments, descriptor and two validated member manifest/portrait bindings; no second transcript, condition or receipt. The native-only portrait reply has required fields Version, View, StoreID, JobID, ProspectID, ActivationID, CareerDigest, ResultDigest, DescriptorID, ManifestDigest, PortraitDigest, SVG, in that order. Portrait.ResultDigest equals the selected CareerPreviewResult.EngineResultDigest; the private whole-result SHA is correlation-only and is never substituted into that field. It is wrapped by Version, OK, Portrait,omitempty, Error,omitempty. SVG comes only from the existing typed local portrait renderer, matches its fixed digest, and is disclosed only after fresh original-view exact-job authorization on render and cache hit. An old-family job is not an acceptable authorization. No browser SVG/render bypass is introduced.

The current preview panel selects this family only on the explicitly ready Career route. Replace its XP0 limitation with the two captured XP values and common as-of; every result/replay is labelled **Unrecorded preview**. Existing result-derived finalized/provisional labels elsewhere remain separate. Selection, View or family changes clear obsolete replay/portrait work; playback remains paused on completion, local, bounded, reduced-motion aware and paused on hidden UI. No polling/navigation automatically Begins. Old v1 UI/route retains its XP0 wording and semantics.

## 5. Scheduling, cache and size ceilings

Keep the same shared work lane: one active plus one explicitly queued task across mining/Stable/Entry/Event/Seed/Card/Bout/Result and both preview families. Preserve inherited preview busy exclusion exactly: an active legacy **or** Career preview makes both Begin variants return busy; any already queued task makes both Begin variants busy. A preview may queue only behind an eligible active non-preview operation under the existing lane rules. This does not allow two preview jobs (one active, one queued) merely because their families differ. There is no second preview worker. Career preview uses the inherited 30-second job context including queue delay, linked to the original session and owner lifetime; no source/account/controller lock is held across proof/engine. Stop/cancel/Close joins actual work. Snapshot/Job/Cancel and final publication retain the existing short View authorization budget (250ms, or shorter caller deadline); source proof/projection/final token read live within the 30-second job. Career page's separate 500ms budget is not silently applied to preview controls.

One latest complete preview slot is shared across both types. A refused Begin preserves the prior authorized replay; an accepted Begin clears that slot, reserves capacity and can queue only under the existing lane rules. A finishing older active task cannot install itself over a newer accepted selection. Cancel/timeout releases the pair token and reservation. Stop retains only an already completed, still-authorized replay; logout/reset/helper failure/Close clears all preview selection, tuple, token and portrait state. Subsequent polls copy cached bytes and authorize the original View/owned controller lifetime; they neither re-observe the source/Career prefix nor simulate. Dependency failure learned by an explicit operation invalidates the corresponding owned cache.

All byte limits below are inclusive canonical UTF-8 lengths without LF, except request/response frame limits include their terminating LF. Parser input limits charge actual transport whitespace. Limits are new-family checks; none widens or narrows old schemas.

| Boundary | Bytes/count |
|---|---:|
| Embedded ManifestViewPin | 1024 |
| Embedded Stable Status | 4096 |
| Activation / Totals | 512 / 128 |
| CareerPreviewFighter | 8448 |
| Pair / opaque token accounted bytes | 24576 / canonical Pair+512 |
| Recipe / EngineResult / complete Result | 4096 / 196608 / 229376 |
| Null-result job metadata / full Job | 2048 / 231424 |
| Snapshot wire / retained Snapshot+current-authority accounting | 4096 / 2304 |
| Request / entire response frame | 4096 / 262144 |
| Maximum nested containers (root=1) | 12 |
| Pair/binding/training members | exactly2 |
| Engine ticks | at most180 |
| Shared preview allowance, across both families | 262144 within existing aggregate1048576 |
| Native presentation metadata | 24576 |
| Each SVG / two-SVG presentation total | 32768 / metadata+65536=90112 |
| Native portrait response including wrapper | 98304 |
| Frontend aggregate retained cache | existing1048576 |

Bound arithmetic (source-only cap substitution, not generated vectors): inherited XP0 Item<=8192; inserting required Totals adds at most138 bytes and XP-adjusted Condition decimal widths add at most5 more (ProXP+1, league Skill+2, rating+2): `8192+138+5=8335<8448`. No cap increase is needed. Pair keys plus Status4096+Activation512+two Fighter8448+max timestamp/64-hex digest are at most21667 bytes. Result keys plus Pair24576+Recipe4096+Engine196608+two64-hex commitments are at most225492 bytes, fitting229376. Job fixed framing plus bounded View1024 is at most1582 bytes with null result (1578 excluding null), so full Job<=230954 fits231424. A response wrapper plus that Job fits262144 with substantial margin. Snapshot fixed framing plus View1024 and Activation512 is at most2018 bytes, fitting its2304 retained accounting allowance (wire cap4096). The Recipe cap-substitution bound is1624 bytes; the largest full-job response using the Job cap is231477 bytes including LF. Deepest engine response branch is10 containers and the Pair/manifest/DNA/attributes branch9, both within12. Current inherited fixed identity shapes bound Status below4096 and View below1024; the arithmetic deliberately allows max integer widths/filled slots rather than relying on observed small fixture values.

During active work charge the full Job allowance, pair token, one queued null-result job and one retained Snapshot/current-authority tuple simultaneously: `231424 + (24576+512) + 2048 + 2304 = 260864 <=262144`, leaving1280 bytes. Do not separately retain a duplicate Snapshot/authority tuple; if existing parent caches own the same facts, this conservative charge still remains reserved. The controller reserves the larger of old retained bytes and this existing262144 allowance before ACK, and charges it together with Career read token/Stable cache/receipt and every other family's retained/reserved bytes. Charge reciprocal Begin/portrait/page admission checks as well. Drop the pair token before shrinking to actual terminal Job bytes. Do not retain separate full legacy and Career results, or a second canonical transcript in a hidden cache. Temporary bounded engine/encoding working allocations are not claimed to be an overall1MiB process-heap limit. Independent review must confirm these envelopes and worst nested depth before reference freeze.

## 6. Reference and acceptance plan; no vectors yet

After this contract is reviewed/frozen, independently compose the pinned old Python engine, preview encoding, manifest/condition/portrait and Career arithmetic/prefix producers. Bind all copied/read source inputs and the public Set49 evidence; use no new Go outputs as expected values. Preserve old v1 corpus exactly. New finite examples must distinguish modeled Career events and transport fixtures from genuinely admitted assignments/finality. The public two-fighter pair does not itself confer a live session or import authority.

Reference cases must cover XP0/1/unequal/15, standard draw XP, retained first Finality after later conflict, before/equal/after as-of, same-ms later prefix, canonical reversal, original birth, retired inputs, exact new domains and field order, complete totals and double-XP rejection. Include wrong activation/requested-ID prefix/source/time/condition/recipe/seed/InputID/result digest, old/new type cross-decoding, null/duplicate/unknown/extra-array/numeric/depth/byte controls. Keep transport validation under engine/Argon/source/RPC traps. Model lane, unknown ACK, late terminal, original View and reciprocal cache admission separately from cryptographic calculations.

Later application acceptance should prove concrete two-proof acquisition, cancellation before second proof and before each engine call, owner/source/reader close, held/revoked/reset/logout controls, token owner/epoch/alias isolation, unrelated source-count append during engine gap accepted without rebasing, count regression/source identity change/selected assignment or checkpoint/revision change refused, no synchronous proof work in polls, immutable replay under later same-ms award, the known two engine calls per explicit successful job and zero in transport/replay, no journal change except existing observations, and shared cache/queue limits across both modes. Cold two-proof timing with256 Career events must fit the unchanged30s job, with final metadata/authorization deadlines separately measured; warm proof cache is not a substitute. Race tests establish correctness, not latency.

Actual awarded integration can use a cold owned copy of the fully completed Set49 journals and the real v9 parent/helper/native path under the existing **normal-PoW scoped-clock test route**. The public Career rows[7]/[11] use AsOfMS1796500801001 (2026-12-05T20:00:01.001Z), also the activation time, which is later than the drafting calendar of2026-11-18. Choose a compatible monotonic scoped time at or after retained clocks; never roll clocks back, rewrite births/activation or relabel this as normal-calendar awarded acceptance. A separate normal-calendar package gate exercises the new family at XP0 unless legitimate real-calendar finalized awards actually exist. Keep all existing history and conflict latches. Neither new admission/mining nor a fresh finalized fight is needed for a positive awarded preview. Use fresh original authenticated sessions, never historical public View as live authority. Compare exported new input/recipe/transcript against independent arithmetic after implementation; inspect no operator seed/private DB for this planning stage.

### Genuine partner availability from public attempt3 evidence

Successful owner report `set49-actual-careers-attempt3.json`, SHA256 `b519b928b83c2de2dfd8bbc5c59c250b6ce51a99802de51affe1b57ac95ba267`, binds public `artifacts/set49/career-chain-attempt3/public-career-native-evidence.json`, SHA256 `91934c45ba3609a52b3ed64847c988e33c19cef2ea7055d5337af00b6e805069`. Its full bounded inventory walks and paired completed Stable source jobs establish these existing middleweights:

| Gym / public row | Awarded member | Same-gym partner | Captured totals |
|---|---|---|---|
| zero256, rows[7] `reopened-after-finality` | `067121d1f9be9eee0eab884de24e24df996e3b15d4385f5123930c6405d71fa3`, sequence2 | `164913fbdff8365f9187dbdfa22c4a6b667399f5fbde0e4f4ddd718daa1cbb3c`, sequence3 | awarded W1/L0/D0, XP1, base14; partner zero |
| incrementing256, rows[11] `after-c1-refusal` | `22a2c799c122ac002a3ca277e0f7f84dd89c8b5f1fe1e1a18e098b1aad27e717`, sequence3 | `ebfddc68ebcaac81047e4d409458575f41b9509b7c330bfdd0f087e13c5288db`, sequence4 | awarded W0/L1/D0, XP1, base0; partner zero |

Zero gym is `a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5`; incrementing gym is `28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510`. Each row's `inventory_source_jobs[].page.items[]` supplies the original assignment and `manifest.physical.weight_class`; its corresponding `inventory[].page.items[]` supplies captured Career totals. The zero walk has27 members in8/8/8/3 pages; the incrementing walk has4. They are actual admitted sources in the already-reviewed Set49 run, not the modeled8-proof capacity fixture. Rows[8..9] additionally preserve the first award after conflict and under a held zero session. This drafting step checked owner-report hash correspondence and these public fields only; it did not rerun proofs or inspect retained stores. There is no partner-availability blocker and no justification for inventing a new source or borrowing the opposing booked corner.

## 7. Proposed implementation boundary

Only additive `gymstable/career_preview*.go` and focused tests; new typed helper/native preview files and narrow shared-lane/cache routing; v9 parent forwarding; existing preview panel/tests. No engine/condition/award/source-journal arithmetic changes. Independent inputs/byte bounds and this contract must be frozen before new expected vectors; independent vectors must be reviewed before runtime implementation. This draft settles routine new-family encoding/resource choices and preserves existing gameplay. Remaining work is engineering review/qualification, not a new title/ranking/record policy question.

## Source basis for this draft

These SHA256 values bind inspected local source, not new execution or a contract freeze. The intake additionally lists previous reference indexes; a future reference input manifest must bind every transitive producer dependency actually read/copied.

- `FIGHT_PREVIEW.md` — `e5dddc1129a7561a125fa1eebe8847523ff6e49c00647817374c6320fab12d60`
- `FIGHTER_BOUT.md` — `7f4a3683097877e3463523c045dd1c78668c128f3675efab14d33d5b926bd28d`
- `FIGHTER_CAREER.md` — `07de417554cf2a61085e8545f78fbf43bb5b02fc63a3d3d4d6889acadbe193d1`
- `CAREER_ACCOUNTING.md` — `a5b4d716a9b26f0cdcd469b623d34f0c218377149c376d5e22775bb46b10782b`
- `CAREER_CONSUMERS.md` — `622e7b96d6a784555dd96c6eb86e5c4e1c66db5cf441aeb8e655e3a1dc7945a6`
- `gymstable/preview.go` — `56c1f126f84b5d5c9d46d838bea9209ead789cc777d45584e9ff61b580a167bd`
- `gymstable/read.go` — `3107ab73cdcfc5f79dba7097ab23a119db4302acd1b07ba6e8d6fde0b19666ae`
- `gymstable/store.go` — `f27101ad01805ac7d48ac39430ee7527be4a3ad8c95199c32c642ed8ae77e338`
- `gymstable/career.go` — `aef30959bb54fde9d7a8a8734953d5267b6d6c0793be631fb84ee4d9317d7c4a`
- `gymstable/career_read.go` — `e9c8c6d6ee54f111f2fa38509c0c25353ef5f9dce51650150a5d5ddace632f89`
- `internal/careerstate/state.go` — `eaedd8f52777cbb11b1e085474a1b51d2a144bd5179542156f5c2d8713463528`
- `internal/careerconsumer/guard.go` — `f592047da0031c2d4931d1bf7ae606d10bd08e3f0a959269ee678cadfccf6984`
- `desktop/pilot/preview_types.go` — `6fccac0c575ceb89784cd425628c7b186ed9324c58a6bcbae05c4c3006ca1f9f`
- `desktop/pilot/preview_binding.go` — `8aebb8b014eabb5d66e8b17b7238087f6dfbc79c9fa215e0e48e8e9207e3da3c`
- `desktop/pilot/preview_controller.go` — `5b381f3aeb630d1c78c36d2564a7656b0bcee5d1ac2bac1cf9176e68ecad0879`
- `desktop/pilot/preview_validation.go` — `1094eb74d19d2fa7de1ad661b6b0111d52fd1d230a64f56140f1488dd7087ce9`
- `desktop/pilot/career_protocol.go` — `bd9d46b5b335637cdb9042cd87bb5fdc57ee3e01f8c114d80f1d022f35a98456`
- `desktop/internal/bridge/preview_bridge.go` — `bd06c34bdea6476e85d400584758a4c9ee8f1a13273067a41428ae7b595bf02a`
- `desktop/frontend/stable.js` — `ccce87415808f8ca224a87d58772faac437d05417d1ce1d24b19fc0fca8617db`
- `fighterbout/input.go` — `75c29f8c30f481b155beec5d91b4f304a9e5064929efb8cf1f347d74b223431d`
- `fighterbout/codec.go` — `95a76026eb68322958b1cec915ada910f71957ca8f1068929536b5b8133f9673`
- `fighterbout/testdata/reference/reference_bout.py` — `c3247a0d856270cc5043b5767ace9f628b2efb9cd120d786773f9221d928bd7f`
- `fighterbout/testdata/reference/generate_preview.py` — `5f3ae24d63e339c5c4f174a7943eca233aae27fc0ca7031762d675cf824e25ed`
- `internal/careerstate/testdata/reference/generate.py` — `9cd4b150c274ef4bc54b0a21e35013d86b540405bbf003c8b7b0dd8899a53bdc`
- `fightercareer/testdata/reference/generate.py` — `506cb30affe74039b16bd0254f236fbfea4cf4b69f8a35760b0373666565857c`

Review preservation: the original reviewed draft `set50-career-preview-contract-before-review.md` retains exact SHA256 `dffe80e9cb4b69032a8e11b34332bc76d583649fd85f5ec9ab0820bdba3866c6`. This successor incorporates the approved source-count, shared-lane, inherited cancellation, identifier grammar, finite unavailable-error, portrait digest, scoped-clock and byte-width clarifications; no repository or expected-vector bytes changed.
