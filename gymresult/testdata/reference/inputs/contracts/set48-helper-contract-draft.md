# Set48 desktop/helper contract — candidate result publication

Draft engineering contract, not implementation authorization or a frozen reference. Basis: Set47 implementation `ed3b6de7c1f46df4bbba1ec7de515d5dfce90a8c`, the frozen Set47 helper/core contracts, and root's Set48 scope and acceptance outline. This file is outside the repository. Source, accepted evidence, icons and engine remain untouched during Set47 observation.

## 1. One saved bout, explicit publication

Consume one already saved Set47 Bout selected by original ViewID, Saturday and booked BoutID. Commit its exact canonical RecordDigest, InputID, ResultDigest, source/card/event identities and original fighter attribution. Never accept a caller's transcript, seed, outcome, checkpoint, public key, source Pin, funding coin, raw transaction or claimed verification. No engine run, new match, changed cutoff, professional/exhibition record, XP, training, title, prize, ownership or production league authority is introduced.

Use a new closed Result family. Existing SeedJob is date/EventKey-scoped and its adapter accepts the fixed seed carrier; BoutJob creates an immutable transcript. Neither enum can safely mean result publication. The four core phases may remain Prepare/Publish/Reconcile/Attest, but user controls are **Publish test result**, **Check status**, **Finalize test result**, and **Save public proof**. No generic Run/poll/navigation callback dispatches a transaction. There is no additional Connect control/API: only these explicit network actions own readiness; earlier source preparation remains a separate existing flow. A separate result checkpoint must cover the result's own transaction inclusion block with at least60 descendants. Source seed finality is only an input prerequisite.

D62's provisional visibility, result depth and signed checkpoint are settled. An unchanged result transaction can be re-included before its first accepted result checkpoint without changing fight randomness. After checkpoint acceptance, preserve the first checkpointed inclusion/certificate and historical finality; a crossing reorg is a separate latched conflict, not a new result, a branch-selection vote or a node-consensus veto. Current node availability, last verified chain time, present canonicality, source health and historical result finality remain separate fields/labels. Local metadata polling cannot refresh chain freshness. D77 seed waiting/incident policy is unchanged and is not automatically a deadline for result publication.

The fixed result fund is the published fixture **key2 P2WPKH** script on the same pinned zero256 chain. Seed funding stays exact key1. Pin purpose, funding key/script, declaration domain and checkpoint key; no caller selection, shared key1 reservation book, automatic transfer/funding, fee bump, input reselection or generic wallet/signing API. One result journal owns all same-purpose key2 reservations, including uncertain writes. A visible empty fund is an honest state.

## 2. Concrete authority and source fences

Add `gymaccount.ResultPublish = "result-publish"` to the closed capability set and held/fresh-authentication restrictions. This is a separate narrow fixture permission, not SeedPublish or gym ownership promoted to league authority. It requires the original current zero256 account, matching immutable source identity, fresh login, no recovery hold, signed_in state and no parent operation in progress. Add it to the parent guard beside SeedPublish. It accepts neither mining-only credentials nor a replacement session after expiry/reset.

Signed preparation, dispatch/rebroadcast and checkpoint attestation require ResultPublish. Ordinary result metadata, saved evidence export and network-only reconciliation require View; a held original View can inspect historical evidence without signing or spending. The other gym can read retained results but cannot start a zero256 publisher, borrow its node processes or publish through its own chain. No cross-login hidden node owner is retained. A newly authenticated zero256 session after reset must honor its actual generation and hold.

The concrete saved-Bout source seam validates the full chain of source Pins/StoreIDs and creation times, dates, CardID/BoutID, immutable RecordDigest/InputID/ResultDigest and attribution. Authorization precedes retained source work and repeats under the same original capability in the final commit fence. Never nest ResultPublish WithSession inside an existing View callback. Preparation, source projection, signatures, RPC, block proof work and file construction run outside final account/source locks. Only bounded, prevalidated final checks and durable write publication occur inside the prescribed core callback order. Later source writes must not occur after that invocation's foreign final commit callback.

Existing saved Bout audit on route startup remains exactly once as required by Set47. Result store audit/actions/export add no engine calls, and cached Result Snapshot/Job and private wire/native validators do no source projection, manifest/condition/name derivation, proof hashing, signing, RPC or SVG rendering. Explicit source operations may have the existing bounded semantic source audit cost; measure it rather than claiming it is free.

Core must freeze the exact distinction between an absent/prepared publication and a later source conflict: new signing/first dispatch/explicit rebroadcast/attestation require healthy exact source; read/reconcile retain and expose the historical record even when the source later conflicts. Reconciliation cannot silently undo a synced publication or erase its reservation. This draft assumes the same refusal for a conflicted source applies to any explicit rebroadcast; no transaction replacement is an escape route.

