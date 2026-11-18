# Candidate career accounting — Set49 contract

Status: contract reviewed and implemented; local integrated acceptance passes,
with normal packaging and release pending. D78 is approved; this document fixes
only the standard two-fixture candidate integration. Pure arithmetic is specified
in `FIGHTER_CAREER.md`.
No new matching, ranking board,
quality-rank authority, title allocation, exhibition, funding or signing policy.

## 1. Layered authority and activation

`gymresult.RulesID()` and its `awards:"none"` remain byte-identical. The original
publisher attests only publication evidence. Its Pin, StoreID, signed statements,
ResultKeys, transaction bytes and funding/reservation book do not change.
A separate **CareerPolicyID** authorizes a new local accounting interpretation of
first finality occurring **after explicit activation**. Publication evidence is
not itself an award grant. There is no new signed statement or second ledger.

Legacy marker/metadata v1 and all v1 public result DTOs retain their exact codecs.
An activated Result store has marker/metadata v2; its stored Result records remain
v1 publication evidence. Old constructors/writers refuse v2. Existing v1 sources
with any retained `Finality != nil` cannot activate, including conflicted, offline
or shallow finalized history. They remain intact and readable through the legacy
route, with no career awards. Unfinalized old rows may subsequently finalize in
the activated profile. No `max(finality,activation)` backdating or imported
finalized flag is accepted.

CareerPolicyID is SHA256 of ASCII `SLUG candidate career policy\0v1\0` followed by
LE32 byte length and these exact ASCII bytes (no LF):

```json
{"version":1,"scope":"two_fixture_standard_career","source":"result_v1_first_finality_after_activation","xp":"D42_standard_1_cap15","base_points":"D41_no_quality","effective_time":"first_finality_observed_at_ms","cutoff":"strict_before","later_conflict":"retain","c1":"zero_professional_record_only"}
```

New `gymresult.CareerActivation` fields, in declaration/JSON order:
`Version uint32`, `PolicyID string`, `ActivationID string`, `ResultStoreID string`,
`ResultCreatedAtMS int64`, `SourceCreatedAtMS int64`, `ActivatedAtMS int64`.
JSON uses lower_snake_case, with all fields required. Version=1; IDs are lowercase
64-hex. Times are positive existing safe timestamps. ActivationID is SHA256 of
`SLUG candidate career activation\0v1\0` + raw PolicyID + raw ResultStoreID +
LE64 ResultCreatedAtMS + LE64 SourceCreatedAtMS + LE64 ActivatedAtMS. Original
source/store creation times remain unchanged. ActivatedAtMS is sampled internally
after the activation barrier is durable, never supplied by a caller.

## 2. Exact constructor and recovery surface

Add only these public core entry points (and same-shape `ForTestClock` wrappers
under the existing clock tag):

```go
func CareerPolicyID() string
func CheckCareerActivation(ctx context.Context, path string, expected Pin,
    bouts *gymbout.Store) error
func ActivateCareer(ctx context.Context, path string, expected Pin,
    bouts *gymbout.Store) (*Store, error)
func ResumeCareerActivation(ctx context.Context, path string, expected Pin,
    bouts *gymbout.Store) (*Store, error)
func OpenCareer(ctx context.Context, path string, expected Pin,
    bouts *gymbout.Store) (*Store, error)
func (s *Store) CareerInfo(ctx context.Context) (CareerActivation, error)
func (s *Store) CareerReader() (*careerstate.Reader, error)
```

Activation is trusted parent installation, not a browser command or ordinary View
mutation. Parent v9 first drains/closes existing owners, retains its exclusive
root lock, first calls read-only CheckCareerActivation (full v1 audit/no Finality
under the real DB lock, then closes it), and only on success persists a
`career_prepared` intent for the exact child identities.
Result constructors independently acquire the real exclusive DB lock and validate
the concrete Bout source incarnation. `ActivateCareer` accepts only an established,
fully audited v1 store with no Finality; it never creates an absent journal.
Fresh parent setup first completes the existing Result initialization, then
activates it while still unpublished. `OpenCareer` accepts only exact ready v2;
it never repairs or activates. v1 `Open/Create/ResumePrepared` remain unchanged.

