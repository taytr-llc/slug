# Set48 core contract draft: candidate result publication

**Status: proposed engineering contract; not frozen or implemented.** Root reviews this with the separate helper contract and independent security/bounds review. Base is the unchanged Set47 implementation `ed3b6de7c1f46df4bbba1ec7de515d5dfce90a8c`. No reference outputs have been generated. This document does not activate production authority.

## 1. Scope and authority

One explicit action publishes the outcome of one already saved C1 bout. The unchanged Set47 record, original noon inputs, engine result, source digests and BoutID are immutable. The new journal records a signed candidate statement, one fixed publication transaction, canonical inclusion observations and independent result finality. It never calls the fight engine, awards a professional/exhibition record, XP, training, ranking, title, prize or ownership, or expands the two-fixture zero-professional-record C1 matcher.

D62 is the rule: the saved candidate outcome remains visible immediately, while its publication state stays separately absent/prepared/uncertain/included; it is finalized only with the separately pinned signed result checkpoint and 60 descendants **of the result carrier's block**, excluding that block. Seed A+1 and its earlier finality do not finalize a result. Report-only checkpoints never enter node consensus or select a replacement branch. D77's fixed seed and following-Saturday incident boundary remain source facts, not a newly invented deadline for result publication.

This is a fixture authority on the existing immutable zero256 owned chain. The statement and result checkpoint use separate published fixture Ed25519 domains. The result fund is the fixed published secp256k1 scalar **2**, native P2WPKH, on that same chain. Seed key1 funding and its reservation schema remain unchanged. There is no key, destination, fee, endpoint, header, statement or arbitrary transaction input in the untrusted UI. No automatic funding, transfer, mining, broadcast, finalization or engine run occurs on login/poll/read.

The trusted process-owner factory, closed source APIs and original account leases establish in-process authority. Published fixture signatures are reproducible conditional evidence, not protection against someone who knows these public test keys. There is no official league signer, public production announcement feed or transferable ownership authority.

## 2. Small concrete component boundary

New package `gymresult` owns one result journal per parent root. New concrete `seednode.ResultPublisher` borrows the exact same zero256 primary/peer process pair as the old publisher, with a purpose-fixed key2/carrier/checkpoint policy. It never owns or stops the nodes. Old `seednode.Publisher` remains seed-only. Sharing private owned-node/RPC/proof mechanics is allowed; exporting a generic key/carrier policy is not.

The helper's new explicit network actions may establish a **node-only** owned pair without a mining/admission worker. That is a reviewed shipped helper path, not an integration-test bypass. Passive metadata never starts/connects nodes. Only the exact zero home may attach a publisher; switching gyms, Stop, expiry/reset/logout and Close cancel/join the borrow before closing its process owner. Normal Start may replace the node-only lifetime only after draining it. No source object retains a child/source/node pointer across reopen. Parent owns shared journals and closes result before bout/card/seed/event after all jobs drain.

`gymresult` retains no full Input or Result transcript. Existing Set47 reads provide those separately, with their existing authorization and immutable digests. The result statement commits their exact bytes. Independent publication evidence combines the existing exported transcript with the new result record and actual block/transaction bytes; compact inclusion observations alone are not a self-contained proof that a payload is in a transaction.

## 3. Exact new source boundary

Add these transport-independent owned types to `gymbout`. Struct order below is canonical JSON order; tags are the lower_snake_case spelling shown by field names. Times are `int64`, versions/round/tick `uint32`, IDs/string enums `string`.

```go
type ResultSummary struct {
    Winner string `json:"winner"`
    Method string `json:"method"`
    EndRound uint32 `json:"end_round"`
    EndTick uint32 `json:"end_tick"`
}
type ResultSource struct {
    Version uint32 `json:"version"`
    StoreID string `json:"store_id"`
    StoreCreatedAtMS int64 `json:"store_created_at_ms"`
    RecordDigest string `json:"record_digest"`
    SourceDigest string `json:"source_digest"`
    InputID string `json:"input_id"`
    ResultDigest string `json:"result_digest"`
    CreatedAtMS int64 `json:"created_at_ms"`
    Source gymcard.BoutSource `json:"source"`
    Summary ResultSummary `json:"summary"`
}
type ResultView struct { // owned in-process view; never an imported verdict
    Version uint32
    StoreID string
    Pin Pin
    CreatedAtMS int64
    ParticipantAccountID string
    ParticipantGymID string
    ObservedAtMS int64
    Status gymbout.Status
    Source *ResultSource // nil only if booked but not yet saved
}
```

`ResultSource` maximum20KiB. The embedded old `BoutSource` remains <=16KiB and retains its existing full pin-dependent semantics. Winner/method/round/tick must exactly equal the audited saved record's result; methods are `decision_draw`, `double_corner_stoppage` (empty winner), or `decision`, `ko`, `tko_referee`, `tko_three_knockdowns`, `tko_corner` (winner exactly one corner ProspectID); round1..3, tick1..60. `StoreCreatedAtMS <= CreatedAtMS`; saved time also >= original card creation and finalized-source time. The source's full original owner/assignment/noon facts remain candidate assignment evidence.

`ValidateResultSource(p gymbout.Pin,v ResultSource)(ResultSource,error)`, `MarshalResultSource`, `ParseResultSource`, and `ResultSourceDigest` validate only the information actually present. They can validate source semantics and summary shape; they cannot recompute the omitted full RecordDigest/InputID/ResultDigest or prove a simulation. Only projection from a concrete fully audited Set47 journal establishes that connection. No public parser accepts a trusted flag. No new result operation calls `gymbout.ValidateRecord` or `fighterbout.Simulate`.

Exact concrete source APIs:

```go
func (s *gymbout.Store) WithResultView(ctx context.Context,
  cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store,
  entries *gymentry.Store, session gymstable.Session,
  capability gymaccount.Capability, saturday, boutID string,
  commit func(ResultView, func() error) error) error
func (s *gymbout.Store) WithResultSource(ctx context.Context,
  cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store,
  entries *gymentry.Store, session gymstable.Session,
  saturday, boutID string,
  prepare func(ResultSource) (func(ResultView,func() error) error,error)) error
```

`WithResultView` permits **only** View or ResultPublish. `WithResultSource` is specifically ResultPublish and requires a saved, healthy source at both capture and final fence. `WithResultView(View)` permits historical source conflict, but never changed/missing same-incarnation source. Missing booked ID returns NotFound; booked-but-unsaved has Source=nil and cannot Prepare. A different source CreatedAtMS, changed saved digest, missing audited row/ref or mismatched full Pin refuses; it never becomes an empty successful view.

The prepared source traversal authenticates the original capability before retained row work, validates current underlying seed evidence once outside the final account lease, checks exact audited Card/Bout bytes against their existing private indices, projects an owned ResultSource outside the lease, then publishes it under the same original capability. Nested Participants/corners slices must be deep owned. No condition/name/SVG derivation, signature creation, source reentry or simulation occurs in the final lease. Existing bounded semantic seed validation is not skipped or relabelled as a cache hit.

