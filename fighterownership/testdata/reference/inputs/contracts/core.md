# Set73 public ownership core — final candidate chapter

For root adoption, November 19, 2026. This is an implementation contract candidate, not adopted bytes or completed code. It replaces the alternatives in `admission-replay-cutoff.draft.md` for the public core. D76/D83 remain as adopted. The companion carrier/wallet/consumer chapters supply their exact domain bytes, schema14 hosted / schema15 restored owners and native surfaces. No expectation, product code or runtime was generated for this chapter.

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

`Pin.source` is the entire original ManifestAuthorityPin, including descriptor. `rule_id` refers to the separately frozen canonical rule recipe; it is not a self-referential hash of a document containing its own ID. Fixture key bytes and the rule recipe are input-lock material, not generated from implementation. Alias `fighterownership.Pin` to a neutral `registry.PublicManifestPin` declaration if necessary to avoid an import cycle; registry performs structure/source/config checks and core additionally requires the exact supported rule recipe.

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

Accept consensus-valid outer P2SH representation changes while requiring the exact committed internal script/envelope: last pushed redeem, hash match, gym authorization and signed reveal discriminator. Do not discard a valid earlier transfer merely for nonminimal pushes, high-S outer funding signature, harmless stack items or changed reveal TxID. Support the relevant 10000-byte script ceiling. Unsupported capacity/unknown prior data means incomplete, never void-by-skipping.

Resolve exact initial or finalized predecessor evidence. The final StateID commits the first valid winning intent, **earliest qualifying committed checkpoint-publication EntryID** and canonical inclusion+60 block hash. Verify complete prefix, signature and ancestry. Require inclusion+60 height strictly before the successor **commit** height; require commit before reveal in canonical position (same-block increasing transaction order permitted). Initial state uses its authenticated admission preimage and has no invented transfer confirmations. This proves dependency-preimage availability; no publication clock/block timestamp comparison or extra observed-tip field is used. It does not prove global fetch availability. Missing evidence/prefix remains unavailable, not void.

The first valid canonical competitor for a state wins; later competitors are void. Semantic duplicate representations are one transfer. The old owner remains effective until the winner's own block checkpoint is in the declared publication prefix and 60 actual descendants exist. New state increments generation once; reacquisition cannot reactivate an old outgoing intent. Processing the same complete frontiers with different step/page partitions yields identical IDs/owners/digests.

Before finality, replay canonical reorgs and all affected competitors. After finality, preserve the official finalized prefix, retain/report checkpoint-crossing conflict and stop unsupported new transfer/cutoff mutation. Continue normal node consensus. A public replay head reports its conflict ID and evidence; it must not erase finalized history or manufacture an empty owned set. Later publications cannot alter an already sealed cutoff.

## 6. Exact noon capture with atomic node observation

Adopt the mandatory new read-only `getchainobservation` seam from `predecessor-cutoff-freeze-recommendation.md`: `{version,instance_id,revision,hash,height}`, revision canonical uint64 decimal text. Chain revision increments under the existing stateLock on every best snapshot connect/disconnect; startup resets to 1 in a new node/RPC instance; saturation disables only observations, never chain processing. BestObservation uses **TryRLock chain then TryRLock state**, returns typed busy if either fails and releases all locks before response. No blocking RWMutex acquisition in the synchronous handler, no notifications-as-proof fallback, no consensus/persistent-DB change.

Exact source map is `blockchain/chain.go` two runtime snapshot replacements/private counter, new observation getter; additive btcjson command/result; rpcserver instance initialization/handler/read-only registration; help; optional typed rpcclient wrapper. Existing chainio assignments are startup only. There is no current equivalent revision API. This changes compiled node inputs and needs its own profile/chain/race/smoke/rollout qualification; the unchanged-node carrier probe does not qualify it.

Ordered `BoundaryObservation` is `{request_start_ms,response_end_ms,instance_id,revision,height,hash}`. Ordered `CutoffIntent` is `{version,pin_id,event_id,cutoff_ms,first,second,accepted_admission_sequence,publication_sequence,publication_head,prior_cutoff_id}`. Ordered `CutoffRecord` is `{version,pin_id,event_id,cutoff_ms,intent,admission_sequence,admission_head,head,ownership_digest,signature}`. Event/noon comes only from the exact existing calendar/event pin. It is not a caller timestamp. Prior cutoff is the previous retained chronological event, zero only for the first configured event.

The scheduled concrete parent coordinator reserves capacity and the single source publication/admission permit outside account leases. It drains earlier commits, then obtains the first observation with **response completion before cutoff**. The second **request starts at or after cutoff**. Require identical instance/revision/height/hash and `second.response_end_ms-first.request_start_ms <=10000`. Both RPC budgets are five seconds within the original 30-second action. Retry only typed busy within those deadlines; a first read that misses cutoff cannot be retried after it as a historical first sample. Start near the boundary; ten seconds is a ceiling, not a planned pause or mining suspension.

Identical nonwrapping revision and instance prove no change of that node's published best state between the two samples, including A→B→A. The coordinator's clock brackets the node samples; the source remains an explicit trusted owned node, not global network/time consensus. Different revision/hash/instance, known connection loss, missed first sample, backward clock or unavailable call refuses exact capture. Do not pick the old or new owner opportunistically.

The held source permit freezes the accepted sequence and committed publication prefix. Persist the small exact CutoffIntent synchronously; release immediately. No signature/normal hash/network/account acquisition occurs in this commit. Afterward, drain admission statements only through that accepted sequence, replay that exact node frontier and checkpoint prefix, then sign the cutoff. A later checkpoint or admission cannot be inserted. Initial ownership already committed before the boundary is preserved despite later statement publication. Old event corners/condition/roster/entry references remain immutable.

