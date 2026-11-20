# Set71 independent source review — proposed, not frozen

Read-only review, 2026-11-19. No repository source, stored wallet, vault, key,
producer output or test was changed or opened. This file is separate from the
controller agent's HOSTED_CURRENT_RECOVERY draft.

## Conclusion

The milestone is feasible as an explicitly selected, normal-package **published
fixture** route. It can demonstrate schema-2 signed login/reset, the same retained
wallet under a hold, gym-purpose-only signing, and a freshly captured recovery
file that restores real fixture funds. It must not claim a live provider,
continuous disaster recovery, production issuance or recovered withdrawal
journal authority. No new owner decision is needed for the engineering slice.

The serious implementation traps are mixed vault/account lock order, secret
file staging before final authorization, an apparently public wallet helper that
still reads private recovery fields, and enrollment before the local wallet
exists. These are concrete source-backed boundaries, not hypothetical policy.

## Source findings

1. `slugwallet/recoveryvault/store.go:385` requires DefaultAccounts, immutable
   ContentRevision 1 and the same original ManifestHash in both purpose envelopes.
   `store.go:360` derives RetentionID from binding, initial manifest hash and
   content ciphertext/nonce; wrapping metadata is excluded. Current account
   cursors belong in a separate checkpoint, never an in-place envelope/receipt
   edit. Rewrap must leave the checkpoint's retention identity unchanged.
2. `recoveryvault/operations.go:28` and `:289` acquire `v.lock` inside
   WithSession. SignAdmission at `:389` uses these methods. Simply wrapping old
   SignAdmission in a typed hosted adapter does not remove account-to-vault
   waiting. If a new path owns the vault permit before acquiring an account
   fence, allowing legacy calls on that same live object creates the converse
   lock order. Use distinct CreateHosted/OpenHosted constructors, a distinct
   guarded namespace/marker, and an opaque HostedVault facade with no exposed
   legacy Vault. Share private cipher/validation functions, not public legacy
   locking methods. Old constructors must fail before opening the hosted store.
3. `wallet/recovery_export.go:153,179,207` returns public data, but invokes
   Manager.RecoveryData(false) and RecoveryAccounts. Both reach
   `waddrmgr/recovery_state.go:55`, which Gets `slug-recovery-private`.
   RecoveryAccounts also decodes private-key ciphertext fields of account rows.
   A new public-only contract should not promise those reads never happen while
   directly exporting this old helper. Build an explicit schema-9 public path,
   reusing/factoring the bounded public checks from
   `waddrmgr/own_activity.go:37`, with a bounded cursor/inventory audit. Do not
   fetch or decrypt the private recovery envelope, perform unlock/KDF, or copy
   account private ciphertext merely to obtain public cursors.
4. The existing inventory audit at `waddrmgr/recovery_state.go:239` is bounded
   and useful as a semantic reference: exactly four fixed coin-0 scopes,
   exactly default/imported account rows, empty imported material, the v1
   mixed-49 schema, eight cursors <=4096 and fixed 26-byte chained-address rows.
   The new capture must own its four Accounts and read all fields in one
   walletdb read transaction, with cancellation at each bounded iteration.
   No schema-11/12 acceptance or profile-2 inference is needed.
5. `slug/desktop/pilot/pool_runtime_provision.go` refuses a missing wallet once
   account.Enrollment exists. Calling old EnsureEnrollment before provisioning
   therefore cannot initialize a fresh hosted root. Create/verify the wallet
   under the original setup session first, then use the typed vault signer for
   the already-issued enroll/recover challenge, CompleteChallenge, and finally
   publish the combined exact READY binding. Partial setup retains original IDs;
   it never creates a substitute wallet or vault reservation.
6. `slug/gymprovider/login.go:86` never replays session tokens; an uncertain or
   consumed login needs a fresh signed flow with preserved original-auth rules.
   `reset.go:106` retains exact semantic provider operation/event identity and
   distinguishes pending/completed barriers. Use these concrete APIs. Reset's
   durable BeginReset barrier must precede child Stop/Close waits and provider
   completion; after an uncertain response recover the same event, not a new
   reset that restarts the hold.