For `WithResultSource`, acquire the Bout gate once and use an explicitly private `withResultViewHeld` traversal for both capture and final reacquisition. Never call the public WithResultView while that gate is held. Capture owned source and its digest, release every lower source gate/account lease, run consumer preparation holding **only the Bout gate** among source gates, then reacquire the complete fresh source traversal with ResultPublish and compare all source/record/creation/digest identities and health before callback. The caller may also hold its result gate. No signer/RPC runs under an account lease. Final foreign callback may perform preflighted canonical encoding/hash and one short synced batch only. Source observations must precede that callback within each invocation; no source writes afterward. Later explicitly specified transitions may reacquire the original capability. A post-write source/lease/context failure is outcome_unknown.

Required additive lower seams are purpose-fixed `gymentry.WithResultView`, `gymevent.WithResultView`, `gymseed.WithCardPreparedResultView`, and `gymcard.WithResultPreparedView`. The first two mirror the existing typed source observations but allow only View/ResultPublish; ResultPublish enforces the concrete zero256 identity. Seed's prepared callback signature is the existing `WithCardPreparedView` signature plus the closed capability argument. Card's new prepared callback receives owned `gymcard.Info` and `[]gymcard.BoutSource` outside the account lease and returns `func(gymcard.BoutView,func()error)error`. It selects exactly one date, no generic import. Existing public View/SeedPublish APIs retain their accepted capability sets and behavior. Private helpers may take the closed purpose selector to avoid duplicated logic.

Lock order is result gate → bout gate → card gate → seed gate → event gate → entry gate → original account lease → entry/event/seed/card/bout/result short mutexes as each existing callback unwinds. Never acquire an outer account lease then call a public source method. No callback reenters any held store. Close cancels then joins its gate. Preserve lifetime-first refusal before foreign mutation, caller cancellation separately, opaque foreign errors unchanged, and unknown priority after a possible sync. A completed body's final successful guard is the publication linearization; a wrapper must not retroactively replace that nil result because Close began afterward.

## 4. Candidate Pin, domains and canonical bytes

`gymaccount.ResultPublish = "result-publish"` is a new closed capability, valid only for original login sessions with enrollment and no recovery-required state. It joins the existing fresh-authentication/24-hour-hold restricted set. Parent `withCapability` applies signed-in/nonbusy checks to it, as for SeedPublish. A View lease never authorizes a new statement/signature/dispatch. This does not broaden any wallet command.

Pin fields, in canonical order:

```
Version uint32; SourceStoreID string; SourcePin gymbout.Pin;
PublisherGymID string; PublisherAccountID string; PublisherSourceAuthorityID string;
PublisherChainID string; Network uint32; Genesis string;
FloorHeight uint32; FloorHash string;
PublisherPublicKey string; CheckpointPublicKey string;
FundingPublicKey string; FundingScript string; RulesID string
```

`CandidatePin(source gymbout.Pin)(Pin,error)` derives every field from the validated source and compiled candidate constants. SourceStoreID=gymbout.ID(source). Publisher account/gym/source authority and PublisherChainID come from the same exact zero participant and `seednode.ChainIdentityFor` over the underlying event pin. FloorHeight=0 and FloorHash=Genesis. Funding public key is compressed scalar2, FundingScript=`00 14 || HASH160(compressedKey2)`. No sampled activation tip, caller floor, caller key or endpoint. Result component creation may occur after the event: the source event/seed already enforce their own pre-cutoff establishment. Persist exact source store CreatedAtMS separately and never substitute a new incarnation.

All new hashing uses SHA256, domains written as literal ASCII plus the shown NULs. `raw(ID)` is32 decoded bytes in displayed hex order (not Bitcoin wire reversal), `LE32` is four bytes, `LP(b)=LE32(len(b))||b`. Existing chain hashes/headers/transaction hashing retain their old byte contracts.

```
RulesJSON = {"version":1,"source":"saved_c1_bout_v1","carrier":"SLUGRSLT-v1","fund":"fixture_key2_p2wpkh","inclusion":"exact_local_tx","pre_checkpoint_reorg":"same_tx_reinclude","finality":"carrier_checkpoint_60","max_step_blocks":8,"max_scan_restarts":16,"max_prior_inclusions":16,"fee_satoshis":1000,"awards":"none"}
RulesID = SHA256("SLUG candidate result rules\0v1\0" || LP(RulesJSON))
StoreID = SHA256("SLUG candidate result store\0v1\0" || LP(canonical(Pin)))
ResultKey = SHA256("SLUG candidate result key\0v1\0" || LE32(Network) || raw(Genesis) || raw(PublisherChainID) || raw(SourceStoreID) || raw(CardID) || raw(BoutID) || raw(PublisherPublicKey))
ResultSourceDigest = SHA256("SLUG candidate result source\0v1\0" || LP(canonical(ResultSource)))
StatementID = SHA256("SLUG candidate result statement id\0v1\0" || LP(canonical(Statement)))
StatementSigningBytes = "SLUG candidate result statement\0v1\0" || LP(canonical(Statement))
RecordDigest = SHA256("SLUG candidate result record\0v1\0" || LP(canonical(Record)))
```

ResultKey excludes mutable observations, transaction inclusion, checkpoint signature, creation clocks and outcome digests. A different outcome for the same fixed authority/card/bout is an equivocation, not another result identity. Source record/time/digests are nevertheless all signed in the statement. StoreID is deterministic; source incarnation time is separately fenced, not rediscovered from a browser snapshot.

Publisher fixture Ed25519 seed=`SHA256("SLUG candidate result statement fixture\0v1\0")`; checkpoint fixture seed=`SHA256("SLUG candidate result checkpoint fixture\0v1\0")`. These are deliberately published fixtures, never real operator material. Statement Signature is lower-case128-hex. Checkpoint JSON is exactly `{version:uint32,network:uint32,genesis:64hex,height:int32,hash:64hex,signature:128hex}` in that order, max4096bytes; height1..2147483647. Its signing bytes are the unchanged `SLUG signed checkpoint`+NUL domain, LE32 version/network, internal-wire-order genesis hash, LE32 height and internal-wire-order block hash. Checkpoint uses unchanged `chaincfg.SignedCheckpoint` signing bytes but the **result-specific key** and the result inclusion height/hash. The old Seed key/domain and A+1 rule cannot satisfy it.

Carrier is exactly73 bytes: `ASCII("SLUGRSLT") || 0x01 || raw(ResultKey) || raw(StatementID)`. Script is exactly `OP_RETURN 0x49 carrier`. Local carrier output value0. Only the exact retained transaction is an admissible inclusion: another tx copying the carrier cannot finalize this record. Do not claim first global publication or a public equivocation feed.

