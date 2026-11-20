# Set74 operator public-authority profile — proposal

This source-reviewed proposal separates operator custody from the published
Set73 fixture authority. It is not a frozen codec, implementation, key-creation
instruction or deployment approval. All preparation is external to the checkouts.

The intended result is a standalone pool owner that can publish authenticated
fighter admissions/history with an operator-controlled signer, while the remote
worker and recovery reader independently select the same public authority. It
composes with the separate payout-runtime plan; a real settlement key alone does
not fix the current fixture-only public-history signatures.

## 1. Smallest explicit format boundary

Add **public pin version 2**, selected only by new operator constructors and
read/verification entrypoints. Keep the ordered `PublicManifestPin` field grammar:

`version,source,publication_id,admission_key,publication_key,cutoff_key,checkpoint_key,rule_id,scan_origin,limits`.

For v2, `version=2`; the four role keys are independently expected lowercase
32-byte Ed25519 public keys. Each must be nonzero and must not equal a known
published candidate authority key (seed73, seed declaration/checkpoint, or result
statement/checkpoint). Freeze this finite exclusion list from independently
derived public reference inputs before implementation. Equality between roles is
allowed; no key is inferred from another role. The first provisioning recipe can
share admission/publication/cutoff custody and keep checkpoint custody separate.
Excluding known fixture keys prevents accidental fixture selection; it cannot
prove that an arbitrary key has never been published or compromised.

The underlying `source` remains the exact schema5 ManifestAuthorityPin. Keep the
current supported network/genesis, candidate descriptor resolver, genesis scan
origin, D83 rule ID and exact finite limits. This is an operator-custody profile
on the current candidate chain, **not** production descriptor activation,
permanent-genesis selection, a new consensus profile or an unbounded service.
Capacity continuation requires its own explicit design before those limits can
support ongoing public operation.

PinID remains the existing canonical digest recipe/domain over the complete pin;
the integer version and all public role keys are included. Retain the existing
v1 admission/publication/boundary/cutoff/frame/state record layouts and domains.
Every authority-bearing ownership statement includes the exact PinID directly
or through its checked parent. Therefore the different v2 PinID separates the
new source without rewriting D83 or the 202-byte transfer envelope. Version1 in
these records denotes their record grammar, not permission to trust a v1 source.
This reuse requires independent cross-profile refusal vectors before freezing.

The inherited SignedCheckpoint is the explicit exception: its canonical message
binds network/genesis/height/hash, not a publication PinID. Its existing v1 bytes
can remain unchanged: verify against the exact separately pinned checkpoint key,
then publish under the v2 source entry. A checkpoint is not an ownership source
or proof of current membership merely because its signature verifies.
The node's existing compiled report-only checkpoint public key is a separate
authority. This profile neither replaces it nor grants access to its private key.
Calling a source the existing official authority requires that operator's actual
approved custody binding, not a newly chosen key with the same display label.

## 2. Keep legacy selection closed

Existing `Validate/Parse/NewPublicManifestPin`, fixture store constructors,
fixture-signing helpers, readers and goldens remain v1-only and byte-identical.
Add an explicitly named `OperatorProfile` containing an owned immutable v2 pin;
it is validated configuration, never evidence of ownership or permission to sign.
Its constructor/parser has no network, key-loading or directory side effects.

Expose separate operator codec/constructor entrypoints, backed by a private
closed profile dispatch (`fixture_v1` or `operator_v2`, no fallback). Legacy codec
wrappers select fixture_v1. Operator wrappers require OperatorProfile and select
operator_v2. Refactor shared validators to receive the selected profile internally;
do not change a legacy validator to accept arbitrary keys or relax its version.
An existing DTO may represent wire version2 as an owned projection, but passing
it into a v1 validator/open still refuses. Object-supplied pins never choose trust.

Proposed concrete seams, with names to settle at implementation review:

| Package | Added explicit entrypoint | Retained implementation |
| --- | --- | --- |
| registry | `ParseOperatorProfile`, `NewOperatorProfile`, `OperatorCodec` | Pure canonical pin/record verification; the full original expected pin is supplied independently. |
| registry | `CreateOperatorManifestAuthority`, `OpenOperatorManifestAuthority`, `OpenOperatorManifestAuthorityAudit` | Existing ManifestStore with a private immutable selected mode and the same record/outbox transaction discipline. |
| registry | `OperatorPublication(expected, signer)` | Concrete ManifestPublication bound to that store, expected profile and original signer lifetime; old PublicPublication refuses this mode. |
| fighterownership | `CreateOperator`, `OpenOperator`, `OpenOperatorAudit`; operator source/chain-reader constructors | Existing Store and PreparedTransfer/PreparedRecovery machinery in a selected private mode; remote/audit stores have no signing owner. |
| fighterownership | operator public reader/server constructors and codec | Exact same bounded methods/frontier semantics under independently expected v2 profile; use `/slug/ownership/v2` and request/response version2 to avoid route ambiguity. |

Returning the existing concrete store/capability types lets wallet preparation
continue consuming an original `*PreparedTransfer` or `*PreparedRecovery`. The
wallet/private carrier already binds an opaque OwnershipPinID and must not be
given a caller-asserted replacement. No wallet schema bump follows merely from
changing a source PinID; explicit creation still chooses a fresh bound owner.

## 3. Fresh stores, no repinning

Use `SLUG-MANIFEST-OWNERSHIP-V2.json` and `SLUG-FIGHTER-OWNERSHIP-V2.json`, each
with guard version2 and the exact v2 pin. Keep the underlying manifest schema5
marker/records and the current closed database row grammar. The registry's
`ownership/guard` is also version2 and must equal its file guard. Core marker
version2 and its exact metadata PinID bind the same profile. Existing row-format
versions remain unchanged; the guard selects the profile before their audit.

Old opens reject the unexpected filename/version before writable database open;
old ordinary manifest auditing also rejects ownership rows. New opens require
their exact marker, expected pin and full audit; mixed V1/V2 markers or renamed
guards refuse. Test refusal before mutation, including read-only and incomplete
setup paths. A selected fresh root durably records all expected IDs before
creation; no endpoint response or marker inspection may select them.

There is no v1→v2 in-place migration, re-signing of fixture admissions, copying of
private journals, or automatic retarget of a current source. Existing fighters
and histories keep their original source. Operator key rotation is likewise not
an in-place field edit; this finite profile stops/holds rather than inventing a
cross-source continuity protocol. A later rotation/migration protocol needs
separate design and qualification.

## 4. Concrete custody owner; public records remain the authorization boundary

Add a concrete internal `operatorauthority.Owner`, retained by the standalone
runtime and borrowed by the original registry/core/checkpoint owners. No generic
`func(digest) signature`, caller-selected crypto.Signer, HTTP signing endpoint or
runtime flag that enables fixture secrets is part of the API. Signatures can be
created only through the existing validated store operations; public DTOs cannot
mint a signing capability.

For the first concrete online adapter, propose read-only OS-selected protected
seed files for admission/publication/cutoff only, one exact 32-byte Ed25519 seed
per distinct online role key. Online configuration cannot name a checkpoint
private-key path. The existing official checkpoint private key remains on the
Mac; root never reads it and it is never exported to the pool host. The daemon
imports and verifies exact public checkpoint envelopes under its independently
pinned CheckpointKey. A separately selected local checkpoint owner/tool may
sign, but neither this profile nor the online daemon grants that authority. Paths are
private runtime configuration, not public PinID fields. Provisioning is a separate
operator action; this proposal creates or reads no keys. The loader verifies
owner-only permissions, regular non-symlink files and original inode identity,
bounds the read before allocation, derives each public key and compares it to
the independently expected role key. It refuses known fixtures and mismatches.
No secret appears in argv, environment, logs, manifests, errors or public health.
Secret backup/custody acceptance is required before real operation; protected
files are a bounded initial adapter, not a claim of hardware protection.

Keep raw crypto primitives in the internal package. They are a private mechanism,
not an authority callback exposed by the registry/core. Runtime-facing methods
must not return arbitrary signatures. The public constructor attaches one exact
owner/profile/role binding; signing leaves check that lifetime before and after
work and verify their result against the pinned public key. A replacement owner
cannot revive an old prepared handle. Context cancellation, poison and Close
retain the existing joined/sticky-owner semantics. Close prevents new calls,
cancels and joins active signing, then clears retained key buffers; do not promise
that the Go runtime can prove erasure of every historical memory copy.

