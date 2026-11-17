# Set 41: candidate stable, roster and trusted condition

Status: precise engineering draft for root and independent review. No implementation or new expected-value corpus is authorized by this document alone. Baseline is set 40 commit `78d04c9c`, staged at `/Users/taytrrip/slug/set41-stable-stage`; full base provenance is in `set41-stage-base.json`. The accepted scope is `/Users/taytrrip/slug/set41-next-slice-proposal.md`. Existing set-35/37/39/40 contracts and independent inputs remain unchanged.

## 1. Scope and guarantees

Update the existing SLUG Candidate Pilot bundle/data root with automatic candidate inventory, ten reversible roster slots, paged fighter details and exact current condition. There is no new demo bundle. Keep old noncandidate pilot modes and their namespaces untouched. No professional entry, training award, XP mutation, result, transfer, discard, ownership-token creation, marketplace or official descriptor activation is added.

D44/D35 initial assignment is automatic from the authenticated mining gym. This store records **candidate gym assignment**, not transferable production ownership. The immutable schema-5 admission is not rewritten. Public receipt JSON, an observer argument, UI cache and inspected directory metadata cannot authorize a new assignment. The only source is a constructor-borrowed concrete pinned `*registry.ManifestStore`, using its full verified read APIs.

D72 interpretation: a newly authorized current login may make these reversible roster moves while held. Existing asset/recovery/market restrictions remain exact. Pending reset, revoked/expired sessions and stale account views still fail. This is an interpretation of the existing enumerated restrictions, not a new D decision. No new Mike policy decision is required for this scope.

## 2. Package, constants and pins

New root-module package `gymstable`; no wallet-module change and no generic receipt-import API. Use existing LevelDB durability primitives, not a new dependency. `Version1 uint32=1`, `RosterSlots=10`, `MaxPageSize=8`, `MaxAssignments=1000`, `MaxOperations=4096`, `MaxMarkerSize=4096`, `MaxMetadataSize=4096`, `MaxAssignmentSize=2048`, `MaxOperationSize=8192`, `MaxRequestSize=8192`, `MaxResponseSize=131072`, `MaxItemSize=8192`, `MaxSafeInteger=9007199254740991`, `MaxTimestampMS int64=8640000000000000` (the existing native Date bound). All are hard limits, not user-overridable parameters. Assignment/operation limits are fixture resource capacity, not gameplay ownership or weekly signing limits.

`Pin` fields, JSON names/order, all required:

1. `Version uint32` → `version` (1).
2. `AccountAuthorityID string` → `account_authority_id`.
3. `AccountID string` → `account_id`.
4. `Identity gymaccount.Identity` → `identity` (existing exact field order).
5. `Source registry.ManifestAuthorityPin` → `source` (existing exact field order).
6. `ConditionVersion uint32` → `condition_version` (fightercondition.Version1).
7. `ProjectorID string` → `projector_id` (fightercondition.ProjectorID).

Both IDs are canonical 64-character lowercase hex; zero is structurally allowed. Identity must exactly equal one of the existing two compiled `gymaccount.FixtureIdentity` tuples. Source must be supported candidate schema 5, exact Phase-A network/genesis and compiled descriptor; its chain must equal Identity's chain. Source.AuthorityID must differ from AccountAuthorityID, preserving the existing strict ManifestViewPin namespace check; either ID may individually be all zero when the other differs. The recovery profile is still the existing fixture profile, not candidate production-wallet profile 30. Generation, view ID, session token, origin/CA selection and observation time are **not** mutable additions to this stable pin. Existing set-40 manifest identity continues to pin its service origin/transport trust separately.

`ValidatePin(Pin) (Pin,error)` returns a validated owned value. `ID(Pin) (string,error)` is lowercase SHA256 of ASCII `SLUG candidate stable\x00v1\x00` + LE32 length of canonical Pin JSON + those JSON bytes. This is a configuration namespace, not a signature or portable proof of authority.

No production pin catalog, caller `official` flag, descriptor override, arbitrary fixture identity or custom resource ceiling is accepted. Create/Open and every source-trusting operation refuse `wire.TestPoW` before filesystem creation/disclosure, following schema-5 normal-profile handling.

## 3. Core API and trusted dependencies

Exact proposed exported surface (error returns always carry zero result values except an explicitly successful replay):

```go
func Create(path string, pin Pin, source *registry.ManifestStore) (*Store, error)
func Open(path string, expected Pin, source *registry.ManifestStore) (*Store, error)
func (s *Store) Close() error

type Lease func(context.Context, gymaccount.Capability,
    func(gymaccount.Account, func() error) error) error

type Session struct {
    Generation uint64
    ViewID string
    Lease Lease
}

func (s *Store) Status(ctx context.Context, session Session) (Status, error)
func (s *Store) Sync(ctx context.Context, session Session) (SyncResult, error)
func (s *Store) Inventory(ctx context.Context, session Session,
    after uint64, limit uint32) (Page, error)
func (s *Store) Roster(ctx context.Context, session Session,
    firstSlot uint32, limit uint32) (RosterPage, error)
func (s *Store) PrepareSlot(ctx context.Context, session Session,
    request SlotRequest) (Intent, error)
func (s *Store) ApplySlot(ctx context.Context, session Session,
    operationID string) (MutationResult, error)
func (s *Store) Operation(ctx context.Context, session Session,
    operationID string) (Operation, error)
func (s *Store) Pending(ctx context.Context, session Session) (*Operation, error)
func (s *Store) Abandon(ctx context.Context, session Session,
    operationID string) (Operation, error)
```

`source` is borrowed, never closed by Store. Constructor checks source.Info against the exact Pin, including normal-profile support and count <=1000. It retains this concrete handle for its own lifetime; it cannot be replaced by an interface returning arbitrary records. A Store must be closed before its source closes. No public `Insert(record)`, `SetXP`, `SetOwner`, `SetClock`, verification bypass or raw database getter exists. Deterministic clock/random/write/crash seams are private tests only.

`Session` is private trusted helper configuration, never JSON. Generation is 1..MaxSafeInteger; ViewID is canonical 64 lowercase hex. Parent captures original token, account, generation and view epoch into Lease. The lease checks the actual account-authority handle against Pin.AccountAuthorityID. Core checks account ID, generation and exact nonnil enrollment identity inside every final lease. It never substitutes a later active token. The trusted caller must not call core methods from inside an already held account lease.

Add `gymaccount.RosterManage Capability = "roster-manage"` to valid capabilities, not held capabilities. It uses the existing enrolled/not-recovery-required and login-session rules; mining sessions remain Mining-only. No persisted session format changes, fresh-authentication timeout or new hold restriction. Read/status/source-derived synchronization uses View; only player selection/prepare/apply/abandon uses RosterManage. Synchronization writes only deterministic source-derived facts, never caller assignment data. Operation/Pending queries use View and may disclose historical outcomes to a newly authorized view of the same account.

## 4. Durable representation and audit

