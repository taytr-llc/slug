# Set37 candidate manifest-bound authority contract

Status: exact engineering proposal for root review, November 16, 2026 Pacific.
No runtime implementation or new expected-value corpus is authorized by this
document alone. This is a local candidate authority library, not official
generator activation, a public service, portable receipt authentication or an
ownership/roster system. D74 remains a separate joint release review.

## Scope and fixed prerequisites

Implement one explicit registry schema 5 with a separately typed store, durable
Job v2 issuance, atomic admission-time static binding, and verified receipt
reads. Add no service/client protocol, worker/outbox mode, CLI, desktop route,
account/reset rule, migration or current-condition projection in this set.

The normative prerequisite is the approved set36 contract, SHA256
`6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66`, archived at
`manifest-job-reference/inputs/manifest-job-contract.md`. Its sole candidate
descriptor ID is
`c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303`.
Use the existing compiled resolver; do not replace it with a registry-selected
generator, caller callback, mutable map, or a second test-only supported bundle.
The original manifest contract hash remains
`ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80`; the portrait
contract hash remains
`8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5`.

Context/DNA v1, proof/auth v2, Job v2, `SLUGPR02`, D68 rule 1, D69 eligibility
rule 2, Argon parameters, header input, and prospect-ID calculation are unchanged.
The descriptor binds static DNA/manifest/portrait choices only. It does not bind
XP accounting, condition-projector version, fight rules or account authority.

The review inspected current `registry/{store,jobs,eligibility,info}.go`, their
durability tests, `prospect/proof.go`, `gymauth`, `prospectservice/server.go`,
`fightermanifest`, `fighterportrait`, `AGENTS.md`, `PROSPECT_BIRTH.md`, and spec
sections 3, 10, 11, 12, 14 and 17. The older broad
`fighter-admission-contract-proposal.md` is background only; this document
narrows its second step and supersedes its provisional API/schema suggestions.

## API and compatibility isolation

Add separate files in package `registry`. Do not add optional fields to old
`Record`, reinterpret old `Job`, expand old decoders to guess schema 5, or change
the old marker/metadata bytes. Existing `Store`, `Open`, creation and admission
methods retain schemas 1–4 and reject a schema-5 marker. New methods accept only
schema 5. Common private filesystem, key-layout and synchronization helpers may
be reused where old behavior stays covered by unchanged tests.
In particular, do not broaden old `supportedVersion`/`jobVersion` or call old
`create`, `readMarker` or Record/Job decoding as if they understood schema 5;
reuse value-independent helpers only, or factor an exactly tested private
primitive beneath both typed implementations.