## 3. Shipped node-only readiness

Publication must work without enabling automatic candidate mining. `ManifestController.start(false)` remains the existing finite initialization operation; changing it to persist would break login Prepare. Add a separate private tracked `result_nodes` owner, only on the new route and an already validated ready zero256 home. Reuse `ManifestController.openNodes`: exact configfile-only primary/peer, canonical chain directories/markers, actual Cmd and done-after-Wait channels. Do not call openWallet, wallet.Signer, openStores, openLive, admission server, ManifestWorker, getblocktemplate or submitblock in this path. The old Seed publisher may borrow this same ready zero pair under its unchanged capability, but old Seed commands do not implicitly start it. The stopped concrete stable/entry/source handles and existing shared work loop stay in place.

An explicit network Result action may request node readiness. Passive Snapshot/Status/Record, job polling, login/reopen and proof metadata do not start nodes. No automatic reconnect loop follows a process failure. The pair is visibly connected until Stop, logout, reset, expiry, Close or explicit mining Start; a completed/refused/cancelled Result job does not imply disconnection. The operation's borrowed ResultPublisher closes/joins on every path, but must never kill a borrowed mining owner. A later explicit request can reconnect after an observed failure.

Use one concrete operation/owner epoch, not a generic process framework. At most one owned pair for a child: normal mining or node-only, never both. Capture the exact owner pointer/epoch/Cmd/Done before asynchronous work. Old startup failure, cancellation and unregister must not remove a successor's pair or cancel a later operation. Register only after complete ready validation; partial startup kills/joins every process and publishes no online state. Parent's original child/view guard applies before owner admission and when publishing readiness; creating a new owner additionally requires the published signed_in/nonbusy parent state even for a View-only Check/export. A held signed_in zero256 View remains eligible for those non-signing operations. Node-only startup is bounded by the caller's remaining job deadline and controller lifetime until readiness; readiness transfers the pair to the same controller/view lifetime. Cancellation before readiness cancels that captured startup; cancellation after transfer ends only the job. Readiness transfer and the startup-cancellation callback check the same exact owner epoch and phase under the owner lock; an already-fired callback cannot cancel a transferred owner or a successor. Initial malformed/unauthorized/absent-source refusal happens before any node startup. A Stop/view invalidation always cancels startup and all borrowers and joins the pair.

Drains occur outside account/controller/store locks and outside the work lane held by a borrower. Unregister cancels both Seed and Result borrowers, then joins the global lane before closing nodes. Never call a join-on-lane unregister from the task that currently owns that lane. A coordinator/drain path owns that sequence; a task cancellation releases its Publisher and then the lane. Explicit Start captures and drains only the node-only owner before entering the existing authorized mining path; it cannot acquire a mining credential implicitly from publication. Stop closes either mode. Close, login switch, reset and expiry retain their existing invalidate→cancel/join→detach order.

Serialize node-only startup with existing tracked projection, wallet signing, recovery and backup operations that may touch the same chain paths. No silent duplicate database open. A conflicting operation returns busy; explicit Stop releases the node-only owner so existing recovery/backup operations remain usable. Native modal identity/Stop behavior stays intact. Manifest mining state and wallet state are not relabelled as running/ready merely because result nodes are online: wallet remains closed, hashes/sec0, no accepted/pending/attempt increment from this owner. ResultSnapshot exposes current connection state separately; earlier wallet/chain figures may be stale.

## 4. Additive parent v8 and startup

Add opt-in `accountpilot.NewResults(ctx, Config)` and private `pilot.NewResultManifestAccount(Config, StableAccountConfig)` plus `AttachResults(ctx,*gymresult.Store)`. Scoped test constructors exist only under `slugtestclock`, with a shared immutable clock and no production CLI/env/DTO setter. New default `candidate-results` requires the complete manifest/stable/entry/event/seed/card/bout flag chain; version0.4.8/build48 uses `resultBuild=1`. Old constructors/modes continue refusing the new root version.

The root v8 marker appends `result_schema:1`, `result_phase`, `result_store_id` to the complete v7 marker, in that order. Phase is waiting_source/prepared/ready. Preserve every inherited stable/entry install and event/seed/card/bout field in every mutation path; never send a v8 marker through an older writer. The child reader is opt-in and validates a v8→v7→v6→v5→v4→v3→stable base chain. The result journal directory is `results`; it is not a second Bout transcript store.

