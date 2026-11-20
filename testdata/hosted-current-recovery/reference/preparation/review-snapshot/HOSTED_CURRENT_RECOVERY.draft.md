# Set71 draft — hosted current recovery for the explicit fixture Pool

Status: candidate contract for root review, November 19, 2026. Not frozen and
not implementation authorization. This draft is outside both repositories.
No new expected outputs may be generated until the contract and independent
producer inputs/source have been reviewed and locked.

## 1. Deliverable and preserved policy

Add an explicitly selected, runnable fixture-hosted Pool mode to the normal
packaged desktop/helper. It uses the existing fixture selection, login/reset,
mining and backup controls and existing shared Pool source. Its account
authority is schema 2; all login/reset evidence is signed and goes through
`gymidentity` and `gymprovider`. The locally installed wallet remains the same
schema-9 Phase-A fixture wallet across reset. A purpose-separated fixture vault
provides enrollment/recovery and prospect signing, and an authorized current
export combines its retained entropy with a fresh concrete wallet cursor
checkpoint. Independently restore an actual exported file after change
allocation and prove funds and the same gym.

This is a candidate fixture controller and local simulated hosted custody, not
real signup, a live provider/KMS integration, a production wallet or cross-device
release. Publicly documented fixture RSA/KEK material is permitted only in the
explicit fixture driver/wrapper; no real key, credential, wallet entropy or
provider resource is read or created. Do not substitute unsigned claims for
signed evidence. A test-only constructor without the packaged user path does
not satisfy this deliverable.

D70 remains 24 English BIP39 words, empty phrase passphrase, and separate
login/encryption passwords. D72 remains same-wallet reset, immediate revocation,
durable pending barrier, fresh authorization and the 24-hour hold. Viewing and
mining remain allowed; withdrawal, recovery export, fighter transfer,
recovery-authority change and marketplace purchase/listing remain blocked.
Completed-event replay never restarts the hold. Export does not revoke hosted
authority or migrate to exclusive self-custody.

D73 remains separate: preserve profile-1 coin-0/mixed-49 derivation and the
version-1 recovery file. No profile-2 activation, final coin allocation, permanent
genesis or production issuance is part of this set. D74 remains the final joint
design review. Do not reorganize navigation or change logos.

Legacy schema-1 Pool selection/data, schema-2 account/coordinator APIs, the old
vault constructors and immutable export API, and all existing file/profile
formats retain their behavior. No silent migration, import of existing Pool
histories, source repinning or fallback between modes.

## 2. Explicit activation and public route

Native and helper accept `--candidate-hosted-pool` as a separate boolean mode.
It requires an explicitly supplied canonical absolute `--data-dir`, and the
helper's explicit normal `--node-binary`. It conflicts with `--candidate-pool`
and every legacy mode flag. A packaged default for ordinary Pool does not count
as explicit hosted selection. Go boolean spellings such as `=true` are accepted.
`--candidate-name-version=1|2` retains the same descriptor options, but version 2
requires an explicitly selected Pool or hosted-Pool mode and explicit root.
Unknown/mismatched selections refuse before directory/lock/store mutation.

The new runtime/native mode string is `local-hosted-fixture-pool`. The existing
PoolSnapshot field shape is unchanged; its Mode discriminator explicitly adds
this value. The native bridge pins the expected mode from launch arguments,
alongside the existing exact descriptor/source/full View, and rejects a response
from the other mode. The frontend recognizes both as Pool layouts, with a
visible “Hosted public-fixture pilot” caption for this route. No JSON/provider
configuration, arbitrary issuer, assertion, key, token or verified flag is
accepted from the webview. Current backup selection/Verify controls and existing
version-1 file parser remain unchanged.

`accountpilot.NewHostedFixturePool(ctx, pilot.Config, pilot.PoolOptions)` is the
normal candidate constructor. It constructs only the compiled fixture
coordinator/driver/wrapper. `ServePool` may dispatch to its existing controller
surface; native launch has a separate constructor that supplies the hosted flag
and expected mode. A private test factory may replace typed dependencies for
fault injection, never expand the production fixture allowlist.

