# Public fighter ownership and direct transfer

Adopted engineering contract, November 19, 2026. Root reviewed both chapters before adoption. D76/D83 policy is unchanged. Chapter I fixes admission, replay and concrete ownership APIs; Chapter II supplies the exact closed storage/codec grammar and final portability/unknown-prior distinctions. Chapter II is authoritative wherever an earlier description was general. Carrier and wallet economics/transaction codecs remain their separately adopted contracts. This file grants no production key/authority, automatic migration or completed qualification claim.

# Chapter I — admission, replay and ownership APIs

## 1. Fixed boundaries

Fresh accepted admission establishes immediate initial ownership in the existing admission batch. Public signing publishes that already established fact. It neither delays ownership nor creates another birth. Complete public reconstruction requires the entire declared authenticated admission prefix, complete canonical traversal through its named chain frontier and the entire declared signed checkpoint-publication prefix. Local assignments, cached flags, selected transaction proofs and a recovery phrase do not establish completeness.

`registry` owns the fresh guarded admission/publication mode and its same-DB outbox. `fighterownership` owns public validation, replay, effective states and cutoff resolution; it imports registry types, never wallet or desktop. Concrete wallet owners enforce original `FighterTransfer` and shared coin exclusions. Public replay does not require a hosted session. New consumer constructors preserve immutable BirthAssignment separately from current EffectiveState and historical CutoffRecord/corners. Old receipt, stable, event, history and inspection Verify APIs retain exact behavior.

There is no migration or public attestation retrofit for existing unsigned authorities. The new candidate uses independently configured fixture identities/keys and a fresh guarded root/source. These signatures are reproducible public-fixture evidence, not a deployed operator authority. No real checkpoint private key is created or read.

## 2. Canonical types and byte rules

Closed ordered JSON, no whitespace, duplicate/unknown fields or trailing value; integer numbers only; lowercase hex; arrays never null. New uint32 counters reject overflow, JSON-safe timestamps are `0..253402300799999` milliseconds. Zero sequence and all-zero link are allowed only for an explicitly empty prefix. IDs are 32 bytes represented by 64 hex; random identities are not all zero. Compressed gym keys are 33 bytes; Ed25519 keys 32/signatures 64. Inherited ManifestRecord and SignedCheckpoint nested objects keep their exact old canonical codecs/signing preimages. No parser accepts a claimed source pin as its own trust input.

New digest function is `SHA256(domain || 00 || "v1" || 00 || LE32(len(canonical_object)) || canonical_object)`, with the ASCII domains below. Signed statements sign this digest with Ed25519, then receive a separate ID over the complete signed object. IDs are never inside their own preimage. Carrier intent/envelope/TransferID use **only** the root's adopted compact carrier codec, not a JSON-equivalent second signature scheme.

Ordered schemas:

| Type | Fields |
| --- | --- |
| `Pin` | `version, source, publication_id, admission_key, publication_key, cutoff_key, checkpoint_key, rule_id, scan_origin, limits` |
| `scan_origin` | `height, hash`; exactly pinned genesis at height 0 |
| `limits` | `admissions, publications, carriers, checkpoints, cutoffs, headers, logical_bytes` |
| `AdmissionStatement` | `version, pin_id, admission_sequence, previous_admission_id, record_digest, record, signature` |
| `PublicationEntry` | `version, pin_id, sequence, previous_id, attested_at_ms, kind, payload, signature` |
| admission payload | `admission_sequence, admission_id` |
| checkpoint payload | `checkpoint`; exact inherited SignedCheckpoint |
| chain payload | `height, hash, previous_chain_entry_id` |
| cutoff payload | `cutoff_id` |
| `InitialStateEvidence` | `version, pin_id, prospect_id, admission_id, owner_public_key` |
| `FinalStateEvidence` | `version, pin_id, prospect_id, prior_state_id, transfer_id, checkpoint_publication_id, sixtieth_descendant_height, sixtieth_descendant_hash, owner_public_key` |
| `EffectiveState` | `version, pin_id, prospect_id, state_id, owner_public_key, birth_admission_id, generation, evidence` |
| `HeadRef` | `version, pin_id, admission_sequence, admission_head, publication_sequence, publication_head, chain_height, chain_hash, replay_digest, finalized_prefix_id, conflict_id` |

`Pin.source` is the entire original ManifestAuthorityPin, including descriptor. `rule_id` refers to the separately frozen canonical rule recipe; it is not a self-referential hash of a document containing its own ID. Fixture key bytes and the rule recipe are input-lock material, not generated from implementation. `fighterownership.Pin` aliases the neutral `registry.PublicManifestPin` declaration to avoid an import cycle; registry performs structure/source/config checks and core additionally requires the exact supported rule recipe.

Domains: `SLUG fighter ownership pin`; `SLUG fighter ownership admission record`; `SLUG fighter ownership admission statement`; `SLUG fighter ownership admission id`; `SLUG fighter ownership publication statement`; `SLUG fighter ownership publication id`; `SLUG fighter ownership state`; `SLUG fighter ownership cutoff statement`; `SLUG fighter ownership cutoff id`. Admission and publication prefixes start at 1 and have exact previous-ID links. Empty head IDs are zero only when count 0. Publication kinds are exactly the four listed shapes. `attested_at_ms` is the source's retained monotonic sample; it never proves worldwide publication/creation time. Prefix membership, not a claimant timestamp, controls checkpoint eligibility.

