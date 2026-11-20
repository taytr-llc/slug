# Owned fighter consumers — adopted candidate contract

Set73. Root reviewed and adopted on 19 November 2026. D83 ownership is distinct from immutable birth attribution and previous event corners. This does not widen the existing C1 zero-record matchmaking route or introduce login/navigation changes.

## Consumer API

Set73 implementation mapping, 19 November 2026. This narrows the adopted D83 consumer slice; it does not widen old candidate snapshots. Root has approved separate concrete `OwnedStore` types and closed owned codecs. Implementation follows reviewed independent byte inputs and concrete public-core declarations.

## Concrete ownership dependencies

`fighterownership.Store` supplies `Pin/Identity` metadata matching constructor expectations, a complete `Head`, opaque `CaptureOwned` and eight-item `OwnedPage` with original authenticated admission plus `EffectiveState`, `AcquireHead` with lock-free `Check`, and `ResolveCutoff` for one exact retained cutoff and up to ten sorted ProspectIDs. The agreed `CutoffOwners` returns the exact CutoffRecord/Head plus `[]OwnedItem`, each containing the original authenticated Admission and historical State: current-owned pages cannot supply an outgoing fighter at an older cutoff. No source callback accepts an asserted admission/owner pair.

The agreed core `OwnedItem(ctx, capture, prospectID)` returns only a member of that original opaque capture. No durable duplicate receipt cache is needed in the owned stable: core remains the retained admission authority, while stable retains only bounded roster/entry references and history. This avoids a second 1000-receipt store and cannot turn a cached boolean into verification.

Constructor and capture names in this document are final consumer spellings. Core read counterpart names have been agreed with its owner; declarations are forthcoming. This is not a new policy decision.

## Stable: separate mode, same roster rules

```go
// OwnedPin ordered JSON: version, stable, ownership.
// stable is the existing complete Pin2 account/source/identity/condition pin;
// ownership is the complete independently expected public-core Pin.
type OwnedPin struct {
    Version uint32
    Stable Pin
    Ownership fighterownership.Pin
}
func CreateOwned(path string, expected OwnedPin, source *fighterownership.Store) (*OwnedStore, error)
func OpenOwned(path string, expected OwnedPin, source *fighterownership.Store) (*OwnedStore, error)
func OpenOwnedAudit(path string, expected OwnedPin, source *fighterownership.Store) (*OwnedStore, error)
func OpenOwnedCareer(path string, expected OwnedPin, source *fighterownership.Store, activation careerstate.Activation) (*OwnedStore, error)
func (s *OwnedStore) BindCareer(ctx context.Context, reader *careerstate.Reader) error
func (s *OwnedStore) AttachCareer(ctx context.Context, reader *careerstate.Reader) error
func (s *OwnedStore) Status(ctx context.Context, original Session) (OwnedStatus, error)
func (s *OwnedStore) Capture(ctx context.Context, original Session) (*OwnedRead, error)
func (s *OwnedStore) Page(ctx context.Context, original Session, read *OwnedRead, after uint32, limit uint32) (OwnedPage, error)
func (s *OwnedStore) PrepareSlot(ctx context.Context, original Session, request OwnedSlotRequest) (OwnedIntent, error)
func (s *OwnedStore) ApplySlot(ctx context.Context, original Session, operationID string) (OwnedMutationResult, error)
func (s *OwnedStore) Operation(ctx context.Context, original Session, operationID string) (OwnedOperation, error)
func (s *OwnedStore) Abandon(ctx context.Context, original Session, operationID string) (OwnedOperation, error)
func (s *OwnedStore) Close() error
```

`OwnedRead` retains exact source/store generation, source capture/head, original session generation+ViewID and observation time; replacing login cannot retarget it. Final disclosure checks the same original View. Source/header/receipt verification occurs outside account leases. The source read handle is immutable; current-owner authorization is rechecked before disclosure where required, and an expired/stale capture is explicit, never replaced with the latest.

Owned page item ordered fields: `birth, admission, ownership, manifest, condition, career`. Birth is the unchanged `Assignment`, including original GymID and sequence. Its projection verifies the full authenticated admission while allowing its birth gym to differ from the viewer. Career holds the same immutable activation/digest/totals, not an XP0 rewrite. Page ordered fields: `version,status,head,as_of_ms,after,next_after,has_more,items`. Status ordered fields: `version,store_id,pin,head,roster_revision,slots,operation_count,pending_operation_id,last_operation_id,last_as_of_ms`. There is no assigned/mined count masquerading as currently owned count.