Directory contains `SLUG-GYM-STABLE.json` (canonical marker + exactly one LF) and `db/`. Marker is `{application:"slug-candidate-stable",version:1,store_id,pin}` in that order. Metadata repeats exact store ID and Pin plus `checkpoint`, `observed_source_count`, `roster_revision`, `operation_count`, `pending_operation_id`, `last_operation_id`, `clock_ms`, `last_as_of_ms` in that order. Empty optional IDs are empty strings, not omitted/null. Initial checkpoint/revisions/operation counts/as-of are zero; observed_source_count is the validated source.Info count N already known at creation. Clock is an observed positive Unix millisecond value <=MaxTimestampMS; a known backlog therefore cannot be advertised as caught up. Root and files are private, real directories/regular files, not symlinks; use exclusive creation, synced database batches and file/parent-directory synchronization. Existing nonempty unknown state is never overwritten.

Keys are exact ASCII `meta`, `roster`, `a`+8-byte big-endian sequence, `i`+32 raw prospect-ID bytes, and `o`+32 raw operation-ID bytes. The `i` value is exactly the 8-byte sequence. Metadata invariants are `0<=checkpoint<=observed_source_count<=1000`, `0<=last_as_of_ms<=clock_ms<=MaxTimestampMS` with clock_ms>0, and `0<=roster_revision<=operation_count<=4096`. Assignment time obeys `0<born_at_ms<=accepted_at_ms<=MaxTimestampMS`. Operation sequences are exactly1..operation_count, unique; last_operation_id identifies that final sequence. prepared state occurs at most once and exactly matches pending_operation_id; a terminal row is never the pending row. Every operation time is positive and <=clock_ms; committed/abandoned times are >=prepared time. No unbounded key is used in another lookup before its shape is checked. `roster` is canonical `{version:1,revision,slots}` with exactly ten strings, in slot order; empty string means empty slot. Repeated nonempty IDs are invalid. The revision must match metadata.

Assignment required fields/order: `version`, `sequence`, `prospect_id`, `gym_id`, `work_hash`, `born_at_ms`, `accepted_at_ms`, `descriptor_id`, `manifest_digest`, `portrait_digest`, `source_record_digest`. Digests and nonempty IDs are canonical lowercase 64 hex. Sequence is 1..1000. Source-record digest is SHA256 of ASCII `SLUG stable source record\x00v1\x00` + LE32 canonical schema-5 record byte length + exact canonical record bytes (no LF). This pins the complete original receipt, including acceptance/birth evidence; matching static digests alone is insufficient. The full proof is retained by the source, not duplicated in this DB.

The source is deliberately **dedicated to this one gym**, matching set 40. Every consumed sequence must have this exact enrolled gym/public key. A foreign-gym row is corruption/incompatible source, not skipped progress. Thus assignments are exactly consecutive 1..checkpoint and source indexes are reciprocal/unique. Count regression below previously observed source count fails closed. Disappearance or mutation of a consumed source row fails closed when that row is accessed/selected; cheap Open/Status and a no-op Sync do not reauthenticate every retained receipt. A concurrent append beyond the initial count snapshot is allowed; it is handled by the next batch.

Open performs bounded raw shape/length/count checks before decoding/copying, exact marker/metadata equality, all assignment/index links, roster membership, operation graphs and pending/last links. Unknown keys, nested/synthetic keys, missing mandatory rows, noncanonical encoding, impossible counters or unexplained rows fail. At most1000 assignments,1000 ID indexes and4096 operation rows are scanned; no Argon on Open. Cheap Open also reconstructs the ten slot IDs from the committed changed-operation receipts in roster-revision order: revisions exactly1..roster_revision, previous_revision=revision-1, matching previous slot value and resulting full slots. Each committed no-op matches the reconstructed state at its unchanged revision. A changed operation has exactly one slot change; abandoned/prepared rows never affect slots. Cross-operation intent sequences and event timestamps must not contradict that order. Final reconstructed slots equal the roster row. Operation count and terminal/pending graph must exactly match metadata; a self-consistent receipt is not permission to silently omit its row. Cheap Open does not cryptographically authenticate all source records. Every delivered item and newly selected member is reverified as described below. Coherently rewriting all local authority/storage state is outside these unsigned local-store guarantees; this does not create portable authentication or rollback-proof hardware storage.

Storage JSON is exact canonical struct-order UTF-8 from Go encoding/json with no insignificant whitespace; marker alone adds LF. DTO parsers are whitespace/order tolerant but reject duplicate decoded keys, unknown/null fields, trailing tokens, floats/exponents/noncanonical signed spellings for integer fields, invalid UTF-8, noncanonical IDs, wrong enums, excessive raw bytes or nesting. Top-level requests <=8192 bytes/depth2; core stored marker/meta/operation <=their caps/depth5; public responses <=131072 bytes/depth9. Depth counts every JSON object/array container, including the root as1; the roster Item→manifest→DNA→attributes path reaches9. Validate exact canonical serialization fits the relevant bound before every write/publication. No successful typed validator may produce bytes its parser rejects.

## 5. Clock, revisions and serialized operations

Use a private wall clock returning Unix milliseconds, with persisted `clock_ms` and in-memory highwater. Only after initial successful authorization, observe under the short DB gate before expensive work/rejection; reject nonpositive, >MaxTimestampMS or rollback. Persist a newly observed highwater synchronously before returning a time-based rejection. Observe again after source work and before final commit/publication. No caller timestamp or web clock is authoritative. Never clamp a backward clock into apparent freshness.

`checkpoint` changes only on source assignment. `roster_revision` changes only when slot membership changes. Clock/as-of persistence, reads, catch-up, intent preparation, replay and abandon do not increment roster_revision. `operation_count` counts all retained intent rows, including abandoned/committed ones. No automatic tombstone eviction. Capacity cannot prevent completing/replaying/querying/abandoning an already reserved intent; it only refuses a new Prepare. Existing evidence remains readable at capacity.

All Store operations serialize through one cancellation-aware operation gate. DB mutex is held only for bounded local reads/commits, never across a source read or account callback. Order: short initial account authorization; release it; capture stable metadata/rows; release DB gate; perform concrete-source verification; final original account lease → short stable DB gate → final context/account recheck → one synced commit/publication. Never call source while holding account or DB gate. Parent/controller locks are not held while joining operations. Public callbacks may not reenter Store. Close cancels queued/running work, joins the actual current noninterruptible source hash, allows no following hash, closes DB once and retains close failure.

A caller cancellation after a commit, outer lease post-check error, or uncertain synced write must not be reported as a definite rejection. Return `ErrOutcomeUnknown` with zero output; retain the journal state. A write that reports error poisons the handle even if it may have committed. Reopen audits and reconciles. For reads, a post-check error returns no page; a clock highwater write is not a roster mutation. Do not rollback already synced source assignments or slots.

## 6. Synchronization and verified reads

Every public operation performs at most eight **serial** normal-PoW source verifications, including selected-member verification. No speculative goroutines or second hidden legacy cold-eight pass. Source APIs do not take context: check cancellation before and after each call, join its actual Argon work, and never start the next call after cancellation.

Sync snapshots source.Info count N and the current checkpoint C, verifies exact contiguous records C+1..min(C+8,N) using `ListManifestAdmissions(seq-1,1)`, and validates exact pin, canonical key/gym/descriptor, D68 birth, verified work/DNA and static hashes. Produce candidate assignments from those exact records. Before committing, final View lease checks original account/enrollment, and stable gate rechecks C and immutable rows. Assignment rows, reciprocal indexes, new checkpoint, observed N and clock/as-of commit in one batch. No partial batch. Capacity failure cannot advance consumed position. Duplicate retry after a lost acknowledgement sees the existing exact prefix. Never infer absence of new records beyond the captured N.

