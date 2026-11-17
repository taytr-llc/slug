# Provider Action conformance fixture

This is a **test-only model**, not an Auth0 tenant configuration, deployable
Action, HTTP adapter, JWT signer or production-provider approval. It uses only
Node built-ins and public synthetic identifiers. No provider API, secret store,
password, wallet or network connection is accessed.

Run from the chain repository:

```sh
node --test gymprovider/testdata/provider-actions/harness.test.mjs
```

The exact Go signed-evidence vectors and coordinator crash tests are separate.
This harness tests the JavaScript integration requirements around that boundary;
it does not substitute unsigned JavaScript objects for signed `gymprovider`
inputs. `resolver` and `coordinator` are explicitly injected fixture ports,
**not methods offered by Auth0**. The test coordinator models receipts; it is
not the real Go coordinator. The real normal-PoW composition is tested elsewhere.

## What the executable model checks

- `onExecutePostChallenge` allows continuation only after an exact pending
  `CanCommit` acknowledgement and a durable permit linked to the same operation.
  Foreign or completed receipts, ambiguous/missing correlation, unavailable
  storage and lost acknowledgements deny continuation. The provider fixture
  consumes a permit at most once for that immutable operation; it never turns
  a historical acknowledgement into another password change.
- `onExecutePostChangePassword` resolves that **committed** operation, including
  its pinned client. It does not select the newest pending reset for a subject.
  Completion failure reports reconciliation required; it cannot undo a
  password that the provider already changed. Retry retains the same event,
  verification time and operation.
- `onExecutePostLogin` requires a durable original-authentication mapping.
  A refresh token identifier can locate that mapping but cannot supply new
  authentication time or today's credential epoch. Unknown origins, modified
  epochs/times and refresh flows labelled fresh fail. Body/user metadata is
  never an authority source. The disk-backed test resolver is reopened to
  exercise retained mappings, including an old refresh after an epoch change.
- One deadline covers all resolver/transport/permit calls, with a maximum of
  18 seconds. Tests shorten it to 10–25 milliseconds and check abort and late
  responses. There is no retry loop in an Action. Abort cannot undo a remote
  commit, so loss requires same-operation reconciliation. JavaScript timers
  cannot preempt synchronous CPU work; the injected calls must be bounded and
  asynchronous in any eventual real integration.
- Epochs remain BigInt internally and canonical raw JSON integer tokens when
  serialized. Tests include 2^53+1, MaxUint64, overflow and Number-first loss.
  Number coercion, decimal/exponent/quoted integers and unknown claim fields
  fail. `parseEpochToken` expects an already isolated raw token: it is not a
  permissive whole-JSON decoder. An eventual wire decoder must preserve raw
  integer tokens before any Number conversion. No Auth0 `setCustomClaim`
  behavior for BigInt or raw JSON is assumed or demonstrated here.
- Failures expose static messages only. Credentials, event bodies, provider
  error text and signed evidence are not logged or returned.

The fixture resolver persists synchronized temporary files plus directory
renames. Its small local journal models required provider behavior; it is not
an implementation of Auth0 persistence or a claim about distributed atomicity.
In particular, its operation commit and permit methods are requirements on the
future provider integration, not extra undocumented Auth0 APIs. The production
Go verifier still independently checks every signed binding and the authority
checks pending state, epoch, nonce, freshness and the fixed 24-hour hold.

## Official shape references checked 2026-11-16

The pre-reset handler is `onExecutePostChallenge(event, api)`. Auth0 documents
this as a blocking Universal Login hook after the first challenge and before
setting a new password; it does not cover Classic Login. The fixture requests
no additional MFA and makes no new assurance-policy decision.
[Trigger documentation](https://auth0.com/docs/customize/actions/explore-triggers/password-reset-post-challenge)

`api.access.deny(reason)` stops the reset flow after the Action finishes; it
does not cancel other side effects already requested. The harness therefore
uses a static denial and retains uncertain durable state.
[Pre-reset API](https://auth0.com/docs/actions/reference/password-reset-post-challenge/password-reset-post-challenge-api-object)

Pre-reset events expose `tenant.id`, `connection.id`, `client.client_id`,
`user.user_id` and optional `transaction.correlation_id`. That correlation can
be supplied by the application; it is only a lookup hint, not authenticated
proof of a unique provider operation.
[Pre-reset event](https://auth0.com/docs/actions/reference/password-reset-post-challenge/password-reset-post-challenge-event-object)

`onExecutePostChangePassword` is asynchronous and its outcome does not change
the provider transaction. The documented event includes tenant, connection,
user and optional correlation, but no client object. The fixture requires the
durable exact mapping to recover the pinned client instead of assuming one is
present or matching a subject's latest reset.
[Post-change trigger](https://auth0.com/docs/customize/actions/explore-triggers/post-change-password),
[post-change event](https://auth0.com/docs/actions/reference/post-change-password/post-change-password-event-object)

Post-login has optional transaction/session information and refresh-token
information, including `transaction.protocol === 'oauth2-refresh-token'` and
optional `refresh_token.id`. The documentation marks refresh-token details as
Enterprise features. Neither presence nor absence proves a new authentication.
The fixture requires a configured durable resolver and refuses missing links.
[Post-login event](https://auth0.com/docs/actions/reference/post-login/post-login-event-object)

Auth0 documents a 20-second execution limit and warns that Action cache is not
reliable across executions. Its transaction metadata is not durable beyond the
authentication trigger. The model uses an 18-second ceiling for external work
and does not use either storage mechanism for authority records.
[Action limitations](https://auth0.com/docs/customize/actions/limitations)

## Explicit remaining provider gates

These tests do **not** establish real Auth0 route coverage, trustworthy stable
operation correlation, original-authentication/epoch provenance, atomic permit
consumption, reliable post-change delivery, abandoned-flow reconciliation or
resume guarantees. The documented optional correlation fields alone do not
solve those requirements. No timeout abort/unlock, fabricated completion,
provider selection, real signing key, MFA policy, production profile or public
release authorization is introduced. Those remain the gates in the set31
provider plan and the required joint product-design review with Mike.