```go
const Version5 uint32 = 5
const StaticBindingVersion1 uint32 = 1
const MaxManifestMarkerSize = 4096
const MaxManifestMetadataSize = 4096
const MaxManifestRecordDepth = 4
const MaxManifestPageSize = 8
const MaxManifestPageBytes = MaxManifestPageSize*MaxRecordSize + MaxManifestPageSize + 1
// MaxRecordSize remains 65536; MaxManifestPageBytes is 524297.

type ManifestAuthorityPin struct {
    Version      uint32 `json:"version"`
    Network      uint32 `json:"network"`
    Genesis      string `json:"genesis"`
    AuthorityID  string `json:"authority_id"`
    DescriptorID string `json:"descriptor_id"`
}

type ManifestAuthorityInfo struct {
    Pin        ManifestAuthorityPin       `json:"pin"`
    Descriptor fighterdescriptor.Descriptor `json:"descriptor"`
    Records    uint64                     `json:"records"`
    Jobs       uint64                     `json:"jobs"`
    ReadOnly   bool                       `json:"read_only"`
}

type StaticBinding struct {
    Version        uint32 `json:"version"`
    DescriptorID   string `json:"descriptor_id"`
    ManifestDigest string `json:"manifest_digest"`
    PortraitDigest string `json:"portrait_digest"`
}

type ManifestRecord struct {
    Version       uint32                 `json:"version"`
    Sequence      uint64                 `json:"sequence"`
    Context       prospect.Context       `json:"context"`
    Proof         prospect.Proof         `json:"proof"`
    Verified      prospect.Verified      `json:"verified"`
    Authorization gymauth.Authorization  `json:"authorization"`
    Job           prospectjob.ManifestJob `json:"job"`
    Birth         Birth                  `json:"birth"`
    Eligibility   Eligibility            `json:"eligibility"`
    StaticBinding StaticBinding          `json:"static_binding"`
}

type ManifestStore struct { /* private gate, DB, metadata, state */ }

func CreateManifestAuthority(path string, network uint32,
    genesis, descriptorID string) (*ManifestStore, error)
func InspectManifestAuthority(path string) (ManifestAuthorityPin, error)
func OpenManifestAuthority(path string, expected ManifestAuthorityPin,
    readOnly bool) (*ManifestStore, error)
func (s *ManifestStore) Info() (ManifestAuthorityInfo, error)
func (s *ManifestStore) Close() error

func (s *ManifestStore) IssueManifestJob(chainContext prospect.Context,
    publicKey string, check func() error) (prospectjob.ManifestJob, error)
func (s *ManifestStore) GetManifestJob(id string) (prospectjob.ManifestJob, error)
func (s *ManifestStore) RecordManifestJobChecked(proof prospect.Proof,
    authorization gymauth.Authorization,
    check func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error),
) (ManifestRecord, bool, error)
func (s *ManifestStore) GetManifestAdmission(id string) (ManifestRecord, error)
func (s *ManifestStore) ListManifestAdmissions(after uint64,
    limit int) ([]ManifestRecord, error)
```

Record fields above are required value fields, in that exact canonical JSON
order; none uses `omitempty`. An all-zero optional-looking child is invalid,
not an absent feature. Authority and descriptor are already present in the
required Job v2; no separately mutable receipt authority field is needed.
`StaticBinding` uses the listed order. No manifest/SVG blob is stored in receipts.
No public `Verify/Import/Project(arbitrary ManifestRecord)` or raw trusted-handle
constructor is added. Future transport parsing needs its own explicit contract.

All getters return owned values, including a fresh proof branch slice. Failure
returns a zero DTO/nil slice and `inserted=false`; it never returns partial
trusted evidence. An empty successful page is a nonnil empty slice. The new
store uses existing error categories `ErrClosed`, `ErrReadOnly`, `ErrPoisoned`,
`ErrNotFound`, `ErrCorrupt`, `ErrConflict`, `ErrJobNotFound`, `ErrJobExpired` and
`ErrClock`; add `ErrManifestAuthority` for a malformed/unsupported creation pin
or an expected-pin mismatch. Preserve wrapped resolver errors for local
diagnostics. No new public wire error protocol is defined here.

All methods serialize under one store operation gate. Callbacks run under it
and must not reenter any method on that store. A caller must bound its queue;
this library does not create background work or an unbounded goroutine queue.
Close joins actual in-flight work, releases the DB once, and preserves its first
close error on repeated calls. There is no automatic reopen after poisoning.

## Initialization, discovery and explicit pinning

Create validates the complete chain and descriptor before filesystem mutation.
Network is nonzero and genesis is nonzero hex32; creation may normalize genesis
hex exactly as existing constructors do. Descriptor ID must already be
canonical and resolve through `ResolveCandidate`. `ResolveProduction` remains
empty. Actual authority creation/open/issuance/admission/evidence reads refuse
`wire.TestPoW` before opening or creating storage. Pure marker inspection may
remain available in that build because it makes no issuance/work claim.