Inventory returns at most `limit` contiguous assignments after `after`; `limit` is1..8, `after`0..checkpoint. Fetch each by full ID through source.GetManifestAdmission and compare the complete source-record digest and every saved assignment field before projection. Roster returns the fixed ten slot IDs from one roster revision, but details only for slot interval `[firstSlot, min(firstSlot+limit-1,10)]`; firstSlot1..10, limit1..8. Only nonempty slots consume proof checks. If roster revision or a captured immutable row changes before publication, fail `ErrConflict` with no mixed page. No automatic retry loop or concealed extra eight verifications in one operation.

Both pages derive manifest/condition from those verified source records, never stored summaries. Use condition Version1, exact canonical DNA, original issuance birth and one newly observed final `as_of_ms` for every item. XP is exactly0 in this no-professional-events namespace; there is no missing-history fallback for another profile and no XP setter. Source birth or accepted_at_ms > final as-of is a clock error; an observation cannot precede the admission whose condition it presents. Persist that as-of before returning. Aged values, no fractional floor, XP-after-aging and D65 weights remain exactly set35. Condition does not assert official eligibility. No training/form/record/points/belt data is synthesized.

Core `Item` has exported `Assignment`, `Manifest fightermanifest.Manifest`, `Condition fightercondition.Snapshot` and an unexported owned source record. `func (Item) Receipt() registry.ManifestRecord` returns a deep-owned copy for the existing private receipt/portrait cache; this is a read result, never an accepted input. Each item serialization <=8192 excluding its private record. The controller may feed existing appearance/receipt views from this same verified pass without a second hash. Retained in-memory full evidence remains bounded to8 items plus the existing one selected export slot and existing1MiB cache. The UI retains only the current inventory/detail page; ten slot IDs are separate cheap metadata, not ten extra full proof caches. Before retaining any completed public job/Page DTO, the controller composes new Item values from only Assignment/Manifest/Condition, leaving the unexported receipt zero after transferring selected evidence into the bounded private cache. Merely JSON-marshaling an original Page and also retaining that original Go value would keep a second full receipt set and is forbidden. Public results/returned copies are not hidden proof caches.

The active page policy is explicit. The initial/live Recent view follows the bounded latest-eight policy and may evict its oldest unselected item as new accepted candidates arrive; the independent selected export slot remains protected. Selecting a fixed inventory interval or roster page pins that visible page's private evidence until navigation/cancellation/view change. A live Sync never silently replaces that page or its export selection: it updates durable counts and a bounded public arrival summary/badge. Newly arriving public summaries/manifests may be retained for the Recent view without their raw receipts, subject to the same aggregate1MiB budget; preserve the selected page/export slot first, falling back to the arrival count if there is no public-cache space. Portrait rendering may use that already validated in-memory manifest under the existing final View check; it does not require a duplicate proof. Selecting a receipt absent from the private cache schedules the normal bounded source-read job before native export preflight. Returning to Recent uses an asynchronous verified page; an evicted completed job returns not-found and prompts an ordinary page reload, never trust of lost terminal JSON. Do not mutate a retained completed job result into a different page under the same job ID. Per-card as_of remains its actual observation; a new live row does not freshen older cards with the browser clock.

## 7. DTOs and the one pending roster intent

All following JSON names are snake_case in the listed order. Values are required, without omitempty; nullable fields are explicitly noted. Nested Pin/Identity/Manifest/Condition retain their frozen field order. Arrays are nonnil; slots always have length10. All public counters/IDs/timestamps and request integers are nonnegative plain decimals <=MaxSafeInteger, with narrower bounds stated here. Preserve the frozen manifest exception `item.manifest.portrait.shoulder_jitter`: canonical signed integer -6..6 is valid. No other signed fields, -0, plus signs, floats or exponents are accepted. Native/frontend structural validation follows exact nested field paths, not a blanket unsigned-token rule.

- `Status`: `version`, `store_id`, `pin`, `checkpoint`, `observed_source_count`, `caught_up`, `roster_revision`, `slots`, `operation_count`, `pending_operation_id`, `last_operation_id`, `last_as_of_ms`. `caught_up` means checkpoint equals the **last observed** source-count snapshot; it does not certify no later source append. Counts <=1000/4096; revisions <=4096. Initial last_as_of_ms may be0.
- `SyncResult`: `version`, `status`, `as_of_ms`, `items`. Items are the <=8 assignments committed/returned in this batch, in ascending sequence. Empty batch has an empty array; no fake item.
- `Page`: `version`, `status`, `as_of_ms`, `after`, `next_after`, `has_more`, `items`. Sequence order ascending, next_after equals last returned sequence or input after for an empty page. has_more compares next_after with this page's checkpoint snapshot, not with unassigned source evidence.
- `RosterPage`: `version`, `status`, `as_of_ms`, `first_slot`, `next_slot`, `items`. Items are nonempty requested slots in slot order, each `{slot,item}`. next_slot is the next requested interval start or0 after slot10. The status's full slot vector/revision belongs to the same captured state as these details.
- Core `Item`: `assignment`, `manifest`, `condition`. No raw proof, authorization or private receipt member serializes. The nested manifest is the frozen bounded v1 value. All condition as_of values equal the page/batch as_of; names may duplicate and are never keys.
- `SlotRequest`: `expected_revision uint64`, `slot uint32`, `prospect_id string`. Slot1..10; empty ID means clear, otherwise canonical64hex. No client timestamp, XP, owner, generation, receipt or array of arbitrary operations.
- `Intent`: `version`, `sequence`, `operation_id`, `generation`, `view_id`, `request`, `prepared_at_ms`. Generation/view are captured from Session, not caller request fields. Sequence is the reserved operation_count+1 (1..4096). Operation ID is fresh32-byte randomness encoded lowercase hex, generated only inside Prepare. Collision retries are bounded to8 then fail, with no overwritten row.
- `MutationReceipt`: `version`, `operation_id`, `generation`, `view_id`, `request`, `previous_prospect_id`, `changed`, `previous_revision`, `revision`, `slots`, `committed_at_ms`. This is the immutable original outcome, not necessarily the current roster after later edits.
- `Operation`: `version`, `intent`, `state`, `receipt`, `abandoned_at_ms`. States exactly `prepared`, `committed`, `abandoned`. receipt is null except committed; abandoned_at_ms is0 except abandoned. A prepared intent never contains a predicted/fake receipt.
- `MutationResult`: `version`, `receipt`, `replayed`.

Prepare is metadata work with no proof verification, but its initial account lease and source.Info can wait behind a live admission. Therefore helper Prepare also runs as an asynchronous tracked job; it cannot block serialized stdin. It requires current RosterManage, a caught-up source count observed for this operation, known local assignment, no duplicate placement in another slot, and matching expected roster revision. It reserves a full worst-case8192-byte operation row before publishing the intent. Only one prepared operation may exist. After current RosterManage authorization, an identical pending request from the same original generation/view returns that exact intent before fresh source-count, capacity or CAS checks; later admissions cannot invalidate an already prepared intent replay. Another request returns ErrPending. Only creation of a new intent performs the readiness/capacity/CAS admission checks. A new current view must first query and explicitly abandon an old uncommitted intent; it cannot silently replace it. Capacity refusal cannot delete the existing intent or prevent its completion/reconciliation.