## 3. Ownership and namespace

The hosted parent owns, in order of dependency: original schema-2 account
authority, coordinator, local fixture credential driver, HostedVault/current
checkpoint store, shared PoolRuntime, and the active child. The runtime borrows
the authority/vault and retains immutable bindings. The child owns one opened
installed wallet and one exact original login/View, and borrows the vault/source.
No second loader opens its installed wallet. Quit/failed construction invalidates
the View, cancels and joins the child and runtime borrows, closes runtime, then
hosted vault/current store, driver/coordinator and account authority. Shutdown
failure retains the responsible owner and prevents replacement/reopen; there is
no best-effort release of a wallet still in use.

Use a fresh separate managed root. Top-level entries are exactly:

```
instance-lock/
SLUG-HOSTED-POOL-INIT.json
SLUG-HOSTED-POOL-READY.json       # only after complete initialization
authority/
coordinator/
fixture-provider/
hosted-vault/
current-recovery/
shared/
```

INIT is written/synced before dependent stores. It records version 1, the mode,
descriptor ID, exact immutable fixture configuration hashes, and fixed provider
and wrapper IDs, in that order. After each store is created, a bounded immutable
stage receipt inside INIT's owned namespace pins its generated ID/config hash.
Crash after creation but before receipt may reopen only that exact next
initializer with the expected compiled configuration; it may not delete,
replace, adopt arbitrary nonempty contents or skip ahead. READY pins the account
AuthorityID, coordinator ID, hosted-vault identity, source BindingID and both
fixture retention/current-source bindings, in fixed fixture order. Once READY
exists all listed stores/receipts are open/verify-only; missing or mismatched
established data fails closed. The same holds for existing inner Pool READY,
recipient, league, pipeline and withdrawal markers.

All control/marker JSON has exact required fields, fixed version, no unknown,
duplicate decoded, null, case-alias or invalid-UTF8 values. Marker cap is 32 KiB;
single fixture stage receipt cap 8 KiB. Paths use existing canonical-directory,
regular-file/no-symlink and managed-root overlap checks. Hosted and legacy roots
must also refuse ancestor/descendant overlap with each other. No directory scan
selects a mode. Old Pool cannot open this namespace; no old marker is fabricated
as a compatibility shim.

The inner shared source, Pin2 stable histories and original registry sequences
retain their existing contracts. The new outer root may initialize that source
only once, and never adopts a source from an old authority. This is ordinary
fixture source selection, not a new chain or production genesis.

## 4. Signed local fixture provider

The fixed provider is `published-fixture-v1`, issuer
`https://identity.slug.invalid`, tenant `fixture-tenant`, client
`fixture-client`, connection `fixture-database`. Use
`gymprovider.FixtureVerifierConfig()` exactly, including its two audiences,
RS256/kid and credential-epoch claim. The driver is explicitly
`local-published-fixture-evidence-v1`. Its signing key is only the already
published RFC7515 A.2 fixture copied from the frozen old source input; no
environment key, key-file selector, live endpoint or arbitrary assertion input.
Pin its exact public JWK/config digest before stores open.

The driver selects only `zero256` or `incrementing256`. It persists its own
fixture credential epochs, original authentication facts and reset operations
separately from the coordinator, using bounded synced atomic state. It is not
the account authority. Transport assertions are transient strings <=16 KiB;
never save them, log them, put them in errors or return them to native/webview.
All claim grammar, number encoding, exact field order used by the independent
producer, audience/type separation and verification follow GYM_PROVIDER.md.
Fresh transport has `iat=now`, `exp=iat+120s`, but does not change original
`auth_time`, epoch, verification time, auth ID or operation ID.

The narrow code-only driver API is:

```go
type HostedFixtureEvidence interface {
    ID() string
    FreshLogin(context.Context, gymprovider.LoginFlow, string) (string, error)
    BeginResetEvidence(context.Context, gymprovider.ResetRequest, string) (string, error)
    CommitReset(context.Context, gymprovider.ResetRequest, gymprovider.ResetResult) error
    CompleteResetEvidence(context.Context, gymprovider.ResetRequest) (string, error)
    Close() error
}
```

