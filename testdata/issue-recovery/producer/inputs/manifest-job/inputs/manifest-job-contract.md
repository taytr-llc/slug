# Set36 candidate descriptor and typed manifest-job contract

Status: exact engineering proposal for root review, November 16, 2026 Pacific. No oracle output or runtime implementation is authorized by this document alone. This slice adds only an immutable static-appearance descriptor codec/resolver, a separately typed Job v2 assertion, and the small typed builder adapter specified below. Registry, service, worker, outbox, CLI and desktop behavior remain unchanged. Production issuance and official generator activation remain unavailable; D74 is unchanged.

This refines the first step of `fighter-admission-contract-proposal.md`. It does not implement the proposal's later admission, receipt, ownership or roster features. The descriptor binds DNA/manifest/portrait choices only. It does not select the condition projector, XP accounting, league/fight rules, account authority, current ownership or a trusted clock.

## Existing behavior that must remain intact

The review read `slug/prospectjob/job.go`, `json.go`, `json_test.go`, `slug/prospect/json.go`, `proof.go`, and `slug/prospectwork/build.go`/`types.go`.

- `prospectjob.Job`, `New`, `Validate`, `ParseJob`, and `CommitmentScript` stay Job v1. Its SHA256 domain, binary preimage, field order, canonical values, fixtures and reader behavior stay unchanged. Do not add optional fields to that struct or make `ParseJob` dispatch to v2.
- Existing `ParseJob` is **value-canonical, not byte-canonical**: whitespace and object field order are accepted; decoded escaped field spellings are accepted when they name a required field exactly once. Duplicate detection uses decoded names. Case aliases, missing/unknown/null fields, invalid scalar types and trailing data fail. Keep that behavior rather than retroactively imposing a canonical JSON-byte parser.
- Context v1, policy ID, DNA v1, proof v2/`SLUGPR02`, authorization v2 and prospect ID stay unchanged. New Job v2 is an independent version namespace; proof v2 already carries an opaque 32-byte JobID.
- `prospectwork.Build(Job,...)` stays the v1 builder. `Candidate.Version` remains 1 because its DTO/claims do not change; a candidate's `Proof.Version` remains 2.
- No existing database is migrated, enriched with a descriptor, or reinterpreted by its timestamp, branch, current configuration or software version. All previous strict readers continue to reject a new job shape.

## Immutable archived normative inputs

The single compiled candidate descriptor must reference these exact archived artifacts, not mutable top-level prose:

| Artifact | Archived local source | Required SHA256 |
| --- | --- | --- |
| Original manifest contract | `/Users/taytrrip/slug/portrait-renderer-reference/inputs/fighter-manifest-contract.md` | `ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80` |
| Approved portrait contract | `/Users/taytrrip/slug/portrait-renderer-reference/inputs/portrait-renderer-contract.md` | `8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5` |

These archived bytes were checked directly. The descriptor carries the **raw SHA256** of each contract artifact. `manifest_table_hash` remains the existing domain-separated table hash, not a raw-file checksum. A later edited status paragraph or moved path cannot silently change an issued descriptor. Paths themselves are not encoded.

## Descriptor shape, codec and support boundaries

New leaf package: `fighterdescriptor`. It has no issuer store, clock, network, key, mutable registration catalog or executable/plugin loading. The DTO is value-only:

```go
type Descriptor struct {
    Version              uint32 `json:"version"`
    DNAVersion           uint32 `json:"dna_version"`
    ManifestVersion      uint32 `json:"manifest_version"`
    GeneratorID          string `json:"generator_id"`
    ManifestTableHash    string `json:"manifest_table_hash"`
    ManifestContractHash string `json:"manifest_contract_hash"`
    PortraitTraitVersion uint32 `json:"portrait_trait_version"`
    RendererVersion      uint32 `json:"renderer_version"`
    RendererID           string `json:"renderer_id"`
    RendererContractHash string `json:"renderer_contract_hash"`
}
```

The order above is the exact canonical JSON order. DescriptorID is not a field of the descriptor.