Apply first authorizes current RosterManage and looks up the exact operation ID. If already committed, it returns the original receipt with replayed=true, before stale roster-CAS checking and without another source hash, provided the original intent generation/view matches this mutating Session. Abandoned returns ErrConflict. A prepared intent with another generation or view returns ErrStaleView and never executes. Querying a committed old-view outcome uses Operation under a new current View session; it does not replay a mutation.

For a prepared nonempty target, Apply verifies that one selected source receipt outside the lease and compares its entire saved assignment binding. Clearing consumes no proof verification. It then rechecks original intent/state, current roster revision, assignment bytes, duplicate membership, context and captured account/view under the final lease, and writes roster + immutable receipt + cleared pending link + clock in one synced batch. A no-op setting a slot to its current value is a successful committed operation with changed=false and unchanged roster revision. Actual membership change increments roster revision by1. Old assigned candidates are never deleted. There is no swap/reorder/transfer operation; replacing one slot is atomic.

Prepare and execution of a prepared Apply require source catch-up at their observed source-count snapshot; terminal replay/query does not require renewed synchronization or reinterpret the old outcome; if a new admission makes the stable lag, return ErrSyncRequired without executing the intent and schedule automatic catch-up. The intent remains reserved. No-op/replay and existing-operation queries remain available at operation capacity. Query/Pending returns exact old generation/view facts in the result, enclosed by the **current** authorized view; do not confuse those two identities in native validation.

Abandon requires current RosterManage for this same account and can mark a prepared older-view intent abandoned, clearing its pending link atomically. It does not clear a slot, delete an operation or alter a committed outcome. Committed or already-abandoned query returns its historical terminal operation; never change that terminal state. One player click normally runs Prepare then asynchronous Apply without another confirmation. After ambiguity, query the same durable operation ID before any retry. If Prepare's acknowledgement was lost, Pending/last-operation metadata locates the reserved intent; do not invent a fresh ID. New login/restart cannot execute an old-view prepared intent: display its unresolved state, query, then explicitly abandon before a new user action. No session token is persisted in this journal or passed through IPC.

## 8. Additive installation in the existing set-40 root

Keep the same Candidate Pilot application and `LocalManifestPilot` directory. Existing wallet DBs, child manifest/READY/CHAINS markers, schema-5 authority, outbox2, account authority and all original receipt bytes remain unchanged. No registry or wallet schema migration occurs.

A missing standalone stable marker is insufficient evidence that this feature was never installed. Upgrade the **mandatory account-root marker** `SLUG-MANIFEST-ACCOUNTS.json` to a separate exact v2 type, under its existing exclusive instance lock. Fields/order: `version:2`, `mode` (unchanged), `account_authority_id`, `descriptor_id`, `stable_schema:1`, `stable_installs`. Each install is `{account_id,pin,phase}`; phase is `prepared` or `ready`; list is sorted by account ID, unique, max2 (the existing two-fixture account catalog). Parent marker cap is8192 bytes. The old exact v1 decoder/bytes are retained for upgrade input; do not parse a v1 object through a v2 struct and silently add zero fields.

Validate original root v1 marker, actual account authority ID/configuration and known gym-directory names first; validate ready-child evidence where ready. A recognized existing new/initializing child retains its old fail-closed behavior and is not silently replaced or declared ready by this root-format upgrade. Atomically publish v2 with an empty install list using file/parent sync. An older set-40 binary then rejects the unsupported marker version instead of writing around the new state. This is an explicit additive feature upgrade in this same app, not adoption of a foreign directory. An orphan/invalid existing account authority remains an error; Inspect does not turn it into trusted configuration. A clean old root can be upgraded without replacing credentials, keys, payout, admission pin or original history.

After child Prepare/enrollment has established its normal ready identity, derive the exact stable Pin from that trusted state. For an account absent from stable_installs, require that its fixed child `stable/` path is absent; write a prepared install entry **before** creating that component. Path is always `<root>/gyms/<accountID>/stable`, never caller-selected. Create an empty stable, close/sync it, then atomically change the root entry to ready. **No source assignment or roster operation is allowed before ready is durably acknowledged.**

On restart, prepared+absent path may resume empty creation because no stable writes were ever permitted. Prepared+complete existing store may finish only after exact pinned Open and proof that checkpoint/operation_count/revision/slots are all empty. Prepared+partial/unknown/nonempty component fails closed and is retained; no RemoveAll/recreate. Ready requires complete matching stable state forever; missing marker/DB, changed pin, missing parent entry or an unlisted existing component fails. A lost ready-ACK is recovered by rereading the mandatory marker, not by trusting an empty-looking directory. Store assignment loss cannot be disguised as an ordinary set-40 first launch unless the entire trusted root state is rolled back, which is outside this local durability claim.

The parent performs only this bounded establishment during login (no history hashing). Current token/account enrollment checks bracket installation, but even a later logout cannot cause a completed component to be deleted. On uncertainty, retained intent/markers decide restart; the UI does not show successful attachment from an uncertain write. New account roots start directly with v2. Old noncandidate roots and constructors remain unchanged. Do not activate the new capability through an optional missing sidecar alone.

## 9. One automatic work lane and borrowed source lifetime

Cold login must not wait for all1000 receipts. Once private construction/enrollment/install validation succeeds, attach the authorized view with an honest `syncing` state, then run tracked automatic catch-up. One batch verifies at most8, commits, releases its lease, publishes progress and yields before the next batch. A generation/view change cancels the sequence. Error stops automatic retry; the player sees a plain Sync/Retry control, not implementation instructions to verify eight records. Stop pauses catch-up; Retry resumes from the durable checkpoint. Snapshot/polling does not start hashing.

The old set-40 cold recent-eight pass is **replaced** in the stable-enabled path, not run before it. Existing old constructor behavior remains available; the updated Candidate Pilot explicitly selects the stable-enabled constructor. A batch/page's same verified evidence can feed the appearance cache and new stable view. Older candidates are reached by ordinary page navigation. Do not label a prefix page `complete_recent`; preserve old frame semantics if publishing an old projection at all.

Concrete-source verification is intentionally retained: the present observer supplies a public record and has no opaque source-issued verification capability. Re-reading a newly accepted row through the owned source normally costs one additional normal256MiB Argon verification. State/measure this cost; do not create an unchecked import path merely to remove it. The callback ignores its argument as an assignment authority and synchronizes only from the concrete source. It may compare the notification ID/sequence as a hint, not grant trust from it. Missed/cancelled notifications recover by automatic source catch-up.

At most one stable job is active and one explicit job is queued. Explicit player work takes priority over the automatic catch-up-needed flag; a newly queued explicit read/action cancels an active automatic batch, joins its current proof, and runs before resuming automatic catch-up. This internal cancellation is not shown as a failure. The coalesced catch-up flag is metadata, not an unbounded queue of jobs. While mining runs, all stable proof work is served synchronously by the worker's single cycle lane; it never runs in a competing proof goroutine. Add optional `ManifestCycleHook func(context.Context) error` and `NewManifestWithHooks(config, authority, node, signer, outbox, observer, cycleHook)`. Existing NewManifest/NewManifestWithObserver delegate with nil hook and retain behavior. Call the hook at the top of each Run cycle, before pending reconciliation or a new work attempt, outside worker/outbox/account locks. Check ctx before/after. It serves at most one queued stable job or one outstanding automatic catch-up batch, then normal work gets a turn. The receipt observer may perform one direct Sync in that same lane after durable acceptance; it must not enqueue-and-wait on its own hook. No callback can supply a hash, seed, key, signer result or wire request; this is trusted in-process scheduling only.