The driver validates every supplied flow/ack against its retained immutable
record, including coordinator/authority, raw EventID versus derived Barrier ID,
nonce, fixture/account, provider operation, original epochs/time and phase.
CommitReset is one exact compare-and-swap from FromEpoch to ToEpoch after an
acknowledged pending barrier with CanCommit=true; repeating its same saved
operation returns its stored result without another credential mutation.
Historical completed receipts cannot authorize new mutations. Completion
evidence requires that stored successful credential mutation. No generic
“verified” method or caller-populated VerifiedLogin/VerifiedReset entry point.

Fixed capacities: account MaxLoginAttempts=1024, MaxSessions=4096,
MaxChallenges=4096, MaxEvents=16384; coordinator MaxLoginFlows=1024,
MaxResetFlows=256. Driver retains at most 1024 original auth rows and 256 reset
rows plus exactly two epoch rows, without eviction/recycling. Each original-auth
row <=2048 bytes, reset row <=4096 bytes, canonical driver state <=4 MiB.
Counter overflow and capacity fail before mutation; reserved reset completion
space cannot be consumed by new flows. One fail-fast driver operation, a
16-MiB accounted transient budget, and no unbounded request queue. These are
logical byte/count bounds, not a total Go heap cap.

Login: IssueLogin -> driver durable fresh authentication -> signed evidence ->
FinishLogin -> original private Session. Lost completion never returns an old
session token; discard/revoke an unpublishable token and require a new explicit
login flow. On startup, the same signed flow provisions the two fixtures in
fixed order, then revokes setup sessions. It does not start mining.

Reset: invalidate old native/controller View immediately, persist the exact
selected reset request/driver operation, verify BeginReset evidence and obtain
the durable authority/coordinator pending barrier BEFORE credential mutation
or child joins. Then join the old child, perform the exact driver credential CAS,
verify CompleteReset evidence, and obtain the original completed receipt before
fresh-login/recovering the same wallet. Begin failure still joins/revokes the old
view and never reopens it. An uncertain stage preserves the exact EventID and
provider operation; it is never replaced with a “latest pending” lookup.

After restart, no automatic credential mutation occurs. Existing fixture login
selection discovers only its own pinned pending record and reports reset
pending. The same Reset control becomes “Resume fixture reset”; it can operate
while signed out only for that exact retained fixture/event. It replays fresh
signed transport for the same operation under the existing coordinator rules.
No timeout unlock, forged completion or restart of verified_at. An expired
never-created begin may be retired only after the existing authority/coordinator
show no barrier; it cannot be turned into a different reset under the same IDs.
This does not add a manual override for a committed pending reset.

## 5. Explicit hosted vault and typed signing

Add `recoveryvault.CreateHosted/OpenHosted`, returning a distinct `*HostedVault`
with no accessor for the underlying legacy `*Vault`. Its store marker explicitly
selects hosted mode; old `Open` refuses it, and hosted open refuses a legacy
store. Envelope Binding/Purpose/ContentRevision/ManifestHash, original
DefaultAccounts, ciphertext plaintext formats, Receipt/RetentionID and wrapping
AAD remain exactly version 1. A new store-mode discriminator is not a new
cryptographic content format. Do not rewrite either envelope to track cursors.

This separation is mandatory for lock order. Legacy methods may take the account
gate and then wait for `v.lock`; new hosted methods acquire a fail-fast lifecycle/
vault permit outside account leases, then use short original-session fences.
Those two orders must not coexist on one exposed live object. Refactor private
pure helpers as needed, but preserve old constructors and method behavior.

Hosted methods expose only typed ProvisionFixture, SignAccountChallenge,
EnsureEnrollment, SignAdmission, Status and current-recovery operations.
They return owned public receipt/signature values and sanitized errors only.
The caller captures the exact original token before work. Vault/provider/DB
reads, proof verification, signature derivation, encryption/decryption and
permit acquisition occur outside account leases. Final fences reauthorize the
same token/generation, validate retained revision/binding in memory under the
already-owned permit, and perform only bounded local commit/disclosure. A lost,
omitted, double, retained or swallowed-error fence callback cannot publish.