Validate existing ready dependencies before upgrade and require results absent. waiting_source has empty result_store_id and no adopted results data; it may coexist with a ready Bout source during the durable crash window before result prepared publication. Derive full ResultPin from the trusted ready BoutPin and fixed result purpose/fund; no live node, funding or extra login is needed to install it. Persist exact prepared intent before creation. Resume only absent/real-empty/exact recognized initialization intent/exact matching initialized-empty. Ready recovery requires exact source Pin/StoreID and SourceCreatedAtMS equal to concrete Bout.Info.CreatedAtMS; initialized-empty preparation must have result count0. Missing ready source/result stores and unknown partial data remain retained/refused, never recreated.

Parent owns the shared result→bout→card→seed→event journals across fixture switches; child attaches once before ready View publication. Nil attachment is allowed only waiting_source. Logout/reset/expiry close the child and its borrowed source/process handles but retain shared journals. Parent Close drains jobs/node owners first, then closes shared journals in reverse order and account/root lock last. No source handles are retained across child epochs.

Keep the25-second startup admission/audit context inside the native30-second first request. Forward cancellation where supported; check before/after bounded legacy synchronous opens and join/close them without detached timeout goroutines. Result audit must not re-simulate saved Bout data; the inherited Bout Open audit still executes once. Full worst-case startup including inherited256-row audit and result audit must be measured below the existing30-second outer bound. No test result here is claimed yet.

## 5. Closed Result v1 transport proposal

Core names below align with the accompanying Set48 core contract §§3/5/6 and Appendices A/B. The final external approval must bind both complete hashes after review; this draft grants no implementation authority. No global history endpoint: existing Card/Bout navigation selects one saved Bout, and ResultStatus/Record describes that publication only.

```go
const MaxResultRequestSize = 8192
const MaxResultResponseSize = 262144 // complete frame including LF
// Proposed partitions; exact core maximum-shape formula must pass before freeze.
const MaxResultJobSize = 245760      // 240KiB, wraps core Action/ReadResult224KiB
const MaxResultMetadataSize = 49152  // 48KiB
const MaxResultCacheSize = 294912    // 288KiB, part of unchanged aggregate1MiB

type ResultControllerAPI interface {
    BoutControllerAPI
    ResultSnapshot() ResultSnapshot
    ResultStatus(context.Context, string, string, string) (gymresult.Status, error)
    ResultRecord(context.Context, string, string, string) (gymresult.ReadResult, error)
    BeginResultPublish(context.Context, string, string, string) (ResultJob, error)
    BeginResultReconcile(context.Context, string, string, string) (ResultJob, error)
    BeginResultAttest(context.Context, string, string, string) (ResultJob, error)
    ResultJob(context.Context, string, string) (ResultJob, error)
    CancelResultJob(context.Context, string, string) (ResultJob, error)
    BeginResultExport(context.Context, string, string, string, string) (ResultJob, error)
}
func ServeResults(context.Context, ResultControllerAPI, io.Reader, io.Writer) error
func MarshalResultRequest(ResultRequest) ([]byte,error)
func ParseResultRequest([]byte) (ResultRequest,error)
func MarshalResultResponse(ResultResponse) ([]byte,error)
func ParseResultResponse([]byte) (ResultResponse,error)
```

Arguments after context are ViewID, Saturday, BoutID; Job/Cancel take ViewID, JobID. Publish is one explicit action, not a background Prepare-completion callback. Proposed implementation is one typed publish job encompassing core signed Prepare-if-absent and fixed Publish, with explicit core phase/result semantics. Preserve known prepared/scan-incomplete/waiting-for-funds evidence without converting a known refusal to a claim of submission. Any unacknowledged durable write or possibly sent raw transaction is unknown. The job uses core Prepare (idempotent without re-signing) and then core Publish with the same captured original session and selector. It never calls Reconcile or mines to make a stale funding cursor current. Definite scan/funds refusal leaves its acknowledged prepared row available to explicit Status/Read. Preserve an unknown from either phase; a later fresh-view/public-shape/size failure after a successful durable action is outcome_unknown. Core Publish itself still refuses missing preparation. No UI poll schedules this continuation; it occurs only inside the one admitted publish job.

All fields required in this canonical snake_case order; no omitempty/generic maps:

```go
type ResultRequest struct {
    ID uint64
    Command string
    ViewID string
    Saturday string
    BoutID string
    JobID string
    Path string // empty except private native export command; never a JS argument
}
type ResultResponse struct {
    ID uint64
    OK bool
    View *ManifestViewPin
    StoreID string
    Snapshot *ResultSnapshot
    Status *gymresult.Status
    Record *gymresult.ReadResult
    Job *ResultJob
    Error string
}
type ResultSnapshot struct {
    Version uint32
    State string
    Error string
    View *ManifestViewPin
    StoreID string
    Pin *gymresult.Pin
    SourceCreatedAtMS int64
    CreatedAtMS int64
    PublisherAccountID string
    NodeState string
    NodeOwner string
    ActiveJobID string
    QueuedJobID string
    LastJobID string
}
type ResultJob struct {
    Version uint32
    JobID string
    Kind string
    State string
    View ManifestViewPin
    StoreID string
    CardID string
    Saturday string
    BoutID string
    ResultKey string
    Error string
    Result *gymresult.ActionResult
    Export *gymresult.ProofSummary
}
```

`ResultKey` is exactly `gymresult.ResultKey(pin,cardID,boutID)`, distinct from the engine ResultDigest. Export summary is exactly core ProofSummary (§8); the fourth string argument of BeginResultExport is a native-selected destination, never a browser argument.

Commands: result_snapshot uses no selector; result_status/result_record/result_publish/result_reconcile/result_attest use ViewID+Saturday+BoutID; result_job/result_cancel use ViewID+JobID; result_export uses ViewID+Saturday+BoutID+native Path only. Every unused field is its required empty/zero/null value. IDs lowercase64-hex; request ID1..2^53−1 strictly increasing across prerequisite requests; exact Saturday parser and old timestamp/integer limits. Path is the sole private UTF8 textual exception: decoded length<=1024bytes, absolute, valid UTF8, no NUL/CR/LF/tab, with unchanged native/protected-root/symlink checks. The complete actual JSON request plusLF must still fit8192bytes after escaping; do not assume a raw path length guarantees frame admission.

Snapshot states ready/busy/setup_incomplete/unavailable/failed. Ready/busy have original full View, derived Pin/StoreID and both immutable creation times; SourceCreatedAtMS equals Bout creation. Busy includes foreign lane occupancy but all IDs are Result-family only. At most one active/queued Result ID, nonempty only when busy; active/queued/last cannot alias. Ready may retain last only. Setup-incomplete has current View, no Pin/store/jobs/times/publisher, node offline/owner none and result_setup_incomplete. Unavailable has no View or authority; failed retains only an already validated current authority and a closed typed fatal error. Exact failure union follows core. NodeState is offline/starting/online and NodeOwner is none/publication/mining. Offline requires none; starting has publication; online has publication or mining. These are current owned-process observations, never chain-finality assertions; failed startup is offline plus typed last job error.

Job kinds publish/reconcile/attest/export; states queued/running/completed/failed/cancelled. Accepted ACK is exactly queued. Queued/running have no result/error/export. Completed has exactly one purpose-appropriate payload. Failed/cancelled carry no success payload; cancelled uses result_cancelled, possible durable/file/dispatch uncertainty uses outcome_unknown. Terminal canonical bytes are immutable, including observed-at fields. Known running→queued is refused. At most3 compact active/queued/last tuples refer to one authority; no duplicate deep-owned Pin.