Activation disk sequence (all stages have deterministic crash checkpoints):

1. Audit all existing rows, reservations and source binding; reject any Finality
   before mutation. Sync one closed `career_init` row containing Version=1,
   PolicyID, ResultStoreID, ResultCreatedAtMS, SourceCreatedAtMS and the SHA256
   of the exact old canonical marker bytes (including LF), named OldMarkerSHA256.
   Intent field order is exactly the order just listed; JSON is lower_snake_case.
   Retain this immutable
   small row after activation. Old v1 audit rejects this previously unknown key.
2. Write an exclusive same-directory temporary marker, Sync/close, atomically
   replace only the exact validated old marker, and Sync the directory. Marker
   v2 adds `career_phase:"prepared"`, `career_intent_digest` (SHA256 of canonical
   career_init JSON without LF), and `career:null` to the original fields.
   No old writer can reopen before activation metadata commits. No symlink,
   unexpected temporary file or unrelated inventory is adopted. The sole recognized
   activation temporary name and recovery rule are specified below.
3. Sample a valid nondecreasing local clock at least the old metadata ClockMS.
   In one synced batch replace metadata with Version=2 plus nonnull `career`
   activation; all old fields remain identical except ClockMS may advance.
   The retained career_init row is unchanged. No result row/index/fund is rewritten.
4. Atomically install and directory-Sync marker v2 `career_phase:"ready"` with
   the exact same activation as metadata; then publish the Store/reader. Parent
   synchronizes `career_ready` only after exact CareerInfo/source checks.

`ResumeCareerActivation` is callable only under the parent's exact prepared
intent. It accepts: untouched valid v1/no Finality; v1 + exact career_init;
prepared-v2 + exact intent + old audited metadata/no Finality; prepared-v2 + exact
v2 metadata/activation; or exact ready-v2. It completes only the remaining steps.
The sole permitted third root entry is the regular, non-symlink file
`.SLUG-CANDIDATE-RESULTS.career-marker.tmp`, created exclusively with mode0600.
It is permitted only alongside the exact retained career_init and an otherwise
valid stage above. Its bytes must be a bounded exact prefix (including empty)
of the canonical next marker plus LF for that stage, or the exact current
marker plus LF after a completed replacement. Prepared-marker bytes are derived
from the validated old marker and intent; ready-marker bytes require the exact
already committed activation. Resume checks file identity before/after reading,
then removes only this validated temporary and Syncs the directory before
recreating the needed marker exclusively. Other bytes/types/names refuse without
cleanup. Each replacement checks the exact predecessor marker again before
rename; after successful rename there is no temporary entry. Thus a crash during
temporary writing or after its Sync does not require adopting unknown content.
An already committed activation/time is never resampled. An absent/real-empty
Result path, mismatched intent/pin/time, prior Finality, missing established data,
unknown keys or other mixed state refuses without deletion/downgrade. Parent
ready + missing child never recreates it. All original created times survive.

Before any write, ordinary refusal is definite. A write/Sync/rename/dirSync error,
or cancellation after the first durable activation mutation, returns existing
`ErrOutcomeUnknown`, closes the unpublished handle, and requires exact resume.
Never return a successful reader after partial activation. Known v1 finality
refusal is new `ErrCareerLegacyFinality` (`career_legacy_finality`); absent
activation is `ErrCareerInactive` (`career_inactive`). Existing clock/corrupt/
profile/closed/cancelled errors retain their meanings. OutcomeUnknown takes
precedence after possible mutation. Completed successful final publication is
not retroactively changed by a later Close.

## 3. Award events, owned index and arithmetic