Create obtains a 32-byte authority namespace from `crypto/rand` internally before
writing its marker. There is no caller-chosen authority ID, random-source option,
clock, challenge, lifetime or activation flag. Canonical all-zero ID encodings
remain legal: randomness and uniqueness are generation responsibilities, not a
nonzero-hash fiction. Internal test seams are unexported and never CLI/config
options. Randomness failure returns no handle and creates no authority.

Use the existing application name `slug-prospect-registry`, dedicated
`SLUG-REGISTRY.json` marker and `db` child. Create refuses every existing path,
creates the directory privately, uses exclusive marker creation, synchronizes
the initial DB batch, marker and directories, and acknowledges only afterward.
Do not erase an incomplete directory to retry. Preserve existing filesystem
durability/platform limitations; do not silently weaken fsync on a platform.

The schema-5 marker is compact ASCII JSON followed by exactly one LF, ordered:

```text
application, version, network, genesis, authority_id, descriptor_id, descriptor
```

The canonical DB metadata row at `meta` has no terminal LF and is ordered:

```text
version, network, genesis, authority_id, descriptor_id, descriptor,
last_sequence, job_count, last_clock_ms
```

All fields are mandatory, including zero initial counters/clock. The nested
descriptor is the complete canonical resolved descriptor, copied into both
places; its ID must equal the stored/pinned DescriptorID. Marker and metadata
must agree exactly on schema, chain, authority and descriptor. Retain no
`latest`, mutable implementation pointer, fallback selector or repair path.
Metadata limits are 4,096 bytes each and object depth 2, with decoded duplicate,
missing/null/unknown/case-alias and noncanonical-byte refusal before acceptance.

`InspectManifestAuthority` reads only a bounded regular marker in a regular
directory, checks its canonical schema and compiled descriptor, and returns its
public pin. It does not open/create the DB, verify synchronization, authenticate
the path/issuer, or bless receipt contents. Inspection success is **unsigned
discovery evidence**, never permission to adopt a foreign authority.

Open requires an explicit expected pin. The pin's version is exactly 5; all its
hex fields are canonical lowercase, network/genesis valid, descriptor supported.
It compares the supplied pin against both marker and DB metadata, opens only an
existing regular DB directory, and audits structure before returning a handle.
There is no empty-pin/default/TOFU mode. A trusted application ordinarily
persists the pin obtained from the successful Create handle's Info alongside
its provisioning configuration. After lost Create acknowledgement, it may
inspect its intended retained provisioning directory and explicitly reconcile
that discovered pin, then call pinned Open. This makes recovery possible without
making inspector output self-authenticating. If initialization is incomplete,
Open fails and keeps the evidence; it never silently issues a new authority ID.

No two-database atomicity is claimed for a future application's separate pin
configuration. Copying/replacing an entire store and its externally trusted pin
is outside the detection guarantee. The registry owns local durable assertions,
not an issuer signature or globally unique namespace registry.

## Key layout and opening audit

Keep the proven local layout: `meta`, `job:<canonical JobID>`,
`record:<canonical ProspectID>`, and `sequence:` plus an 8-byte big-endian positive
sequence. A sequence value is exactly the lowercase 64-character ProspectID.
The isolated schema-5 decoder interprets job rows only as ManifestJob, records
only as ManifestRecord, and rejects all other keys. No old record row is
accepted because its work can be re-rendered.

Open does not run Argon or consult a clock/node. It does check every row's bounded
canonical encoding, versions and reciprocal links; full job reconstruction;
authority/descriptor/chain equality to the pin; authorization validity;
eligibility's exact internal binding/target comparison; birth intervals;
consecutive sequence/count integrity; and clock highwater above every issuance
and acceptance. Recompute DNA and static binding from each stored WorkHash as
a cheap integrity check, comparing the entire stored DNA. The hash is still an
unverified assertion until a verified receipt read redoes its production proof.
Open success and Info counts must not be described as complete PoW verification.