A crash after exact intent can resume delayed validation/signing. Missing/uncertain intent requires original-store audit; absence after restart refuses historical lock. It cannot be replaced by latest Head, old cached observation or caller timestamp. Same event returns exact old cutoff; changed input conflicts. Source busy/unknown/missed evidence remains visible waiting/incident state; it does not cancel/reschedule the event or change D83.

## 7. Concrete core API and state ownership

The API declarations below are the proposed mapping for adoption. Constructors/parser validate exact expected identities; no generic proof/signer/watch callbacks.

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
(*Store).PrepareTransfer(ctx context.Context, head *HeadLease, prospectID, fromGym, recipient string, tuple FundingTuple) (TransferIntent, error)
(*Store).TransferStatus(ctx context.Context, transferID string) (TransferStatus, error)
(*Store).Cutoff(ctx context.Context, eventID string) (CutoffRecord, error)
(*Store).ResolveCutoff(ctx context.Context, cutoffID string, prospectIDs []string) (CutoffOwners, error)
(*Store).Close() error
```

Page items contain original authenticated AdmissionStatement/ManifestRecord plus EffectiveState. This supports transferred-in fighters without relying on a local birth-only stable. Opaque capture holds exact store/generation/head/gym and ordered IDs; after is original admission sequence, page limit 8. ResolveCutoff accepts at most 10 unique sorted ProspectIDs and returns exact historical effective states under that verified signed cutoff, never latest Head. Gym ownership keys are independent of hosted AccountID; consumers map configured event participants to those keys at capture.

Store uses a preselected identity marker and closed metadata `{version,pin_id,store_id,revision,admission_head,publication_head,scan_cursor,finalized_prefix_id,logical_bytes,clock_high_water_ms}`. Closed namespaces retain signed admission/publication objects, normal headers/coverage, complete recognized carrier witnesses, immutable state/predecessor/finality records, conflict evidence and cutoffs. Ordered counters use BE32; ID keys use raw32. Sync batches atomically publish each verified replay cursor with its rows/indexes. A partial cursor is resumable progress, not complete Head. Missing established files/rows, count/byte mismatch, dangling crosslink or changed old evidence fails closed. No automatic repair/import/repin.

Open performs complete bounded structural/signature/crosslink audit; expensive normal-work verification is performed by the registered replay/audit work before exposing a complete Head. No untrusted durable `verified` boolean bypasses work. Reopen may take resumable verification steps and report unavailable progress; it cannot publish cached ownership prematurely. The registry's inherited normal receipt verification remains mandatory; construction/audit work is cancellable and joined outside account leases. A concrete incomplete owner is never a ready signing owner.

One fail-fast replay/mutation slot and one projection slot bound concurrent retained copies. HeadLease is exact original owner/generation/effective-state evidence and a registered lifetime, acquired outside account fences. Check is bounded/no-I/O. A native review does not hold it for minutes: release after preparing, then reacquire/verify the same fighter state and exact bytes at confirmation. Unrelated head advancement need not invalidate an unchanged selected predecessor. Close cancels/joins actual work, including registered lease acquisitions, before storage close. Sticky uncertain cleanup retains exact owner.

## 8. Caps, errors and required independent evidence

Candidate caps: admissions 1000, publications 16384, semantic transfers 4096, recognized witnesses 8192, checkpoints 1024, cutoffs 256, normal headers 65536, logical store bytes 268435456. Old receipt 65536 bytes; admission statement 69632; small publication/cutoff 8192; public page 8 items/655360 bytes. Full block read≤4000000; bounded prewalk before Tx/CompactSize/script allocations, contextual checks during loops. Normal work is serial through the established process-wide gate: each Step at most 8 header hashes or one complete block scan, and at most one admission proof. One joined 256 MiB Argon workspace; independent sizing must account for parser/projection allocations without claiming an unmeasured total heap ceiling. All counts and indexes, including conflicting branches, consume capacity. No eviction of signed obligations or skip-to-recent fallback.

Interactive action 30 seconds, node call 5, combined delivery/observation 15 inside action; running normal hash is joined on cancellation. Typed core errors distinguish invalid, source mismatch, not found, unavailable/incomplete, busy, capacity, stale head/view, clock, conflict, cancelled and outcome unknown. Missing proof or a network timeout cannot become void/empty/success. Original View authorizes app disclosure; original FighterTransfer authorizes hosted intent/sign/dispatch. Public committed publication/replay may continue after login reset; fresh private authority may not.

Before implementation, freeze this chapter with carrier/wallet/consumer bytes and literal keys/rule recipe, independently size them, and freeze producer inputs/source. The corpus must test exact codecs/signatures/state IDs; atomic admitted/publication gaps; first valid competitor and malleated witnesses; missing prefixes ; 59/60 descendants; earliest checkpoint publication; predecessor descendant before successor commit; prefinal reorg and postfinal retained conflict; node revision ABA/restart/busy/overflow; strict cutoff request/response ordering; intent crash recovery and late-publication exclusion; old APIs and every configured resource refusal. Then qualify actual normal-PoW transfer, full public replay and exported-wallet restoration; source-only models and the mechanical probe do not replace those outcomes.

Only integration input literals, final carrier bytes/economics and independently measured sizing remain for assembly; this chapter selects one public admission, replay and exact-cutoff mechanism. It introduces no user policy question or alternative latest-tip fallback.