Only activated stores derive events. Each fully validated record with nonnull
Finality contributes exactly one event; require its ObservedAtMS >= ActivatedAtMS.
The first Finality remains immutable through later conflict. Pre-finality rows,
certificates lacking sixty descendants, and seed finality contribute nothing.

`careerstate.Event` is an internal owned value, not an import authority. Fields
in order: Version; AwardID; ResultKey; CardID; BoutID; SourceRecordDigest (saved
Bout digest); InputID; ResultDigest; FinalityDigest; EffectiveAtMS; Saturday;
CornerA; CornerB; Winner; Method. Version uint32=1, EffectiveAtMS int64, all other
listed scalars strings except corners. Each corner has ProspectID, AccountID,
GymID strings in that order, taken from the original bound source. All IDs are
64-hex; Saturday is the original ten-byte date; Method is the closed engine enum;
Winner is empty for draws or exactly one corner's ProspectID. All Event/corner
fields are required and use lower_snake_case JSON in the stated declaration order.
Canonical JSON is compact UTF-8 with Go-compatible HTML and U+2028/U+2029
escaping, integer decimal spelling, no whitespace/LF and no omitted fields.
The canonical index is the array of Events sorted as specified below; empty is[].

AwardID = SHA256(`SLUG candidate career award\0v1\0` + raw ActivationID + raw
ResultKey). FinalityDigest = SHA256(`SLUG candidate career finality\0v1\0` +
LE32 length + canonical original Finality JSON). EffectiveAtMS is that Finality's
ObservedAtMS. Finality JSON uses the unchanged v1 field order and nested codecs
from `gymresult/types.go`: checkpoint, inclusion, observed_at_ms, observed_tip,
descendants; this is the exact retained first Finality, not a fresh projection.
Do not use mutable RecordDigest, re-inclusion generation, phase,
login or current gym attribution as a new event identity.

Normalize stored Winner to `a`/`b`/`draw` only after exact corner/method validation
(`gymbout.validateSavedBout` uses actual IDs and empty draw, not those labels).
Call `fightercareer.Award(normalized, method, "standard")`; fold using Apply in
(EffectiveAtMS, AwardID) order. Both corners gain one standard XP; cap XP at15
without suppressing records/base points. No title classification or quality
bonus is inferred. BasePoints is cumulative standard base-point arithmetic,
not rolling standings or total ranking points.

No second DB counter, per-fighter mutation journal or Apply endpoint exists.
Open builds the bounded event index while auditing the original result rows.
Before any Result batch that can change first finality, derive the complete
prospective owned index and validate all identities, immutable-event comparisons
and per-event/aggregate bounds. Ordinary capacity/validation refusal therefore
precedes the write. After successful Sync, install that exact already validated
owned index under the same short mutex before releasing publication; no fallible
recomputation remains. An unexpected post-Sync install invariant failure poisons
the owner and returns OutcomeUnknown, never a definite capacity refusal.
A failed Sync poisons the owner and publishes no
new index; reopen audits whichever bytes actually committed. Later status-only
changes do not change an event/digest. Reject duplicate ResultKey/AwardID or a
changed immutable event for a retained first finality. Re-inclusion and duplicate
requests cannot award twice. A detached owned copy prevents returned values from
mutating owner metadata/index.

Hard bounds: 256 events; each encoded Event <=2048 bytes; entire canonical index
<=544KiB, in addition to the unchanged 96KiB Result row index. No transcript,
block, raw transaction or duplicate full Pin is retained in this new index.
Activation/intent each <=2048 bytes. Existing marker36KiB/metadata64KiB limits
remain and must accommodate the additive fields. These are internal bounds,
not permission to send the whole index in a desktop response.

## 4. Sealed read handle and historical fence