Ordinary roster conflicts, stale requests, cancellation and page errors complete that job with a fixed code and return nil from the hook. Poisoned/corrupt source/stable state is a fatal local dependency failure. The new worker sentinel `prospectworker.ErrManifestCycleHook` maps callback error/panic to worker state failed and fixed status code `cycle_unavailable`; no raw error text enters worker status. The stable-enabled child maps this to StableSnapshot.error=`stable_unavailable` and the already-supported old ManifestSnapshot.LastError=`worker_failed`. Do not add this new stable code to, or weaken, the old ManifestSnapshot decoder. A nil hook never emits the new worker code. Hook failure drains normally. Stop cancels active/queued contexts, prevents any later request from executing, and joins the currently running source hash/commit only; no next proof starts. Hooks are not invoked from status polling. Waiting on an unavailable node/backoff may delay the next cycle; queued jobs have a30-second enqueue/execution deadline, so controls/status remain usable and a timed-out unstarted job is cancelled rather than executed later. In-flight cancellation follows commit/unknown rules.

The controller owns the operation lane and handles source lifetime. During mining it borrows `c.store`, already open for the authority service; it does not reopen the LevelDB path. While stopped it opens a pinned source handle and stable for its tracked work. Start cancels/joins the stopped loader before opening its live source. A short resource lock obtains/releases a tracked source borrow; **do not hold it across the final account lease** (recovery-export paths may already take account then resources). On shutdown fence new borrows, cancel/join jobs outside all account/controller/resource locks, then close stable before source. The existing service can finish its own writes during this process; source gate supplies serialization. No closed pointer may survive a stop/restart, and no callback may recursively acquire an already held setup gate.

UI read/action requests must not block helper stdin while hashing. They return an immediate tracked-job acknowledgement; a cheap query retrieves progress/result later. Controls can invalidate the captured view and cancel/join the active hash. Parent signed-in attachment does not wait for history and cannot exceed its existing login timeout because of a full stable scan. Roster mutations are disabled until the last observed source prefix is caught up; partial inventory/condition pages are honestly marked partial and remain readable.

## 10. New typed transport without changing old frames

Leave existing ManifestRequest/ManifestResponse and ManifestSnapshot Version1 byte grammar unchanged for old commands. Add separate `StableRequest`, `StableResponse`, `StableSnapshot`, `StableJob` and `StableOperation` Version1 types. The same helper process uses a bounded raw dispatcher: inspect only decoded command, reject duplicate/null/unknown fields under the selected exact parser, and route to the old or new decoder. Native read logic chooses the exact response decoder from its pending request's expected command family, not from arbitrary response fields. A mixed old/new union is rejected. No new fields are silently added to old frames.

Every new request has required `id` and `command`; id1..MaxSafeInteger. Commands and additional required fields:

| Command | Fields | Result |
| --- | --- | --- |
| `stable_snapshot` | none | cheap StableSnapshot |
| `stable_sync` | view_id | job ACK; resumes automatic sync |
| `stable_inventory` | view_id, after, limit | job ACK |
| `stable_roster` | view_id, first_slot, limit | job ACK |
| `stable_prepare_slot` | view_id, expected_revision, slot, prospect_id | job ACK |
| `stable_apply_slot` | view_id, operation_id | job ACK |
| `stable_job` | view_id, job_id | cheap tracked StableJob |
| `stable_operation` | view_id, operation_id | StableOperation |
| `stable_pending` | view_id | StableOperation (nested operation may be null) |
| `stable_abandon_operation` | view_id, operation_id | StableOperation |
| `stable_cancel` | view_id, job_id | acknowledged cancellation/status |

No path, arbitrary JSON record, owner, XP, as-of, token or private key is accepted. Old native-only receipt/recovery export routing remains exact. Every ID is canonical64hex except numeric request/sequence/slot/revision fields. No command accepts unrelated fields. Numeric strings/coercions are rejected.

StableResponse fields/order: `id`, `ok`, followed by exactly one of `snapshot`, `job`, `operation`, or `error`. All successful outer result variants are nonnull. Only StableOperation.operation may be null for a successful empty Pending result. ParseStableResponse alone permits that structurally possible null shape; the native expected-command correlation must reject it for stable_operation/stable_abandon_operation and allow it only for stable_pending. Parsing alone cannot identify the requested command. Error is a fixed string, never a nested exception. Failed responses have only id/ok/error. Existing per-line limits still apply, with new results bounded to131072 bytes including envelope/LF; preflight before emitting any bytes.

StableOperation required fields/order: `version`, `view`, `store_id`, `operation` (nullable only for Pending). The enclosing view/store ID are the current captured authorized response identity; nested historical intent/receipt author fields remain unchanged. They must match the current account and installed stable, so an old-view result cannot be accepted merely because its account ID matches.

StableJob required fields/order: `version`, `job_id`, `kind`, `state`, `view`, `store_id`, `operation_id`, `error`, `sync`, `page`, `roster`, `intent`, `mutation`. kind=`sync|inventory|roster|prepare|apply`; state=`queued|running|done|cancelled|failed|outcome_unknown`; view is the captured exact ManifestViewPin; operation_id is nonempty only for apply. Five result pointers are required nullable fields; only the matching one is nonnull on done, all are null otherwise. Job IDs are helper-generated public random32-byte IDs; no secret capability is inferred from knowing one. Job query/cancel always final-check the current original view. One latest completed result plus active/queued metadata is retained; starting a different completed-result job replaces its bounded cache only after acknowledgement. Full evidence cache still obeys section6.

StableSnapshot required fields/order: `version`, `state`, `error`, `view`, `status`, `active_job_id`, `queued_job_id`, `last_job_id`, `pending_operation_id`. state=`unavailable|syncing|ready|paused|failed`; view/status nullable and both null when signed out/unavailable; then IDs/error contain no previous-account details. last_job_id identifies the retained latest completed job, including automatically completed catch-up/live-sync work, so polling cannot miss a completion merely because active_job_id cleared between polls. It copies existing authorized cached status, never opens a DB, observes a clock, hashes or schedules. As-of fields are observations, not a continuously recomputed browser age. PendingOperation/history are separately queried; mutation receipt generation/view may be old only in a response enclosed by the new current view.

Metadata-only Operation/Pending/Abandon must either finish bounded local work or report busy; they cannot wait behind eight source hashes on serialized stdin. The async lane owns longer work and caches reconciliation results for cheap retrieval. StableSnapshot/stable_job polls read immutable controller cache, never blocking core Status behind an active eight-hash operation; metadata query/abandon returns busy instead of waiting behind that lane. Those metadata-only paths do not call source.Info. Their initial/final account acquisition uses a250ms context; if the account lease is busy, return busy without disclosing data or leaving an orphan callback. Once a tiny synchronous disk commit has begun, normal filesystem completion is joined, not falsely bounded by a wall-clock promise. Parent/native request code must preserve typed fixed errors. Once Apply may have been dispatched, timeout/EOF/malformed response/final view failure is `outcome_unknown` tied to the retained operation ID, not generic rejection or an automatic new Prepare. Preflight/busy before dispatch is definite. Prepare ambiguity is reconciled through Pending/last_operation_id, never by assuming no intent exists.