`OwnedSlotRequest` ordered fields: `expected_revision,slot,prospect_id,expected_state_id,head`. Empty ProspectID clears a slot and requires empty StateID; a nonempty selection requires exact current state and same gym. Prepare/Apply both resolve the source and acquire its concrete head lease outside the final RosterManage fence. Slot history is permanent. A transferred-out slot is retained but is not eligible; receiving never auto-fills a slot. Limits stay ten slots, 4096 operation rows, one unresolved operation, pages at most eight. Operation IDs and original-view replay semantics follow the existing roster implementation. Separate owned marker/domain prevents an old stable open from interpreting this namespace.

## Entry, cutoff and event

```go
// Each owned constructor takes its concrete owned upstream store(s), never a
// generic assertion callback. Pins include the exact complete owned source pin.
gymentry.CreateOwned(ctx, path, OwnedPin, *gymstable.OwnedStore) (*OwnedStore,error)
gymentry.OpenOwned(ctx, path, OwnedPin, *gymstable.OwnedStore) (*OwnedStore,error)
gymentry.OpenOwnedAudit(ctx, path, OwnedPin, *gymstable.OwnedStore) (*OwnedStore,error)
// Prepare/Apply retain the existing five-class selection and original EntryManage.
gymevent.CreateOwned(ctx, path, OwnedPin, *fighterownership.Store) (*OwnedStore,error)
gymevent.OpenOwned(ctx, path, OwnedPin, *fighterownership.Store) (*OwnedStore,error)
gymevent.OpenOwnedAudit(ctx, path, OwnedPin, *fighterownership.Store) (*OwnedStore,error)
func (s *gymevent.OwnedStore) Capture(ctx, *gymentry.OwnedStore, original gymstable.Session, saturday string) (OwnedCaptureResult,error)
```

An event uses its exact signed `CutoffRecord`; the public source's declared complete frontiers choose effective owner. The consumer never substitutes current Head or a private capture timestamp. Local roster/entry histories keep their inherited strict-before-cutoff ordering. Both participant captures must reference the same CutoffID/frontiers. Stable cutoff selection resolves authenticated birth and effective state against that exact cutoff, including a fighter no longer owned today. Final callback only checks retained leases/identities and publishes the bounded local record; it performs no proof work or lock acquisition.

Owned member ordered fields: `slot,slot_assigned_at_ms,birth,ownership,class,condition_at_cutoff,eligibility`. Participant ordered fields: `version,participant_id,entry_store_id,owner,roster,entry,members,excluded`. `excluded` is a sorted, bounded list of `{prospect_id,state_id,owner_public_key,reason}` with reason exactly `not_owned_at_cutoff`. It retains an explicit explanation for an old saved entry whose fighter transferred away by noon; that fighter is omitted from this participant’s engine members. The new owner may independently enter it, so there is no duplicate-engine entrant and no changed old entry receipt. ResolveCutoff covers the complete selected ID list before partitioning; absence of evidence is an error, not an exclusion. `owner` is the actual configured account/gym corner with `ownership_basis:"finalized_public_chain"`. Birth is never rewritten to that corner. Snapshot input ordered fields: `version,event,cutoff,participants`; receipt: `version,input,snapshot_digest,captures,completed_at_ms`. New distinct types/domain IDs are used end to end. Old `SnapshotInput`/`ParticipantSnapshot` and old reflected validators remain unchanged.

## Seed/card/bout/result and permanent history

Add `CreateOwned/OpenOwned/OpenOwnedAudit` (and career variants where presently needed) with concrete owned predecessor handles and distinct OwnedPin/OwnedRecord source wrappers. Keep current seed/result publication adapters and private deterministic seed/card/bout engine leaves; factor their already-validated physical input handling where necessary. New wrappers supply exact event corner and immutable birth/appearance, and retain the owned snapshot digest in every downstream identity. No whole duplicate matching or fight engine and no relaxed old codec branch.