`internal/careerstate` imports no Result/Stable/Entry/Event/Card/Bout package.
Result is the sole application minting site for Reader. Its package-internal
factory stores the fixed activation and owner-lifetime callback; consumers cannot
construct a valid zero value or parse JSON into a trusted handle. A factory in
this internal package is trusted Go wiring, not protection against arbitrary
malicious in-process code. No production external input reaches it.

```go
// in internal/careerstate; Snapshot fields are private and own their data
func (r *Reader) Activation() (Activation, error)
func (r *Reader) Scope() (Scope, error)
func (r *Reader) Snapshot(ctx context.Context, ids []string,
    asOfMS int64, mode Mode) (Snapshot, error)
func (s Snapshot) Activation() Activation
func (s Snapshot) Digest() string
func (s Snapshot) AsOfMS() int64
func (s Snapshot) Mode() Mode
func (s Snapshot) ProspectIDs() []string
func (s Snapshot) Totals(prospectID string) (fightercareer.Totals, bool)
```

Activation is an alias/shared lower-level shape for the fields in §1; avoid an
import back into gymresult. Mode is a string type with only `Current="current"`
and `Cutoff="cutoff"`. AsOfMS is a positive safe timestamp at least ActivatedAtMS.
IDs are 1..10
unique sorted 64-hex strings; no freeform source choice. Current includes events
with EffectiveAtMS <=asOfMS and binds the exact captured event-prefix digest;
its result is not a promise that no same-millisecond event can appear later.
Cutoff includes only EffectiveAtMS <asOfMS and refuses unless the owner's durable
ClockMS >=asOfMS. Unknown fighters have zero totals only from a live, validated
complete prefix; owner unavailable/corrupt/closed/poisoned never means zero.
Totals returns (zero,true) for a requested ID with no award, (totals,true) for any
other requested ID, and (zero,false) for an unrequested ID. Mode returns the captured mode; ProspectIDs returns an owned copy of the exact
validated requested list, allowing consumers to refuse a Current/different-ID
snapshot when committing a cutoff fence. No error or invalid
Snapshot may be interpreted as an authorized zero-history answer.

Snapshot Digest is SHA256 of ASCII `SLUG candidate career prefix\0v1\0` + raw
ActivationID + LE64 AsOfMS + LP(mode ASCII) + LE32 requested-ID count + each
requested raw32 ID in sorted order + LE32 included-event count + each
LP(canonical Event JSON) in (EffectiveAtMS, AwardID) order. LP is LE32 byte length
followed by exactly those bytes. The included events are the **entire** filtered
temporal prefix across both gyms, not only events matching requested IDs. This
binds activation, temporal mode, as-of, selection and all prior event provenance.
Every owner Event.EffectiveAtMS must be <= its durable ClockMS; malformed owner
state refuses even for Current, without requiring Current asOfMS<=ClockMS.
Snapshot retains only the requested owned totals and exact bound context/digest,
not an externally constructible event source.

Scope is trusted factory wiring: `StableStoreIDs [2]string` in exact validated
participant order and `CardStoreID string`, derived with the existing Stable/Card
ID functions from the concrete Result Pin. Reader.Scope performs the same
owner-state/lifetime check and returns this owned value. Consumers reject a
foreign scope before binding; supplying a matching foreign Activation is not
sufficient. Scope is not a JSON input or an additional hash preimage.

Reader.Activation performs the same owner-state/lifetime check under the short
mutex and returns an owned activation, without requiring IDs/as-of or observing
a clock. Shared careerstate sentinels are ErrInvalid (`career_invalid`),
ErrInactive (`career_inactive`), ErrUnfenced (`career_unfenced`), ErrClosed
(`career_closed`), ErrPoisoned (`career_poisoned`), ErrCancelled
(`career_cancelled`) and ErrCorrupt (`career_corrupt`). Consumers map these
precisely; no unavailable/fence error becomes zero totals. Invalid zero handles
return ErrInactive; valid handles propagate the actual owner-state error.