## Narrow public capture and provenance API

Names below are suggestions to the draft author, not frozen declarations.

- `Wallet.CaptureCurrentRecovery(ctx) (*CurrentRecoveryCapture, error)` returns
  an opaque owned capture from the concrete, open installed schema-9 wallet.
  It may expose an owned public Manifest and a capture time for status, but no
  exported constructor that accepts a claimed Manifest. The type retains its
  originating wallet pointer and can check exact owner identity; a serialized
  Manifest or equal root key alone cannot be an authorization capability.
- The hosted runtime creates a typed current-source binding only from its
  retained wallet owner and immutable mode/account/installed-wallet marker.
  Vault methods accept that concrete source/capture capability and verify its
  exact retained owner, authority/account, root/gym, profile and RetentionID.
  Do not accept a webview-selected path, arbitrary callback, exported Boolean,
  another live wallet with the same fixture root, or an inspected disk marker
  as current-source authority.
- Wallet closure/capture cancellation is joined by the original child owner;
  no second loader opens the installed wallet. Reopen creates a new ephemeral
  source capability from the same established immutable runtime binding.
  A source handle from the former wallet object is invalid after close.
- Initial original View authorization precedes capture; a second original-token
  and generation recheck gates checkpoint publication. RecoveryExport is
  separately required before any recovery unwrap and at final file publication.
  A held View can capture public metadata and use the gym purpose; it cannot
  convert a checkpoint into export permission.

## Checkpoint storage and replay

Keep checkpoint accounting separate from the old vault's provisioning/rewrap
MaxOperations counter. A concrete candidate choice is at most 128 retained
checkpoint actions per two-account hosted root, four Accounts/eight cursors per
record, uint32 revision with overflow refusal, and a fixed independently sized
record/store byte cap. Freeze the exact count/byte budget before implementation.

The record must bind hosted-store identity, account authority, account and
principal, immutable vault Binding/RetentionID, explicit installed-wallet source
identity, original profile/chain/root/gym, exact canonical Manifest, action ID,
expected prior revision and internally sampled capture/publication timestamps.
No session token or private material belongs in it. Revisions and clock
highwater never regress; each cursor is >= the previous cursor and <=4096.
All other manifest fields are identical to the immutable retained profile.

Use one exact prepared transition and synchronous atomic batch for checkpoint,
revision, operation index and clock. Pre-encode/hash/validate outside the account
lease. Acquire the hosted permit outside the account lease and retain it through
its short final CAS/write fence; no provider, wallet read, RNG, encoding or lock
acquisition occurs inside. Poison on uncertain writes and require exact reopen
reconciliation. A retry with the same action ID returns the already-committed
exact result; a different intent under that ID conflicts. Do not regenerate a
capture/time and silently call it the same action after an acknowledgment loss.

A new source capture whose cursors are unchanged can either create a new bounded
dated checkpoint action or return an unchanged content checkpoint plus a separate
fresh capture observation. Choose one explicitly. Do not retrospectively change
the old checkpoint timestamp merely to make it look current.

State/marker audit must detect missing established checkpoint state and dangling
operation references. No claim of protection against a coherent hostile rollback
of the entire filesystem is justified. Deleted established state is not a fresh
empty vault; initialization resumes only the exact retained INIT identities.

## Secret publication seam

A neutral relocation of all desktop/internal/recoveryfile APIs is unnecessary.
Add a small vault-owned **create-only** `CurrentDestination` owner. It accepts
only the public fields of the already native-captured CreateMode Pin plus trusted
protected roots, opens the canonical selected parent, verifies the exact parent
identity, rejects existing/symlink/managed destinations, and retains the concrete
os.Root/descriptor until completion. The native process continues holding its
original Selection through the helper acknowledgment. Existing read/verify APIs
and all old vault exports remain unchanged.