```go
const Version1 uint32 = 1
const MaxDescriptorSize = 2048
const MaxDescriptorDepth = 1

var ErrDescriptor error
var ErrUnsupportedDescriptor error
var ErrProductionUnavailable error

func Normalize(d Descriptor) (Descriptor, error)
func Validate(d Descriptor) error
func Marshal(d Descriptor) ([]byte, error)
func Parse(raw []byte) (Descriptor, error)
func ID(d Descriptor) (string, error)
func CandidateDescriptor() Descriptor
func ResolveCandidate(descriptorID string) (Descriptor, error)
func ResolveProduction(descriptorID string) (Descriptor, error)
```

The three boundaries are deliberately different:

1. **Normalization:** `Normalize` accepts only descriptor format `version=1`, positive uint32 component versions, identifier/hash shapes below, and lowercases only the three hash fields. Hashes may initially contain ASCII uppercase hex. It does not trim input, rename identifiers, alter identifier case, default missing values, select implementations or claim support. Its returned value is structurally canonical. It returns the zero descriptor on error.
2. **Structural canonical codec:** `Validate`, `Marshal`, `Parse`, and `ID` accept only already-canonical values. They do not silently call normalization to accept uppercase hashes. A structurally valid descriptor can refer to an unsupported future/test bundle; codec success is never resolution or activation. Unknown descriptor **format** versions fail. Positive but unsupported component selectors remain encodable so their immutable identity can be archived and rejected explicitly by resolvers.
3. **Compiled resolution and activation:** `ResolveCandidate` accepts an exact canonical lowercase 64-hex descriptor ID and looks it up in the fixed one-entry candidate catalog. That ID is computed over the **entire** canonical compiled descriptor, not just its generator name or versions. It returns a fresh value copy of that exact tuple. A well-formed unknown ID returns `ErrUnsupportedDescriptor`. Malformed IDs return `ErrDescriptor`. `ResolveProduction` has an empty catalog: every well-formed ID, including the candidate's ID, returns `ErrProductionUnavailable`; malformed IDs return `ErrDescriptor`. No caller can supply an activation flag, replacement catalog, cutoff, arbitrary implementation, or `latest` alias.

Canonical descriptor JSON is compact ASCII with no surrounding/inter-field whitespace, terminal newline, BOM, escapes, reordered fields, duplicate/unknown/missing/case-alias keys, arrays, nested objects, nulls, booleans, numeric strings, fractional/exponent/negative version numbers or trailing bytes. Integers use minimal positive decimal spelling. Root object depth is 1. Check byte length and depth before decoding, own the bounded input copy, and reject decoded duplicate names before accepting any value. Finally require exact equality to `Marshal(parsed)` bytes. Direct typed validation is equally strict about field values. No custom `UnmarshalJSON` that weakens these public boundaries is needed.

Both IDs are 1–96 ASCII characters from `[A-Za-z0-9_-]`; they are case-sensitive and are never normalized. Each hash is exactly 64 lowercase hex characters. A zero digest is structurally allowed; improbability is not an encoding rule. No URL, path, Unicode, whitespace or caller markup is an identifier. All component version fields must be greater than 0. `Marshal` returns owned bytes of at most 2,048 bytes; an error returns nil bytes. `ID` returns an empty string on error.

Descriptor identity is exactly:

```text
SHA256(ASCII "SLUG fighter issuance descriptor" || 00 || ASCII "v1" || 00
       || canonicalDescriptorJSON)
```

Return lowercase hex in SHA256 output order, with no reversal or extra length prefix. No descriptor digest is generated in this planning document; the independent oracle follows approval.

### The sole candidate tuple

| Field | Exact value |
| --- | --- |
| version | 1 |
| dna_version | 1 |
| manifest_version | 1 |
| generator_id | `slug-fighter-manifest-candidate-v1` |
| manifest_table_hash | `72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44` |
| manifest_contract_hash | `ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80` |
| portrait_trait_version | 1 |
| renderer_version | 1 |
| renderer_id | `slug-fighter-portrait-candidate-v1` |
| renderer_contract_hash | `8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5` |