Canonical JSON follows existing closed codecs: UTF8, unique known keys, no trailing data, required fields including explicit null pointers/empty arrays, canonical Go declaration-field order for output, literal bools and minimal decimal integers, no floating point/exponent/-0 for public DTOs. Object key reordering/insignificant whitespace may be accepted by Parse then canonicalized; storage bytes must equal canonical output. All new strings are bounded ASCII; inherited DTO contracts remain exact. Hashes are lower-case64-hex; signatures/headers/rawtx/payload lower-case exact-length or explicitly bounded hex. No `omitempty`. Integers <=2^53−1; timestamps1..8640000000000000 where present,0 only where specified absent. Within the compact new result family, only Cursor.Height/CompleteToHeight (including restart.previous) and Info/Status scan-height fields admit -1. The export-only full old Bout additionally preserves the exact Set47 signed fields: `bout.input.a.manifest.portrait.shoulder_jitter` and the b counterpart are -6..6; `bout.result.initial[0..1].form_q` is -100000..100000. No wildcard signed path is introduced. RPC amount parsing still permits exact numeric exponents and rejects fractional satoshis/overflow; that is separate from public DTO integer grammar.

## 5. Public result DTOs and bounds

All types are `gymresult` unless named otherwise; declaration order below defines JSON order. Fields use lower_snake_case tags, with exact acronym exceptions `TxID`→`txid` and `SHA256`→`sha256` everywhere, matching the explicit proof tags and unchanged aliases. `Tip`, `Outpoint`, `FundingCoin`, `Anchor` alias unchanged seednode shapes; `Cursor` and `ScanRestart` use unchanged gymseed v1 shapes/semantics with new domains. No imported alias is trusted authority.

```
Info{Version uint32, StoreID string, Pin Pin, SourceCreatedAtMS int64,
 CreatedAtMS int64, ResultCount uint32, FundingHeight int32, FundingHash string,
 FundingComplete bool, FundingGeneration uint32, FundingRestartCount uint32}
Statement{Version uint32, ResultKey string, StoreID string, SourceStoreID string,
 Source gymbout.ResultSource, PublisherChainID string, Network uint32, Genesis string,
 RulesID string, PublisherPublicKey string, CheckpointPublicKey string}
SignedStatement{Version uint32, Statement Statement, Signature string}
Publication{Version uint32, RawTx string, TxID string, Coin FundingCoin,
 BuiltAtMS int64, DispatchCount uint32, FirstDispatchAtMS int64,
 LastDispatchAtMS int64, State string}
Inclusion{Version uint32, Generation uint32, Anchor Anchor,
 ObservedAtMS int64, ObservedTip Tip}
PriorInclusion{Version uint32, Inclusion Inclusion, OrphanedAtMS int64,
 ObservedTip Tip, ReplacementHash string}
Finality{Checkpoint chaincfg.SignedCheckpoint, Inclusion Inclusion,
 ObservedAtMS int64, ObservedTip Tip, Descendants uint32}
Conflict{Kind string, ObservedAtMS int64, ExpectedHeight uint32,
 ExpectedHash string, ObservedHash string, ObservedTip *Tip,
 Checkpoint *chaincfg.SignedCheckpoint}
Observation{ObservedAtMS int64, ChainObservedAtMS int64, Tip *Tip,
 NodeState string, Descendants uint32, CurrentFinality bool}
Record{Version uint32, StoreID string, ResultKey string,
 Statement SignedStatement, PreparedAtMS int64, Publication *Publication,
 Cursor Cursor, ScanRestarts []ScanRestart, Inclusion *Inclusion,
 PriorInclusions []PriorInclusion, Checkpoint *chaincfg.SignedCheckpoint,
 Finality *Finality, Equivocation *SignedStatement, Conflicts []Conflict,
 ConflictOverflow bool, Observation Observation}
Status{Version uint32, StoreID string, ResultKey string, CardID string, BoutID string,
 Event gymevent.Event, Exists bool, Phase string, RecordDigest string,
 SourceRecordDigest string, InputID string, ResultDigest string,
 PreparedAtMS int64, PublicationState string, TxID string,
 ScanHeight int32, ScanTipHeight int32, ScanGeneration uint32,
 InclusionHeight uint32, InclusionHash string, InclusionGeneration uint32,
 PriorInclusionCount uint32, Descendants uint32, CheckpointPresent bool,
 FinalizedAtMS int64, ConflictCount uint32, ConflictOverflow bool,
 ObservedAtMS int64, ChainObservedAtMS int64, NodeState string,
 FundingState string, CurrentFinality bool, Source gymbout.Status}
ActionResult{Version uint32, Record Record, Status Status}
ReadResult{Version uint32, Record Record, Status Status}
```

No new global History API. Existing Card/Bout date/selection surfaces locate records; one Status/Read selects date+BoutID. Status includes the fresh exact original source status, not a merged cached observation. Source status can be healthy while the result's own conflict is latched. Result `Phase` priority is `conflict`, `finalized`, `provisional`, `prepared`; absence is `absent`. Conflict includes a currently unusable existing saved source or retained result conflicts/equivocation/overflow. Provisional is a result-publication category covering at least one retained possible dispatch or canonical inclusion; dispatch_unknown must be labelled possible/uncertain publication, never confirmed submitted; prepared means the signed statement exists but neither happened. Raw tx built without dispatch is still prepared. Finality remains historically present through offline/shallow above-inclusion reorganizations; do not imply last-chain eligibility is timeless.

Publication.State: `built`, `dispatch_unknown`, `submitted`. Status.PublicationState is `none` exactly when Publication is null (including absent/prepared-without-transaction), otherwise the exact retained Publication.State. Status.TxID is empty iff Publication is null, otherwise the exact retained Publication.TxID. First/last dispatch0 iff DispatchCount0; count bounded by 4294967295 (uint32 maximum) without wrap; another dispatch at that count refuses capacity. An explicit retry increments it under a new original fresh/unheld ResultPublish fence and sends only the original raw bytes. Reconcile never retries. NodeState: `unknown`, `online`, `offline`. FundingState: `unknown`, `scanning`, `available`, `waiting_for_funds`; a retained transaction uses its reservation rather than selecting again. Absent Status uses empty record/digest/tx/anchor strings and zero timestamps/counts, scan heights-1, source state still exact; no result row allocated. Existing source NotFound/unsaved is a typed source-incomplete refusal for new publication, not evidence of an empty completed fight.

Status.ObservedAtMS is authorized local observation time. ChainObservedAtMS advances only after actual owned-node validation. CurrentFinality is eligibility **at that last chain observation**; historical Finality does not disappear because a later tip has fewer than60 descendants or a node is offline. Conflicts remain visible and cannot be cleared by a later healthy observation. No new result incident/deadline field is inferred from D77.