Define a concrete pilot `HostedVaultSigner` binding, created only from the
parent's HostedVault, exact immutable Receipt/RetentionID, source Policy,
AuthorityID, AccountID and fixture identity. PoolChild admits that concrete
typed mode only through hosted resources, not a generic callback. It still
rejects AccountConfig.AdmissionSigner in legacy mode. Each signing invocation
uses one original mining token, copies exact Context/Proof, checks source/gym,
performs the vault call outside authority leases and verifies the returned
signature over the original bytes before final same-token disclosure. Vault
failure is failure; no local wallet-key fallback. D69 block-first worker order,
D80 expiry suppression and source issuer ownership are unchanged.

During held login, only the gym envelope may be unwrapped for typed enrollment/
recovery or admission signatures. No recovery plaintext/DEK, mnemonic, root
private key, arbitrary path/digest signer or withdrawal signer is added. The
retained fixture wallet continues to serve its existing guarded wallet functions;
the hold is not claimed to revoke independent copies of public fixture keys.

Fresh setup ordering is concrete: obtain original schema-2 session; prepare the
exact local wallet in its new owned root; provision/verify vault envelopes; use
the vault to sign the runtime's existing enroll/recover challenge; CompleteChallenge;
then EnsureEnrollment acknowledges the vault record without issuing another
challenge. Only afterward commit recipient/current-source/withdrawal READY.
Calling EnsureEnrollment before creating the local wallet is prohibited: current
ProvisionFixture correctly refuses a missing wallet for an already enrolled
account. Established missing wallet/vault remains a hard refusal.

## 6. Concrete public current-wallet capture

Add an opaque `wallet.CurrentRecoveryCapture`. Its constructor is a method on
the concrete retained Wallet, with context and no caller-supplied manifest:

```go
func (w *Wallet) CaptureCurrentRecovery(ctx context.Context) (*CurrentRecoveryCapture, error)
func (c *CurrentRecoveryCapture) Manifest() recovery.Manifest // owned copy
func (c *CurrentRecoveryCapture) CaptureID() string
func (c *CurrentRecoveryCapture) CapturedAtMS() int64
```

No JSON parser can construct the handle. Private fields bind it to the exact
wallet/manager owner, capture identity and original public retained profile.
The vault's current-source attachment receives that concrete wallet privately
from its verified runtime owner; an arbitrary same-root restored wallet or
webview manifest is not interchangeable authority.

The public reader must be new/factored bounded schema-9 logic. Do NOT merely
export recoveryPublicManifest/RecoveryAccounts: their existing call chain reads
private-retention ciphertext metadata through fetchRecoveryState. Reuse/factor
Set70's public identity checks, then read the exact four account-zero scopes and
eight issued cursors in the SAME walletdb View. No private retention Get, private
decryption, unlock/KDF, seed/root-private extraction or address derivation.
Reject watch-only, legacy, profile2/schema10/11, pending/completed schema12,
foreign chain/profile, extra scope/account or nonempty imported account.

Require fixed scope order 44,49,84,86; coin/account 0; existing mixed scope49
external/internal types; each cursor 0..4096. Validate persisted manager version,
derivation/recovery version, public root/manifest identity and current account
records, with limits before copies/decryption. At most five scope cursor entries
and three account entries per scope (the extra item is a refusal witness), plus
one imported-address presence probe per scope. No whole address/UTXO/tx scan.
Check context before View, between bounded reads and before return. The database
operation cannot be abandoned on cancellation; it is joined before owner close.

The runtime holds its fail-fast wallet mutex only for this local capture, never
under an account lease. The capture does not need node sync/PoW or transaction
history: it is a dated committed cursor inventory, not chain finality. It must
retain all non-cursor manifest fields exactly, and match the published fixture
root/gym and immutable vault receipt.

## 7. Separate current checkpoint

Attach a current-recovery owner to the concrete HostedVault and installed wallet
using a trusted immutable source binding established by the parent. Proposed
public types use this exact field order and snake_case JSON:

```go
type CurrentBinding struct {
    Version uint32               // 1
    HostedVaultID string         // random persisted hosted store identity
    VaultBinding Binding         // exact immutable version-1 vault Binding
    RetentionID string           // original immutable receipt identity
    WalletOwnerID string         // random persisted local owner namespace ID
    SourceBindingID string       // exact shared Pool source
}
type CurrentCheckpoint struct {
    Version uint32               // 1
    Binding CurrentBinding
    Revision uint32
    CaptureID string
    ExpectedRevision uint32
    CapturedAtMS int64
    CommittedAtMS int64
    Manifest recovery.Manifest
    ManifestSHA256 string
}
```

The candidate private API is concrete rather than a supplied-manifest callback:

```go
CreateCurrentStore(ctx context.Context, path string, vault *HostedVault,
    bindings []CurrentBinding) (*CurrentStore, error)
OpenCurrentStore(ctx context.Context, path string, vault *HostedVault,
    bindings []CurrentBinding) (*CurrentStore, error)
(*CurrentStore).AttachSource(ctx context.Context, originalLoginToken string,
    binding CurrentBinding, installed *wallet.Wallet, epoch EpochFence) (*CurrentSource, error)
(*CurrentSource).Capture(ctx context.Context) (*wallet.CurrentRecoveryCapture, error)
(*CurrentSource).Checkpoint(ctx context.Context, expected uint32,
    capture *wallet.CurrentRecoveryCapture) (CurrentCheckpoint, bool, error)
(*CurrentSource).LookupCheckpoint(ctx context.Context, captureID string) (CurrentCheckpoint, error)
(*CurrentSource).PrepareExport(ctx context.Context, captureID string,
    destination *CurrentDestination) (*PreparedCurrentExport, error)
(*PreparedCurrentExport).Publish(ctx context.Context) error
(*PreparedCurrentExport).Close() error
(*CurrentSource).Close() error
(*CurrentStore).Close() error
```

AttachSource pins one original login token/account/generation and current
parent View for its entire ephemeral lifetime, verifies the actual installed
wallet public identity outside account leases, and checks the exact compiled
store binding against the parent's READY ownership. It never closes the borrowed
wallet. Closing its parent child cancels/joins it before wallet Close; reopening
creates a new source from the same established owner namespace, not a new stored
WalletOwnerID. Capture handles from a former or different concrete wallet cannot
be used on it. PrepareExport requires the capture produced/checkpointed by this
live source, not an arbitrary historical ID from another source lifetime.

EpochFence is a code-only parent-epoch fence, **not** an account authorizer:
`func(context.Context, func(check func() error) error) error`. It acquires only
the parent view mutex, matches the captured helper/child/token/View/generation,
and retains that match through the bounded callback. It never calls WithSession.
The hosted vault itself takes the real original-token account capability fence,
then enters EpochFence, avoiding a nested account lease. It checks both
reauthorizers before and after commit/disclosure. Missing/double/late/outer-error
epoch callbacks fail closed. Passing a permissive epoch callback cannot bypass
the concrete vault-owned account authority. The parent does not acquire or wait
for vault/current/wallet permits from inside EpochFence.

The two source bindings are created only after both original fixture wallet and
vault reservations are verified; initialization may retain exact partial
receipts while the second fixture is prepared. Creation/open rejects duplicate,
foreign or reordered bindings. HostedVaultID is returned only by a bounded
owned `HostedVault.Info(ctx)` value pinned on create/reopen. A similar public
`Coordinator.Info(ctx)` exposes its existing immutable coordinator ID/config
binding without issuing an expendable login flow or reading marker files as
authority. These getters convey identity, never account capability.

IDs/digests are 64 lowercase hex; generated owner/capture IDs are nonzero random
32-byte IDs. Root/gym/profile fields use the unchanged recovery validators.
Revision is 1..128 in this bounded store; overflow refuses. ExpectedRevision is
0 for the first checkpoint and exactly Revision-1 thereafter. Time is
positive Unix milliseconds <=253402300799999. CapturedAtMS is sampled inside the
trusted capture operation; CommittedAtMS is sampled before staging the local
commit, must be >=CapturedAtMS and the prior store highwater, and is retained on
replay. Any rollback refuses rather than relabeling the old observation. Manifest
hash is SHA256 of existing canonical recovery.MarshalManifest bytes, no newline.
The current binding digest, if used by a marker, is SHA256 of ASCII
`SLUG hosted current recovery binding\x00v1\x00` followed by canonical ordered
CurrentBinding JSON bytes. No session generation is recovery identity.