Every admission statement includes the complete original receipt, exact original sequence and prior admission ID. Verify source/descriptor, original job and gym authorization, normal work and recomputed static binding. Retain all immutable birth/appearance fields. Issuer signature does not permit replacing the underlying proof with its cached Verified struct.

## 3. Fresh registry guard and atomic outbox

Keep old schema5 receipt/marker bytes. Add immutable `SLUG-MANIFEST-OWNERSHIP-V1.json` with ordered `{application,version,pin,authority_id,publication_id}` and a required same-DB namespace. Application literal is `slug-manifest-ownership`; version 1; repeated IDs must match Pin. The parent durably preselects exact identities before creation. New constructor refuses nonempty unknown/partial directories except one explicitly audited empty setup state under that retained intent. Ordinary old Open/Record paths cannot create or adopt it.

The DB begins with a required `ownership/guard` row containing the same canonical guard and `ownership/meta` row. Old schema5 audit already rejects unknown keys, giving downgrade refusal before mutation. New open audits the complete old key grammar plus these exact new keys, not a broad allowed prefix. Its metadata is ordered `{version,pin_id,accepted_sequence,signed_admission_sequence,admission_head,publication_sequence,publication_head,chain_entry_id,checkpoint_count,cutoff_count,logical_bytes,clock_high_water_ms,pending_publication_id}`. Zero heads/counts and empty pending ID are the exact initial state. Original registry LastSequence equals accepted_sequence.

New keys are ASCII prefix plus BE32 counter or raw32 ID:

- `ownership/outbox/`+BE32 admission sequence → `{version,admission_sequence,prospect_id}`.
- `ownership/admission/`+BE32 → complete AdmissionStatement.
- `ownership/publication/`+BE32 → complete PublicationEntry.
- `ownership/checkpoint/`+raw32 canonical checkpoint-object digest → first public sequence.
- `ownership/cutoff-intent/`+raw32 event ID → exact CutoffIntent; `ownership/cutoff/`+raw32 event ID → complete signed CutoffRecord.
- `ownership/pending` → one exact prepared publication intent, or absent when metadata pending ID is empty.

Do not delete accepted outbox references after publication; they are bounded permanent crosslinks. The publisher cursor identifies what remains. Every complete admission has exactly one original row/index/outbox; every signed admission through signed_admission_sequence matches that row and one public admission reference. There are no gaps or silently skipped foreign-gym rows.

Both fresh write paths, `CommitManifestAdmission` and `RecordManifestJobChecked`, must call the same private new-mode batch hook. The original account recheck, accepted clock, expiry/eligibility and normal prepared-proof behavior remain unchanged. The hook reserves capacity first and adds the small outbox/meta change to the **same sync batch** as the existing record, source sequence index and metadata. On success initial ownership exists. On uncertainty poison the store and retain the exact possible outcome; close/reopen audits it. Historical retry returns exact old bytes/crosslink, not another accepted timestamp or outbox sequence.

No full-record hashing, public signing, network or lock wait is added under the account lease. The existing bounded record encoding/write remains local. The private hook adds only fixed-size metadata/reference work. New-mode admission through an uninstrumented old entrypoint must refuse rather than bypass publication.

## 4. Concrete publisher and public reads

`ManifestPublication` is a concrete borrowed owner tied to the exact live ManifestStore and expected Pin. It has one fail-fast admitted work slot and registered lifetime; no independent DB owner/close and no arbitrary signer callback. The fixture constructor supplies only the locked published fixture key for each role. Signature bytes are private scratch until their exact committed public row exists; no pending signature accessor/IPC output.

The next admission to publish is signed_admission_sequence+1. Outside all account leases/gates, read and fully verify its exact immutable record, hash it, form/sign the admission statement, then stage the next publication body. The source's single pending publication intent fixes `{version,pin_id,sequence,previous_id,attested_at_ms,kind,payload,admission_id}`; admission_id is empty for other kinds. Its canonical ID uses domain `SLUG fighter ownership publication intent`. It is sync-persisted before externally exposing anything. The immutable body/ref is bounded, its expected original-store revision retained, and subsequent public entries cannot pass it.

Sign the public entry outside gates. One short original-store CAS batch commits the statement when applicable, publication/index/meta and deletion of the pending row. It checks exact pending bytes, sequence/previous head, original owner and no poison/close. A crash before completion resumes the exact durable intent; deterministic signing reproduces bytes. A failed/uncertain sync poisons and requires original-store reopen/audit. It never samples a new timestamp for an uncertain old intent. Signature preparation cannot itself advance a public frontier.

Checkpoint ingestion validates the inherited signature against Pin.checkpoint_key and exact network/genesis outside gates. Canonical equal checkpoint bytes return the original publication sequence; conflicting hash at a finalized height is retained conflict, not overwritten. Public canonical proof/replay determines whether a signed checkpoint matches chain, independently of parsing its signature. Source publication does not change node consensus.