Each stored job is at most 8,192 bytes with set36's exact canonical output.
Each stored receipt is at most 65,536 bytes and depth 4 (root 1, Verified 2,
DNA 3, Attributes 4). The only permitted array is the bounded proof merkle
branch; canonical empty branches are `[]`, never null. Apply byte/depth/shape
bounds before allocation-heavy decoding and require exact re-encoding equality.
Unknown, duplicate, null, missing, alias, malformed numeric and trailing data
fail. Proof limits remain exactly 80 header bytes, 1–16,384 coinbase bytes, and
at most 20 display-order siblings, each 32 bytes. Signature bounds/low-S/DER
and compressed public-key rules remain `gymauth`'s existing rules.

Use canonical lowercase hex for stored proof/auth values. The admission input
may use the case accepted by existing proof/auth helpers, but is bounded and
copied before normalization/verification; the first **canonical** proof and
authorization are retained. This rule is schema-5-only and changes no old
receipt. Canonical job/context/Verified/static-binding fields are already
produced by their existing helpers. Canonical JSON is not a signature.

The audit iterates actual entries and validates count equality, not loops bounded
by an untrusted LastSequence value. Overflow is rejected. Total stored history
remains append-only, with the current registry's O(history) opening audit and
exclusive LevelDB ownership; this set does not claim unbounded scalable startup
or impose a new player prospect cap. A later service must bound durable resource
usage and queues explicitly, as existing service protocols do.

## Durable issuance

`IssueManifestJob` is a trusted-authority API. The context comes from its
separately trusted node/policy resolver, not a public claim. The requested gym
public key is validated, but this method does not by itself establish a login,
wallet owner, session capability or current account identity.

Under the gate, require a writable healthy schema-5 handle and a nonnil final
check. Validate the complete context/key, stored chain and descriptor DNA
linkage. The authority and descriptor are always taken from pinned metadata;
the caller cannot select or replace either. Check count exhaustion and perform
the trusted final node/policy/authorization callback. Generate the unpredictable
32-byte challenge internally; read the positive authority clock and construct
the exact set36 ManifestJob with that issue time and `+300000` expiry.

Check job-key collision, encode within its bound, and atomically write job plus
incremented JobCount and monotonic clock highwater with `Sync:true`. Return the
job only after success. A collision is an error; do not overwrite or silently
regenerate within the same call. A successful job may later produce multiple
distinct qualifying proofs, with no per-job fighter cap.

`GetManifestJob` returns only this store's immutable issued row, with all pins
checked. It does not observe a clock, renew the job, authenticate an honest
issuer, or claim a current admission opportunity. ID arguments accept either
hex case and return canonical stored values, matching existing lookup behavior.

Every clock observation used by a new operation is durably preserved before
returning a definite time-based rejection; a later failure may leave only a
clock advance. A read error or uncertain synchronous job/clock write returns no
job. An uncertain write poisons the handle; its error never proves no commit.
Lost issuance output can strand a durable unused job. After reopen, a known ID
can be read; otherwise issuing another unpredictable job is an explicit new
operation, not reuse of the earlier ID/challenge/time. Do not add hidden request
idempotency tokens or expose an unauthenticated enumeration endpoint here.

## New admission and immutable static binding

`RecordManifestJobChecked` takes proof/auth only, never a caller-supplied job,
Verified value, descriptor, manifest, portrait, birth, receipt or timestamp.
The callback is mandatory even for an eventual duplicate, but is invoked only
for a new admission. It receives the registry's internally verified result and
a separate owned proof slice. It must not reenter the store or mutate captured
inputs. No caller-controlled generator callback is accepted.
This admission method requires a writable healthy handle before hashing, even
for a historical duplicate, matching existing RecordJobChecked behavior.
Read-only consumers use the verified Get/List methods instead.

The ordered new-admission path is:

1. Under the store gate, check state/production profile, bounded-copy and
   canonicalize proof/auth, and require both versions 2. Load the job by proof
   JobID from this DB. Validate the full Job2 and authority/descriptor/chain pins;
   require its gym and public key to match the proof/auth.
