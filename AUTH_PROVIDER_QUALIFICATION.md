# Set84 authentication provider qualification

Status: evaluation in progress, November 21, 2026. No provider selected,
tenant provisioned, credentials registered or real-login acceptance claimed.
D82 and [D85](ACCOUNT_FACTOR_RECOVERY.md) are settled product policy.

## Decision and constraints

Evaluate Auth0 Universal Login first, subject to the checks below. Its documented
blocking reset extension is a concrete starting point, not proof of compatibility.
Keep the existing fixture provider strict. Do not replace it with configuration
that assumes a real provider supplies the fixture's signed operation identities.

The first useful delivery is one real system-browser authentication leading to
the same bound isolated test wallet while mining remains stopped, then sign-out,
reopen and refusal of the old session. Full enrollment/recovery follows through
that same authority. A new real principal must not be mapped to an old fixture
wallet by email. Production wallet issuance remains a separate gate.

Existing `gymprovider/reset.go` requires durable BeginReset before credential
completion and reconciles the exact original operation after uncertainty.
`gymidentity/coordinated.go` accepts a closed fixture assertion with original
authentication/epoch facts, but no MFA method or passkey user-verification proof.
The real path needs a separately versioned assurance contract and independent
reference inputs before implementation; widening the old claim parser is not it.

## Options and evidence

### Auth0 Universal Login: first qualification candidate

Auth0 documents a synchronous post-challenge hook before the new password is
set. It supports additional MFA and external verification, and requires Universal
Login. This gives us a possible place to obtain the SLUG barrier before allowing
the reset to continue. [Trigger documentation](https://auth0.com/docs/customize/actions/explore-triggers/password-reset-post-challenge).

Its separate post-change-password hook is asynchronous and cannot block the
credential change. Using that hook alone would violate SLUG's existing ordering.
[Post-change documentation](https://auth0.com/docs/customize/actions/explore-triggers/post-change-password).

The reset event lists optional request correlation and application state; those
fields alone do not prove a committed credential operation. The API's result
redirect is browser navigation, not an authenticated durable completion receipt.
These are our integration conclusions, not claims that Auth0 cannot supply a
solution. [Reset event](https://auth0.com/docs/actions/reference/password-reset-post-challenge/password-reset-post-challenge-event-object),
[reset API](https://auth0.com/docs/actions/reference/password-reset-post-challenge/password-reset-post-challenge-api-object).

Qualification remains open for exact lost-response reconciliation, factor
replacement, D85 two-proof recovery, method/UV evidence and every enabled
administrative/API route. Do not assume the password hook governs all of them.
Costs and required plan have not been selected or approved.

### WorkOS AuthKit: alternative, not qualified

Required MFA supports authenticator codes and user-verified passkeys. Its current
documentation explicitly says there is no built-in per-session MFA completion
claim; the initial authentication method and timestamps are insufficient. Hosted
flow enforcement can supply a narrower guarantee, but a SLUG integration must
retain its exact verified meaning and rule out other session sources. No reset
ordering or D85 recovery qualification has been performed. [MFA documentation](https://workos.com/docs/authkit/mfa).

This may reduce hosted-login implementation work, but neither a dashboard setting
nor a locally invented `mfa=true` replaces the required trusted evidence.

### Operator-controlled authentication server: fallback

Keycloak offers authentication/required-action extension interfaces and custom
forgot-credential flows. Its event-listener SPI is internal and may change across
versions. These are extension points, not demonstrated coverage of all mutations.
[Developer guide](https://www.keycloak.org/docs/latest/server_development/index.html).

Owning that boundary could make durable operation control explicit, at the cost
of another server, storage, upgrades and reviewed extensions. Do not choose this
fallback merely to avoid qualifying the same security requirements. No custom
authentication cryptography is proposed.

## Next implementation gates

1. Bind an isolated provider tenant/client and an exact supported test RP/origin
   before registering engineering credentials. Mike clarified that no server
   domain has been chosen. His intended account email is a separate input, not
   proof of email verification, ownership of a domain or an existing provider
   account. Keep personal account addresses out of committed fixtures.
   The permanent owned login domain is [EF-21](docs/EDGE_CASE_FOLLOW_UPS.md#ef-21--permanent-login-domain-and-test-passkey-transition),
   a launch decision rather than a prerequisite for implementation. The three
   existing SLUG Railway services have no HTTP service/custom domains and do not
   establish an authentication origin. A supported provider test hostname or
   isolated test service can be evaluated without buying a permanent domain;
   do not claim either has been provisioned or qualified yet.
2. For the selected provider, map login, ordinary reset, factor changes, D85
   recovery, administrative/API changes, refresh and imported/impersonated
   sessions. Every enabled mutation must enter the durable SLUG barrier first
   or be unavailable to the integration. Record exact operation reconciliation;
   latest-user timestamps are not a substitute. Unknown outcomes remain blocked.
3. Freeze provider-specific verified method/UV, original authentication, nonce,
   purpose, epoch and mutation bindings. Verify them independently before adding
   the new coordinator/account-session mode. Preserve original wallet/View fences.
4. Exercise actual browser login, missing-factor/UV refusal, refresh/replay,
   sign-out/reopen, interrupted reset and D85 same-wallet recovery. Require code
   save-confirmation and email/all-device notification delivery evidence; an
   outbox record alone is not delivery.

Auth0's current [Universal Login passkey prerequisites](https://auth0.com/docs/authenticate/database-connections/passkeys/configure-passkey-policy)
do not list a custom domain, while its [embedded React passkey example](https://github.com/auth0/auth0-react/blob/main/EXAMPLES.md)
explicitly requires one. This is a distinction between integration paths, not
proof that a particular default tenant domain is usable. Qualify the exact
selected system-browser flow. Do not substitute an embedded flow or treat a
test RP as the permanent identity merely to avoid choosing the release domain.

No runtime or product source changed during this evaluation. Nonblocking findings
continue in [the follow-up log](docs/EDGE_CASE_FOLLOW_UPS.md). The full joint D74
review remains last; marketplace remains on hold.