Chain entries attest observed exact node frontiers for public progress; **they are not used as a cached substitute for the noon node bracket**. Cutoff references are appended after the exact cutoff is signed; signing can be delayed after the boundary intent. Accepted/publication counters are both exposed. A live immediately accepted fighter may say `public_evidence_pending`; ownership itself is not pending. Portable transfer preparation requires its signed initial-state evidence.

Public bounded read seams take an independently expected Pin and named immutable prefix: `PublicationPage(after,limit<=8,through)`, `AdmissionPage(after,limit<=8,through)`, exact `Admission(sequence)` and `Cutoff(eventID)`. Pages include ordered `{version,pin_id,through_sequence,through_head,after,next,complete,items}`; `complete` means next equals requested through, not that no private/unpublished admission exists. Requesting a prefix ahead of the retained signed head is unavailable. A reader must verify the entire prefix and source links, not trust each page's complete flag. HTTP serving uses a new closed public-data family; it grants no mining/account action and accepts no source-selected policy.

## 5. Replay, effective states and predecessor order

Scan every canonical block from pinned genesis through the declared frontier, with transactions in actual height/index order. Use complete bounded block bytes from the original owned validating node or a complete independently validated archive. Header/normal work, transaction framing/IDs, Merkle and carrier semantics are checked; selected proofs alone cannot establish absence. Recognized payloads must remain public/auditable. Refund discriminator, actual spent commit output and funding tuple are part of validation.

Accept consensus-valid outer P2SH representation changes while requiring the exact committed internal script/envelope: last pushed redeem, hash match, gym authorization and signed reveal discriminator. Do not discard a valid earlier transfer merely for nonminimal pushes, high-S outer funding signature, harmless stack items or changed reveal TxID. Support the relevant 10000-byte script ceiling. Unsupported capacity or a required known dependency gap means incomplete. An unknown prior outside the closed verified reachable set is invalid under Chapter II §6, not a permanent scan blocker.

Resolve exact initial or finalized predecessor evidence. The final StateID commits the first valid winning intent, **earliest qualifying committed checkpoint-publication EntryID** and canonical inclusion+60 block hash. Verify complete prefix, signature and ancestry. Require inclusion+60 height strictly before the successor **commit** height; require commit before reveal in canonical position (same-block increasing transaction order permitted). Initial state uses its authenticated admission preimage and has no invented transfer confirmations. This proves dependency-preimage availability; no publication clock/block timestamp comparison or extra observed-tip field is used. It does not prove global fetch availability. Missing evidence/prefix remains unavailable, not void.

The first valid canonical competitor for a state wins; later competitors are void. Semantic duplicate representations are one transfer. The old owner remains effective until the winner's own block checkpoint is in the declared publication prefix and 60 actual descendants exist. New state increments generation once; reacquisition cannot reactivate an old outgoing intent. Processing the same complete frontiers with different step/page partitions yields identical IDs/owners/digests.

Before finality, replay canonical reorgs and all affected competitors. After finality, preserve the official finalized prefix, retain/report checkpoint-crossing conflict and stop unsupported new transfer/cutoff mutation. Continue normal node consensus. A public replay head reports its conflict ID and evidence; it must not erase finalized history or manufacture an empty owned set. Later publications cannot alter an already sealed cutoff.

## 6. Exact noon capture with atomic node observation

Adopt the mandatory new read-only `getchainobservation` seam from `CHAIN_OBSERVATION.md`: `{version,instance_id,revision,hash,height}`, revision canonical uint64 decimal text. Chain revision increments under the existing stateLock on every best snapshot connect/disconnect; startup resets to 1 in a new node/RPC instance; saturation disables only observations, never chain processing. BestObservation uses **TryRLock chain then TryRLock state**, returns typed busy if either fails and releases all locks before response. No blocking RWMutex acquisition in the synchronous handler, no notifications-as-proof fallback, no consensus/persistent-DB change.

Exact source map is `blockchain/chain.go` two runtime snapshot replacements/private counter, new observation getter; additive btcjson command/result; rpcserver instance initialization/handler/read-only registration; help; optional typed rpcclient wrapper. Existing chainio assignments are startup only. There is no current equivalent revision API. This changes compiled node inputs and needs its own profile/chain/race/smoke/rollout qualification; the unchanged-node carrier probe does not qualify it.

Ordered `BoundaryObservation` is `{request_start_ms,response_end_ms,instance_id,revision,height,hash}`. Ordered `CutoffIntent` is `{version,pin_id,event_id,cutoff_ms,first,second,accepted_admission_sequence,publication_sequence,publication_head,prior_cutoff_id}`. Ordered `CutoffRecord` is `{version,pin_id,event_id,cutoff_ms,boundary,admission_sequence,admission_head,head,ownership_digest,signature}`. Event/noon comes only from the exact existing calendar/event pin. It is not a caller timestamp. Prior cutoff is the previous retained chronological event, zero only for the first configured event.