`gymresult` emits the same career award semantics using the locked event corner. Owned career history uses a separate capture/page validator: each row's corner comes from that historical event and selected ProspectID, never the current viewer or birth gym. Existing career index, awards, results, activation and before-transfer rows remain immutable. The existing C1/zero-professional-record candidate eligibility limits are not silently broadened by this ownership integration; permanent history remains visible even when a fighter is presently ineligible for that candidate match route.

The native/desktop owner selects these concrete types explicitly. Old routes receive only old handles. There is no exported `LegacyStore()` escape hatch and no synthetic old Assignment constructed with the recipient gym.

## Codec and qualification boundary

Owned codecs use closed ordered JSON with exact finite shapes, immutable nested old codecs and independent reference bytes. Version is1 within the separately named owned family; IDs use separate `SLUG owned ...\x00v1\x00` domains and a length-prefixed canonical preimage, never old candidate domains. The detailed per-row sizes are derived before output generation from the fixed existing eight-item/ten-slot/five-member/256-event caps plus core admission/effective-state limits; admitting a row reserves its terminal-state capacity. The source/core proof remains concrete even when a public DTO passes a shape validator.

First implementation order: source declaration agreement; freeze the small owned type/byte appendix and literal old inputs; independent expected bytes; stable store/read/roster; exact cutoff/entry/event; thin downstream engine wrappers; owned history; finite actual A→B before/after-finality and earlier-event-corner proof. Failed cleanup retains the concrete owner, Close joins admitted work, and no account fence waits for source work or mutex acquisition. No login/navigation/provider policy changes are part of this lane.

## Closed owned codec appendix

Companion to owned-consumer-api.candidate.md. Adopted byte appendix; independent output generation still requires its reviewed execution lock. Inherited inner types keep their existing exact codecs. Every new object below has version1 in a separately named owned family; old constructors and parsers never accept it. JSON is ordered compact UTF-8, integer scalars only, arrays non-null, no unknown/duplicate/missing keys or trailing bytes. A parsed shape is not authority.

`H(label, object) = SHA256(ASCII("SLUG owned "+label)+00+ASCII("v1")+00+LE32(len(canonical(object)))+canonical(object))`. Every new store pin has a complete typed source, never a caller-selected digest without its independently retained pin. Digests are lowercase64hex; empty optional IDs are `""`, never all-zero sentinels. Existing ViewID grammar and inherited old timestamps stay unchanged. New source times obey public-core bounds. The original `Session` remains an in-process capability, never encoded authority.

## Stable ordered fields

- `OwnedPin`: `version,stable,ownership`. Stable is the existing full Pin2; source equality with Ownership.Source is exact.
- `OwnedStatus`: `version,store_id,pin,head,roster_revision,slots,operation_count,pending_operation_id,last_operation_id,last_as_of_ms`.
- `OwnedCareer`: `activation,career_digest,totals,current_gym_wins`. Activation is the existing exact activation, digest is the immutable authenticated career prefix.
- `OwnedItem`: `birth,admission,ownership,manifest,condition,career`. Admission/ownership are the exact public-core item; birth preserves original admission sequence, GymID and appearance. Item is never accepted to mutate a store.
- `OwnedPage`: `version,status,head,as_of_ms,after,next_after,has_more,items`. `after` is original admission sequence; pages have at most8 items. No replacement head after capture.
- `OwnedSlotRequest`: `expected_revision,slot,prospect_id,expected_state_id,head`.
- `OwnedIntent`: `version,sequence,operation_id,generation,view_id,request,prepared_at_ms`.
- `OwnedMutationReceipt`: `version,operation_id,generation,view_id,request,previous_prospect_id,changed,previous_revision,revision,slots,committed_at_ms`.
- `OwnedOperation`: `version,intent,state,receipt,abandoned_at_ms`. State exactly prepared/committed/abandoned; receipt null only before commit, abandoned time0 except abandoned. Terminal rows cannot be rewritten.
- `OwnedMutationResult`: `version,receipt,replayed`.

Stable identity is H("stable store",OwnedPin). Metadata has `version,store_id,pin,clock_ms,last_as_of_ms,operation_count,pending_operation_id,last_operation_id,roster_revision`; exact ten-slot row and existing operation sequence/history leaves are reused privately. No receipt-sized admission cache is persisted. Separate immutable marker `SLUG-OWNED-STABLE-V1.json` contains `format,version,store_id,pin`, format `slug-owned-stable`. Existing marker inventory refuses it.