Reader copies only bounded audited data under Result's short mutex. It never
acquires Result's action gate, traverses source/account stores, writes clocks,
simulates or makes RPC calls. It checks cancellation/lifetime and exact activation
before returning. Result never holds this mutex while acquiring lower source
locks or joining work. Consumer original-View authorization remains mandatory;
the handle itself grants no account access. Consumers bind their final response
to its exact activation, as-of and digest rather than combining differing reads.

Add `Store.FenceCareer(ctx, entries *gymentry.Store, session gymstable.Session,
saturday string) error`. The parent/top-level coordinator, which already owns
both handles, calls this before entering any consumer operation. Lower consumer
packages never import Result or call it through a callback. Derive cutoff through
the pinned calendar. Before consumer gates, acquire Result's action gate, use the actual entry
WithSnapshotView/original View lease, and require its StoreID/Pin to match exactly
one participant in Result's pinned source chain. Under the final original check
and Result short mutex, observe/sync real local time >=cutoff and source observation.
Future cutoff returns `ErrCareerNotDue` (`career_not_due`), never advances time
synthetically. Known rollback fails. The normal Result action clock now cannot
commit first finality before that cutoff; equal-time events are excluded by `<`.
Return errors/unknown under the existing post-write rule. Held View is sufficient
for this observation; it grants no publishing/signing permission. Never call
FenceCareer while already holding account/Stable/Entry/Event/Card gates.

## 5. Consumer integration boundary (separate coordinated edits)

Parent v9 retains the exclusive root lock and all inherited marker suffixes.
It opens/audits lower stores without publishing operations, completes Result v2
activation/Open, then attaches the fixed Reader before admitting new operations.
Ready missing/mismatched bindings fail closed. Shared authority survives gym
switches; shutdown drains consumers before Result. Old parent routes refuse v9.

The first slice uses separately versioned CareerPage/current-condition output;
old Item, entry, snapshot, Card and Bout row codecs remain unchanged. Verified
Stable assignment/manifest plus accounted XP feeds unchanged condition/D65.
Label legacy zero-XP views as historical, not current career. Existing saved
outcomes in the active profile are provisional until trusted same-outcome first
finality; a later conflict is shown separately without removing awards. Legacy
v1 finalized history remains labeled legacy finalized/no-career authority.

Core entry, capture and new Card booking require the fixed career binding,
never just a frontend check. The parent/top-level coordinator first calls
FenceCareer outside consumer locks. Core capture/booking only read the sealed
Cutoff snapshot and fail closed unless its durable fence already covers the
cutoff; they cannot lazily acquire the Result action gate. A selected fighter with nonzero
professional record at cutoff produces explicit `career_candidate_ineligible`
with bounded affected IDs; do not disguise it as retirement/no opponent or widen
C1. Entry-time checks are only early feedback. Post-cutoff awards do not change
that event, and historical saved cards replay unchanged. Every accepted new C1
snapshot therefore still truthfully has zero record/XP. Preserve/check the
activation and cutoff-prefix commitment in the activated component authority
metadata; legacy constructors must refuse activated writable markers. No
optional per-call boolean can disable this guard. Exact helper/page schema and
component-writer changes belong to coordinated follow-up contracts, not an
implicit widening of this core scope.

## 6. Required evidence before acceptance

Independent arithmetic/source cases precede Go expected comparisons. Cover
v1/no-finality activation and every crash stage; definite refusal of any historical
finality without mutation; old writer refusal; exact unknown recovery/time;
first-finality two-corner atomic effect; replay/restart/re-inclusion idempotency;
post-finality conflicts; source incarnation/alias tampering; XP cap and all actual
method mappings; before/equal/after-noon and late capture; Current prefix drift;
shorter caller cancellation/Close; held reads and stale original View; direct-core
C1 bypass attempts. Measure full256 startup/index and actual-account advancing-clock
consumer latency under the accepted profile; race correctness is separate from
normal latency. No automatic chain activity or fight-engine rerun is introduced.