The scheduled concrete parent coordinator reserves capacity and the single source publication/admission permit outside account leases. It drains earlier commits, then obtains the first observation with **response completion before cutoff**. The second **request starts at or after cutoff**. Require identical instance/revision/height/hash and `second.response_end_ms-first.request_start_ms <=10000`. Both RPC budgets are five seconds within the original 30-second action. Retry only typed busy within those deadlines; a first read that misses cutoff cannot be retried after it as a historical first sample. Start near the boundary; ten seconds is a ceiling, not a planned pause or mining suspension.

Identical nonwrapping revision and instance prove no change of that node's published best state between the two samples, including A→B→A. The coordinator's clock brackets the node samples; the source remains an explicit trusted owned node, not global network/time consensus. Different revision/hash/instance, known connection loss, missed first sample, backward clock or unavailable call refuses exact capture. Do not pick the old or new owner opportunistically.

The held source permit freezes the accepted sequence and committed publication prefix. Persist the small exact CutoffIntent synchronously; release immediately. No signature/normal hash/network/account acquisition occurs in this commit. Afterward, drain admission statements only through that accepted sequence, replay that exact node frontier and checkpoint prefix, then sign the cutoff. A later checkpoint or admission cannot be inserted. Initial ownership already committed before the boundary is preserved despite later statement publication. Old event corners/condition/roster/entry references remain immutable.

A crash after exact intent can resume delayed validation/signing. Missing/uncertain intent requires original-store audit; absence after restart refuses historical lock. It cannot be replaced by latest Head, old cached observation or caller timestamp. Same event returns exact old cutoff; changed input conflicts. Source busy/unknown/missed evidence remains visible waiting/incident state; it does not cancel/reschedule the event or change D83.

## 7. Concrete core API and state ownership

The API declarations below are the concrete integration mapping. Constructors/parser validate exact expected identities; no generic proof/signer/watch callbacks.

```go
// registry; PublicManifestPin is the neutral Pin schema of this chapter.
CreatePublicManifestAuthority(dir string, descriptor fighterdescriptor.Descriptor, expected PublicManifestPin) (*ManifestStore, error)
OpenPublicManifestAuthority(dir string, expected PublicManifestPin) (*ManifestStore, error)
(*ManifestStore).PublicPublication(expected PublicManifestPin) (*ManifestPublication, error)
(*ManifestPublication).PublishNext(ctx context.Context) (PublicationStatus, error)
(*ManifestPublication).PublishCheckpoint(ctx context.Context, checkpoint chaincfg.SignedCheckpoint) (PublicationEntry, error)
(*ManifestPublication).Close() error // joins borrowed work, never independently closes DB

// fighterownership; StoreIdentity contains one preselected StoreID.
Create(dir string, expected Pin, identity StoreIdentity) (*Store, error)
Open(dir string, expected Pin, identity StoreIdentity) (*Store, error)
OpenAudit(dir string, expected Pin, identity StoreIdentity) (*Store, error)
(*Store).BeginReplay(ctx context.Context, source *PublicSource, node *OwnedChainSource) (*Replay, error)
(*Replay).Step(ctx context.Context) (Progress, error)
(*Replay).Close() error
(*Store).Head(ctx context.Context) (HeadRef, error)
(*Store).AcquireHead(ctx context.Context, expected HeadRef) (*HeadLease, error)
(*HeadLease).Check() error
(*HeadLease).Close() error
(*Store).CaptureOwned(ctx context.Context, gymPublicKey string) (*OwnedCapture, error)
(*Store).OwnedPage(ctx context.Context, capture *OwnedCapture, after uint32, limit uint32) (OwnedPage, error)
(*Store).OwnedItem(ctx context.Context, capture *OwnedCapture, prospectID string) (OwnedItem, error)
(*Store).PrepareTransfer(ctx context.Context, head *HeadLease, prospectID, fromGym, recipient string, tuple FundingTuple) (*PreparedTransfer, error)
(*PreparedTransfer).Check(ctx context.Context) error
(*PreparedTransfer).Intent() TransferIntent // owned immutable projection
(*Store).TransferStatus(ctx context.Context, transferID string) (TransferStatus, error)
(*Store).Cutoff(ctx context.Context, eventID string) (CutoffRecord, error)
(*Store).ResolveCutoff(ctx context.Context, cutoffID string, prospectIDs []string) (CutoffOwners, error)
(*Store).Close() error
```

OwnedItem is `{Admission AdmissionStatement, State EffectiveState}`. Page items contain original authenticated AdmissionStatement/ManifestRecord plus EffectiveState. OwnedItem reads only its original captured membership. CutoffOwners returns the exact CutoffRecord/Head and historical OwnedItems, including authenticated birth admission even when that fighter has since left the latest owned set. PreparedTransfer has no public constructor; it retains exact Store/source/node/predecessor evidence and checks that retained authority before disclosure/signing. FundingTuple is `[]FundingOutpoint{TxID string, Index uint32}` in the carrier's wire36 sorted order; private amount/path review remains the wallet's separate responsibility. This supports transferred-in fighters without relying on a local birth-only stable. Opaque capture holds exact store/generation/head/gym and ordered IDs; after is original admission sequence, page limit 8. ResolveCutoff accepts at most 10 unique sorted ProspectIDs and returns exact historical effective states under that verified signed cutoff, never latest Head. Gym ownership keys are independent of hosted AccountID; consumers map configured event participants to those keys at capture.

