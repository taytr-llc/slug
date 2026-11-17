# Fixture provider coordination — change set 31

The `gymprovider` package keeps a durable journal for signed fixture login and
password-reset evidence. `gymaccount` remains the account authority. The
coordinator neither re-signs assertions nor exposes an unsigned claims endpoint.
This protocol is implemented for the two published fixtures; it is not enabled
in the desktop login screen or deployed as a hosted authentication service.

## Scope and dependency boundary

`gymprovider` lives in the chain repository, with narrow typed verification
methods in `gymidentity`. The coordinator owns a durable protocol journal;
`gymaccount` schema 2 continues to own accounts, credential epochs, session
generation, pending barriers and the 24-hour hold. No wallet import is added to
the chain workspace. Cross-repository acceptance composes the coordinator with
the existing fixture vault and normal-PoW worker.

Only the existing two configured public fixture principals/wallets are supported.
Issuer is exactly `https://identity.slug.invalid`; provider kind is exactly
`published-fixture-v1`; the public key is the existing published RFC7515 A.2
fixture. No production issuer, discovery, live HTTP endpoint, password, code,
provider resource, real signing key, entropy source for wallets, or new custody
policy is added. Runtime randomness remains appropriate for nonsecret journal
IDs/nonces. Fixture private RSA material stays in test support, not Config.

The exact accepted verifier profile uses audiences
`slug-provider-fixture-login` and `slug-provider-fixture-reset`, epoch claim
`slug_auth_epoch`, and the published RSA public JWK with kid `rfc7515-a2`.
Coordinator creation derives that expected ConfigDigest from an embedded
public-only fixture JWK and compares it to both the supplied verifier and
AuthorityInfo. A caller-selected digest or different real key cannot expand
this allowlist. No private JWK is embedded in coordinator runtime.

D48/D70/D72 and set 29's immutable recovery snapshot remain unchanged. No MFA,
manual recovery, pending-reset timeout override, production profile or provider
compatibility is implied. The Auth0-shaped fixture tests demonstrate our required
contract, not that Auth0 implements every part of it.

## Public coordinator API

```go
type Config struct {
    Version uint32 // exactly 1
    Provider string // exactly "published-fixture-v1"
    Issuer string // exactly "https://identity.slug.invalid"
    TenantID, ClientID, ConnectionID string
    AuthorityID, VerifierConfigHash string
    MaxLoginFlows, MaxResetFlows uint32
}

Create(ctx context.Context, path string, cfg Config,
    authority *gymaccount.Store, verifier *gymidentity.Verifier) (*Coordinator, error)
Open(ctx context.Context, path string, cfg Config,
    authority *gymaccount.Store, verifier *gymidentity.Verifier) (*Coordinator, error)
(*Coordinator).IssueLogin(ctx context.Context) (LoginFlow, error)
(*Coordinator).FinishLogin(ctx context.Context, flowID, signedEvidence string) (gymaccount.Session, error)
(*Coordinator).IssueReset(ctx context.Context) (ResetRequest, error)
(*Coordinator).BeginReset(ctx context.Context, eventID, signedEvidence string) (ResetResult, error)
(*Coordinator).CompleteReset(ctx context.Context, eventID, signedEvidence string) (ResetResult, error)
(*Coordinator).Status(ctx context.Context, kind, id string) (FlowStatus, error)
(*Coordinator).Close() error
```

These are private helper/service APIs. `Session.Token` remains a private return
value, never JSON IPC, logs or journal content. Status returns sanitized phase,
IDs and uncertainty only, without assertions, tokens, nonces, subject strings,
or wallet material. There is no generic `Reconcile` mutation: replay the same
BeginReset or CompleteReset method with fresh signed transport for the same
immutable event. A distinct operation must get a new request.

`LoginFlow` contains Version, CoordinatorID, AuthorityID, ID, AttemptID, Issuer,
Nonce, Sequence, IssuedAtMS and ExpiresAtMS. Sequence/times/nonce come from the
authority's persisted LoginAttempt; ID is a distinct coordinator-issued ID.
`ResetRequest` contains Version, CoordinatorID, AuthorityID, EventID, Nonce,
IssuedAtMS and ExpiresAtMS. Its event ID and nonce are generated before any
provider operation; issuance alone does not affect an account.