`current-recovery` is an additive exclusively owned store, not legacy vault
record mutation. Exactly two account bindings, at most 128 retained checkpoint
actions across both accounts, and a current-checkpoint pointer per account.
Retain every committed action ID/result so exact replay and same-ID/different-
intent conflict are enforceable after later captures. No eviction or reuse.
Fixed maximum canonical state is 2 MiB; checkpoint <=8 KiB and manifest <=4 KiB
for this exact fixture inventory (the unchanged external recovery file remains
64 KiB). Creation pins complete bindings/config. READY loss refuses, never
silently creates an empty checkpoint store after established use.
Store canonical bounded action rows, current pointers and metadata in one
exclusively owned LevelDB namespace. A commit batch contains one <=8-KiB action,
one current pointer and counters/highwater, with total prepared key/value bytes
<=12 KiB. Full audit is bounded by 128 actions/two bindings and occurs outside
account leases. Do not rewrite a 2-MiB aggregate file under the final fence.

Checkpoint admission is original View, not RecoveryExport: it moves only public
metadata, cannot change recovery authority, and performs zero envelope unwraps.
Capture concrete wallet outside the account lease, then reserve the one current
store permit fail-fast before entering final original-token publication.
Publish by expected current revision and exact binding. Every branch cursor
must be >= its prior checkpoint and >= immutable initial inventory; reject any
regression rather than silently taking a max. Non-cursor fields must equal the
original receipt's manifest. Same CaptureID+identical capture/expected-revision
returns the original committed result, including its timestamp; same ID with
different capture or expected revision conflicts. A fresh capture with unchanged
cursors is a new dated action and consumes one slot. In one synced atomic commit
write the new immutable action, current pointer, revision and clock highwater.
Do not rewrite a previous action's time to make it look current. The 128-action
limit is explicit candidate capacity, independent of vault MaxOperations; it
does not promise an unbounded production backup history.

The trusted source owner keeps the admitted CaptureID until its commit
outcome is resolved. A write/sync error after possible replacement poisons that
owner/store; preserve exact bytes and require reopen/audit. Reopen resolves
whether the exact checkpoint committed before a new action may be admitted.
Do not blind-retry with new IDs or accept “whatever latest checkpoint” as the
result of the old operation. A new session may read the already committed public
checkpoint, but cannot rescue an old View's disclosure/publication.

The default user export always captures/checkpoints the currently installed
wallet first. The saved checkpoint is explicitly dated; it is not automatically
current after every future allocation. A missing/unavailable current wallet
cannot produce a “current” export merely from a saved checkpoint. Continuous
allocation-to-vault disaster-recovery coupling remains later work. Restoring this
file does not recreate payment journals, signed-input reservations or account
authority, and never authorizes deletion/replacement of them.

## 8. Current export and native publication

Add a distinct HostedVault current export path; never change legacy
ExportRecoveryFile from its original immutable snapshot. The new path privately
receives the attached concrete source/capture, current checkpoint owner and the
original login token/View. Revalidate retention, account, source, capture and
checkpoint; unwrap only the recovery-purpose envelope, validate entropy against
the original immutable manifest, then assemble recovery.NewExport with the
captured current Accounts. Root/gym/non-cursor identity must remain equal.
No generic seed/entropy/DEK/bytes getter or secret callback is exposed.

Use an opaque `recoveryvault.PreparedCurrentExport` and a concrete create-only
`CurrentDestination` owned by recoveryvault. The desktop converts only the
already-captured public CreateMode native selection to:

```go
type CurrentDestinationPin struct {
    Version uint32               // 1
    Path string                  // canonical absolute, <=2048 UTF8 bytes
    ParentDevice string          // exact16 lowercase hex
    ParentInode string           // exact16 lowercase hex
}
```