The exact preselected identity marker, metadata, row grammar, atomic work/head publication and mandatory public finality evidence are fixed in Chapter II. Missing established files/rows, count/byte mismatch, dangling crosslink or changed old evidence fails closed. No automatic repair/import/repin.

Open performs complete bounded structural/signature/crosslink audit; expensive normal-work verification is performed by the registered replay/audit work before exposing a complete Head. No untrusted durable `verified` boolean bypasses work. Reopen may take resumable verification steps and report unavailable progress; it cannot publish cached ownership prematurely. The registry's inherited normal receipt verification remains mandatory; construction/audit work is cancellable and joined outside account leases. A concrete incomplete owner is never a ready signing owner.

One fail-fast replay/mutation slot and one projection slot bound concurrent retained copies. HeadLease is exact original owner/generation/effective-state evidence and a registered lifetime, acquired outside account fences. Check is bounded/no-I/O. A native review does not hold it for minutes: release after preparing, then reacquire/verify the same fighter state and exact bytes at confirmation. Unrelated head advancement need not invalidate an unchanged selected predecessor. Close cancels/joins actual work, including registered lease acquisitions, before storage close. Sticky uncertain cleanup retains exact owner.

## 8. Caps, errors and required independent evidence

Fixed candidate caps: admissions 1000, publications 16384, semantic transfers 4096, recognized witnesses 8192, checkpoints 1024, cutoffs 256, normal headers 65536, logical store bytes 268435456. Old receipt 65536 bytes; admission statement 69632; small publication/cutoff 8192; public page 8 items/655360 bytes. Full block read≤4000000; bounded prewalk before Tx/CompactSize/script allocations, contextual checks during loops. Normal work is serial through the established process-wide gate: each Step at most 8 header hashes or one complete block scan, and at most one admission proof. One joined 256 MiB Argon workspace; independent sizing must account for parser/projection allocations without claiming an unmeasured total heap ceiling. All counts and indexes, including conflicting branches, consume capacity. No eviction of signed obligations or skip-to-recent fallback.

Interactive action 30 seconds, node call 5, combined delivery/observation 15 inside action; running normal hash is joined on cancellation. Typed core errors distinguish invalid, source mismatch, not found, unavailable/incomplete, busy, capacity, stale head/view, clock, conflict, cancelled and outcome unknown. Missing proof or a network timeout cannot become void/empty/success. Original View authorizes app disclosure; original FighterTransfer authorizes hosted intent/sign/dispatch. Public committed publication/replay may continue after login reset; fresh private authority may not.

Before implementation, freeze this chapter with carrier/wallet/consumer bytes and literal keys/rule recipe, independently size them, and freeze producer inputs/source. The corpus must test exact codecs/signatures/state IDs; atomic admitted/publication gaps; first valid competitor and malleated witnesses; missing prefixes ; 59/60 descendants; earliest checkpoint publication; predecessor descendant before successor commit; prefinal reorg and postfinal retained conflict; node revision ABA/restart/busy/overflow; strict cutoff request/response ordering; intent crash recovery and late-publication exclusion; old APIs and every configured resource refusal. Then qualify actual normal-PoW transfer, full public replay and exported-wallet restoration; source-only models and the mechanical probe do not replace those outcomes.


# Chapter II — fixed literals, closed storage and public finality evidence

## 1. Fixed literals

`rule_id = SHA256(ASCII("SLUG ownership and direct transfer") || 00 || ASCII("v1") || 00)`. No document hash, length prefix or generated output is in this preimage. The public admission/publication/cutoff fixture signer seed is exactly 32 bytes each equal 0x73; all three Pin key fields use its independently derived Ed25519 public key. This is deliberately public fiction, never a secret operator key. The checkpoint role seed is SHA256 of the exact ASCII bytes `SLUG candidate seed checkpoint fixture` +00+ `v1` +00, inherited from `seednode/attest.go`; it is distinct from both seed0x73 and the result-checkpoint fixture. No operator/mainnet key substitution.

For every new signed object, the signing preimage is its ordered body with the final `signature` member **omitted**, not an empty signature string; the ID then hashes the complete signed object. All object-domain hashes use the chapter's `domain || 00 || v1 || 00 || LE32(canonical length) || canonical bytes`, except the above RulesID and the root's compact carrier codec. Hash/ID fields are lowercase 64 hex. The only empty link is `""` where explicitly stated; a zero prefix head is 64 zero hex. Counter keys below are four-byte big endian; ID keys are 32 decoded bytes. Canonical JSON has no LF. Marker files add one LF, which is included only in their raw file SHA, not semantic IDs.

## 2. Core marker, metadata and namespaces

Marker `SLUG-FIGHTER-OWNERSHIP-V1.json`, ≤8192 bytes including LF, ordered `{application,version,store_id,pin,pin_id}`; application=`slug-fighter-ownership`,version1; StoreID is preselected random nonzero32 bytes, not inferred from Pin or disk. Directory contains only this marker and `db`. Exact expected StoreID/Pin is supplied by retained setup or independently trusted configuration. Established missing marker/meta refuses. No create-on-open, merge, repin or stage adoption.

