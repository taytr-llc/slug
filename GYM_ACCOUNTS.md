# Fixture account authority — change set 27

This is a private, provider-neutral account boundary using two independently
published wallets: `zero256` and `incrementing256`. Their keys and credentials
are fixtures, not protection for real funds. No production identity provider,
real wallet issuance, public account service, production custody or fighter
ownership/transfer system is provided by this package.

## Durable account binding

`gymaccount` creates its own schema-1 LevelDB directory, with an external
`SLUG-ACCOUNTS.json` marker. Both marker and metadata pin the normal Phase-A
network/genesis, fixture recovery profile, schema, authority ID, approved
24-hour hold and exact two-principal configuration. Open never creates a
missing database, migrates a schema, swaps a profile or repairs missing state.

The trusted identity is the exact UTF-8 `(issuer, subject)` pair returned by the
provider adapter. Email, display name and account IDs in request bodies are
not login authority. `Config.Principals` binds each allowed principal to exactly
one of the two public fixture identities. An account ID is random and stable.
Its enrollment permanently binds the full root-public-key/gym-public-key/GymID
identity. Unique principal, root and gym indexes are committed transactionally.
The fixture allowlist is required because a signature by the gym key alone does
not prove that a claimed hardened gym path derives from a root public key.

Login atomically creates a missing account and its first session. Concurrent
logins preserve the same account ID; enrollment fixes one personal gym. Branding
and public directory behavior are not implemented by these cryptographic IDs.

## Enrollment proof

The client obtains a five-minute server-issued `EnrollmentChallenge` and signs
it automatically through the wallet's narrow `SignAccountChallenge` method.
It includes authority, account, exact principal digest, random challenge ID,
authentication generation, action, chain/profile, full public identity and
issue/expiry times. The helper pins the authority ID independently of the
challenge; the wallet checks the retained public identity before and inside its
bounded private-key lease. No arbitrary digest/path signing API is added.

The SHA256 signing domain is `SLUG account enrollment\0v1\0`; signatures are
canonical strict-DER low-S secp256k1 ECDSA. See `gymaccount/challenge.go` and the
standalone Python oracle in `gymaccount/testdata` for the exact byte encoding.
This domain is distinct from prospect-admission authorization. Neither
signature proves PoW, session validity or current account eligibility.

Completion consumes the challenge and stores its immutable result in the same
synchronized batch as the binding. Exact successful replay in the same current
generation returns that result without a second effect, including after the
challenge's expiry. A fresh authorized login may be needed to read it. Another
account, conflicting signature, expired unused challenge or superseded
generation fails. The `recover` action can only re-prove the original identity;
it cannot replace the wallet or gym.

## Session and recovery policy

Login sessions last one hour; sensitive actions require provider authentication
no more than five minutes old. Future authentication times are rejected.
Distinct mining credentials last at most five minutes and never outlive their
parent login. They authorize only mining/admission; they cannot enroll, export,
rotate, mint more credentials or reset the account. Parent logout/rotation and
account reset revoke derived mining credentials as well. Rotation preserves the
original authentication and absolute expiry; it is not fresh login.

Only SHA256 bearer hashes are stored. Returned `Session.Token` is private helper
state, excluded from JSON and formatted projections. It must never enter the
webview, audit, errors, process arguments or logs. A lost login acknowledgment
is resolved with fresh verified login; the store cannot reconstruct a bearer
from its hash. Principal assertions are verified outside the store and never
accepted as caller-selected issuer/subject JSON.

Mike approved reset access to the **same retained wallet without the phrase**,
with this fixed security hold:

- Each new verified reset starts 24 hours and immediately advances the account
  generation, invalidating old sessions and pending challenges.
- During the hold, withdrawals, fighter transfers, recovery phrase/file export,
  recovery-authority changes, marketplace purchases and listings are blocked.
- Viewing and mining into the existing wallet remain available after the new
  login re-proves the original gym identity. No wallet or gym replacement occurs.
- Replay of the same verified provider recovery event returns the original reset
  result without extending the hold. A distinct new verified reset restarts the
  full hold; it never shortens a current one. At the exact hold endpoint the hold
  restriction ends, while normal authentication requirements still apply.

Unsupported transfer/marketplace/withdrawal operations are not implemented just
because their capability names are reserved and held. The fixture adapter is not
a production recovery authority or encryption protocol. Backup export makes a
usable recovery copy; it does not remove hosted recovery authority or establish
exclusive self-custody.

## Protected-operation boundary

`Store.WithSession(ctx, token, capability, fn)` supplies an owned public account
copy and a scoped `recheck` function. It serializes reset/logout/rotation against
the complete protected callback. Call `recheck` after slow PoW or node work, in
the final registry guard immediately before mutation. Keep the lease through
the synchronized registry write, then release it before writing an HTTP response.
The lock order is account then registry. Callbacks must remain bounded, must use
recheck synchronously, and must never reenter the account store.

The final successful guard observation is the authorization linearization point,
analogous to D68 acceptance time; it is not a guarantee that disk synchronization
finishes before the next wall-clock tick. The account store performs no critical
write after callback success. A later session expiry therefore cannot convert
an already committed receipt into a clean rejection. Historical registry receipts
intentionally bypass fresh eligibility checks; they still need current account
access and a final recheck before their unchanged evidence is returned. Logout
or reset never erases existing births or renews a historical receipt's eligibility.