2. Verify authorization and call `prospect.Verify` on the stored job context.
   This establishes the internally trusted WorkHash, DNA and ProspectID for
   this call. No extra Argon call is introduced by generation or the D69 guard.
3. Look up the full ProspectID. If already recorded, take the historical path
   below. Otherwise observe/persist clock highwater, require issue <= observed
   time < expiry and sequence space. No claimant arrival/discovery time applies.
4. Resolve the store's exact descriptor and derive
   `fightermanifest.GenerateText(descriptor.ManifestVersion, verified.WorkHash)`.
   Require its full DNA to equal `verified.DNA`, and require exact generator,
   table, manifest/portrait component selectors from the resolved tuple.
   Validate/serialize the complete manifest using its canonical helper.
   Derive ManifestDigest through `fightermanifest.Digest`, render only the
   selected version through `fighterportrait`, and derive PortraitDigest through
   its existing domain-separated helper. Respect the existing 8,192-byte
   manifest and 32,768-byte SVG bounds. Use display-order WorkHash, never its
   reversal, a claimant seed or mutable current tables. The SVG is temporary;
   neither input markup nor its bytes enters a receipt row.
5. Construct StaticBinding version 1 from the pinned descriptor ID and those
   exact digests. Preflight the eventual receipt's maximum/canonical encoding
   before the final node callback so a known size failure cannot become a write.
   The size calculation must reserve the longest valid final acceptance time,
   eligibility source and height; it must not persist or expose a
   provisional birth/eligibility assertion. Actual final encoding is checked too.
6. Run the trusted D69 guard with owned proof and internally verified work. It
   must establish the trusted node/policy boundary and any caller-configured
   session authorization. The store itself has no session implementation. Validate the
   returned Eligibility with the existing rule-2 helper: either exact original
   current parent, or exact canonical winning block at parent height+1 meeting
   the block target. No arbitrary RPC-error fallback, successful-submit shortcut,
   stale-share/side-chain exemption or additional work hash is introduced.
7. After all hashing, generation, rendering and the guard, read the final
   authority time. Require `issued_at_ms <= accepted_at_ms < expires_at_ms`.
   Equality at expiry fails; persist a later observed clock on rejection.
   Record BornAtMS exactly equal to durable job issuance. No grace, extension,
   retrospective enrichment or later-job rebinding is allowed.
8. Encode the complete bounded receipt; in one synchronized LevelDB batch write
   receipt, sequence index, next LastSequence and clock highwater. Acknowledge
   record/inserted=true only after the batch succeeds. All receipt, birth,
   eligibility and static binding exist together or not at all.

No successful receipt exists without the static binding. Generation, rendering,
authorization, work, support, guard, expiry or encoding failure creates no
receipt or sequence; already durable clock observations and the issued job may
remain. The final callback is an observation boundary, not a transaction with a
node. A reorg can occur after it or after the receipt commit.

The library is a trusted storage boundary, not an account service. Future use
under a `gymaccount` lease must preserve the existing lock order (account lease,
then registry operation), bind the session to the job's gym key, keep it held
through commit, and recheck authorization at the final trusted callback. A
session expiring while rendering cannot be authorized only by an earlier check.
Historical disclosure needs a live session check too. No new account dependency
or cross-store transaction is added by set37 itself; fixture tests may compose
the existing lease without changing its implementation.

## Historical duplicate recovery

A qualifying resubmission for an already accepted ProspectID returns the
original canonical ManifestRecord and `inserted=false`. It revalidates incoming
authorization/work and verifies the stored receipt and exact issued-job link.
Equivalent valid retry signatures need not be byte-identical: preserve the
first stored signature rather than rewriting it. A conflicting verified result
or issued job under the same full ID returns ErrConflict, never merge/overwrite.