`meta` value order:

`version,pin_id,store_id,revision,clock_high_water_ms,counts,logical_bytes,current_head,current_map_id,work_id,finality_sequence,published_finality_sequence,finality_head`.

Counts order: `admissions,publications,headers,carriers,witnesses,states,maps,cutoffs,boundaries,conflicts`. Frame/outbox counts are exactly published_finality_sequence/finality_sequence; boundary count≤cutoffs+1 and≤257. All uint32; revision increments once per sync state mutation with overflow refusal. Empty state has counts/logical accounting exact, current_head=`null`, current_map_id/work_id empty. `logical_bytes` counts **all actual DB keys+values including meta**, solved by canonical encoding before mutation; no filesystem/LevelDB overhead claim. current_head is the full HeadRef after a completed replay, never a partial progress object. current_map_id is then a nonzero ID.

Closed DB keys and values:

| Key | Exact value |
| --- | --- |
| `meta` | metadata above, ≤8192 |
| `a/`+BE32 sequence | AdmissionStatement, ≤69632 |
| `p/`+BE32 sequence | PublicationEntry, ≤8192 |
| `h/`+raw32 header hash | HeaderRow, ≤1024 |
| `c/`+BE32 height | raw32 selected canonical header hash, not JSON |
| `w/`+raw32 WitnessID | CarrierWitness, ≤65536 |
| `t/`+raw32 TransferID+raw32 WitnessID | zero-length value; exact witness index |
| `s/`+raw32 StateID | EffectiveState, ≤4096 |
| `v/`+raw32 StateID+raw32 FrontierEvidenceID | FinalityWitness, ≤4096 |
| `m/`+raw32 MapID | OwnershipMap, ≤262144 |
| `k/`+raw32 event ID | complete signed CutoffRecord, ≤8192 |
| `x/`+raw32 ConflictID | ConflictRecord, ≤8192 |
| `f/`+BE32 frame sequence | FinalityFrame, ≤8192 |
| `fo/`+BE32 frame sequence | exact unsigned FinalityFrameBody outbox, ≤8192 |
| `b/`+raw32 event ID | SignedBoundary, ≤8192 |
| `work` | single ReplayWork, ≤16384, absent iff meta.work_id empty |

No other prefix/key length is accepted. `c/` is the currently selected traversal, not proof that old headers vanished; reorg retains `h/` rows. Witness/state/map/cutoff/conflict objects are immutable and content addressed; equal-key changed bytes are corruption/conflict, never replacement. Index/row counts and all duplicate links are audited in both directions. No separate mutable per-gym owner index can disagree with a committed map.

## 3. Exact row objects and domains

`HeaderRow`: `version,pin_id,height,hash,header`; header is exactly160 lowercase hex (80 inherited header bytes), hash equals actual normal PoW header hash in display order, height int32≥0. For genesis it equals the compiled expected genesis. Parent link is decoded from header, not a duplicate field. Stored bytes never bypass required normal verification. One header counts once even if multiple source frontiers reference it.

`TransactionWitness`: `height,block_hash,transaction_index,transaction_count,txid,raw,merkle_branch`; count1..1000000, index<count, branch≤20, raw lowercase hex and recomputed TxID/Merkle exact. Commit raw≤4096 bytes; spend raw≤12000 bytes. The containing block bound and semantic carrier grammar still apply; these are storage ceilings, not permission to accept a generic oversized transfer.

`FundingPrevout`: `txid,index,atoms,script`; atoms1..MaxMoney, script exact native P2WPKH22 bytes encoded hex. Order equals the carrier's wire36 outpoint order, unique1..16. Values/scripts come from actual validated previous transactions, not claimant authority. Complete node/archive validation is required for historical canonical spend validity.

`CarrierWitness`: `version,pin_id,transfer_id,prior_state_id,commit,spend,funding_prevouts,envelope,kind`; commit/spend are TransactionWitness; envelope exact202 bytes hex; kind=`reveal` or `refund`. WitnessID domain=`SLUG fighter ownership witness` over this entire object. TransferID comes only from the compact129-byte intent and may therefore match multiple witnesses, including a refund that changes no ownership. Actual raw transaction identity is never normalized away. The `t/` index links every witness, not just local submitted TxIDs. Missing commit/prevout/predecessor prevents a complete valid/invalid classification.

`MapItem`: `admission_sequence,prospect_id,state_id`. `OwnershipMap`: `version,pin_id,items`; items are strictly ascending original admission sequence, exactly one item per admitted fighter through the completed head's admission frontier, max 1000. State's birth admission and ProspectID must match. MapID domain=`SLUG fighter ownership map`. `finalized_prefix_id` is domain=`SLUG fighter ownership finalized prefix` over the **same map object**. It commits the effective per-fighter lineage endpoints, not a global ordering by when replay jobs ran. Every predecessor state remains available, so audit can expand each complete lineage. No pending transfer changes this map.