Result response errors are the core closed union `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `outcome_unknown`, `result_unavailable`, `result_corrupt`, `result_capacity`, `result_clock`, `result_calendar`, `result_not_found`, `result_not_prepared`, `result_source_incomplete`, `result_source_changed`, `result_source_conflict`, `result_node_offline`, `result_waiting_for_funds`, `result_scan_incomplete`, `result_not_mature`, `result_conflict`, `result_cancelled`, `result_proof_unavailable`, plus helper `result_setup_incomplete` and the shared `invalid_frame`. Missing/unprovable export evidence uses result_proof_unavailable; export oversize uses result_capacity, preserving the fixed vocabulary. Never fold unauthorized/closed/poison/cancel/unknown by string matching. Non-OK replies have View null, StoreID empty and every success pointer null, with one nonempty allowed Error. Success has exactly one command-appropriate payload and Error empty; its nonblank outer View/StoreID equals the payload/current authority. ID0 is allowed only for an uncorrelated invalid_frame, never a normal reply.

Core result phases are absent/prepared/provisional/finalized/conflict in the core-defined priority. Publication state is none for absent/no-transaction, otherwise built/dispatch_unknown/submitted; an existing checkpoint may lock inclusion before historical Finality exists. Result Status.NodeState unknown/online/offline describes its last chain observation and is deliberately distinct from Snapshot current owner state. Do not collapse current disconnected owner, historically online observation, seed finality and result finality into one badge.

## 6. Correlation, lane and cache

Native bootstrap uses the gate-held Manifest→Stable→Entry→Event→Seed→Card→Bout chain, then derives expected ResultPin/StoreID and source creation from trusted BoutSnapshot. A result response cannot discover its own authority. Establish/validate the selected saved Bout's RecordDigest/InputID/ResultDigest through existing authorized BoutStatus; no full transcript prefetch is required to publish. Bind every request/reply to original full View, date, CardID/BoutID, ResultKey, fixed purpose/chain and immutable saved identities. StatementID, source RecordDigest/InputID/ResultDigest and the first local raw transaction/txid are immutable. The publication RecordDigest legitimately changes when its observations/inclusions change; do not confuse it with the immutable source RecordDigest. Every returned Status.RecordDigest must match that same Action/ReadResult.Record, while old terminal Job bytes remain fixed. Historical source owners are not the current View. A stale/replaced session cannot complete a job under a new login.

Keep one global active1/queued1 lane, explicit Result family in every dispatch/expire/cancel/snapshot path. At most one Result action active or queued. Reserve the complete result-family budget and terminal capacity before accepting Begin. Final same-view ACK publication either succeeds or rolls back the unpublished exact task and cache reservation; rejection must retain prior accepted data. Do not clear work early merely to expose a signed_in blank parent Snapshot. Node-owner connection admission is coordinated outside that lane's join path as described above.

Network/action jobs keep30seconds including queue and owned adapter time. Explicit Result Status/Record get a named private500ms admission/operation budget, preserving any shorter caller deadline. This is an explicit new-family engineering choice because it adds a deeper real account/source traversal; it does not change Bout250ms, Card500ms, old Seed/Event/Entry budgets, native30seconds or any cache/frame limit. An already entered Sync joins normally and can produce observation uncertainty. A hard wall-time preemption guarantee is not claimed. Measure actual advancing clock+real account+parent rechecks, not only stationary clocks or mocked leases.

Cached ResultSnapshot/ResultJob only copy bounded validated cached objects under fresh original authorization and final child/view check. No chain work, signature, source derivation, engine or source/result journal clock advancement. Existing original-account authorization may observe/sync its own clock; those unchanged account checks are not falsely described as free or read-free. Explicit Check status is a network Reconcile action, different from these polls and from local ResultStatus. Failed/lost post-dispatch reads mean uncertain observation; they do not prove a transaction was or was not sent. Recover by explicit ResultStatus/Record and Check status. No automatic publication retry.

Fresh explicit status overlays survive older terminal job polls. A prior job may establish selection only if no newer explicit selection exists, or refresh the same immutable record. It cannot replace a newer different saved Bout or hide a learned conflict/source change. The new result panel must also preserve Set47's current-status SVG guards: showing result publication does not authorize cached portrait disclosure.

Charge `resultBytes` reciprocally at every old family cache admission, lazy portrait/export pin, protected page, job callback and frontend retained object. Zero when inactive. One selected compact result record/action result; no duplicate saved transcript, engine input, CardSource or full proof in this family. Existing selected Bout transcript remains charged once to Bout. Metadata owns one ResultPin/View/Snapshot, one selected Status,3 compact job tuples and combined4096 JSON-encoded render/identity allowance. Partition proof must include JSON escaping and all correlated retained fields. No silent eviction of unrelated protected evidence or cap increase; over-cap replacement refuses transactionally.

Closed parsers reject duplicate decoded keys, unknown/missing fields, invalid nullable arms, floats/exponents/-0/Boolean numeric coercion, extra trailing data and size/depth/array violations before unbounded allocation. Private shape/hash/correlation checks may verify compact signature/hash fields but never invoke public validators that regenerate source or simulate. Appendix A lists exact path-specific arrays and signed sentinels. Root container depth1, maximum20 pending tighter independent formula proof; reject over-depth before decoding. No suffix wildcard height rule.

## 7. Native/UI behavior

Add LaunchResults and StableBridge.ResultRequest on the existing pipe; exact exported Gym methods follow the controller names above. `ExportResultProof(viewID,saturday,boutID)` is native-only chooser orchestration and never takes a path from JavaScript. Build/CLI routes preserve all old flag matrices/defaults, icon bytes, bundle identity and process cleanup. No new network API is exposed to the webview.

Use the existing selected saved-result panel. Show the already saved outcome immediately with candidate/provisional wording; publication status is an additional section, not a second fight. Explain that signed preparation is not chain inclusion, submitted/unknown is not confirmed, 59 descendants is not60, and a finalized seed is not a finalized result. Distinguish public candidate fixture authority from production league authority. Show current connected/offline state and separate last-chain-check time. Show result checkpoint conflict and source conflict independently while retaining the exact saved transcript.

Publish test result is enabled only for the current fresh unheld fixed publisher with a saved exact usable source; waiting funds/source/node states have useful text and no automatic funding or Run. Check status is explicit read-only chain reconciliation and can be used by held zero256 View with node-only readiness. Other gym views can read local retained facts; they cannot connect the publisher's chain. Finalize test result is an explicit ResultPublish action only once the exact current inclusion has60descendants and no conflicting source/result condition. Save public proof is authorized by current View and does not sign/broadcast/mine. Stop visibly disconnects node-only ownership as well as stopping mining. No new automatic polling loop upgrades itself into a network action.

## 8. Bounded native public proof export

Use exactly core Appendix B's PublicProof/ProofHeader/ProofBlock/ProofSummary and ExportProof, not a second independently ordered helper format. The file binds the full unchanged saved Bout.Record, immutable Card.Record, compact Result.Record/current captured Status, fixed public Pin and the result/funding/source-seed bodies. Card already contains the snapshot and retained seed evidence, so no duplicate Seed.Record or cross-fixture live registry read is added. ProofSummary is the core<=2KiB shape (Version, ResultKey, CardID, BoutID, RecordDigest, SourceRecordDigest, InputID, ResultDigest, TxID, SHA256, Bytes, HeaderCount, BlockCount, FinalizedAtMS, ObservedTip); it is the completed export job's only payload. All other completed Result jobs have Result set and Export null. Summary.Version=1, HeaderCount1..512, BlockCount1..3, Bytes1..25165824 and exact lower-case64-hex identifiers/digests are required; FinalizedAtMS may be0 for a provisional proof. Summary immutable saved IDs, txid and current exported RecordDigest must match the same selected/publication identity, with no hidden second result payload.

Output is<=24MiB includingLF, at most3 deduplicated full blocks of<=4,000,000binary bytes (result inclusion, key2 funding and source seed anchor), and<=512 UNIQUE header entries total. Header inventory is the union of complete paths to the pinned genesis for the current captured tip, original result Finality tip if present, original source Card finality tip and every required full block. Sort height then displayed hash, deduplicate shared prefixes, and preserve all historical tip identities. Full blocks are fetched by their exact retained hashes, never substituted by current blocks at the same height. The retained funding occurrence must be on the selected result inclusion branch with correct maturity; if wider re-inclusion moved that funding transaction, bounded export refuses unless the contracted retained occurrence proves it. This does not rebuild the transaction or erase its reconcilable history.

A provisional included result may export without claiming a checkpoint. Prepared/unincluded, missing historical data or an unprovable selected funding occurrence returns result_proof_unavailable; over-cap returns result_capacity. No truncated or partial file is a success. The producer normal-verifies<=3 selected full bodies, including witness commitments, within the existing8-work/30second action budget, and streams one block at a time. It copies bounded raw header paths structurally from the exact owned node; the independent verifier must hash every normal-PoW header. This is independently checkable public evidence conditional on the pinned candidate checkpoint/source assumptions; it does not prove global best-chain choice, full-chain UTXO validity, seed firstness from all bodies, external admission provenance, private wall clocks or production ownership.

No current ViewID/session/token/challenge/nonce, private key/config/path or helper JobID enters PublicProof. Existing original public provenance already present in immutable source receipts is retained honestly. Complete public blocks may include unrelated public transactions needed for their full Merkle/witness commitment. The current View remains mandatory to collect and disclose the file; public contents do not bypass authorization.

`Gym.ExportResultProof(viewID,saturday,boutID)` uses the existing native modal/selection guard, captures the exact saved tuple before opening the native chooser, and accepts no path from JavaScript. After a selected path passes protected-root checks, the private bridge dispatches result_export and returns the queued ResultResponse. Dialog cancellation is a no-op. The dialog mutex need not persist after ACK: a subsequent logout/reset cancels the tracked export and its final original-view guard still prevents stale publication. No normal request exposes a generic file writer.

Helper BeginResultExport captures the native destination privately; it is not in retained Job/ProofSummary or public error text. As one bounded lane job it creates one pinned-parent temporary0600 file and calls core ExportProof with that unpublished writer. Core collects source/proof bytes outside leases with a counting+SHA256 bound; helper's prepare callback Syncs/closes the temp outside leases and returns only the short guard/exclusive-link/directory-Sync callback. Core then reacquires the original View and exact source/result/card/incarnation/conflict fence before invoking it. No large write/proof/engine/RPC occurs under the final lease. Never overwrite an existing destination. Remove only the task's own unpublished temp on failure; after possible final link/Sync, uncertainty is outcome_unknown and cannot claim no file exists. Successful Summary.Bytes/SHA256 cover the exact full canonicalJSON+oneLF, not a metadata-only subset.

The proof is never returned through the256KiB pipe, placed in the selected transcript cache or retained by the webview. Transient private file/block work has explicit bounds separate from the unchanged aggregate1MiB presentation cache. Tests may choose an explicit fixture temporary destination through the trusted native operation without claiming chooser/GUI acceptance. Historical data retrieval may start only the same explicit action-owned zero256 node path; an alternate gym cannot borrow a hidden publisher session/chain and receives a typed unavailable/offline response where required data is not locally available through its own authorized path.

## 9. Acceptance gates and finite remaining engineering items

Before freeze: close independent maximum-shape cache/frame/root-marker/proof formulas and exact lifecycle review; pin both final contract bytes. Core ActionResult/ReadResult, ResultKey, proof schema/API/header union, original capability and source-conflict/rebroadcast semantics are aligned. Normal test execution belongs after implementation authorization, not before contract freeze. These are engineering contract items, not player-policy questions. No runtime or expected-vector authoring follows from this draft.

Required evidence after authorization:

- Independent immutable reference for key2/script/domain/commitment/signatures/transactions/IDs and state/reorg/codec/transport semantics before Go comparisons. No runtime-derived expected values.
- Actual original account/parent authority and500ms advancing-clock metadata at empty and maximum populated boundaries; shorter caller deadlines preserve refusal/no payload. Separate race correctness from normal timing. Complete inherited256-Bout plus result-capacity startup below30seconds, cancellation publishes no partial controller/index.
- Real node-only normal-PoW processes through native Publish/Check/Finalize, unchanged accepted counts, no mining job/server/worker/wallet signer; mining Start handoff, busy backup/export, Stop/logout/reset/expiry/Close, stale owner cleanup and borrowed-mining cancellation. Actual pair cleanup and no residual listeners/processes.
- Purpose separation key1/key2, wrong script/carrier/chain/signature refusal, one coin/two Result operations, synced reservation/raw-tx/dispatch crash and lost ACK, reopen/explicit exact replay without replacement. Seed's prior raw transactions/reservations unchanged.
- Genuine saved Bout publication with immutable source/input/result/attribution, pre-checkpoint loss and same-transaction re-inclusion, depth59/60/61, captured-tip replacement before attestation, first certificate preservation, post-checkpoint crossing conflict and later reappearance. Source conflict remains visible without rerunning or modifying the result.
- v8 real SIGKILL prepared/created/ready, both fixture preservation, old-route refusal, retained unknown partial state, initialized-empty resume, ready-missing refusal and lock/drain cleanup.
- Full public proof export and independent normal-PoW/transaction/signature/checkpoint/source/result verification with explicit conditional-authority limits; negative semantic mutations, cap, cancellation/view-loss/path/symlink/exclusive publication tests. No private material in export or logs.
- All old family normal regressions after shared lane/cache/process changes; targeted race for new authority/lifetime/index logic. Normal release excludes clock/cheap-PoW hooks. Exact clean package exercises absent/ready/refused states/restart and keeps all prior assertions. Scoped-clock successful closed publication is separate from real-calendar package coverage.

Actual chain work must use a genuine healthy saved source or a later Saturday on a cold copy while preserving the old Set47 conflicted history. It must use the shipped node-only native flow, not substitute Set47's test-only core preparation for the new publication path. Explicit isolated fixture funding/mining in a harness remains separate from production code. No public-chain mining, real keys or GUI bypass is authorized.

D74's full joint review with Mike, production account/ownership/signer authority, ordinary product GUI/portrait/chooser/accessibility validation and awards/record timing remain uncompleted by this contract. The only genuine future policy question identified by the scope memo concerns when permanent professional records/XP become effective; Set48 does not write them and therefore does not require that decision.

## Source anchors inspected

- `slug/BRAWLCHAIN_SPEC.md` §14/D62; `set48-scope-recommendation.md`; `set48-acceptance-outline-draft.md`; `set48-publication-design-audit.md`.
- `slug/gymaccount/types.go` and `sessions.go`: closed capabilities, held/fresh restrictions and synchronous WithSession recheck.
- `slug/desktop/accountpilot/manifest_controller.go`: original session/child epoch guard, Start/Stop/reset/Close; `bout_controller.go` and `bout_files.go`:25second startup and all-writer v7 preservation.
- `slug/desktop/pilot/manifest_controller.go`, `manifest_account.go`, `manifest_node.go`, `manifest_projection.go`, `manifest_backup.go`: finite Prepare versus mining, node-only primitive and tracked competing operations.
- `slug/desktop/pilot/seed_controller.go`/`seed_lane.go`: actual Cmd/Done borrow, one global lane, fixed30second action, cancellation/join and reciprocal cache; `bout_types.go`/`bout_controller.go`: closed selected transcript API and distinct250ms metadata budget.
- `slug/desktop/pilot/manifest_export.go` and `slug/desktop/manifest.go`: original-view native chooser, protected paths and exclusive helper file publication.
- `slug/gymseed/types.go`, `slug/gymbout/types.go`, `slug/gymcard/bout_types.go`, `slug/seednode/types.go`: existing finite envelopes/source attribution and purpose-specific carrier boundary.

## Appendix A. Closed transport paths and bindings

Let R be `record.record` or `job.result.record`, S be outer `status`, `record.status` or `job.result.status`. No transcript appears in these paths. The only arrays are:

- `snapshot.pin.source_pin.source_pin.source_pin.source_pin.participants`: exactly2 (Result→Bout→Card→Seed→Event).
- `R.statement.statement.source.source.corners`: exactly2, original A/B order. The same shape under nonnull `R.equivocation.statement.source.source.corners` is exactly2.
- `R.scan_restarts` and `R.prior_inclusions`:0..16.
- `R.inclusion.anchor.merkle_branch`, `R.prior_inclusions[*].inclusion.anchor.merkle_branch`, `R.finality.inclusion.anchor.merkle_branch`:0..20 when their nullable parents exist.
- `R.conflicts`:0..8.

All other inherited DTOs are named objects/scalars; unknown arrays fail closed. These required arrays are nonnull, including empty arrays. Nullable Inclusion/Publication/Checkpoint/Finality/Equivocation/Tip and conflict evidence arms follow exact core semantics; a null permitted parent does not license extra fields. Field-specific bounds for Member/Owner/Assignment/Condition/Bout/SavedBout/Event and checkpoint are the unchanged source contract limits, not a generic string/int maximum. Eligible corner conditions preserve XP0 structural invariants, class, original birth and cutoff bindings; parser does not regenerate them. Source summary winner/method/endRound/endTick must have the exact frozen engine enum shape and match saved source summary in the returned Status when present.

Only signed sentinel paths `R.cursor.height`, `R.cursor.complete_to_height`, `R.scan_restarts[*].previous.height`, `R.scan_restarts[*].previous.complete_to_height`, `S.scan_height`, and `S.scan_tip_height` permit-1; all other wire numeric fields are nonnegative and obey their narrower actual bounds. No full Manifest portrait or engine form_q is transported, so no inherited negative jitter exception is needed. Lexical Boolean allowed only at exact Boolean fields.

Pure transport validation computes StatementID and mutable RecordDigest using core's exact domain/LP/canonical field order and checks the signature against the fixed candidate result key. It binds ResultKey to trusted Pin/CardID/BoutID, statement's source identities/time/summary to S.Source.Saved, and Status.SourceRecordDigest/InputID/ResultDigest to the signed saved tuple. The Result Record is mutable evidence: current cursor/generation/inclusion/observation may advance; immutable statement, saved source tuple, first rawtx/input, first accepted certificate and historical Finality cannot be replaced. Core remains authority for actual chain/source truth; the wire never imports a trusted source.

Full Action/ReadResult validates Status.RecordDigest against its same Record, phase/count/depth/tx/inclusion/finality relationships and explicit source overlay. A standalone Status may expose only the relationships actually present: do not infer missing full-record identity or declare a source healthy from incomplete fields. A different source or result conflict cannot be made usable by an older cached Job. Snapshot and all current wrappers remain bound to the original full View and store; the proof file itself contains none of that transient authorization.

Read-only navigation: [D62](/Users/taytrrip/slug/slug/BRAWLCHAIN_SPEC.md:151), [capability freshness and hold](/Users/taytrrip/slug/slug/gymaccount/sessions.go:149), [parent original-view guard](/Users/taytrrip/slug/slug/desktop/accountpilot/manifest_controller.go:247), [finite Prepare](/Users/taytrrip/slug/slug/desktop/pilot/manifest_account.go:25), [node-only primitive](/Users/taytrrip/slug/slug/desktop/pilot/manifest_node.go:9), [current borrower unregister](/Users/taytrrip/slug/slug/desktop/pilot/seed_controller.go:227), [Start/Stop transition](/Users/taytrrip/slug/slug/desktop/pilot/stable_controller.go:342), [exclusive export publication](/Users/taytrrip/slug/slug/desktop/pilot/manifest_export.go:68). These links describe the inspected Set47 basis, not already implemented Set48 behavior.