Caps:256 result records,128 key2 inventory coins,256 permanent reservations,8 distinct full blocks/Argon checks per publisher attachment operation,16 scan restarts per record and16 funding restarts,16 prior inclusions,8 compact conflicts plus one first equivocation. Refuse further new transitions at a full evidence cap, preserve existing bytes and expose capacity; never prune/replace evidence to continue. ConflictOverflow may latch without storing a ninth full conflict.

Bytes: Pin32KiB; Info36KiB; marker36KiB; metadata64KiB; ResultSource20KiB; Statement24KiB; SignedStatement25KiB; Publication4KiB; rawtx1024bytes; Anchor2KiB; Inclusion3KiB; PriorInclusion4KiB; ScanRestart1KiB; Conflict2KiB; Record192KiB; Status16KiB; ActionResult/ReadResult224KiB; wire response256KiB including LF. Maximum JSON container depth20, exact path-specific array caps, no generic8192 limit where a smaller inherited scalar bound applies. Record total cap takes precedence over summed optional caps. Independent formula review must prove realizable maximum shapes before freeze; these are draft finite engineering caps, not a league limit. New family metadata deadline proposed500ms, old Bout250/Card500 and all old wire caps unchanged. Measure full original-account and parent before/after rechecks, advancing wall clock and capacity. No race-test latency claim.

## 6. Public APIs and action semantics

```go
CandidatePin(source gymbout.Pin) (Pin,error)
ID(Pin) (string,error)
RulesID() string
ResultKey(p Pin,cardID,boutID string) (string,error)
StatementID(Statement) (string,error)
Carrier(Statement) ([73]byte,error)
RecordDigest(Pin,Record) (string,error)
Create(ctx context.Context,path string,pin Pin,bouts *gymbout.Store) (*Store,error)
Open(ctx context.Context,path string,pin Pin,bouts *gymbout.Store) (*Store,error)
ResumePrepared(ctx context.Context,path string,pin Pin,bouts *gymbout.Store) (*Store,error)
(*Store).Info(ctx context.Context) (Info,error)
(*Store).Close() error
```

For each public DTO above provide `ValidateX(p Pin,v X)(X,error)`, `MarshalX(p Pin,v X)([]byte,error)`, `ParseX(p Pin,raw []byte)(X,error)`; Pin uses no separate p. StatementID validates standalone statement structure/signing-domain inputs; pin-dependent validation verifies expected full identity. All return owned copies. Signature verification, bounded pure condition/source validation and canonical hashing are allowed outside leases; no signing, RPC, PoW or engine in codecs/Open.

Every following store action has the common prefix:
`(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday,boutID string)`.

- `Prepare(common) (ActionResult,error)`: ResultPublish; no node/funding. Require saved healthy source; sign one exact statement outside account lease, then guarded sync. Existing same statement replays original bytes/time and does not sign again. It may not replace a prior statement. No actual result-finality claim.
- `Publish(common,p *seednode.ResultPublisher) (ActionResult,error)`: ResultPublish. It **does not implicitly Prepare**; missing record returns `result_not_prepared`. The helper exposes one explicit composite publish job: Prepare-if-absent, then Publish under the **same original session**, with the known preparation state separately recoverable. No polling or automatic continuation dispatches. Known waiting-for-funds/scan-incomplete after an acknowledged Prepare remains a definite refusal with a retained prepared record; a successful Publish followed by response/borrow-close/final-view publication failure is unknown; a possible unacknowledged write/dispatch is unknown.
- `Reconcile(common,p *seednode.ResultPublisher) (ActionResult,error)`: View, held-compatible, never creates a statement, signs, spends, resubmits or attests. It may sync funding/scan/inclusion/conflict/finality observations of the selected prepared result. Missing record returns not_prepared; Check only reconciles an existing prepared record and never prepares one. The Publish job may explicitly prepare before its Publish step; that does not authorize a Check-side preparation.
- `Attest(common,p *seednode.ResultPublisher) (ActionResult,error)`: ResultPublish, source healthy. Verify exact canonical result inclusion and captured tip with>=60 descendants, obtain purpose-fixed fixture checkpoint outside lease, then guarded sync. Retain first exact matching checkpoint bytes; an alternative valid signature never overwrites it. Existing finalized matching record is an authorized no-op, not another signature or finality time.
- `Status(common) (Status,error)` and `Read(common) (ReadResult,error)`: View. No node/sign/engine; one current concrete source traversal plus bounded own row read. Status absence is valid only when both record/index are absent; Read absent returns not_found. After source conflict historical bytes remain visible with fresh conflict overlay. Closed/unavailable/changed source never becomes healthy cached disclosure.
- `ObserveCheckpoint(common,c chaincfg.SignedCheckpoint)(ActionResult,error)` and `ObserveEquivocation(common,s SignedStatement)(ActionResult,error)`: View, typed core evidence-only APIs, no browser import/normal helper command and no trust in caller verdict. Validate exact separately pinned public key/message/source/result identity; conflicting evidence only records a conflict, never chooses a branch or result. They operate only on an existing record. See checkpoint acceptance below.

Nil context/source/publisher/required callback or malformed selection refuses before effects. An actual previously bound publisher going offline may produce an authorized offline observation; a missing capability is not simulated chain evidence. All common source handles are exact concrete installed instances. New writes require monotonic clock and original source time lower bound; rollback refuses. Final guard samples/checks just before Sync. No invented timestamp establishes public inclusion time.

`seednode.AttachOwnedResult(ctx,binding OwnerBinding)(*ResultPublisher,error)` uses the existing exact Cmd/config/PID/Done/canonical-home proof and closed topology: primary nolisten connects owned peer listener; peer listens and has its one generated unused loopback connect target. Same actual peer inventory, no proxy/redirect/retry/public peer or configendpoint constructor. `Identity()` returns the owned in-process `ResultIdentity` below with fixed Purpose `candidate_result_v1` and exact key2 funding public key/script; `Close` cancels/joins borrow only. Expose `Observe`, `CanonicalHash`, `Block`, `Unspent`, `BuildPublication(ctx,[73]byte,FundingCoin)`, `SubmitPublication(ctx,[73]byte,FundingCoin,[]byte)`, and `AttestInclusion(ctx,Anchor)(chaincfg.SignedCheckpoint,Tip,error)` with the analogous old method types. Build/Submit accept only SLUGRSLT and exact key2. Every selected full block on the new ResultPublisher.Block/AttestInclusion/ExportBlock paths must additionally pass the existing `blockchain.ValidateWitnessCommitment` (or exact purpose-private equivalent), as well as normal Argon/header and non-witness Merkle checks. This is result-purpose-only validation; old seed behavior remains unchanged. Exact retained full-raw transaction equality is not established by a non-witness TxID Merkle path alone. AttestInclusion verifies the exact result tx/body/payload, inclusion and captured-tip membership; no A+1. No method accepts an arbitrary fee, key, script policy, target header or checkpoint signer.

Exact `seednode` API spellings (in-process identity has no JSON contract):