One opaque capture per concrete consumer is retained at a time; replacement releases the prior handle only after its admitted readers join. Caller binds original ViewID/generation, no public CaptureID becomes a capability. Page serialization is one admitted operation, no queue. Caps: original4096 operations,10 slots, core1000 admissions,8 page items; new maximum pin16KiB, status24KiB, request4KiB, intent8KiB, terminal operation16KiB, item88KiB, page736KiB. The item allowance composes core admission68KiB + state4KiB + birth2KiB + manifest8KiB + condition/career/scalar overhead6KiB. Eight items704KiB + status24KiB + head/cursor8KiB fit736KiB. These are accounted encoded bounds, not a measured total Go-heap cap; exact witness sizing is independently checked before adoption.

## Entry and event ordered fields

Entry `OwnedPin`: `version,namespace_id,calendar_id,rules_id,stable_store_id,stable_pin`; nested source is the complete owned stable pin. Entry selection request retains the inherited `Request` fields `version,event_id,saturday,action,expected_revision,expected_roster_revision,prospect_ids` with at most5 sorted unique IDs and the same one-per-class rule. Its opaque preparation additionally retains exact source state IDs. Saved members use `slot,birth,ownership,class,condition_at_save`. The new entry receipt keeps the inherited ordered receipt fields, replacing `stable_checkpoint,observed_source_count` with one exact `ownership_head`. It retains the immutable owned members, never changes old entry receipt bytes.

Event `OwnedPin`: `version,calendar_id,rules_id,ownership,participants`; participants are the two exact owned entry pins sorted by configured gym. Event fields and calendar computation remain inherited; its StoreID and domain are owned. `OwnedSnapshotInput`: `version,event,cutoff,participants`. Cutoff is the exact signed public-core CutoffRecord; both participants reference that same cutoff and its frontiers.

`OwnedParticipantSnapshot`: `version,participant_id,entry_store_id,owner,roster,entry,members,excluded`. Owner is `ownership_basis,account_authority_id,account_id,gym_id`, basis exactly `finalized_public_chain`. Roster uses inherited exact historical ten-slot projection. Entry uses its own owned cutoff-history row. Members are `slot,slot_assigned_at_ms,birth,ownership,class,condition_at_cutoff,eligibility`; ownership is the state effective at the exact cutoff, not today. Excluded rows are `prospect_id,state_id,owner_public_key,reason`, reason only `not_owned_at_cutoff`, sorted by ProspectID. Together members+excluded exactly partition saved selected IDs after complete core cutoff resolution; missing evidence refuses all capture. Eligibility retains existing eligible/retired/not-rostered meanings for owned members.

Snapshot receipt is `version,input,snapshot_digest,captures,completed_at_ms`; capture fences retain inherited stable/entry history and exact CutoffID. Snapshot digest is H("event snapshot",OwnedSnapshotInput). Participant capture is local disclosure/atomicity evidence, not a second ownership authority. At most2 participants and5 selected IDs each remain;10 IDs is the core cutoff resolution ceiling. Entry operation count and event count remain4096/256. Owned event inputs/receipts have a distinct independently sized cap; they never consume the old126976-byte receipt budget by accident.

## Downstream and history binding

Owned seed/card/bout/result source wrappers carry `version,source_store_id,source_pin,rules_id` plus the existing fixed publisher/chain fields where those are already present. Their nested records use only owned upstream wrapper types. Deterministic seed choice, matching, simulation, result signatures/finality and XP/point arithmetic remain existing private leaves. A wrapper may omit repeated full public admissions after the event owns validated compact birth/state/cutoff references, but cannot omit the exact source digest or substitute a latest source. Original proof rows remain in the concrete public source.

Owned career capture is `version,status,activation,career_digest,as_of_ms,birth,ownership,prefix_count,event_count,totals,current_gym_wins,history_id`; history ID is H("fighter history",the same object without history_id). Rows reuse immutable career events and award arithmetic, but their corner need only match selected ProspectID and authentic historical event—not viewer AccountID or original birth GymID. Page is `version,capture,after,next,has_more,items`, max8 rows, existing256-event/416KiB retained history bound plus the bounded new capture. The concrete career reader remains the authority; the new shape validator cannot authenticate arbitrary historic rows.

## Exact downstream family mapping