This is a compiled **candidate** mapping to the already reviewed manifest and portrait implementations/assets/vectors. It is not an official production generator assignment. Keep the package a small static descriptor leaf; tests tie its selected constants and archived hashes to the reviewed implementation artifacts. Do not expose mutable maps/slices. Changes to rules, tables, geometry, decimal formatting or accessibility bytes require a new immutable tuple, never replacement under an existing ID. Future production activation requires its own reviewed catalog boundary; this set provides none.

## Separately typed Job v2

Add new functions/files alongside v1 in `prospectjob`:

```go
const Version2 uint32 = 2
const MaxManifestJobSize = 8192
const MaxManifestJobDepth = 2

type ManifestJob struct {
    Version      uint32           `json:"version"`
    RuleVersion  uint32           `json:"rule_version"`
    ID           string           `json:"id"`
    AuthorityID  string           `json:"authority_id"`
    DescriptorID string           `json:"descriptor_id"`
    Context      prospect.Context `json:"context"`
    PublicKey    string           `json:"public_key"`
    GymID        string           `json:"gym_id"`
    Challenge    string           `json:"challenge"`
    IssuedAtMS   int64            `json:"issued_at_ms"`
    ExpiresAtMS  int64            `json:"expires_at_ms"`
}

func NewManifest(context prospect.Context,
    authorityID, descriptorID, publicKey, challengeHex string,
    issuedAtMS int64) (ManifestJob, error)
func ValidateManifest(job ManifestJob) error
func ParseManifestJob(raw []byte) (ManifestJob, error)
func MarshalManifestJob(job ManifestJob) ([]byte, error)
func ManifestCommitmentScript(job ManifestJob) ([]byte, error)
```

Every operational helper above requires successful `ResolveCandidate` for the job's descriptor ID and requires `resolved.DNAVersion == job.Context.DNAVersion`. There is no public unresolved-job constructor or a helper that accepts an arbitrary descriptor while implying support. This is an explicit candidate-only API. `ManifestCommitmentScript` is not an official issuance action.

`NewManifest` normalizes hex inputs consistently with v1 `New`: after exact shape checks, lowercase authority ID, descriptor ID, context hash/target strings, public key and challenge. It then resolves the normalized descriptor ID. It computes the gym ID using existing `gymauth.GymID`, validates the complete context using existing public commitment/context semantics, and constructs canonical fields. Reusing `New` internally for shared validation is acceptable, but its v1 ID is discarded; never mutate a v1 `Job.ID` and pass the invalid result to a public v1 builder.

Authority ID and challenge are exact 32-byte hex values. All-zero encodings are allowed, as are zero SHA256 outputs; unpredictable authority namespace/challenge generation belongs to a later trusted issuer, not this deterministic constructor. The public key is a valid compressed 33-byte secp256k1 key as required by existing helpers. Do not broaden any existing gym/context validation while adding these fields.

`Version` is exactly 2 and `RuleVersion` exactly existing `RuleVersion1` (1). D68's lifetime is unchanged: `issuedAtMS > 0`, `issuedAtMS <= MaxInt64-300000`, and expiry is exactly `issuedAtMS+300000`. No current time is read and an already-expired assertion remains syntactically valid; this helper cannot establish actual issuance, an unexpired admission opportunity or accepted birth. In particular authority ID is a public namespace assertion, not a signature, key or authenticated issuer credential.

`ValidateManifest` reconstructs the canonical supported value and requires equality of every scalar field, including ID, resolved descriptor, derived gym and expiry. Uppercase stored/parsed values fail rather than being silently repaired. Unknown job/birth-rule/context versions and unsupported descriptors fail. Value-only DTOs are copied; no caller slice/map is retained. All constructors/parsers return a zero ManifestJob on error, and serialization/commitment return nil bytes on error.

### Exact binary JobID

JobID is lowercase SHA256 of the following concatenation, without any JSON or length prefixes:

```text
ASCII "SLUG mining job" || 00 || ASCII "v2" || 00
|| versionLE32(2) || ruleVersionLE32(1)
|| authorityID32 || descriptorID32
|| context.versionLE32 || context.networkLE32 || context.genesisDisplay32
|| context.dnaVersionLE32 || context.prospectTargetBE32 || context.parentHashDisplay32
|| context.parentHeightLE32 || context.expectedBitsLE32
|| context.minTimestampLE32 || context.maxTimestampLE32
|| compressedPublicKey33 || gymID32 || challenge32
|| issuedAtMS_unsignedLE64 || expiresAtMS_unsignedLE64
```