Errors: `invalid_request`, `unauthorized`, `forbidden`, `stale_view`, `busy`, `stable_unavailable`, `stable_corrupt`, `stable_capacity`, `stable_clock`, `stable_sync_required`, `stable_conflict`, `stable_pending`, `stable_not_found`, `stable_cancelled`, `outcome_unknown`. Existing account/native control errors stay in their old grammar. No raw storage/source paths, provider credentials or error messages enter snapshots or logs intended for UI.

## 11. Actual app behavior

The updated Candidate Pilot opens the same root. Login displays the stable as it synchronizes; no Start, job issuance, wallet unlock, signing or broadcast is caused by synchronization/page reads. Explicit Start retains its existing meaning. Automatic arrival adds the exact source-derived candidate and condition; there is no claim/sign step. On partial sync show plain progress and allow Stop/Retry. New clicks remain responsive while a page job is active; cancel obsolete navigation before queueing another, and do not retry indefinitely.

Stable rows show name/portrait/class, mined values, current values, phase/prime/career remaining and rating, explicitly dated by authoritative as-of. IDs are full keys despite duplicate names. A complete ten-slot roster strip uses cheap slot IDs; visible details load in <=8-item pages (8+2 for all ten) without retaining an extra ten full proof/portrait objects. Selection is add-to-empty-slot or replace/clear one slot. One click performs Prepare then begins Apply; mark it pending until a durable result is queried. A replacement cannot visually remove the original fighter on a failed/unknown request. Unknown outcomes show a reconciliation state; query same operation and current roster before any new action.

After reset/logout invalidate all view-bound jobs/results/images immediately, cancel/join outside locks, and keep durable assignment/roster/intent history. New login shows the same stable/roster with the current authorization; an old pending view intent is readable but not executable. Held sessions can do these reversible moves; recovery export/withdrawal/transfer/market restrictions are unchanged. No sale, discard, entry, training, match or professional-record controls are added.

Preserve set-40 SVG allowlist, CSP, exact brand assets and native-only destination controls. Use text content and typed integer formatting; never injected manifest markup. Announce loading/error/pending states accessibly and preserve focus. Actual window/chooser/screen-reader evidence is separate from DOM/helper tests and still subject to the existing review instructions and D74.

## 12. Required acceptance and implementation split

- Core normal/race/tagged tests: exact pin/closed codec limits; automatic dedicated-gym assignment; max8 proof counter and cancellation before each subsequent read; raw index preflights; full receipt digest drift refusal on access; all CAS/no-op/replay/old-view/query/abandon paths; capacity reserved before Prepare; committed-then-error poisoning; clock rollback across reopen; source count regression and immutable-row mutation; no fake XP/time; both published fixture identities.
- Real kill tests: before/after install prepared/create/ready; before/after assignment+checkpoint batch; Prepare before/after ACK; before/after roster commit and failed final lease response. Reopen must retain exact original membership/birth/operation outcome and never replace a missing established stable. Query of old committed intent works; stale uncommitted intent cannot execute.
- Independent existing35 vectors: integrated condition must match exact integer output for week/prime/decline/retirement boundaries, attributes1 and fractional values, XP0, invalid times/overflow. Keep old nonzero-XP projector tests unchanged; do not generate fake professional events to exercise41.
- Worker/lifecycle: nil hook old behavior, queue1+active1 bound, no simultaneous stable/worker Argon in this lane, hook outside locks, no source reopening while live, no following callbacks/transport after cancellation, live observed and historical acknowledged source catch-up, poisoned dependency fixed code, and Stop/logout/reset at queued/inflight/aftercommit boundaries. A blocked node/backoff cannot make a timed-out queue entry execute later.
- Transport/native/frontend: old frames exact; valid full nested roster at depth9, invalid depth10, and negative shoulder_jitter successes from existing independent manifests; new closed grammar/max pages/depth/JS integers; immediate ACK before long proof; cheap job poll; controls cancel actual work; wrong-view result rejected; ambiguous Prepare/Apply identity survives through durable Pending/Operation; no optimistic success or fresh-ID retry; stable capacity is not described as an ownership limit. Exercise full ten roster IDs beyond recent-eight cache.
- Packaged normal-PoW composition: two existing wallets mine accepted candidates, automatically assign to correct stable, display condition, change roster, close/reopen and recover exact membership/birth; reset keeps assignment/roster and allows current held roster moves while denying existing restricted actions. Mine at least the authentic end-to-end candidates required for each account; use labelled independent source fixtures for large-count/clock/crash boundary tests. Capture exact binaries/sources, owned PIDs/listeners and successful cleanup. Do not claim public activation, external provider, transferable ownership, rendered-window review or a complete league.

Finite ownership proposal: (A) gymstable store/DTO/lease/catch-up and core/crash tests; (B) account capability/root-v2 installation, child/source lifetime and tracked-job/helper/native routing; (C) stable/roster/condition frontend and strict UI tests; (D) independent contract/source review and existing-oracle integration expectations. Root owns final composition, release/docs and source captures. The narrow worker hook can be assigned with B to keep the single lane in one ownership boundary. No parallel generic scheduler, arbitrary source plugin or new cryptographic oracle is required.

## 13. Review status

This is a complete initial draft, not frozen approval. Review should specifically close: root-v2 installation compatibility, Source/Store borrow lifetime, dedicated-source failure semantics, no duplicate cold verification, combined one-lane cancellation, exact old-view Prepare/Apply recovery, fixed response bounds and outcome_unknown propagation. Any correction changes this draft's hash; original40/39/37/35 evidence is preserved. Implementation starts only after root approves an exact revised hash.

## Appendix A. Exact Go core DTO and codec surface

These declarations fix names/scalar types for parallel implementation; JSON tags follow the exact order in sections2/4/7. No anonymous map or `any` is part of the public surface. Every returned slice/pointer/private receipt is owned; fixed slot arrays copy by value.