```go
type ResultIdentity struct {
    Chain ChainIdentity
    Purpose string
    FundingPublicKey string
    FundingScript string
}
func AttachOwnedResult(ctx context.Context, binding OwnerBinding) (*ResultPublisher,error)
func (p *ResultPublisher) Identity() ResultIdentity
func (p *ResultPublisher) Close() error
func (p *ResultPublisher) Observe(ctx context.Context) (Tip,error)
func (p *ResultPublisher) CanonicalHash(ctx context.Context,height uint32) (string,error)
func (p *ResultPublisher) Block(ctx context.Context,height uint32) (*wire.MsgBlock,error)
func (p *ResultPublisher) Unspent(ctx context.Context,outpoint Outpoint) (*UnspentOutput,error)
func (p *ResultPublisher) BuildPublication(ctx context.Context,carrier [73]byte,coin FundingCoin) ([]byte,error)
func (p *ResultPublisher) SubmitPublication(ctx context.Context,carrier [73]byte,coin FundingCoin,raw []byte) (string,error)
func (p *ResultPublisher) AttestInclusion(ctx context.Context,anchor Anchor) (chaincfg.SignedCheckpoint,Tip,error)
```

The two export-only adapter signatures are in Appendix B. All new adapter errors reuse the existing seednode typed errors; gymresult maps them to its own closed vocabulary by identity. ResultPublisher is not assignable to old Publisher and accepts no externally supplied private key or arbitrary signing policy.

## 7. Transactions, funding and bounded scans

Transaction is the old normal version2 one-input native witness shape with fixed fee1000 satoshis, output0 zero-valued exact result carrier and output1 exact key2 change >=294. Sequence/max locktime/sighash use unchanged seed transaction recipe. Input value>=1294, positive exact amount <=MaxSatoshi; confirmed canonical coin and normal coinbase maturity100 at prospective spend height, activation height62 reached. Retained raw size<=1024; txid is nonwitness doubleSHA256, raw digest/size bind full wire/witness bytes. Build/Submit reject key1 inputs/change and wrong purpose even if otherwise valid. Private signing may use public scalar2 only. Audit validates the retained transaction and signature without recreating/signing it.

Inventory is a bounded contiguous genesis-inclusive scan of actual full canonical blocks: remove any inventory outpoint spent by each transaction before adding its positive matching key2 outputs, in block transaction order. Skip zero matching outputs, never turn them into invalid positive FundingCoins. Do not use gym wallet listunspent; node listunspent is unimplemented and no address index is assumed. Funding completeness means the captured tip was fully scanned and final membership checked. At Publish capture tip, require exact FundingCursor height/hash/completeness at it, then deterministic lexicographic displayed txid + numeric output-index selection among mature confirmed unreserved canonical coins. A normal extension requires further explicit scanning, not a false complete flag or a changed first coin.

Result and funding cursors each use generation0/height-1/hash empty/complete_to_height-1 initially. New domains:
`SHA256("SLUG candidate result funding scan\0v1\0"||raw(StoreID)||LE32(generation)||LE32(0)||raw(Genesis))` and result scan adds raw(ResultKey) after StoreID with domain `SLUG candidate result inclusion scan\0v1\0`. Step digest=`SHA256("SLUG candidate result scan step\0v1\0"||raw(previousDigest)||LE32(height)||raw(blockHash))`. CompleteTo is the last fully completed captured tip, not an unscanned target. When the selected exact transaction is found at R, cursor may stop at R; it must not claim the captured higher tip was scanned.

One Reconcile may consume at most8 distinct verified full blocks, including current inclusion refetch. Shared heights may serve funding and selected-result scans from the same verified block; discard block bodies before another body, retain only bounded header/hash metadata. Failed attempted Argon verification consumes budget. No hidden loop over all chain history or all results. Continue on explicit bounded Check actions; unlimited steps with finite cursor/evidence capacity is allowed, false completion is not. Before a local transaction exists, Reconcile scans only funding. After it exists, scan for the **exact same raw transaction** and verify its expected output0 (not any foreign same-payload tx). Canonical byte equality includes witness; txid alone is insufficient. Input/source/statement/rawtx never change.

Sync publication+reservation in one batch before any possible send. Each result owns one permanent `u:<outpoint>` row; unknown/rejected/unconfirmed/reorged transactions retain that reservation forever. Two result actions cannot select the same input; the gate plus durable unique reservation enforces this across restart. Same-script change becomes a new eligible coin only after normal canonical confirmation. One parent result journal is the sole key2 reservation authority for its fixed owned chain; no second coordinator or alternate path is installed. Key1 and key2 inventories, validation and change are disjoint by exact script at every layer.

New raw transaction is built outside leases, then sync'd under ResultPublish and exact source fence. A second original ResultPublish invocation rechecks current source, coin/canonical evidence and capacity and syncs `dispatch_unknown` with incremented count/time immediately before transport. Release every source/account lease, check task cancellation, call Submit once. Once that dispatch intent may have synced, every subsequent unsuccessful exit (including cancellation detected before the actual send, a new source refusal, transport/decode/identity failure or ACK-state sync failure) is outcome_unknown, even if the implementation believes this invocation did not send; source changes cannot promise network revocation. Successful ACK may sync submitted in a third separately guarded invocation. No later source write inside a completed foreign callback invocation. A definitely acknowledged stored built/scan state and a later ordinary funds refusal do not become unknown merely because earlier successful work was durable.

Before each new/duplicate dispatch and Attest, freshly check the fixed saved source seed target's height/hash on the same owned chain, before and after the relevant expensive work, plus full current source fence at final sync. Source target mismatch records a result source-chain conflict and blocks publication/signing; it never rewrites the seed journal. RPC failure is unavailable/offline, not proof of conflict. Current source conflict also blocks **all** new Prepare, every rebroadcast (even same tx), and Attest. View Reconcile/read remains possible. Source became conflicted after an already possible dispatch: preserve tx/history/uncertainty, never reseed/rebuild.

## 8. Reorgs, checkpoint and evidence priority

Before any accepted result checkpoint, losing current Inclusion's canonical height/hash appends PriorInclusion with original exact evidence, actual observed orphaning time/tip/replacement hash, then clears current Inclusion and restarts the selected exact-tx scan from genesis in the next generation. Retain previous cursor in ScanRestarts. Only the same raw transaction may reappear; no replacement tx, fee bump, input reselection, new statement, changed outcome or seed. Ordinary extension does not restart. A verified captured tip below the expected inclusion height represents observed absence with ReplacementHash/ObservedHash empty; RPC failure is not such evidence. The captured tip must survive the final membership bracket. A nonempty replacement hash must be the exact canonical hash at the expected height. A funding cursor reorg separately restarts funding inventory scanning while preserving all reservations and raw transactions; it is not automatically result conflict. Inventory replacement occurs under a new generation: atomically clear old inventory rows when restarting, rebuild from genesis with the new cursor, and retain every permanent reservation independently. This never makes a previously reserved input free.