This table freezes a type substitution, not a relaxation of any old decoder. Each named Owned type has precisely the field order of its corresponding inherited type, with the following substitutions made recursively. The unchanged inherited definitions are snapshotted in `owned-consumer-reference/inputs/inherited`; the producer catalog binds their exact bytes. New exported decoders use the owned type and the new family domain only. Private physical engine leaves receive separately validated physical fields; no old source handle, receipt authority or rewritten birth assignment is fabricated.

| Package | New closed types | Exact substitutions |
|---|---|---|
| gymentry | OwnedIntent, OwnedOperation, OwnedMutationResult, OwnedEventStatus, OwnedHistoryPage | Old ordered fields; OwnedReceipt replaces Receipt, OwnedPin replaces Pin. Request and Event remain the unchanged value types. |
| gymentry | OwnedReceipt | `version,store_id,entry_id,event,operation_id,operation_sequence,generation,view_id,action,previous_revision,revision,roster_revision,ownership_head,committed_at_ms,members`; members are OwnedMember. |
| gymentry | OwnedCutoffEntry | `version,store_id,event,entry_id,committed_count,last_sequence,history_digest,latest_receipt`; receipt is nullable OwnedReceipt. |
| gymentry | OwnedCutoffSnapshot | `version,cutoff,entry,roster,members,excluded`; cutoff is exact CutoffRecord, entry is OwnedCutoffEntry, roster is inherited CutoffRoster, members/excluded are the shapes above. |
| gymevent | OwnedInfo, OwnedEventStatus, OwnedHistoryPage | Old ordered fields; Pin becomes OwnedPin. Event remains the immutable inherited Event type with owned IDs. |
| gymevent | OwnedCaptureEvidence | `version,participant_id,stable_fence,entry_fence,cutoff_id,captured_at_ms`; inherited fence shapes, exact core cutoff digest. |
| gymevent | OwnedParticipantCapture, OwnedCaptureResult | Old ordered fields with owned snapshot/evidence/status substitutions. |
| gymseed | OwnedPin, OwnedInfo | Old ordered fields; source_pin is gymevent.OwnedPin. |
| gymseed | OwnedDeclaration, OwnedSignedDeclaration | Old ordered fields; declarations bind the exact owned event snapshot/receipt digests. Signature is over the new owned domain. |
| gymseed | OwnedRecord, OwnedStatus, OwnedHistoryPage | Old ordered fields; source_receipt is gymevent.OwnedSnapshotReceipt, declaration is OwnedSignedDeclaration; conflict declaration pointers use that type. Scan/publication/target/finality scalar objects stay unchanged. |
| gymseed | OwnedCardSource, OwnedCardState, OwnedCardView | Old ordered fields; source_receipt/declaration/status/pin are owned types. |
| gymcard | OwnedPin, OwnedInfo | Old ordered fields; source_pin is gymseed.OwnedPin. |
| gymcard | OwnedRecord, OwnedStatus, OwnedBuildResult, OwnedReadResult, OwnedHistoryPage | Old ordered fields; source is gymseed.OwnedCardSource or OwnedCardState. Plan, Counts, Entrant and Bout physical shapes remain unchanged; GymID/AccountID come from the locked effective-owner corner. |
| gymcard | OwnedBoutCorner | `owner,member`; owner is finalized_public_chain AssignmentOwner and member is gymevent.OwnedMember. |
| gymcard | OwnedBoutSource, OwnedBoutState, OwnedBoutView | Old ordered fields; corners use OwnedBoutCorner, pins/source states use owned types. |
| gymbout | OwnedPin, OwnedInfo | Old ordered fields; source_pin is gymcard.OwnedPin. |
| gymbout | OwnedRecord, OwnedSavedBout, OwnedStatus, OwnedCardStatus, OwnedRunResult, OwnedReadResult | Old ordered fields; source is gymcard.OwnedBoutSource/OwnedBoutState. fighterbout.Input/Result are unchanged deterministic physical values. |
| gymbout | OwnedResultSource | Old ordered fields; source is gymcard.OwnedBoutSource. ResultSummary remains unchanged. |
| gymresult | OwnedPin, OwnedInfo | Old ordered fields; source_pin is gymbout.OwnedPin. |
| gymresult | OwnedStatement, OwnedSignedStatement | Old ordered fields; source is gymbout.OwnedResultSource, signature uses the owned domain. |
| gymresult | OwnedRecord, OwnedStatus, OwnedActionResult, OwnedReadResult | Old ordered fields; statement/equivocation are OwnedSignedStatement; physical publication/inclusion/finality fields remain unchanged. |