`ResetResult` contains Version, CoordinatorID, AuthorityID, EventID,
ProviderOperationID, Phase (`pending` or `completed`), CanCommit bool, and the
original `gymaccount.ResetBarrier`. CanCommit is true only for an acknowledged
currently pending begin; it is always false for completion/historical results.
This private result is an operation-bound acknowledgement, not a reusable bearer
credential or proof that the provider has changed a password.

Sanitized errors: ErrConfig, ErrEvidence, ErrNotFound, ErrConflict,
ErrExpired, ErrCapacity, ErrUnavailable, ErrConsumed, ErrOutcomeUnknown,
ErrCorrupt, ErrClosed. Context cancellation is preserved. Never echo JWT bytes,
provider bodies, subject identifiers or filesystem contents in errors.

## Exact signed evidence and verification

`gymidentity` uses typed methods and its existing bounded JWS parser,
RS256 verification, canonical numbers/base64, issuer/audience/time validation and
verifier digest. Do not add a second JWT implementation or export arbitrary
verified claims maps. Existing VerifyLogin/VerifyReset behavior stays unchanged.

```go
type CoordinatorBinding struct {
    Version uint32
    CoordinatorID, AuthorityID string
    TenantID, ClientID, ConnectionID string
}
type CoordinatedLogin struct {
    Login gymaccount.VerifiedLogin
    FlowID, OriginalAuthID, OriginalFlowID, Mode string
}
type CoordinatedReset struct {
    Reset gymaccount.VerifiedReset
    ProviderOperationID, ResetNonce string
}
(*Verifier).VerifyCoordinatedLogin(ctx context.Context, token string,
    binding CoordinatorBinding, flowID, nonce string) (CoordinatedLogin, error)
(*Verifier).VerifyCoordinatedReset(ctx context.Context, token string,
    binding CoordinatorBinding, eventID, nonce string) (CoordinatedReset, error)
```

Binding and expected IDs/nonces are read from immutable Config/marker and durable
flow records, never copied from unverified claims. Results are verifier outputs,
not public request DTOs. The nested Login/Reset includes ConfigDigest set locally
by the verifier. There are no unsigned tuple enrichments after verification.

Both coordinated profiles require exact protected header fields `alg`, `kid`,
`typ`. Login requires typ `JWT`; reset requires `slug-reset+jwt`. Config keeps
the existing distinct login/reset audiences; reset stage supplies the additional
begin/complete action binding. Require a single string audience and no `azp`,
`nbf` or unknown claims in this narrow version, while existing general verifier
methods retain their previous acceptance rules.

Exact login payload fields:

```
iss aud sub iat exp auth_time nonce slug_auth_epoch
provider_version coordinator_id authority_id
tenant_id client_id connection_id
login_flow_id original_auth_id original_flow_id auth_mode
```

Exact reset payload fields:

```
iss aud sub iat exp event_id stage from_epoch to_epoch verified_at
provider_version coordinator_id authority_id
tenant_id client_id connection_id reset_nonce provider_operation_id
```

All fields are required and signed. `provider_version` is integer 1. IDs/nonces
are exactly 64 lowercase hex characters (32 bytes); all-zero encodings are valid
shapes, and randomness/collision checks belong to issuance. Tenant/client/
connection are bounded ASCII identifiers matching Config exactly. `auth_mode`
is exactly `fresh` or `reuse`. Original authentication and provider operation
IDs are independently issued by the fixture provider, stable across its retries.
They are not invented by the coordinator on each callback.

Times are positive integral JWT seconds, checked before conversion to int64
milliseconds. Epochs are canonical JSON integers 1..MaxUint64; no fractional,
exponent, string or float64 coercion. JS fixtures must preserve values above
2^53 with exact serialization, never Number arithmetic. Existing 16 KiB total,
2 KiB header, 8 KiB payload, maximum 300-second transport lifetime and original
authentication freshness apply. Fresh transport does not refresh auth_time or
verified_at. Reject nulls, duplicate decoded keys, case aliases and invalid UTF8.