The stored descriptor is not replaced by a new/current bundle. Re-deriving its
binding for integrity uses that exact immutable tuple and compares the original
hashes; it does not update them. Missing compiled support or a mismatch fails
visibly and cannot silently fall back. The candidate resolver is fixed in this
set, so runtime registration is not a route around this rule.

Duplicates do not read the clock or node, run the new-admission guard, recheck
expiry/current policy, allocate a sequence, or change birth/eligibility/binding.
They remain available after expiry, clock rollback, restart, node outage or
later reorg, assuming the store itself is healthy. The result is historical
evidence, never renewed current eligibility/canonicality. Future callers must
still authorize disclosure; skipping the node guard is not a session bypass.

An internal verified-result reuse is permitted only for this same call when
the complete normalized stored proof, context and job equal the just-verified
submission and stored Verified equals the result. Recheck stored authorization,
linkage and binding independently. There is no exported Verified bypass/cache.
If evidence differs, either fully reverify the stored proof serially before the
existing conflict comparison or refuse an actual conflict; do not infer its
validity from header/hash text alone. New admissions and ordinary reads use one
Argon proof verification each; a nonidentical historical retry may require two,
always serialized. No memory arena overlaps or background rehash is introduced.

## Verified reads and resource limits

Get and List read only through this explicitly pinned authority handle. For
each result, check the canonical row, exact own issued job, full chain/authority/
descriptor links, birth bounds, stored authorization and eligibility binding,
index reciprocity, then re-run production `prospect.Verify`. Require exact
equality with the complete stored Verified value, regenerate the fixed manifest
and portrait digests, and compare StaticBinding. Return no partial result on
failure. Reads do not consult a node/current clock, and do not rewrite any row.
GetManifestAdmission accepts either hex case for its exact 32-byte ID argument,
normalizes it for lookup, and returns canonical stored values, matching
GetManifestJob and existing Get. Prefixes, short forms and whitespace fail.

Info and GetManifestJob are cheap public metadata/issuance reads, explicitly not
full work verification. GetManifestAdmission performs at most one production
hash. List accepts a limit in [1,8], uses strictly increasing receipt sequences
greater than `after`, and performs at most eight sequential production hashes.
Its JSON-array output is bounded by 524,297 bytes from eight 65,536-byte records
plus brackets/commas. No SVG or full manifest is appended. An `after` at/above
LastSequence returns an empty page without wrapping the cursor. Validate exact
expected consecutive indexes and count; any corrupt entry makes the whole call
fail, not emit a trusted prefix. Never loop to an untrusted integer counter.

The work-count bound does not promise a disk/network wall-clock timeout or an
interruptible Argon primitive. The library has no network call except its
trusted callback. Callers must bound concurrent requests and pagination; the
operation gate/Close drains actual work rather than abandoning a hashing
goroutine. Do not poll/reverify an entire history every UI tick. A later product
projection can incrementally consume verified sequence pages and render a
selected receipt's fixed tuple without changing this authority boundary.

This set returns verified receipt DTOs, not a complete UI card, current-condition
projection, ten-slot roster or signed archive. A caller can use the pure
manifest/portrait functions on the returned verified work to show a candidate
preview; arbitrary JSON that resembles the DTO has no equivalent provenance.

## Trust limits and unchanged policy

Open/reads detect malformed or internally inconsistent stored evidence under
the trusted-local-storage model. They cannot prove the authority's wall clock
was honest, detect a coordinated administrator rewrite/whole-directory rollback,
authenticate an asserted node observation, or prove a self-consistent changed
acceptance timestamp inside its valid interval. There is no issuer receipt
signature. A valid gym signature authorizes the proof; it does not sign the
authority's acceptance time or prove this DB actually issued the job.

Unsigned inspection/exported JSON therefore cannot enter an admitted-fighter
projection merely because proof work and deterministic hashes verify. No import
API or public arbitrary-record trusted verifier is added. Public portable
provenance/checkpoints need a separate reviewed publication boundary; the
offline checkpoint key is not reused as an online receipt key.