OpenCurrentDestination independently verifies canonical parent components,
pinned directory identity, basename, existing destination absence and protected
roots; it retains its own descriptor. Native holds the existing Selection until
the synchronous helper operation is known joined, including transport failure
or helper shutdown. The original full View/helper is captured BEFORE the native
dialog; an account switch does not retarget the selected destination.

Prepared export keeps <=65536 private encoded bytes in memory. It has no Bytes,
WriteTo or caller-controlled secret callback. Crypto, derivation, serialization,
vault/checkpoint traversal and permit acquisition happen outside account leases.
**No plaintext recovery staging file is created in the user directory before
the final RecoveryExport fence.** Root selected the established bounded private
local commit: under that exact original capability, create an unpredictable0600
temporary, write/sync <=64 KiB, verify its inode/size, exclusive descriptor-relative
link to destination, verify, unlink owned temporary and sync directory. Check
context/authority before commit and before reporting success. This finite local
file transaction is the explicit exception to outside-lease bulk work. Ordinary
filesystem sync can delay a reset through an admitted atomic commit; do not
claim OS disk I/O is interruptible.

Missing/double/late/swallowed fence callbacks and outer errors never report
success. Existing destination is definite conflict and never overwritten.
Possible link/publication followed by error is `export_uncertain`; retain that
classification through cleanup. Close joins operations, clears private buffers
and removes only an owned temporary inode. Sticky cleanup failure prevents new
export/backup ownership until safely resolved; never remove an uncertain final
destination automatically. Old recoveryfile pins, limits, writer/read behavior
and old vault API remain unchanged.

Existing native ExportRecovery routes to this current path only in the hosted
mode. Existing VerifyBackup uses the actual chosen file and Set69's bounded
scratch restore, not a cached in-memory bundle. Export success is “file saved,”
never “backup verified.” Existing backup phase/state/error protocol suffices;
private checkpoint details need not be exposed as a new webview family.

## 9. Lifetimes, locks and fixed bounds

One parent account setup/login/reset operation; one active child; one backup
operation per child, mutually exclusive with mining and joined activity,
payout, withdrawal, stable/history/event work using existing stopMu discipline.
Start remains explicit. A checkpoint/export is admitted only with mining stopped,
the existing backup lane acquired, and the current source owner retained. Pure
public checkpoint setup outside a child also holds the original runtime borrow.

One hosted vault operation permit is fail-fast (busy, no queue). One current
source/capture/export owner per account; the parent permit spans child generations
until actual cleanup joins. Reserve 16 MiB of accounted transient recovery work
before capture/envelope/encoding; keep exact independent component bounds: public
manifest 4 KiB, private recovery export64 KiB, legacy envelope196608 bytes each,
wrapped key8192 bytes, current state2 MiB/128 actions and two current bindings.
At most an old retained state, one staged replacement and their bounded encoded
copies overlap; never a third transition. No address
inventory or raw blockchain allocation belongs to this operation. File output
copies and in-memory encoded material count toward the reservation.

Whole entered backup/export action30s, each wrapper call5s inside it, existing
Set69 VerifyBackup10min with bounded steps, account login/reset2min and initial
hosted startup180s (45s node start plus two60s wallet setup paths and bounded
local remainder, with child deadlines clipped by parent). Driver evidence/RSA
work5s per call. No automatic retry loop extends these deadlines. Expiry stops
new work and final publication; actual non-context-aware DB/file work is joined.

No account lease spans wallet/DB/network/provider operations, lock waits,
hashing/crypto, derivation or joins. Exception only the explicit pre-admitted
bounded synced local checkpoint/file commit, with prepared bytes and no new
permit acquisition. Account reset establishes revocation/pending barrier before
joins; newly held sessions can view and use gym-only signing. Stale token/view
cannot be replaced by a token supplier. Every returned public result is owned,
and final disclosure rechecks the exact full View, helper, account/generation,
retention and source.

## 10. Failures and recovery vocabulary