Owned flags are never added to inherited types. Store families use `SLUG-OWNED-{STABLE,ENTRY,EVENT,SEED,CARD,BOUT,RESULT}-V1.json`, each marker ordered `format,version,store_id,pin`, format `slug-owned-` plus lowercase family. Each StoreID is H("<family> store", its OwnedPin). Other new IDs use H with labels `entry`, `entry receipt`, `entry cutoff history`, `event`, `event participant`, `event snapshot`, `event receipt`, `seed declaration`, `seed declaration signature`, `seed record`, `seed card source`, `card input`, `card`, `card record`, `card bout source`, `bout record`, `bout result source`, `result statement`, `result statement signature`, `result record`, and `fighter history`. Existing private simulation/matching seed domains stay unchanged once supplied the owned snapshot-derived InputID. Identical old physical inputs are still computed by the same engine.

Event `OwnedRulesID` is H("event rules", the inherited gymevent RulesID ordered JSON body with only `ownership_basis` set to `finalized_public_chain`). The inherited exclusion order, XP0/training0, two participants/five members, strict cutoff and immutable later changes stay identical. Entry/seed/card/bout/result physical RulesID functions remain unchanged; the complete owned source pin and family domain supply the new authority binding. IncidentDeadline is the inherited next Saturday noon in the pinned calendar, not a fixed short timeout.

Entry ID preimage is `version,store_id,event_id,gym_id`. Event ID preimage is `version,store_id,source_event_id`. Participant ID preimage is `version,event_store_id,entry_store_id,gym_id`. Seed event key preimage is `version,store_id,event_id` with H("seed event", value); result key is `version,store_id,bout_id` with H("result key", value). Seed/card/bout/result digest and signature preimages otherwise retain the complete corresponding owned typed value or its unsigned body, in the listed field order; no digest drops the ownership evidence. Roster/entry cutoff history starts with `version,store_id,event_id,cutoff_ms` (roster event_id empty) and advances by hashing `previous_digest,operation` under the respective `stable cutoff history`/`entry cutoff history` label for each committed-before-cutoff operation. Strict before means commit timestamp < cutoff_ms.

Card ID is H("card", `version,store_id,event_id`). Card input ID is H("card input", `version,card_id,snapshot_digest,seed_event_key,target_height,target_hash,rules_id`) in that order. Validate the complete owned seed source and the exact event/card/seed-event relationship before deriving either ID. Matching seed and bout seed byte recipes remain the inherited physical engine recipes over this owned input ID. Owned seed declaration and result statement signatures sign the decoded 32 bytes of H("seed declaration signature", the complete OwnedDeclaration) and H("result statement signature", the complete OwnedStatement), respectively; they do not sign the hexadecimal display string or an old-family signature message.

Owned result career activation has the exact inherited closed v1→v2 transition in its own namespace. The v2 marker is ordered `format,version,store_id,pin,career_phase,career_intent_digest,career`; format remains `slug-owned-result`, version is2, phase is `prepared` or `ready`, and career is null only while prepared. The intent is ordered `version,policy_id,result_store_id,result_created_at_ms,source_created_at_ms,old_marker_sha256`. Its digest is SHA256 of compact canonical intent bytes; old_marker_sha256 covers the exact v1 marker including its one LF. The inherited careerstate ActivationID/PolicyID recipes bind the new ResultStoreID and original source/store/activation times. Activation must precede any finalized result. Only exact durable intent/marker/metadata prefixes may resume; an established ready marker never silently resumes, replaces, or rebinds a missing component. Old result namespaces and activations remain unchanged.

## Finite encoded and retained limits

All new pins/markers/info are bounded by64KiB, metadata by96KiB. Stable bounds above remain tighter. Entry member6KiB, terminal receipt48KiB, operation56KiB, cutoff entry52KiB, event participant104KiB, event input224KiB, event receipt232KiB. These include up to5 compact birth+state members and5 exclusions, not repeated full admissions; source authority remains the concrete core handle. Core cutoff is bounded8KiB. Seed record384KiB and CardSource272KiB include event receipt232KiB plus inherited publication/header/scan/conflict limits; seed response448KiB. Card record320KiB and response384KiB include CardSource272KiB + unchanged Plan16KiB. Bout source28KiB, result source32KiB, bout record272KiB/response304KiB include unchanged simulation result192KiB. Result statement36KiB, signed statement37KiB, record224KiB, response272KiB. View/status/history responses reserve their complete maximum declared item count before serialization. No parent IPC frame may claim the old128KiB cap accepts a new owned page.