The evidence itself is already the internal signed assertion accepted by
gymidentity. The coordinator never re-signs it. After checking journal bindings,
it passes only the verifier-produced nested Login/Reset to schema 2 methods.
No network/IPC handler may accept caller-constructed trusted output structs.

## Original authentication identity and epoch

An original-authentication key is the tuple:

```
(verifier config digest, issuer, subject, tenant, client, connection,
 original_auth_id)
```

The immutable record also binds original_flow_id, the original authority attempt
ID and sequence, CredentialEpoch, AuthenticatedAtMS and its state. Its stable
database key is SHA256 of a versioned domain and explicit length-prefixed UTF8
tuple components; do not concatenate variable strings ambiguously. Use the
same canonical tuple encoding for provider-operation uniqueness below.

Exact index encoding: ASCII `SLUG provider original authentication\x00v1\x00`
(or `SLUG provider reset operation\x00v1\x00` for operation indices), followed
by the listed tuple components in order, each as LE32 byte length then UTF8.
Canonical hex components remain their lowercase ASCII form in this encoding.
This index is a deterministic public binding, not a signing digest.

For `fresh`, original_flow_id must equal the current login_flow_id. The origin
must be new, or an exact retry under that same originating flow. Authentication
time must be at or after the issued attempt's floor-second time. The fixture
provider promises an actual new authentication for this flow; a real provider
must later demonstrate how it proves that assertion. No “recent method” or iat
heuristic is implemented as a substitute.

For `reuse`, the record must already be `accepted`. Every immutable field,
principal, epoch and authentication time must match; original_flow_id must match
the recorded initial flow. The new authority nonce binds the current login, but
does not make the original authentication newer. An unknown origin, an origin
still only prepared/submitting after an uncertain outcome, or a `fresh` assertion
attempting to relabel an existing origin under another flow fails closed. The provider cannot
look up today's epoch and attach it to an old session or refresh grant.

The account store finally checks current epoch, no pending reset, attempt issue
sequence after the last reset completion, nonce and 300-second freshness. The
coordinator does not mirror or independently advance the account epoch. Old
accepted origins remain historical records after reset, and fail the authority's
epoch check. No deletion/rebinding allows the same origin to become current.

## Login states and deliberately unrecoverable credentials

State progression: `reserved → issued → submitting → completed`.
`reserved` rows remain retained after restart; Status exposes them as
`unavailable`. A failed/uncertain submitted login can become
`consumed_or_unknown`, with no token returned.

1. Persist and sync the coordinator flow reservation before IssueLogin. Call
   authority.IssueLogin once. Link the returned attempt and nonce durably before
   returning LoginFlow. A crash between the authority write and this link can
   orphan an attempt. Never silently invoke IssueLogin again for that flow.
   Require a new flow, retain the unavailable reservation and account for the
   bounded leak. No account API extension is needed to hide it.
2. Verify evidence, read the authority's still-valid attempt, and validate the
   immutable origin. Persist `submitting`, the semantic evidence binding and a
   new origin as `prepared` before calling LoginWithAssertion. Raw JWTs are not
   retained. Retry must present fresh transport with the same semantic binding.
3. On successful authority return, durably mark completed and origin accepted
   before returning the session. Persist only public account ID/generation and
   semantic receipt data, never Token. If the final journal write fails, return
   no token, attempt bounded Logout of the known token, and poison uncertain
   journal state; cleanup failure must not turn this into a success.
4. On restart in submitting, GetLoginAttempt either permits a retry of the same
   unconsumed attempt or refuses it. A consumed/unknown attempt yields no session
   reconstruction. Even a completed flow never replays a token. A new explicit
   login flow is required. An accepted origin may be reused only within its
   unchanged freshness window; a still-prepared uncertain fresh origin requires
   a new actual authentication/ID. A durably completed/accepted origin remains reusable even
   if the caller lost the original response, within its original freshness. The
   coordinator does not custody session secrets to make response replay convenient.

This honest lost-ACK behavior matches the existing authority, which stores a
token hash rather than a recoverable token. Completed flow status is not a login
credential. The account store remains the deciding check if reset races between
verification, journal writes and session creation.

## Reset identity, state and receipt replay

Provider-operation uniqueness key:

```
(verifier config digest, issuer, subject, tenant, client, connection,
 provider_operation_id)
```