The complete preimage is 328 bytes: 19 domain bytes plus 309 field bytes. ID itself is excluded. The context encodings are identical to Job v1: display-hash hex is decoded directly with no reversal, target is unsigned big-endian/display order, numeric fields are little-endian. Positive signed-int64 timestamps are converted to unsigned uint64 only after range validation. No ambiguous text concatenation or variable-width version integer is permitted.

This changes **JobID only**. The existing proof-v2 output remains:

```text
6a 28 || ASCII "SLUGPR02"
      || SHA256("SLUG prospect job gym\x00v2\x00" || policyID32 || gymID32 || JobID32)
```

`ManifestCommitmentScript` calls the existing `prospect.CommitmentScriptForJob` only after complete new-job validation. Existing authorization v2 signs the same proof fields including its JobID. Existing prospect identity remains exactly:

```text
SHA256("SLUG prospect id\x00v1\x00" || networkLE32
       || genesisDisplay32 || workHashDisplay32)
```

No `SLUGPR03`, alternate Argon input, consensus change, transaction-hash change, signature domain change, new wallet derivation or changed prospect-ID formula is introduced. A changed job changes its coinbase commitment and therefore the corresponding candidate/header; the prospect-ID algorithm itself is unchanged.

## New job transport parsing versus canonical output

`ParseManifestJob` deliberately follows the v1 **transport** model rather than descriptor-byte canonical parsing:

- At most 8,192 bytes including leading/trailing whitespace; one root object only. Own a bounded input copy before parsing.
- Every one of the 11 fields appears exactly once after JSON name decoding. Require exact lowercase decoded field names; reject missing, unknown, duplicate, case aliases and null values.
- Standard JSON whitespace and arbitrary object field order are accepted, including in `context`. Escaped spellings are accepted if they decode to the exact required name/value; an escaped duplicate is still a duplicate. This is not a bypass because binary identity commits decoded canonical values, not transport spellings.
- Scalar versions/context integers decode into their uint32 fields and timestamps into int64; reject strings, booleans, nulls, fractions, exponent spellings, overflow and disallowed signs. Standard JSON syntax rejects leading-zero numbers. Issuance/expiry still must satisfy the positive fixed-lifetime rule.
- Depth is at most 2 (root 1, context 2), with no arrays. Preflight depth before decoding; handle quoted strings/escapes correctly. Context's raw object retains the existing independent 4,096-byte limit and `prospect.ParseContext` strict required-field behavior. Its field values are validated semantically by job validation.
- After the complete object and trailing-whitespace-only check, require `ValidateManifest` on the decoded value. Parsing v1 with this API fails; parsing v2 with old `ParseJob` fails. No auto-detection or downgrade.

`MarshalManifestJob` validates first and emits compact ASCII JSON, listed ManifestJob field order, no escapes/newline and minimal decimal integers. Nested `context` order is exactly:

```text
version, network, genesis, dna_version, prospect_target, parent_hash,
parent_height, expected_bits, min_timestamp, max_timestamp
```

This provides a canonical representation for fixtures/storage without pretending input transport bytes are part of JobID. `MarshalManifestJob(ParseManifestJob(pretty/reordered/escaped valid input))` must equal the canonical output. There is no separate weak JSON unmarshal path described as verification.

## Small typed builder adapter

Include only this contained entry in `prospectwork`:

```go
func BuildManifest(job prospectjob.ManifestJob, template Template,
    payoutAddress string, extraNonce uint64) (Candidate, error)
```

It performs the same production-PoW profile refusal as `Build`, calls `ValidateManifest` (therefore candidate resolution and DNA linkage), and then shares a **private** assembly routine with the existing builder. The private routine receives only internally validated context/gym/jobID/commitment fields. It is not exported as an arbitrary-job bypass. The v1 route validates v1 first; the v2 route validates v2 first.