```go
type Pin struct {
    Version uint32 `json:"version"`
    AccountAuthorityID string `json:"account_authority_id"`
    AccountID string `json:"account_id"`
    Identity gymaccount.Identity `json:"identity"`
    Source registry.ManifestAuthorityPin `json:"source"`
    ConditionVersion uint32 `json:"condition_version"`
    ProjectorID string `json:"projector_id"`
}
type Assignment struct {
    Version uint32 `json:"version"`
    Sequence uint64 `json:"sequence"`
    ProspectID string `json:"prospect_id"`
    GymID string `json:"gym_id"`
    WorkHash string `json:"work_hash"`
    BornAtMS int64 `json:"born_at_ms"`
    AcceptedAtMS int64 `json:"accepted_at_ms"`
    DescriptorID string `json:"descriptor_id"`
    ManifestDigest string `json:"manifest_digest"`
    PortraitDigest string `json:"portrait_digest"`
    SourceRecordDigest string `json:"source_record_digest"`
}
type Status struct {
    Version uint32 `json:"version"`
    StoreID string `json:"store_id"`
    Pin Pin `json:"pin"`
    Checkpoint uint64 `json:"checkpoint"`
    ObservedSourceCount uint64 `json:"observed_source_count"`
    CaughtUp bool `json:"caught_up"`
    RosterRevision uint64 `json:"roster_revision"`
    Slots [10]string `json:"slots"`
    OperationCount uint32 `json:"operation_count"`
    PendingOperationID string `json:"pending_operation_id"`
    LastOperationID string `json:"last_operation_id"`
    LastAsOfMS int64 `json:"last_as_of_ms"`
}
type Item struct {
    Assignment Assignment `json:"assignment"`
    Manifest fightermanifest.Manifest `json:"manifest"`
    Condition fightercondition.Snapshot `json:"condition"`
    record registry.ManifestRecord
}
func (i Item) Receipt() registry.ManifestRecord

type SyncResult struct {
    Version uint32 `json:"version"`
    Status Status `json:"status"`
    AsOfMS int64 `json:"as_of_ms"`
    Items []Item `json:"items"`
}
type Page struct {
    Version uint32 `json:"version"`
    Status Status `json:"status"`
    AsOfMS int64 `json:"as_of_ms"`
    After uint64 `json:"after"`
    NextAfter uint64 `json:"next_after"`
    HasMore bool `json:"has_more"`
    Items []Item `json:"items"`
}
type RosterItem struct {
    Slot uint32 `json:"slot"`
    Item Item `json:"item"`
}
type RosterPage struct {
    Version uint32 `json:"version"`
    Status Status `json:"status"`
    AsOfMS int64 `json:"as_of_ms"`
    FirstSlot uint32 `json:"first_slot"`
    NextSlot uint32 `json:"next_slot"`
    Items []RosterItem `json:"items"`
}
type SlotRequest struct {
    ExpectedRevision uint64 `json:"expected_revision"`
    Slot uint32 `json:"slot"`
    ProspectID string `json:"prospect_id"`
}
type Intent struct {
    Version uint32 `json:"version"`
    Sequence uint64 `json:"sequence"`
    OperationID string `json:"operation_id"`
    Generation uint64 `json:"generation"`
    ViewID string `json:"view_id"`
    Request SlotRequest `json:"request"`
    PreparedAtMS int64 `json:"prepared_at_ms"`
}
type MutationReceipt struct {
    Version uint32 `json:"version"`
    OperationID string `json:"operation_id"`
    Generation uint64 `json:"generation"`
    ViewID string `json:"view_id"`
    Request SlotRequest `json:"request"`
    PreviousProspectID string `json:"previous_prospect_id"`
    Changed bool `json:"changed"`
    PreviousRevision uint64 `json:"previous_revision"`
    Revision uint64 `json:"revision"`
    Slots [10]string `json:"slots"`
    CommittedAtMS int64 `json:"committed_at_ms"`
}
type Operation struct {
    Version uint32 `json:"version"`
    Intent Intent `json:"intent"`
    State string `json:"state"`
    Receipt *MutationReceipt `json:"receipt"`
    AbandonedAtMS int64 `json:"abandoned_at_ms"`
}
type MutationResult struct {
    Version uint32 `json:"version"`
    Receipt MutationReceipt `json:"receipt"`
    Replayed bool `json:"replayed"`
}

func ValidatePin(Pin) (Pin, error)
func ParsePin([]byte) (Pin, error)
func MarshalPin(Pin) ([]byte, error)
func ID(Pin) (string, error)
func ValidateSlotRequest(SlotRequest) (SlotRequest, error)
func ParseSlotRequest([]byte) (SlotRequest, error)
func MarshalSlotRequest(SlotRequest) ([]byte, error)
```

Pin/SlotRequest Parse functions use the strict bounded transport grammar; Marshal returns canonical bytes without LF after validation. No exported ParseAssignment/ParseOperation/ImportItem grants trust. Persisted row decoders and exact byte matching are private. Shared typed result validators at the helper/native boundary are structural only, not substitutes for core source validation.

Exported core sentinels and wire mappings: `ErrInvalid`→invalid_request; `ErrUnauthorized`→unauthorized; `ErrForbidden`→forbidden; `ErrStaleView`→stale_view; `ErrBusy`→busy; `ErrCorrupt`→stable_corrupt; `ErrCapacity`→stable_capacity; `ErrClock`→stable_clock; `ErrSyncRequired`→stable_sync_required; `ErrConflict`→stable_conflict; `ErrPending`→stable_pending; `ErrNotFound`→stable_not_found; `ErrCancelled`→stable_cancelled; `ErrOutcomeUnknown`→outcome_unknown. `ErrClosed`, `ErrPoisoned`, `ErrProfile`, and sanitized unavailable dependency failures map to stable_unavailable. Preserve errors.Is on core sentinels; never render raw underlying error strings. Core catches the capability/ctx causes and returns these sentinels. Parsing a result never claims it came from a trusted authority.

## Appendix B. Concrete activation, child/parent and protocol APIs

Add `pilot.StableAccountConfig{Manifest ManifestAccountConfig; Lease gymstable.Lease}`. The lease is captured by the parent and supports only View/RosterManage as used above; it binds the same original account/generation/view as Manifest.ViewLease. Add:

```go
func pilot.NewStableManifestAccount(Config, StableAccountConfig) (*StableController, error)
func accountpilot.NewStable(pilot.Config) (*StableController, error)
func pilot.ServeStable(context.Context, io.Reader, io.Writer, StableControllerAPI) error
func bridge.LaunchStable(helper, dataDir, node string) (*StableBridge, error)
```

The accountpilot StableController implements the old public ManifestControllerAPI and the new methods below. The pilot child implements the existing private manifestChild surface plus these stable methods; it does not implement account login/reset methods. Both use deliberate delegation/compatible state handling. They may reuse private code, but NewManifestAccount, accountpilot.NewManifest, ServeManifest, LaunchManifests, old requests/response decoders and noncandidate behavior keep existing acceptance tests. NewStable itself rejects wire.TestPoW before root/lock opening or any v1→v2 marker publication; test a valid populated v1 root remains byte-identical under that refusal. The old parent route refuses account-root v2 before side effects. The new child checks the parent-install authorization/ready latch before touching `stable/`; it cannot independently authorize installation merely because a path is empty. Parent installation is an internal trusted step, never an IPC command or arbitrary caller-supplied boolean.

New child/parent interface methods (the parent delegates after captured-view preflight; the child owns final leases, avoiding nested leases):

```go
StableSnapshot() StableSnapshot
BeginStableSync(context.Context, string /*viewID*/) (StableJob, error)
BeginStableInventory(context.Context, string, uint64, uint32) (StableJob, error)
BeginStableRoster(context.Context, string, uint32, uint32) (StableJob, error)
BeginStablePrepare(context.Context, string, gymstable.SlotRequest) (StableJob, error)
BeginStableApply(context.Context, string, string /*operationID*/) (StableJob, error)
StableJob(context.Context, string /*viewID*/, string /*jobID*/) (StableJob, error)
StableOperation(context.Context, string, string /*operationID*/) (StableOperation, error)
StablePending(context.Context, string) (StableOperation, error)
AbandonStableOperation(context.Context, string, string) (StableOperation, error)
CancelStableJob(context.Context, string, string) (StableJob, error)
```