`FrontierEvidence`: `kind,id`; kind=`publication`, `boundary` or `finality_frame`, identifying an exact signed chain publication, SignedBoundary or FinalityFrame. A live concrete owned node establishes current canonicality for D83 finalization; publication is mandatory portability evidence, not another external condition delaying the effective owner. A portable historical finality claim must retain the complete signed frame/source prefixes and the complete old branch evidence through the attested tip. An unsigned local observation, arbitrary 60-block fork or private finalized flag cannot fill that role.

`FinalityWitness`: `version,pin_id,state_id,frontier,checkpoint_publication_id,sixtieth_descendant_height,sixtieth_descendant_hash`. FrontierEvidenceID domain=`SLUG fighter ownership frontier evidence` over the frontier object. Exact source branch, checkpoint prefix membership and depth must agree with the state evidence and its public frontier. Multiple qualifying witnesses may be retained; they do not create another StateID. Earliest qualifying checkpoint publication controls StateID, independent of local arrival order. The first local verification time is not a public field or an ownership clock.

`ConflictRecord`: `version,pin_id,kind,prospect_id,retained_state_id,retained_frontier,contradictory_frontier,evidence_ids`. Kind=`publication_equivocation`, `checkpoint_conflict` or `finalized_branch_conflict`. ProspectID/state may be empty only for source-wide equivocation. Frontier objects are explicit null only when that conflict kind has no chain frontier; IDs list2..16 unique sorted64-hex references to retained exact signed/evidence objects. No arbitrary human error string. ConflictID domain=`SLUG fighter ownership conflict`. A valid conflict is reported without erasing retained finalized state or vetoing node consensus.

HeadID domain=`SLUG fighter ownership head` over complete HeadRef, for local capture/reference only. ReplayDigest domain=`SLUG fighter ownership replay` over ordered `{pin_id,admission_sequence,admission_head,publication_sequence,publication_head,chain_height,chain_hash,ownership_map_id,finalized_prefix_id,conflict_id}`. This makes head/map binding reproducible, not dependent on step boundaries. Empty conflict_id is empty string. `HeadRef.replay_digest` equals this result; other fields exactly match that object.

## 4. Resumable work and atomic publication

`ReplayTarget`: `admission_sequence,admission_head,publication_sequence,publication_head,chain_height,chain_hash,frontier` (frontier is explicit null only for a newly admitted concrete live-node job, never accepted as an untrusted historical target). Ordinary public history targets identify an authenticated chain publication. Exact cutoff work targets its independently checked SignedBoundary, which has no owner-map dependency. The final CutoffRecord binds that boundary and the independently computed owner map. A parsed unsigned intent is never a public target capability. No arbitrary partial/truncated source head is silently promoted to current completeness.

`ReplayWork`: `version,pin_id,work_id,base_revision,target,phase,next_admission,next_publication,next_height,scan_digest,working_map_id,conflict_id`. Phase is `admissions`, `publications`, `chain`, `ready`; cursors are the **next** item to check. start indices 1,1,0; a completed cursor is target + 1, with checked overflow. WorkID is preselected nonzero random32 bytes and does not enter public IDs. base_revision records admitted work's starting metadata revision, not a requirement that its own subsequent steps leave revision unchanged. working_map_id is empty before any computed map or the exact immutable staged map. Work remains exclusive until joined close, complete publication or explicit abort; cancellation retains audited resumable work, not a ready Head.

ScanDigest starts with domain=`SLUG fighter ownership scan start` over `{pin_id,target}`. For each verified complete block, advance with domain=`SLUG fighter ownership scan block` over `{previous_digest,height,hash,transaction_count,witness_ids}`; witness IDs are actual transaction order, including recognized refunds, no sorted rewrite. This is private progress integrity, **not** ReplayDigest/Head identity, so different step partitions produce the same final public output. Admission/publication rows are authenticated in sequence before their dependencies are used.

Steps sync their exact verified immutable rows/indexes and ReplayWork cursor atomically. They never replace `meta.current_head/current_map_id`. Final readiness verifies full coverage, builds the owned immutable map and complete HeadRef outside any account lease, then one sync batch changes current_head/current_map_id and removes work. An in-process old capture retains its old map/HeadRef; a new capture sees only the new complete pair. No full raw-store clone is needed. Stale captured source/node/view cannot publish a new complete head. Outcome-unknown writes poison and require exact reopen audit; no `work` absence inference after a failed write.

States≤1000+4096; maps≤1+1000+4096; finality witnesses≤8192. Those derived caps supplement the Pin limits and all bytes still count toward 256MiB. Every recognized retained witness, including forks/refunds/duplicates, consumes witness capacity; there is no unbounded special evidence side store. Cutoff map ownership_digest is the exact MapID, not a list embedded in the small cutoff. Referenced map/states remain retained; missing them is unavailable/corrupt, not latest-owner substitution.

## 5. Portable finality and noncircular cutoff evidence