Per-store event/operation counts stay256/4096, members5, participants2, saved bouts256, pages8, source admissions1000. Full owned career history retains at most256 rows under512KiB, inclusive of its new capture; existing416KiB row inventory is unchanged. No concurrent duplicate capture/body scratch owner is admitted by one store operation lane. These are accounted encoded/resident-value reservations, not a total Go-heap claim. The independent size composition checks conservative inner caps plus maximal escaped scalar syntax; implementation refuses before growth if a bound cannot fit.

## Reference scope and execution gate

The immutable input catalog is frozen before producer authoring: approved public-core values; original Set64/65 public admissions, assignments and career rows; old independently frozen v1/v2 manifest and exact-integer condition oracles; old consumer field definitions. Their chain-model states remain conditional structural facts, not claims of actual transferred fighters. The producer may create a separately signed modeled cutoff at the exact Saturday noon and owned EventID using the existing published core fixture seed; this is labeled modeled attestation, not observed source chronology. It does not mint work, alter original admission/career bytes, or read product output.

First fixtures cover owned pin/status/slot operation/item/page/history for original→recipient→reacquired states, birth/corner preservation, exact entry/event cutoff composition and transferred-out exclusion, plus new source-wrapper byte/domain composition. Negative inputs target cross-source/state/birth/cutoff/owner binding, closed field order/types/caps and stale-operation semantics. Physical match/bout arithmetic stays covered by inherited independent vectors plus finite integration tests. No expected new Go output may be used. A reviewed exact source+input lock is required before importing/executing the producer.

## Narrow legacy-career compatibility attachment

An owned stable may retain at most two concrete career reader capabilities: the current owned-results reader and one original legacy-results reader. `OpenOwnedCareer` remains current-only. `OwnedCareerBindings{Current careerstate.Activation, Legacy *careerstate.Activation}` and `OpenOwnedWithCareerBindings(path,expected,source,bindings)` make both expectations explicit on reopen. `BindLegacyCareer`/`AttachLegacyCareer` retain the original reader separately; current `BindCareer`/`AttachCareer` preserve their agreed spelling. Neither accepts arbitrary historical rows.

The current reader must authorize the exact OwnedStoreID. The legacy reader must authorize `gymstable.ID(expected.Stable)`, independently derived from the complete original account/source Pin2. This checks its authentic unchanged scope without constructing a legacy Store, altering that reader, or claiming the new store ID was in its scope. The owned stable marker and original core ownership capability separately authorize disclosure to the current owner. Both guard bindings are durable and exactly audited; missing an established reader/activation refuses the operation.

For each requested ProspectID, capture both concrete sources outside account leases. If only one contains professional events, retain that source's original activation, digest, totals and exact rows; the other must prove zero. If neither has events, choose the current activation. If both have events, refuse explicitly as `owned_career_conflict`; never concatenate, choose a latest source, discard events, recalculate old AwardIDs, or clear C1. This is a bounded compatibility restriction of this candidate's existing zero-record match route, not a general production league policy. C1 eligibility checks both sources before admitting a new bout. Source lifetimes and the original current-owner View are checked again after projection and before disclosure.

`current_gym_wins` is a uint32 not greater than `totals.wins`. It counts authentic selected-fighter wins whose immutable winning event corner GymID equals the current owner gym. `totals.wins` preserves the full professional career from the selected original source. A transfer never reassigns a prior gym's win. This is a descriptive count of raw historical wins at the currently selected gym; after reacquisition it includes earlier wins at that same gym. It does not authenticate the current tenure or confer title requalification/eligibility. Title requalification remains unavailable without its separately authenticated current-tenure rule. Exact history rows retain all original account/gym corners. The item and history capture fields explicitly separate these meanings; public field validation alone cannot authenticate a win. No current-gym counter is fabricated for unavailable evidence. The fixed128/256/etc old bounds are not broadened; both readers share the one operation scratch reservation and retain at most one selected history of256 events/512KiB.