Bind it permanently to one coordinator-issued EventID before any authority call.
The same authenticated operation presented with another issued handle is a
conflict, not a new reset. Conversely a handle cannot be rebound to another
principal or operation. A subject/time window or “latest pending” lookup is
never a correlation key. The provider fixture enforces that one operation can
commit at most once and that retries preserve its ID; the coordinator alone
cannot make an uncooperative provider satisfy this rule.

State progression: `issued → begin_prepared → pending → complete_prepared → completed`.
All later states retain the original immutable event/nonce, provider operation,
principal, from/to epochs, verified_at, trust digest and original receipts.

1. IssueReset syncs a 300-second unbound request. It cannot freeze an account.
2. A valid signed begin binds the request and the unique provider operation;
   require fresh verification for a previously unbound request. Reserve all
   journal capacity needed through completion and sync begin_prepared. Call
   authority.BeginReset with verified output, then persist its exact receipt.
   Only a pending authority receipt plus a successful journal sync yields
   CanCommit=true. If the authority returns a historical completed receipt,
   return completed/CanCommit=false; never authorize another provider mutation.
3. A lost begin acknowledgement is reconciled with fresh signed begin transport
   for the exact immutable event. If it reached the authority, replay returns
   the original barrier even after the initial request expired. If it never
   committed and its verified_at became too old, the authority refuses creation;
   no permit is returned. The provider must perform a new verified operation to
   get a different event. Do not change the original event's verification time.
4. Complete requires a signed success assertion for the same provider operation,
   event, nonce, subject, epochs and verified_at. It is accepted only after the
   journal has an acknowledged pending begin. Persist complete_prepared, call
   authority.CompleteReset, and sync the exact completed receipt before returning
   success. No new epoch is assigned by the coordinator.
5. After a crash, a fresh completion assertion resumes the same event. Completed
   replay returns the original receipt, including its original hold, even after
   a later reset. An old begin returns completed/CanCommit=false. Different reset
   operations receive new event IDs and must satisfy the authority's epoch CAS.

`gymaccount.ResetBarrier.EventID` is a principal-derived hash, not the raw
provider/coordinator EventID. Persist both under different field names and test
their distinction. Validate account/receipt bindings before persisting them;
never compare the raw event ID directly to Barrier.EventID.

No abandoned-flow abort, automatic expiry unlock, manual override or synthetic
completion is added. An acknowledged begin whose provider outcome is unknown
stays pending. A fixture can resume its same immutable operation; a real adapter
will need to demonstrate equivalent recovery before it is supported.

## Durability, bounds and lifecycle

The API accepts typed Config; it adds no external JSON configuration parser.
Persisted markers and rows use bounded canonical JSON, rejecting unknown,
duplicate, null or case-alias forms. Version/provider/issuer are fixed above;
TenantID/ClientID/ConnectionID are 1..128 ASCII letters/digits/`._-`, with no
normalization. MaxLoginFlows 1..4096, MaxResetFlows 1..256; every reservation counts,
including abandoned/uncertain ones. Original-auth records cannot exceed the
login-flow limit; operation mappings cannot exceed the reset limit. No eviction
of bindings, recycling IDs, pruning replay history or silent quota extension.

Marker pins Config, generated CoordinatorID, AuthorityInfo including chain/
profile, and verifier digest. Open requires exact normalized configuration and
the same schema 2 authority ID; copying the journal beside another authority
fails. Config trust is fixed; no refresh, discovery, private JWK or key rotation
is introduced. Create is new-directory-only. Missing established marker/DB,
inconsistent rows, truncated files or swapped bindings fail closed.

Use the existing LevelDB dependency, exclusive process ownership, sync batches,
bounded 16 KiB rows and explicit durable counters. Reserve mutation capacity at
the first irreversible transition. Unknown writes poison until reopen and full
audit; never acknowledge a journal mutation whose durability is uncertain.
Clock highwater is durable even on rejected expiry checks; no time rollback can
revive an attempt. Sequence/epoch overflow fails closed.