`FinalityFrameBody`, ordered: `version,pin_id,sequence,previous_body_id,admission_sequence,admission_head,publication_sequence,publication_head,chain_height,chain_hash,ownership_map_id,finalized_prefix_id`. `FinalityFrame` is that exact body followed by `signature`. BodyID domain=`SLUG fighter ownership finality body` over the unsigned body; Signature domain=`SLUG fighter ownership finality statement`; FrameID domain=`SLUG fighter ownership finality id`. Sequence1..4096, previous_body_id zero only for1 and otherwise the exact prior unsigned BodyID. The body chain is computable before any signature, so pending publication cannot block another effective finalization. The key is the Pin publication key. Frame binds the complete effective map, source prefixes and the actual canonical node frontier used to establish new finality. It attests canonical observation, never competing-send order. Ingestion verifies every referenced map/state/source row and the complete old branch through that frontier; a signature alone is not proof of normal work or the claimed state transitions.

When ordinary concrete node replay first proves checkpoint+60, the same core sync batch commits the effective states/map and exact unsigned frame outbox (`fo/`, contiguous frame sequence). This is the finality fact, independent of publisher queue delay. The concrete local signer may stage the signature before that commit and store `f/` in the same batch; otherwise expose `public_evidence_pending` and drain the immutable outbox automatically outside account leases. Neither condition says the former owner is still effective. No frame body can change after the fact. A failed signed publication poisons/reopens without inventing another sequence. Outbox rows remain retained crosslinks. The public `v/` FinalityWitness row is created only once its signed FrameID exists; a locally effective state awaiting that row is allowed only with its exact retained frame outbox and an advertised publication gap. No complete portable claim may use it. published_finality_sequence is the contiguous signed prefix; finality_head is its exact frame ID (zero if empty). finality_sequence records all durable frame intents. Public source status must expose any gap, and a complete-current portable claim cannot hide it.

The prefix of frame observations is verified in sequence; each frame's newly effective states must follow canonical reveal order and exact checkpoint/depth against that frame's branch. Once a frame proves finalized official ownership, later contradictory chain observations retain that state and report conflict. Fresh independent replay after reorg requires those exact signed frames and their complete public inputs. Missing/middle-gap/unpublished finality evidence means unavailable official history; it cannot revert to the prior owner or select a competing owner simply from today's chain. Completeness is always through an independently supplied/advertised authenticated frontier, never proof that an operator secretly retained no unpublished data.

FrameID stays **outside StateID**. The approved StateID still commits earliest qualifying checkpoint EntryID and actual60th-descendant hash; the mandatory FinalityWitness establishes old canonicality beside it. Different processing partitions may produce different attestation frames, but not a different StateID, canonical winner or effective lineage map. No ProcessingAtMS enters arbitration. Frame count/bytes are capped separately from publication count and included in all logical-byte accounting; frames are public data and cannot be silently evicted.

`SignedBoundary`, ordered: `version,pin_id,intent,signature`, where intent is the exact durable CutoffIntent with node instance/revision bracket and captured source prefixes. Signature domain=`SLUG fighter ownership boundary statement`; BoundaryID domain=`SLUG fighter ownership boundary id`, signed by Pin.cutoff_key. Sign it after durable boundary capture and **before owner-map calculation**. This removes the circular use of a final CutoffRecord (which itself contains Head/ownership digest) as its own finality evidence. The final CutoffRecord replaces its old bare `intent` member with `boundary` (the complete SignedBoundary) in the same ordered position. ResolveCutoff verifies signature, exact node bracket, source prefixes, public branch/state replay and resulting map. Neither a later checkpoint nor a latest head can enter the earlier boundary.

## 6. Unknown state versus missing required evidence

Once the complete declared admission/publication/finality prefixes, all earlier canonical blocks and their derived reachable states are verified, an opaque priorStateID absent from that closed set is **provably invalid for that frontier**. The same applies to a proved future/nonfinal predecessor. It does not make arbitrary valid-shaped junk a permanent unavailable condition. A known predecessor whose required signed row/block/parent is missing, a prefix gap, or a source-advertised accepted/published gap remains unavailable; it cannot be skipped as invalid. Never infer or backdate an admission/publication outside the declared frontier. The signer must not leak uncommitted signature preimages. These are separate corpus controls: closed-set unknown prior→invalid, known dependency missing→unavailable. No complete owned set is returned while a required frontier is incomplete.

## 7. Independent producer boundary

The public-core producer will use the unchanged archived Set45 pure-Python Ed25519/RFC8032 primitives and public old receipt inputs. It will derive seed73's public key and RulesID only after the source/input lock is approved. Actual old admissions stay byte/value preserved and retain original source IDs/sequences; new modeled statements wrapping them are labeled synthetic attestation, not new live issuance. Synthetic header/frontier/state cases are typed model inputs, not normal-PoW/canonical-chain proofs. Primitive RFC selfchecks precede generation. No Go73 output supplies expected values.

One corpus covers canonical pin/admission/publication/state/cutoff bytes/digests/signatures; exact row keys/metadata counts and byte sums; reserved/published outbox replay; staged-versus-complete heads; deterministic partition-independent map/lineage IDs; missing/changed source/frontier/predecessor cases; boundary revision/timing/refusal controls; and bounded object/index/capacity controls. Full normal header/canonical transaction/Argon verification remains independently qualified by inherited receipts and actual node tests, not fabricated by these modeled rows.