A checkpoint is accepted only for the current inclusion height/hash under the result checkpoint public key and pinned network/genesis. A structurally valid certificate arriving through ObserveCheckpoint may be retained before a 60-depth observation; this locks that inclusion but does **not** set Finality or CurrentFinality. Normal Attest issues only after independently checked depth>=60. A different signed hash at the retained inclusion height is conflict evidence. Unrelated heights are invalid for this record, not a reason to pick another target. A matching second valid signature is a no-op and cannot replace the first exact certificate.

First accepted checkpoint locks Inclusion and its exact checkpoint forever. A later canonical mismatch at that height latches `checkpoint_chain_conflict`; there is no more reanchoring. Finality, if already present, retains original observed time/tip/depth/certificate/inclusion. First historical Finality is created only when matching certificate plus canonical exact inclusion plus captured tip with at least60 descendants are all independently observed in one guarded transition. Finality signature-only or borrowed seed depth is invalid. Depth59 refuses;60 admits;61 is valid. Captured tip must still be canonical at its height after work; tip replacement above inclusion invalidates that new depth observation but is not by itself a result replacement/conflict. Later depth<60 or offline never erases historical Finality.

Conflict kinds are `source_chain_conflict`, `checkpoint_chain_conflict`, `checkpoint_equivocation`, `statement_equivocation`. Compact conflicts are append-only, deduplicated by canonical semantic evidence excluding observation timestamp; retain first observation time. `Equivocation` retains the first complete different valid signed statement with the same ResultKey/authority; subsequent distinct proofs latch ConflictOverflow if full proof capacity is unavailable. Equivocation has no normal distribution feed in this slice; its typed fixture API and tests must not be advertised as monitoring all public statements. First certificate, statement, transaction, reservation and historical finality cannot be replaced by conflict handling.

For `ObserveEquivocation`, evidence validation fixes the expected Pin/network/publisher/source-store/CardID/BoutID/ResultKey and checks the statement's signature and structural claims. It does **not** require the disputed source facts, RecordDigest, InputID, ResultDigest or Summary to equal the local saved result: those differences are the evidence. They never replace the local source, statement or outcome and never authorize an action. Normal Prepare and the primary local Statement still require exact concrete saved-source equality. The source/result identity check in §6 means this fixed conflict identity for the evidence-only API, not equality of the disputed claims.

No metadata poll refreshes canonicality. Reconcile may update observations while source is historically conflicted but cannot clear latched conflict or cause implicit publishing. Normal chain selection remains unchanged. Finalized candidate history with conflict is shown as such; it is neither an official professional record nor a silently undone result.

## 9. Journal, initialization and recovery

Separate `SLUG-CANDIDATE-RESULTS.json`, application `slug-candidate-results`, version1, StoreID, complete Pin. Expected Pin comes from trusted installed Bout/source configuration, never marker discovery. Parent v8 prepared/ready lifecycle is specified in helper contract; old routes refuse unknown parent version and preserve all earlier bytes/fields. Parent ready never recreates missing result storage.

Create/Resume allow absent, real empty, exact matching marker-only, or matching init-only states under the held parent lock. Marker and durable `init` intent pin full Pin and SourceCreatedAtMS before actual CreatedAtMS is sampled and meta synced. No event cutoff creation restriction is added. Initialized empty recovery preserves original CreatedAtMS; it requires ResultCount0, initial generation0/funding height-1, LastFundingTip null, no funding restarts and no record/inventory/reservation rows. Ready Open rejects partial state. No arbitrary nonempty adoption, symlinks, unknown files/DB keys or replacement source creation time. Constructors close partial resources; cancellation joins, no orphan timeout goroutine.

DB keys: `meta`; transient recognized `init`; `r || raw(ResultKey)`; `c || raw(txid) || LE32(index)` inventory; `u || raw(txid) || LE32(index)` permanent reservation whose canonical value is `{version:1,result_key:<id>,txid:<publication txid>}`. Metadata canonical order: Version, StoreID, Pin, SourceCreatedAtMS, CreatedAtMS, ClockMS, ResultCount, FundingCursor, FundingRestarts, FundingComplete, LastFundingTip (nullable). Unknown/duplicate/oversized keys/rows, missing reverse relation, orphan reservation, reused input, changed statement identity, illegal chronology or mismatched count refuse Open. Inventory bounded128. Every retained publication has exactly one matching reservation and vice versa. No release/pruning.

Full Open performs bounded canonical/semantic/signature/transaction audit of every result row and cross-row reservation graph, but **no engine/PoW/node/signing**. It may verify compact source semantics without claiming the omitted transcript was recomputed; actual Bout store has its existing full Open audit. Establish a private <=256-entry index from audited bytes with exact canonical fields `{result_key:string,saturday:string,bout_id:string,record_digest:string}` in that order, sorted by ResultKey; its canonical array is at most96KiB. IDs are fixed64-hex and Saturday10ASCII bytes. It retains no summary, source payload or transaction. This is an encoded/accounted bound; ordinary fixed per-entry Go allocation overhead is also bounded by256, not claimed equal to encoded bytes. Status reads bounded-decode the checked row to obtain its summary. Establish this index from the full audit; publish no index until all audit succeeds. Runtime updates install the exact new raw-row digest entry only after successful Sync. Read/Status verify current bytes against that index before bounded private decode; unexpected/unindexed/missing/digest-drift rows refuse. No public trusted decoder. Index stores no transcript and no unaudited source verdict. Mutable result observations change rowdigest legitimately only via synced owned transitions.

Each Sync error is outcome_unknown and poisons the store; no further operation except Close, followed by full Open reconciliation. Never decrement counts or reservations from a guessed failed write. Cancellation/lifetime after a possible own/source clock sync similarly returns unknown without disclosing a falsely definite state. Initial input/profile/identity refusals happen before effects. Clock is monotonic across source/result observations, persisted and checked against system rollback; source observed time is a lower bound. First prepared time>=source saved/store times and result creation, publication built>=prepared, dispatch times>=built, inclusion observation>=prepared (not inferred from block timestamp), checkpoint/finality observation>=inclusion observation, all retained times<=metadataClock. Retained prior observations remain historical, never retimestamped by replay.

## 10. Error vocabulary, UI and acceptance