## Storage and failure behavior

Account mutation, indexes, challenge consumption/reset replay record and audit
append use one LevelDB batch with `Sync:true`. A write error poisons the handle;
all further operations fail until close/reopen reconciles whether it committed.
The store retains partial initialization rather than silently replacing it.
The durable clock high-water mark advances before time-sensitive checks,
including expiration of known credentials. Clock rollback cannot revive an
observed-expired credential after reopening.

The 0700 store directory contains a 0600 marker; records are bounded canonical
JSON (maximum 8 KiB). Unknown/duplicate/missing fields, schema/profile changes,
broken indexes, missing parent sessions, inconsistent reset generations/hold,
and missing enrollment/recovery completion evidence fail closed on Open.
The audit is bounded by explicit capacities (at most 4096 sessions, 4096
challenges and 16384 audit events) plus the two accounts. No silent pruning or
capacity migration exists. At audit exhaustion existing protected capabilities
also stop, so capacity cannot leave a usable credential that can no longer be
revoked. Operators must preserve the directory and handle capacity explicitly.

Tests cover concurrent first login, two-account isolation, replay, exact expiry
and 24-hour boundaries, generation/parent revocation, original-key recovery,
reset versus protected-operation serialization, post-work expiry, canceled
operations, before/after-commit injected failures, structural corruption and a
real process kill after synchronized reset acknowledgment. Independent enrollment
vectors are generated without calling the Go implementation.

## Desktop and prospect transport

The set27 shell starts an explicit `--account-fixtures` helper under a new
`LocalAccountPilot` root. Demo labels map to fixed public provider assertions
inside the helper; no issuer, subject, verified flag, event ID, clock or bearer
is accepted from the webview. Account operations are asynchronous and bounded;
Quit cancels/drains them before closing storage. Every child manifest binds the
opaque account ID and allowlisted fixture. An enrolled account requires existing
ready child storage. No missing wallet is silently recreated.

The helper opens the retained fixture wallet and automatically signs enrollment
or recovery through `Wallet.SignAccountChallenge`, pinning the store authority
and exact retained root/gym tuple. Login initialization is separate from Start:
it obtains no job and starts no miner. Separate five-minute mining credentials
are cached privately and renewed only under the current parent login. Login
expiry stops the child worker. Per-child admission authorizers pin AccountID as
well as gym key, preventing use of a valid A session in B's local registry.

`prospectservice.NewSessionServer` explicitly requires v2, empty static Clients
and a SessionAuthorizer. There is no static fallback. Its bounded operation
permit and timeout cover session authentication before body processing. It
reacquires a held session after reading the bounded body and rechecks at the
final job/admission commit guard. Historical receipt recovery checks current
authorization separately. Rate limits follow gym identity across token rotation.

The public snapshot contains account state, fixture label, ID/generation, hold
expiry and export availability, never credentials or private material. Native
recovery export holds its session through wallet work and rechecks before the
atomic file write; custom account roots include authority and all sibling gyms
in path protection. Backup verification emits public progress only and never
replaces the original wallet. Native OS file previews can display the selected
public fixture file; no production secret-reveal security is claimed.

## Explicit signed-identity authority (set 28)

Schema 2 is created and opened only through `CreateIdentity`/`OpenIdentity` with
an `IdentityConfig`; it never upgrades a schema-1 directory. The nested account
policy still permits only the same two published wallet fixtures. The authority
pins the complete public verifier configuration digest and a bounded number of
durable one-use login attempts. Legacy `Login` and `ApplyReset` refuse schema 2.

`IssueLogin` returns an authority-issued nonce and monotonic sequence. The
`gymidentity` verifier checks the signed assertion and obtains that persisted
nonce itself through its `Login` helper. `LoginWithAssertion` consumes the attempt
and inserts one session atomically, checking the exact trusted principal, verifier
pin, original-authentication epoch, pending reset and sequence again. A fresh
assertion issuance time or nonce cannot update an old authentication's epoch.

A signed verified reset begins by atomically advancing the account generation,
revoking existing sessions, storing a pending barrier and reserving completion
audit capacity. All capabilities fail closed while pending. The provider must
wait for this durable acknowledgment before completing its credential change.
A matching signed completion commits the next epoch and fixed 24-hour hold,
preserving the original wallet/gym and requiring its existing recovery proof.
The original verification time and event binding are immutable; replay returns
the same result. Pending state never clears because time elapsed or a process
restarted. A fresh signed transport assertion may reconcile the same old event.

Schema-2 capability checks also fail closed when fewer than two unreserved audit
slots remain, so a session cannot stay usable when a new reset could not be
recorded safely. Acknowledged reset completion retains its reserved slot. Login
attempt lookup persists clock high-water even when rejecting expiry, preventing
a clock rollback from reviving an expired attempt after restart. All records,
nonce consumption, reset sequences, generations and reservations are audited
when reopened; uncertain writes poison the handle until explicit reopen.

The private native test coordinator exercises signed assertions with retained
wallets and normal PoW. The shipped schema-1 desktop default stays unchanged.
There is no browser login flow, deployed identity provider, public endpoint, real
credential issuance, new MFA policy or hosted recovery vault in this component.
A future provider must demonstrate the additional signed authentication-epoch
and reset-ordering guarantees; ordinary OIDC support alone is insufficient.
See `gymidentity/README.md` and `CHANGESET_28_NOTES.md`.