All new Begin methods only validate/capture/queue and return immediate ACK. Initial Prepare is asynchronous too: even its concrete source.Info can wait behind the source's node eligibility callback. Background Prepare never causes a player edit by itself. Poll/query methods use the short view gate and immutable controller result cache; metadata lookup/abandon returns busy rather than waiting behind a source job. Whole operation deadlines are30seconds once explicit work is queued; automatic stopped catch-up is bounded by the fixed1000 source count and repeated cancellable batches, not a synchronous login deadline. A timeout cancels that exact queued/active job, never an unrelated later one.

Expose strict `ParseStableRequest([]byte) (StableRequest,error)` and `ParseStableResponse([]byte) (StableResponse,error)` in pilot for structural wire grammar tests; no source trust follows parsing. ServeStable alone dispatches both command families. Native StableBridge retains one request ID/expected command decoder at a time and typed helpers for begin/query; old ManifestBridge remains exact. New native JavaScript methods use typed stable DTO arguments/results and fixed codes, no arbitrary command/body passthrough. Existing native-only export destination dialogs/receipt selection remain native-only.

Activation is explicit: helper gains `--candidate-stable`, valid only together with `--candidate-manifests` and never `--account-fixtures`; that pair selects accountpilot.NewStable + pilot.ServeStable. Existing `--candidate-manifests` alone still selects the old route. Native gains the same flag with default `stableBuild == "1"`; the existing candidate build script now links `-X main.candidateBuild=1 -X main.stableBuild=1` into the **same** bundle name and bundle identifier `local.slug.gym.candidatepilot`. LaunchStable passes both helper flags and the same LocalManifestPilot directory. The new route opens `/stable.html` with matching new `stable.js`/`stable.css`; old `/manifest.html` and noncandidate entry remain available on their old explicit routes. The title remains “SLUG Candidate Pilot”, and visible wording remains candidate-only. This is the next version of the existing candidate app, not another independently branded/data-root demo.

Stable-enabled login replaces the old synchronous LoadManifestProspects stage with bounded installation + attached syncing view + automatic lane startup. It does not call old cold Load then new Sync. Existing snapshot counters/account/miner/wallet fields retain their exact semantics. The old ManifestSnapshot.Projection is initially empty (`state:idle`, `items:[]`, `through_sequence:0`, `complete_recent:false`, `has_older:accepted_count>0`). It may publish existing live recent cards using the existing checked observer path, or a same-pass page only when that page is actually the last min(N,8) consecutive accepted sequences and all original invariants hold. A prefix/roster/inventory page must not masquerade as the recent page: keep the old projection unchanged or explicitly clear it with HasOlder recomputed. Stable pages appear only in the new DTOs. If no valid recent page is present, old portrait/receipt methods return their existing unavailable result unless the same currently selected verified item is deliberately routed through the existing bounded cache with matching identity. Never weaken native validation to make a partial page pass as complete.

### B.1 Internal installation handshake

The new private child interface additionally has `StablePin() (gymstable.Pin,error)`, `CreateStableComponent(context.Context) error`, and `AttachStable(context.Context) error`. StablePin is available only after the normal child ready identity exists; it derives the pin from trusted configuration, not caller fields. Parent writes the prepared root install latch, then calls CreateStableComponent; the child rereads the bounded required parent-v2 marker and exact expected prepared pin before exclusive empty creation. Parent promotes the latch to ready only after successful closed/synced empty-store acknowledgement. AttachStable rereads ready, opens/audits the paired source+stable without proof hashing, publishes initial cached status and starts the cancellable automatic lane after attachment. It must not internally invoke the old cold recent-eight load. These methods are not exposed through IPC and accept no caller-created authorization flag.

The parent's instance lock remains owned throughout. Failure/cancellation may leave a prepared empty component; recovery follows section8 exactly. A normal existing ready component uses AttachStable only, never CreateStableComponent. The child does not acquire the parent's instance lock recursively, and the parent does not hold an account lease while joining its child.

## Appendix C. Exact new pilot transport DTO types

All types below are in `desktop/pilot`, use section10's strict selected-command grammar and deep-owned result values. Struct optional tags allow envelope/request serialization only; parsers explicitly enforce which fields are required/forbidden/null for each command. Result pointer fields inside StableJob have **no** omitempty, so null state is explicit.

```go
type StableRequest struct {
    ID uint64 `json:"id"`
    Command string `json:"command"`
    ViewID string `json:"view_id,omitempty"`
    After uint64 `json:"after,omitempty"`
    Limit uint32 `json:"limit,omitempty"`
    FirstSlot uint32 `json:"first_slot,omitempty"`
    ExpectedRevision uint64 `json:"expected_revision,omitempty"`
    Slot uint32 `json:"slot,omitempty"`
    ProspectID string `json:"prospect_id,omitempty"`
    OperationID string `json:"operation_id,omitempty"`
    JobID string `json:"job_id,omitempty"`
}
type StableResponse struct {
    ID uint64 `json:"id"`
    OK bool `json:"ok"`
    Snapshot *StableSnapshot `json:"snapshot,omitempty"`
    Job *StableJob `json:"job,omitempty"`
    Operation *StableOperation `json:"operation,omitempty"`
    Error string `json:"error,omitempty"`
}
type StableSnapshot struct {
    Version uint32 `json:"version"`
    State string `json:"state"`
    Error string `json:"error"`
    View *ManifestViewPin `json:"view"`
    Status *gymstable.Status `json:"status"`
    ActiveJobID string `json:"active_job_id"`
    QueuedJobID string `json:"queued_job_id"`
    LastJobID string `json:"last_job_id"`
    PendingOperationID string `json:"pending_operation_id"`
}
type StableJob struct {
    Version uint32 `json:"version"`
    JobID string `json:"job_id"`
    Kind string `json:"kind"`
    State string `json:"state"`
    View ManifestViewPin `json:"view"`
    StoreID string `json:"store_id"`
    OperationID string `json:"operation_id"`
    Error string `json:"error"`
    Sync *gymstable.SyncResult `json:"sync"`
    Page *gymstable.Page `json:"page"`
    Roster *gymstable.RosterPage `json:"roster"`
    Intent *gymstable.Intent `json:"intent"`
    Mutation *gymstable.MutationResult `json:"mutation"`
}
type StableOperation struct {
    Version uint32 `json:"version"`
    View ManifestViewPin `json:"view"`
    StoreID string `json:"store_id"`
    Operation *gymstable.Operation `json:"operation"`
}
type StableAccountConfig struct {
    Manifest ManifestAccountConfig
    Lease gymstable.Lease
}
```

Because valid requests include zeros/empty strings, **do not use a plain Marshal of StableRequest with omitempty as the request encoder**. Freeze `MarshalStableRequest(StableRequest) ([]byte,error)` as command-specific canonical field emission: id,command first, then the required command fields in the table's order, including after=0, expected_revision=0 and prospect_id="" for clear. Invalid unrelated nonzero fields fail; parser requires all selected fields even when zero. `ParseStableRequest`/`MarshalStableRequest` round-trip each valid command. Response serialization is preflighted, and only its one matching result variant may be nonnil.

`StableControllerAPI` embeds the existing `ManifestControllerAPI` and all new methods from Appendix B. `ServeStable` validates its selected response and keeps serving control/status frames while the tracked job runs. Error response mapping is fixed; a result cannot declare ok=true while omitting the matching nonnull outer DTO. Unknown command/version/profile fails closed without falling back to the old route.