New errors: `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `outcome_unknown`, `result_unavailable`, `result_corrupt`, `result_capacity`, `result_clock`, `result_calendar`, `result_not_found`, `result_not_prepared`, `result_source_incomplete`, `result_source_changed`, `result_source_conflict`, `result_node_offline`, `result_waiting_for_funds`, `result_scan_incomplete`, `result_not_mature`, `result_conflict`, `result_cancelled`, `result_proof_unavailable`. Internal closed/poison/profile errors stay typed and map fatal dependency failure honestly. Wrong existing source authority is forbidden; same-Pin changed incarnation/immutable record is source_changed; no saved bout is source_incomplete. Error identity, not string matching, controls cancellation/unknown mapping.

No user-facing raw hashes/protocol/version names in the main flow. Labels may be “Publish test result”, “Check status”, “Finalize test result”; candidate banner remains. Distinguish saved but unpublished, prepared/waiting funds/scan, possible dispatch, included provisional, finalized historical evidence, current conflict and last checked time. “Check status” is explicit chain work; passive snapshot/job polling only copies owned cached values. Unknown messaging must identify whether a result transaction might have been sent versus a read-only observation might have synced. A late job may not overwrite newer selected-view/date/BoutID status. No result portrait/transcript duplicate/cache grant; existing authorized Bout viewer remains source of replay.

Before freeze: independent full-shape/canonical-order/capacity calculation; exact helper/native envelopes and new source signatures; source-hash manifest; separate fixture key2, result statement/carrier/signature/tx/checkpoint arithmetic reference plan. No Go outputs as expected values.

Acceptance: independent canonical codec/domain/key separation controls; wrong source incarnation and callback mutation; original hold/freshness/reset/logout/Close at each preparation/commit/dispatch edge; no nested lease, no engine/sign/network on metadata/Open; two results racing one key2 coin plus concurrent seed key1/result key2; rawtx/reservation/dispatch/ACK crash points with exact reopen; cancel before/after possible send; complete and partial scan/reorg/capacity controls; copied carrier in different tx refused; same tx pre-checkpoint reinclusion and checkpoint-locked crossing conflict; alternate-valid certificate preserved; depth59/60/61 and captured-tip fork; all evidence limits and corruption/reverse-index invariants; full actual-account advancing-clock metadata and parent startup at256 without retry or deadline masking. Engine-entry instrumentation must show zero additional invocation by result actions/read/audit, separate from existing Bout Open audit and actual initial Run.

Actual positive composition must preserve Set47's conflicted journals. Use a cold copy and a later genuinely healthy booked/saved bout (or an independently retained genuinely healthy earlier copy), original normal-PoW admissions and scoped clock; do not clear/relabel conflicts or derive expected outcomes from new Go. Explicitly fund key2 with normal maturity, exercise actual shipped native node-only Publish/Check/Finalize without mining/admission side effects, independently verify result tx/block/checkpoint and unchanged saved Input/Result digests. A bounded512-header export for this new exercise is permitted if justified by funding/maturity; it does not relax earlier Set47 proof gates. Normal-clock packaged app must cover real setup/refusal/restart states honestly. Public chain mining, real keys, production authority and D74 activation remain closed.

## 11. Remaining draft closure items

No gameplay question is required for this bounded candidate publication contract. Permanent professional awards and their timing are explicitly outside it. Before freeze, root/helper must finish node-only lifecycle and composite publish-job acknowledgement alignment; independent reviewer must verify finite formula bounds and the exact lower-source/proof APIs below. These are engineering closure items, not permission to implement against an unfinished draft. Any change to an accepted contract afterward needs a retained explicit addendum.


## Appendix A. Exact additive lower-source signatures

All methods below reject any capability except View/ResultPublish before effects and preserve old public methods unchanged. `WithResultSource` calls the private already-Bout-gate-held path; no self gate acquisition. Source capability propagation uses private parameterized helpers, not a caller-selectable arbitrary account capability or a nested account lease.

```go
// gymentry
func (s *Store) WithResultView(ctx context.Context, session gymstable.Session,
  capability gymaccount.Capability,
  commit func(SnapshotSourceInfo,func()error)error) error
// gymevent
func (s *Store) WithResultView(ctx context.Context, entries *gymentry.Store,
  session gymstable.Session, capability gymaccount.Capability,
  commit func(SeedSourceInfo,func()error)error) error
// gymseed
func (s *Store) WithCardPreparedResultView(ctx context.Context,
  events *gymevent.Store, entries *gymentry.Store, session gymstable.Session,
  capability gymaccount.Capability,
  prepare func(Info)([]string,func(CardView,func()error)error,error)) error
// gymcard: exactly one Saturday, <=5 privately projected booked sources
func (s *Store) WithResultPreparedView(ctx context.Context,
  seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store,
  session gymstable.Session, capability gymaccount.Capability, saturday string,
  prepare func(Info,[]BoutSource)(func(BoutView,func()error)error,error)) error