The permitted signing sites remain narrow:

- Admission: exact original fully verified registry record and next outbox entry.
- Publication: exact durable pending body, sequence, previous ID and original
  attested time; original handle/source CAS checks before commit/disclosure.
- Boundary: retained authentic node/clock bracket and synchronized original
  registry intent, never a reconstructed late noon time.
- Cutoff: complete replay of that boundary and frozen admission/publication/
  finality-body prefixes; no current-owner substitution.
- Finality frame: original verified map and durable frame outbox, with immediate
  effective ownership/public-evidence-pending separation unchanged.
- Checkpoint import: exact signed public envelope under the pinned checkpoint
  key, followed by canonical/depth/publication checks. Signing is offline/local
  only under a separate owner and original verification; the signed message
  itself grants no readiness.

The current seed/result publishers attest their own specific anchor/target shapes.
A separately configured local transfer-history checkpoint owner must select the
canonical carrier-inclusion block; do not pretend it is a seed anchor's successor
or obtain authority from an arbitrary HeaderRef. Keep this operation local to the
retained source/node owner and its verified carrier history, with a durable exact
checkpoint/publication intent. No public miner method requests arbitrary signing. New online delegation of the
official checkpoint key would require a separate user decision; the default is
no delegation and no new online official key.

No unsigned or uncommitted signature is exposed. Automatic startup resumes at
most the existing exact pending chain/cutoff reference before admissions/current
observation. A pending chain block that is no longer canonical stays held; it is
not retargeted. Cutoff reference recovery uses the already signed stored cutoff
and original intent, grants no Head/event readiness, and creates no cutoff time.
Signer unavailability preserves durable pending work and explicit publication
gaps. Remote reconstruction never signs on behalf of the operator.

## 5. Scope and acceptance decisions

TLS origin/CA, credential enrollment, payout custody and recipient mapping remain
separate trust inputs. Add a versioned operator runtime configuration that binds
the existing poolshare Source/BindingID and the full public OperatorProfile, plus
the independently selected transport trust. The old pool source itself need not
change its accounting bytes merely to associate this new public profile. Reject
any source AuthorityID/network/genesis/descriptor mismatch before serving work.
Miner requests cannot supply authority keys or payout destinations.

The current gymaccount, stable/event league adapters and seednode pool binding
hard-code the two public gym fixtures and/or fixture payout address. They cannot
be relabeled operator consumers. The initial standalone pool path may use the
generic worker, authenticated accounting/recipient views and operator public
replay; any native league consumer of v2 needs its own explicit selected adapter
and valid real-account binding. Keep old native/owned fixture routes unchanged.

Routine engineering choices: this additive v2 profile, unchanged record grammar,
fresh guards, concrete signer ownership, startup recovery ordering, explicit
limits and cross-profile tests. None changes fees, share accounting, daily
cadence, confirmation/maturity, transfer order or D83 finality. No new economic
user decision is needed to implement it.

Actual deployment inputs remain necessary later: operator/custodian identity,
approved custody adapter and public role keys, exact source/genesis/descriptor,
origin/certificate trust and allowed miner enrollment. Existing identities cannot
be silently migrated. Permanent-profile/genesis activation, real hosted account
policy, key-rotation continuity and any make-whole/write-off policy are separate
decisions if requested; safe holds do not depend on settling them now. D74 remains
the final joint product gate. No question or deployment action is requested here.

Before implementation freezes bytes, lock independent v1/v2 pin/ID/signature and
guard refusal vectors, with public test keys labeled synthetic rather than real
custody. Qualify unchanged v1 goldens plus wrong-role/profile/source signatures,
fixture keys in v2, rename/downgrade/mixed guards, signer cancellation/replacement,
exact pending recovery and unavailable signer/public gaps. The actual standalone
service acceptance must then join this authority profile with remote workers and
the reviewed real payout backend; a pure codec pass is not public operation.