Private APIs use typed invalid/config, profile, unauthorized, held, stale_view,
busy, capacity, conflict, not_found, unavailable, outcome_unknown, poisoned,
closed and cleanup errors. Preserve context cancellation internally. Public
messages never contain JWTs, subjects, filesystem data, mnemonic, key/DEK,
provider response bodies or private crypto errors. Reuse existing account/backup
bounded errors except one account code `hosted_reset_pending`, shown with its
exact retained fixture and Resume reset action. No payload accompanies failed
private wire responses.

Logical states and permitted recovery are:

| State | Next permitted action |
| --- | --- |
| INIT, no completed dependency | Resume only exact next initializer/config |
| Established dependency missing/corrupt | Refuse; no recreate/adoption |
| Provision reserved/sealed/verified | Same original reservation; fresh authorized continuation as existing vault rules allow |
| Pending reset | Exact retained event begin/complete evidence replay; never timeout unlock |
| Lost login completion | No token replay; next explicit signed login |
| Checkpoint commit uncertain | Reopen and compare exact saved capture/revision; no new capture until resolved |
| Export definite prepublish refusal | Cleanup/join; caller may explicitly select a new destination |
| Export outcome unknown | Inspect selected destination; no automatic duplicate export or overwrite |
| Cleanup/owner shutdown failed | Preserve sticky failure/ownership; do not reopen replacements |

The local fixture driver persists one unresolved reset association per fixture.
The UI never substitutes a reset by subject/time or takes an older account's
pending record. Closing the app preserves pending semantics and clears all View
handles; opening the app alone does not start mining, issue an export, mutate
credentials or reveal recovery words.

## 11. Finite qualification and independent expectations

Freeze this contract and exact old public inputs before producer design/output.
Independent Python source derives checkpoint/manifest canonical bytes and
digests, monotonic/cross-binding cases and signed provider evidence from existing
frozen fixture sources. No Go/desktop implementation output is an oracle. Reuse
unchanged old envelope vectors and Phase-A address/recovery vectors; generate
new vectors only for the additive binding/checkpoint/driver encodings.

Meaningful finite checks include exact mode refusal before mutation; INIT/READY
crash boundaries; held public capture with zero recovery unwraps; schema9-only
public reads with a bucket spy that forbids private keys; scope/account/import
and cursor boundaries; foreign/stale/regressing checkpoint refusal; known
commit/lost-ack/reopen; context/reset while capture or wrapper blocks; original
token/View and failed outer fence; vault busy cannot block reset under a lease;
driver original auth/replay/epoch/CAS; vault failure without local signer
fallback; native dialog switch/selection identity; final publication uncertainty;
Close/backup join and sticky cleanup. Preserve all legacy routes/profile tests.

Actual isolated acceptance uses both published fixtures and the real shared
Pool path. Before/after source bindings record normal PoW profile and exact
compiled inputs. Exercise one real change allocation/send, export through the
new vault path, independently fresh-restore the FILE, synchronize to the exact
owned node hash/height, recover expected outputs and make a controlled spend.
Do not restore into or modify the installed withdrawal journal. Confirm original
root/gym and current cursor inventory with independent old-source derivation.

Then verify schema2 reset through the local signed driver: durable barrier
before one credential mutation, old token/View revocation during blocked work,
same wallet/recipient/retention/checkpoint after fresh login, export/withdrawal
hold refusal, allowed public view and explicit gym-only mining, zero recovery
unwrap during the hold, same-event replay without hold extension, interrupted
begin/completion restart. Use tagged clock advancement only in acceptance for
the fixed hold where needed; normal runtime has no clock override. Run the
normal packaged hosted flag with both fixture controls, actual export/Verify,
restart and genesis/no-implicit-mining checks; also prove ordinary Pool startup
and existing schema1 roots are unchanged. Keep HANDOVER smoke and required
candidate-profile checks, without mining the public seed.

No genuine owner policy choice is required for this slice. Real provider/KMS,
operator-owned production secrets/resources, permanent chain/coin values,
continuous disaster-recovery promises, exclusive self-custody migration and the
joint D74 review remain separate. They are not prerequisites for completing and
qualifying this explicit public-fixture mode.
