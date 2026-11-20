# D82 — Mandatory MFA and passkey support

**Status:** Accepted product requirement; proposed delivery design.
**Date:** November 19, 2026 Pacific.
**Decider:** Mike. Mandatory MFA, passkey support and no support bypass are
accepted. Provider selection and the exact recovery-proof contract remain open.

## Context and accepted requirement

Mike said: “Fine if we are going to email password then we need a strict 2FA auth
system,” followed by “Supporting passkeys would be nice too.” Real hosted login
must therefore require multi-factor authentication and support passkeys. MFA is
not an optional account checkbox, an email-verification badge, or a client-only
screen. These requirements are a release gate, not a claim that the current two
fixture buttons already implement them.

D70's separate 24-word wallet backup and D72's same-wallet verified reset,
session revocation and 24-hour hold remain in force. Password reset must not
silently remove MFA, enroll an attacker's new factor, or grant wallet access
through email alone. Loss of every authentication factor is a separate recovery
case; it is not automatically authorized by the ordinary password-reset rule.

## Proposed delivery design

| Path | Requirement | Tradeoff |
| --- | --- | --- |
| Passkey sign-in, preferred | Verify a WebAuthn assertion for the configured relying party and origin, require user verification, and check that verified result on the trusted server. | Convenient phishing-resistant authentication; device/sync-provider recovery must be assessed. |
| Email/password sign-in | Require an enrolled authenticator-app TOTP or a qualifying WebAuthn factor before issuing a full account session. | Familiar fallback; TOTP can be phished and is not equivalent to WebAuthn phishing resistance. |
| Email or SMS code alone | Does not satisfy this proposed SLUG MFA policy; email verification remains contact verification. | Avoids quietly weakening the stronger paths. |

A user-verified passkey can satisfy MFA through possession of its key plus local
PIN or biometric verification. Mere user presence, such as touching a key with
no user verification, does not establish both factors by itself. This is why
the passkey path need not also request a password or TOTP after a verified
multi-factor assertion. See [NIST's syncable-authenticator guidance](https://pages.nist.gov/800-63-4/sp800-63b/syncable/).

Enrollment precedes normal account/wallet activation. An incomplete signup has
only narrowly scoped enrollment authority. Offer registration of a backup
passkey/security key and separately issued single-use account recovery codes.
Account recovery codes are not the wallet's 24 words, and the app must not ask
players to send either secret to support or chat. No real factors, codes or
credentials are created by this decision record.

Existing authenticated sessions may resume within their eventual fixed lifetime;
“remember me” must not mint password-only sessions or convert refresh time into
fresh MFA. Require a fresh verified authentication event for withdrawals, fighter
transfers, recovery export, recovery-authority changes and changes to login
credentials/factors. Define the exact freshness and action-binding contract
before implementation; the existing generic authenticated-at field is not proof
that MFA happened. A successful challenge cannot lift D72's active hold.

Changing a factor must prove an existing enrolled factor or use the separately
defined recovery procedure. Keep at least one usable qualifying method; no
ordinary “disable 2FA” switch. Plan revocation, durable audit records and user
notifications for recovery and credential changes. Factor changes require more
than possession of an existing session. See [OWASP's MFA guidance](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html#changing-mfa-factors).

## Recovery policy and the two kinds of backup

Mike selected “Require backup factor/code; no support bypass.” Support therefore
cannot override MFA when the player has lost all accepted recovery methods.
He then asked whether the 24-word phrase was the backup code. This distinction
must be explicit before freezing the recovery mechanism:

- D70's 24 words plus SLUG recovery metadata recover wallet/gym keys through
  SLUG recovery software. They are not automatically a hosted-login MFA reset.
- The proposed account recovery codes are separate, single-use secrets for loss
  of login factors. They are not wallet seeds and cannot restore wallet keys.
- A backup passkey/security key is another enrolled login method, separate from
  either kind of recovery secret.

No support bypass is approved; separate account codes remain the recommended
mechanism, not a frozen or implemented protocol. Do not infer that Mike approved
collecting a wallet phrase for account recovery, or that an exported wallet
automatically restores the hosted account. If all accepted login recovery methods
are lost, the hosted account can remain inaccessible while an independently
exported wallet backup remains usable separately. The UI must explain both
backups and what each can recover; neither belongs in support messages or chat.

Before coding recovery, define exactly what proof a code must accompany, atomic
one-time consumption, replacement-factor enrollment, credential/session epochs,
revocation, notifications and hold behavior. Do not assume possession of an
email inbox or a logged-in session may regenerate MFA recovery codes. Keep the
approved password-reset hold unchanged; factor-recovery hold details must be
made explicit rather than inferred.

## Compatibility and implementation boundary

Set71's signed fixture evidence establishes the original identity, authentication
time and credential epoch. It does not attest MFA method, verified WebAuthn user
verification, or factor lifecycle. Its two public fixture buttons cannot qualify
real D82 login. Preserve the old closed codecs, reference vectors and schemas;
add an explicitly versioned trusted assurance path before enabling real accounts.

The provider adapter must verify method/assurance, authentication time, intended
audience/nonce and account/credential epoch from trusted evidence. Do not accept
a webview's `mfa=true`, infer assurance from a provider name, or trust an unverified
extra claim. Refresh, password reset and factor recovery must not downgrade the
required assurance or restore revoked sessions. Server-side action authorization
must retain that evidence through the existing original-session fences.

Use reviewed provider/standard WebAuthn implementations rather than building
cryptographic primitives. Select the provider, relying-party domain, desktop
authentication transport and precise session/factor contract before freezing
their new public inputs and independent producers. D73 production issuance and
the final joint D74 design review remain separate gates. This document provisions
no service and does not change wallet derivation, seed deployment or custody.

## Action items and acceptance

- [ ] Freeze the allowed backup factors and account recovery-code proof/hold
      contract under the approved no-support-bypass rule; make the distinction
      from the 24-word wallet backup explicit before onboarding is finalized.
- [ ] Select and qualify a provider supporting enforced MFA, passkeys and the
      required reset/revocation ordering; document its operational recovery power.
- [ ] Freeze a versioned assurance and factor-lifecycle contract with independent
      references before implementation; old fixture profiles remain explicit.
- [ ] Implement signup/enrollment, both login paths, session resume, fresh action
      authentication and factor replacement through the actual desktop flow.
- [ ] Prove password-only and missing-UV refusal, wrong origin/relying party,
      expired/replayed challenges and codes, rate limiting, refresh downgrade
      refusal, reset/hold preservation, revocation during work and recovery retry.
- [ ] Exercise backup-factor/code recovery, device loss and interrupted enrollment
      end to end; a provider dashboard setting or disabled button is insufficient.
- [ ] Include passkey, password/MFA and recovery journeys in the final D74 review.

Documentation only: no authentication behavior, tests or deployed configuration
changed with this record.