An opaque `PreparedCurrentExport` retains <=64KiB encoded recovery bytes in
memory; it exposes no Bytes, WriteTo, seed-returning method or generic plaintext
callback. It is bound to the exact hosted owner, source/capture, checkpoint,
original session generation and destination capability. Preparation unwraps only
the recovery-purpose envelope, validates its immutable base manifest/entropy,
then constructs the unchanged v1 export using the captured current Accounts.
All crypto, encoding, RNG and permit acquisition happen outside account leases.

**Do not stage plaintext in the user-selected directory before the final
RecoveryExport fence.** That could expose the words during a reset even if the
final link is refused. The public-evidence staging precedent is not suitable.
Prefer the established `desktop/internal/recoveryfile/io.go:86` private-file
pattern: under the short original RecoveryExport fence perform only the bounded
<=64KiB temp create/write/sync, exclusive link and directory sync plus identity
rechecks. The fence carries no plaintext through a callback argument; the
prepared vault owner writes its own private bytes. Temp-name entropy is prepared
before the fence. Clear memory on all terminal paths and join cleanup.

After any possibly successful link, cancellation, postcheck failure or directory
sync failure is an explicit publication-unknown result; retain the destination
for inspection and never auto-retry into a new path. Failed cleanup retains its
owner and directory. Native protocol carries public operation/status/checksum
only, never encoded file bytes or mnemonic.

## Normal-package fixture route

An explicit `--candidate-hosted-pool` (name subject to root review) can use the
existing fixture Login/Reset controls without navigation changes. It selects a
new dedicated root/marker/application identifier, schema-2 authority, exact
provider verifier/coordinator and purpose-separated published-fixture wrapper.
The schema-1 Pool default remains unchanged. Neither mode adopts the other
mode's root; mismatch refuses before authority/wallet mutation. Distinct root
and mode markers must bind source descriptor and all random authorities before
opening established state. Capture/recovery storage is not inferred from disk.

A local fixture evidence driver may use the already documented public RFC7515
fixture RSA key and published wrapping keys. Freeze its exact verifier key IDs,
issuer/audience/tenant/client/connection, fixture-subject mapping, sizes and
journal limits. It signs only the concrete coordinator-issued nonce/flow/event
for the selected published fixture; no arbitrary claims/epochs/key paths enter
from the UI. It does not hand unsigned Verified objects to account authority.
Credential epoch/provider operation completion must be durable and replay-safe,
so a crash cannot sign a fresh epoch inconsistent with the retained reset event.
Raw assertions and ephemeral tokens stay out of public protocol/logs.

The typed hosted signer must not fall back to local wallet signing if the vault
is unavailable. Provisioning, enrollment recovery and mining signing use the
correct purpose and original epoch; count unwraps under held flows. The local
fixture issuer is not a live provider or a meaningful authentication boundary
for people who know its published key. The UI/package evidence must say so.

## Finite qualification and milestone limits

Freeze independent original-profile/manifest/checkpoint byte inputs before new
outputs. Include advancing native84 internal cursors from actual committed
change (including abandoned/dust allocation), every unchanged scope, 4096 edges,
regression, wrong owner/account/retention and exact duplicate/CAS outcomes.

Qualify normal packaged opt-in Login/Reset/held View/gym-only signing/export hold,
separate schema-1 behavior, exact restart and original-view cleanup. Inject reset
while wallet capture/provider unwrap/file preparation is blocked; the barrier
must commit before joins and old results must not escape. Test mixed namespace
refusal before creating wallet state, missing established sidecar, uncertain
commit, exact retry, post-link uncertainty and sticky cleanup failure.

Actual acceptance must allocate real change in isolated fixture wallets, export
through the concrete vault route, and restore the file through the unchanged
bounded Phase-A verifier to exact root/gym/cursors/funds and a controlled spend.
An old file or unavailable installed wallet cannot silently claim fresh current
metadata. Checkpoints describe a dated capture, not continuous synchronization
with later allocations. Restoring the v1 file does not restore the withdrawal
journal or its permanent reservations; it must never install/replace a spendable
child or discard paired payment authority as part of this milestone.