All template/parent/bits/min-time/cur-time bounds, transaction/witness preservation, payout validation, extra-nonce encoding, coinbase output ordering, canonical serialization, weight/sigop limits and failure behavior stay byte-identical for the v1 route. The v2 route changes only the bound job/commitment and resulting dependent transaction/header bytes. It uses nonce 0, does not run Argon, read a clock, sign, mine, broadcast, check expiry or query the node. Payout remains explicit; no default address/target is introduced.

Return the existing Candidate shape with Version1, the new JobID and Proof.Version2. Do not attach a fabricated v1 job, add unsupported-descriptor success, or claim issuance/currentness/admission. Existing worker/service/outbox code is not wired to this function in set36. Pure descriptor/job tests may use the existing test profile according to their current helper contracts; both work-builder routes retain production-profile refusal.

## Independent second-descriptor and compatibility evidence

A second **structurally valid but unsupported** descriptor is sufficient to prove the commitment property. Keep DNA version1 and alter only a renderer selector/contract field using an explicitly named unsupported test tuple. The independent Python oracle may encode/hash both descriptors and mathematically compute both v2 job preimages and generic proof-v2 commitment scripts from otherwise identical inputs.

Required conclusions are separate:

1. Both structural descriptor codecs/IDs are valid and the descriptor IDs differ; the independently calculated hypothetical JobIDs and script commitments differ while original DNA algorithm/version remain unchanged.
2. Only the one compiled candidate tuple resolves. `NewManifest`, `ValidateManifest`, `ParseManifestJob`, `ManifestCommitmentScript` and `BuildManifest` must reject the unsupported tuple/ID, even when its hypothetical ID/preimage is internally consistent.
3. The existing low-level `prospect.CommitmentScriptForJob` may reproduce the oracle's opaque 32-byte job commitment; that operation validates an encoding, not descriptor support, issuance or birth. Do not present it as an admitted job.

Do not add a second synthetic catalog entry, a test-only runtime registration hook or a caller-supplied resolver just to make the test construct two supported jobs. Tests can compare independent preimage bytes outside the operational constructor and must explicitly assert runtime refusal of the unsupported example.

After root approves this contract, independent evidence should cover:

- Exact candidate descriptor bytes/ID and both immutable archived contract hashes; canonical-byte parser rejection and normalization idempotence without implicit support.
- Unknown descriptor format versus unknown component tuple versus empty production resolver, with distinct documented error categories and zero outputs.
- Candidate job canonical JSON and all 328 preimage bytes; context/key/challenge/authority/time mutations; positive/maximum issuance and one-step overflow boundary; all-zero authority/challenge shape acceptance; no zero-digest encoding ban.
- Every job field missing/null/duplicate/case/escaped duplicate, numeric errors, whitespace/order/escaped-spelling acceptance, exact 8,192-byte and nested 4,096-byte bounds, depth, trailing data, uppercase value refusal, unsupported descriptor and v1/v2 cross-reader refusal.
- Unchanged v1 job/commitment fixtures and all existing payable/witness candidate vectors. New BuildManifest cases may reuse independent templates with a new independently serialized commitment; no Go-generated expected block bytes.
- An authority-ID-only mutation and descriptor-only counterfactual demonstrating distinct commitments. No mining or public-network submission is needed to prove these codec properties.
- Nil/zero error outputs, input ownership, concurrent calls, production builder refusal under cheap PoW, and focused package/race tests. A full service/admission flow is deliberately deferred.

## Explicit non-goals and later handoff

Set36 does not issue unpredictable jobs, generate authority IDs, store or authenticate issuance, choose a production bundle, enforce final D68 expiry, accept proofs, create birth/eligibility/static-binding receipts, authorize accounts, modify historical records, or label a fighter currently owned/rostered/eligible. JSON plus valid work does not authenticate an issuer. No new service route, worker mode, CLI action or UI appears implicitly.

The next slice can explicitly initialize a new authority/schema and pin this descriptor through issuance, proof verification and immutable receipt binding. Its activation and trusted read model need their own reviewed contract. The present candidate-only resolver is not that activation. This exact engineering boundary is ready for root review before oracle/runtime work; no new user-policy question is required to review it.