One context-aware coordinator gate serializes its local operations. It may call
bounded local authority methods while holding that gate, but no authority
callback may reenter the coordinator. No provider/network call, wallet operation
or private crypto runs under either database gate. Verify signed evidence before
flow/account transitions. Clock observations synchronize around verification, including rejected evidence, and the authority
rechecks mutable clock/epoch at commit. Close
stops new operations and joins accepted work before closing its own DB; it does
not close the borrowed authority/verifier. Cancellation never fabricates an
acknowledgement for an uncertain commit.

Status exposes only the caller-named opaque flow and sanitized phase; initial
set 31 has no public status endpoint. Errors and test artifacts contain no
password, recovery code, raw assertion, session token or wallet material.

## Verification and remaining provider gates

- An independent standard-library Python signer, using the already published RSA
  key, supplies exact coordinated JWT vectors. Cover all signed fields, cross-
  coordinator/authority/tenant/client/connection/subject substitutions, duplicate
  keys, malformed numbers, values above 2^53, both types/audiences and expiry.
- Persisted auth-origin tests cover fresh success, exact retry, reuse, unknown
  origin, changed epoch/time/principal, stale refresh, a pre-reset flow finishing
  afterward, and refusal to promote an uncertain origin.
- Real subprocess-kill tests stop after flow reservation, authority attempt
  issuance, submitting, authority login commit, begin_prepared, authority begin,
  complete_prepared and authority complete. Assert bounded orphan accounting,
  no replayed session token, same reset receipt/generation/hold and no permit for
  completed events. Inject unknown writes, capacity exhaustion and Close races.
- Provider fixture has its own durable original-auth and reset-operation state,
  separate from the coordinator journal. It consumes each permit for only its
  bound operation and can produce fresh transport for that same event. Tests
  duplicate an operation across two server handles and reorder completion;
  neither may become a different reset or match the latest pending subject.
- Cross-repository normal-PoW acceptance uses both published wallets and signed
  coordinated evidence. Prove enrollment/vault identity unchanged through
  pending restart, completion, fresh epoch login and gym recovery; old sessions
  and stale-auth tokens fail; held mining uses only the gym envelope; recovery
  export/rewrap remain blocked without recovery unwrap; original fighter birth
  and historical receipt remain unchanged. No local signer fallback.
- Existing gymidentity vectors, gymaccount schema1/schema 2 tests, vault fixtures
  and desktop default runtime remain passing and unchanged. Cheap-PoW tests may
  exercise pure codecs/journals; actual wallet/worker admission still refuses the
  cheap production profile. No full unchanged wallet race rerun is required
  solely for this coordinator.

The set is complete when the fixture contract, crash tests and real normal-PoW
composition pass with frozen source evidence. It does not complete real-provider
selection or prove a real tenant's pre-commit, correlation or original-auth
guarantees. Those remain explicit gates before real-provider integration.

## Test entry points

From the chain root, run `go test ./gymidentity ./gymprovider` and their race
and `slugtestpow` variants. The independent signed corpus is reproduced by
`python3 gymidentity/testdata/coordinated/reference_coordinator.py --check`. The
coordinator tests use real subprocess termination at durable transition points.

The Auth0-shaped JavaScript harness under
`gymprovider/testdata/provider-actions` is test support with its own README. It
models blocking pre-change acknowledgement and an asynchronous completion
callback. Optional application correlation fields alone do not establish a
trusted provider operation. No successful fixture test establishes coverage of
real administrator changes, abandoned resets, original-authentication metadata
or a live tenant's provider behavior.

The desktop composition test is
`TestProviderCoordinatorVaultMiningNormalPoW`, under the `desktopintegration`
build tag in the separate desktop workspace. Set `SLUG_DESKTOP_NODE` to an
immutable normal-PoW binary and `SLUG_ACCOUNT_TEST_ARTIFACTS` to a fresh private
artifact directory. Its fixture provider keeps a separate synced state file,
commits its simulated credential change only after the exact pending barrier,
and retains the original operation through restart and lost completion. Both
wallet fixtures export and restore before mining, then recover the same gym
and continue mining under the hold using only the gym envelope. The journal,
provider state and test reports do not retain JWTs or session tokens.

The permanent genesis, final wallet allocation, real authentication provider,
production custody, product UI, and D74 joint design review remain separate
launch gates in `LAUNCH_READINESS.md`.