The initial admitted gym is proof/job-bound. Do not call it current owner after
transfers, make the latest eight/ten receipts a roster, assert fight eligibility,
or attach a set35 condition/XP value to this receipt. D44 automatic admission,
D55 immutable static selection, D68 five-minute issuance birth/no grace, D69's
exact winning-block exception, D27 ten selected roster slots with unlimited
ownership, D63 prospective notice and D72 authorization holds are already
settled. No new player-policy question is needed for this candidate library.

The remaining real decisions/gates are separate: select/activate an official
bundle and authority deployment, calibrate production prospect policy, implement
current ownership/transfer/roster event authority and public authenticated
provenance, and complete D74 before friends/tester/public release. Candidate
schema-5 issuance does not satisfy or bypass any of them. Retroactive assignment
to schemas 1–4, changed expiry, alternate birth, a movable descriptor catalog or
new ownership claims would be a different scope/policy change and are excluded.

## Acceptance before publication

After root approves the exact contract, build independent expectations from
the frozen set36 job/candidate bytes and original DNA/manifest/portrait oracles.
No Go output may become its own expected binding. An unsupported counterfactual
must remain unsupported; do not register a synthetic second runtime descriptor.

- Preserve exact old schemas 1–4 and set36 fixtures/readers/builders. Verify old
  Open refuses schema 5, new Open refuses 1–4, and missing/new fields do not
  silently downgrade or enrich historical records.
- Exercise Create-before-FS validation, entropy failure, marker/metadata pin
  mismatches, lost successful Create acknowledgement followed by unsigned
  inspection and explicit pinned Open, and refusal of incomplete initialization.
  Unknown descriptor/production activation and cheap-PoW authority paths fail.
- Check every canonical marker/metadata/receipt field, bounds, depth, null,
  duplicate decoded spelling, aliases, extra keys, numeric errors, descriptor
  replacement, job/record/index reciprocity, clock/count conflicts and malformed
  proof/authorization. Coherent unsigned rewrites outside the trust model must
  not be advertised as detectable.
- Prove exact manifest/SVG digests from internally verified work, full DNA
  equality, descriptor linkage and immutable first record; descriptor/authority/
  job/target/manifest/digest mutations fail at their actual boundary. Exercise
  a maximum-size valid proof and receipt size preflight with no partial write.
- Cover both D69 sources using a real isolated production-Argon node, including
  canonical winner after immediate broadcast, stale nonwinner/side branch
  rejection, and final expiry reached during rendering or the trusted guard.
  No callback consumes a claimant-created Verified summary.
- Show multiple prospects per job, expiry equality refusal, persisted rejection
  highwater and rollback/reopen, historical retry after outage/expiry/reorg,
  equivalent authorization retry preserving the original signature, and exact
  receipt/binding equality after restart. Reads are historical and node-free.
- Kill real child processes before/after synchronized initialization, issuance,
  receipt/index/metadata commit and lost acknowledgement. Inject both definite
  and commit-then-error writes; prove no success output, poisoning, close/reopen
  reconciliation and no duplicated sequence/binding. Copy all caller slices and
  test callback/result mutation, concurrent operations and Close drain.
- Prove full verified Get/List, exact eight-hash/page and encoded-byte limits,
  no hash on Info/job lookup/empty page, no partial page on corruption, and no
  public JSON-to-trusted-record path. If the duplicate optimization is used,
  test its exact proof/context equality precondition and differing-evidence
  fallback explicitly; do not add a public verification shortcut.
- Run focused normal/race/tagged-refusal checks and the relevant old registry
  regressions, then one source-frozen normal-PoW library composition. No public
  node submission, real keys, service deployment or UI change is part of this
  acceptance slice.

Implementation ownership and later service/worker protocols are assigned only
after this proposal is approved. No new oracle or runtime code accompanies it.