```

Every caller must compare complete nested Pin/StoreID and the persisted expected source CreatedAtMS before reading its own retained rows or returning a source projection. All Info/Pin/slices supplied to callbacks are owned copies. The gymbout layer preauthorizes through this traversal, validates its exact raw saved row against the existing Open/Run-built index outside the final account lease, and derives ResultSource using private canonical hashes from those already audited bytes. It must not invoke the public `gymbout.RecordDigest`/ValidateRecord helpers, because they simulate. Source/summary mutation by consumer cannot affect the store or later callbacks.

## Appendix B. Complete bounded public proof export

Export is an explicit View-authorized native chooser action, held-compatible. It neither prepares nor reconciles/publishes/attests, and never reruns the engine. It requires an existing result Publication and an observed retained Inclusion; absent/prepared-without-inclusion returns `result_proof_unavailable`. Provisional included results may export without pretending to have a checkpoint/finality. Finalized/conflicted records export historical evidence plus the separate observed current branch. Missing historical body/header or an unprovable selected funding occurrence returns proof_unavailable; over-cap evidence returns result_capacity. Do not substitute a current block at a historical height or call a partial bundle complete.

New export-only canonical JSON shape; no parser/import authority is added to the application:

```go
type ProofHeader struct {
    Height uint32 `json:"height"`
    Hash string `json:"hash"`
    Header string `json:"header"` // exactly80 bytes lower-case hex
}
type ProofBlock struct {
    Height uint32 `json:"height"`
    Hash string `json:"hash"`
    RawBlock string `json:"raw_block"` // full wire block, <=4,000,000 bytes, hex
}
type PublicProof struct {
    Version uint32 `json:"version"`
    Scope string `json:"scope"` // exactly candidate_result_proof_v1
    Pin Pin `json:"pin"`
    Result Record `json:"result"`
    Status Status `json:"status"`
    Bout gymbout.Record `json:"bout"`
    Card gymcard.Record `json:"card"`
    ObservedTip Tip `json:"observed_tip"`
    ResultFinalityTip *Tip `json:"result_finality_tip"`
    SourceFinalityTip Tip `json:"source_finality_tip"`
    Headers []ProofHeader `json:"headers"`
    Blocks []ProofBlock `json:"blocks"`
}
type ProofSummary struct {
    Version uint32 `json:"version"`
    ResultKey string `json:"result_key"`
    CardID string `json:"card_id"`
    BoutID string `json:"bout_id"`
    RecordDigest string `json:"record_digest"`
    SourceRecordDigest string `json:"source_record_digest"`
    InputID string `json:"input_id"`
    ResultDigest string `json:"result_digest"`
    TxID string `json:"txid"`
    SHA256 string `json:"sha256"`
    Bytes uint32 `json:"bytes"`
    HeaderCount uint32 `json:"header_count"`
    BlockCount uint32 `json:"block_count"`
    FinalizedAtMS int64 `json:"finalized_at_ms"`
    ObservedTip Tip `json:"observed_tip"`
}
```

PublicProof contains the one full saved Bout and one immutable Card record. The Card already retains the exact snapshot, signed source seed declaration, seed anchor/target and historical seed finality, so it does not duplicate a full Seed.Record. Link all saved source/card/input/result hashes exactly using private canonical byte hashing of concrete audited rows, never a new engine run. No fresh ViewID/session token/challenge, local path, config, RPC credential or key is exported. Original historical public provenance already inside immutable source receipts is retained without relabelling it a current credential. ProofSummary<=2KiB; PublicProof<=24MiB including one trailing LF, kept outside the256KiB wire and aggregate1MiB UI cache.

The header inventory is the **union of complete parent paths to genesis**, for (a) the captured current ObservedTip, (b) Result.Finality.ObservedTip if present, (c) Card.Source.Finality.ObservedTip, and (d) each required full block below. Maximum512 unique (height,hash) entries total, not512 per branch; duplicate shared prefix entries are forbidden. Sort numerically by height then lexicographically by lower-case displayed hash. Every non-genesis parent pointer must resolve to exactly the entry at height-1; the only height0 header is the pinned genesis. Tip fields exactly match retained/captured facts; do not change old finality tips to the new current tip. Higher/longer histories may fail the bounded export rather than truncate. This header inventory is evidence for independent verification, not a new chain authority cache.

Blocks are exactly the distinct retained hashes for the selected result Inclusion, Publication.Coin's original funding block, and Card.Source.Anchor's seed carrier block. At most3, deduplicated by hash, sorted height/hash. Their exact headers must be in Headers. The result block must contain the full retained publication transaction at the exact inclusion index and output0, byte-for-byte including witness, and reproduce all Anchor fields. Funding block must contain the input Outpoint.TxID/index with exact key2 script/value/coinbase facts. Seed block must reproduce the retained source anchor and signed seed payload. A foreign tx with the same payload is not a substitute. For a full funding/maturity proof, the retained funding block must be an ancestor of the selected result inclusion on that header branch and satisfy normal maturity at the result height. A wider pre-checkpoint reorg can legitimately reinclude the same tx while its funding tx also moves; this slice does not rewrite Publication.Coin or invent its new occurrence. Such a record remains reconcilable but complete export returns proof_unavailable unless the retained occurrence is on the claimed inclusion branch. This is a bounded export limitation, not permission to rebuild a transaction.

All selected full bodies are verified by the ResultPublisher against the requested **retained hash**, normal Argon header, non-witness Merkle root and the existing witness-commitment rules (`blockchain.ValidateWitnessCommitment` or equivalent exact private use). This is <=3 distinct full-block hashes within the unchanged8-work operation budget; failed hashes count. The collector fetches historical bodies by hash, so an orphan does not get replaced by the current block at its height. At most one decoded full block is live at a time; stream canonical hex into a private temporary file. No result/publication mutation is made by exporting. Raw headers for the inventory are obtained via bounded nonverbose header RPC by hash and checked structurally/for links/owned identity; the producer does **not** claim to rehash512 Argon headers in a30-second job. The independent verifier performs every supplied header hash/difficulty/link, Merkle/witness, signature and checkpoint-depth check. Raw-header collection never sets Finality or promotes unverified bytes into a trusted journal.

Purpose-fixed adapter additions (not browser RPC):

```go
func (p *ResultPublisher) ExportHeader(ctx context.Context, ref HeaderRef) (HeaderRef,error)
func (p *ResultPublisher) ExportBlock(ctx context.Context, ref HeaderRef) ([]byte,error)
```

ExportHeader requires exact supplied height/hash and, when Header is nonempty, byte equality; it returns the raw canonical80-byte encoding from the owned node by that hash, without a canonical-main-branch requirement for historical evidence and without claiming normal-PoW verification. The collector starts with independently pinned/captured tips and follows decoded parents; callers cannot provide arbitrary endpoints/keys. ExportBlock normal-verifies the full body by hash and consumes the ordinary work budget. Neither method signs/submits or changes chain selection. A final check keeps the captured current tip canonical at its original height; a normal extension is allowed. Historical tips need not be current. Owner/process/context checks remain mandatory throughout.

Concrete core entry point, common source arguments as in §6:

```go
func (s *Store) ExportProof(ctx context.Context, bouts *gymbout.Store,
  cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store,
  entries *gymentry.Store, session gymstable.Session, saturday,boutID string,
  p *seednode.ResultPublisher, unpublished io.Writer,
  prepare func(ProofSummary)(func(func()error)error,error)) (ProofSummary,error)
```

This is a trusted in-process API: unpublished must be the helper's already pinned private temporary file, never a web/network/public stream. It is not a public import or path-selected-by-browser mechanism. Authorize original View before any source read/write to that file. Hold result gate, capture exact owned result/Bout/Card rows through authenticated concrete sources, release all lower gates/leases before node retrieval or writing, and stream once with a24MiB counting+SHA256 writer. Call prepare outside leases to Sync/close the private temp and obtain a short final exclusive-link callback. Then reacquire the original View through the complete saved source fence, recheck exact result/source/card/incarnation/digests and the captured source conflict tuple; refuse changed evidence instead of mixing statuses. The final callback receives the ordinary synchronous guard and only checks/links/syncs the already prepared file. No large write, proof generation, signing, RPC or engine occurs under the account lease. The result gate uses private already-held read paths, never public self reentry. Preserve opaque callback errors and unknown after a possible final publication/link/sync.

The file is published exclusively, never overwrites an existing destination, refuses every managed application/source/binary root, rejects symlinks/path races, and is cleaned on failure before publication. Bytes/SHA256 cover the exact complete canonical JSON plus one LF. ProofSummary is bound to the original full selected View only in the helper response envelope; no fresh session material enters the file. Normal original-account authorization may observe/sync its clock even on a cached read; “no metadata work” means no result/source journal/RPC/engine work on passive polling, not no account authorization.

The proof establishes independently checkable published bytes, normal header paths, result/seed carrier linkage, full witness-bearing transaction bodies, fixed funding script/maturity, unchanged bout derivation and signed checkpoint/depth facts. It does **not** independently execute full-chain UTXO validation, prove source seed firstness from every block body, prove a wall-clock noon, establish production ownership or replace missing external admission provenance. The actual acceptance separately verifies the retained genuine public admission proofs. These limitations are explicit format semantics, not an invitation to cross-read another fixture's live registry under invented credentials.
