# D85 — Account factor recovery

Accepted by Mike on November 21, 2026 Pacific. Implementation and actual
acceptance are pending; this completes D82's recovery-policy decision.

When a player has lost the factors needed to sign in, require **both** an unused
saved account recovery code **and** a fresh code sent to the email address that
was verified before recovery started. Neither proof works alone. An existing
qualifying backup passkey/authenticator remains a normal authentication option.
Support cannot substitute proof, disable MFA or bypass this procedure.

After verifying both proofs, require enrollment and verification of a new
qualifying passkey/authenticator, revoke existing sessions, and apply D72's
24-hour security hold. Keep incomplete recovery restricted; possession of the
two codes must not yield an unrestricted wallet session before replacement-factor
enrollment and the required durable authorization transitions complete.

The hold blocks withdrawals, fighter transfers, recovery-phrase/file export,
changes to recovery authority, marketplace purchases and marketplace listings.
Viewing and mining into the same existing wallet remain available through a
newly authorized session after recovery completes. The specifically authorized
replacement-factor enrollment is part of recovery; it is not a general right
to change recovery authority during the hold. A distinct recovery restarts the
hold. Exact retries recover their original result without consuming another
code, enrolling another factor or extending the hold.

## Saving codes and notifications

At enrollment, require confirmation that codes were saved, through a completed
download or confirmation entry. Showing codes behind a dismiss button does not
satisfy this requirement. Explain the consequence while saving is still possible:
losing every qualifying factor and the required code/email recovery combination
can leave the hosted account inaccessible. No later support exception is offered.

Using a recovery code creates a notification to the already-verified email and
all enrolled devices, as with reset. Tie notification records to the original
durable recovery event so retries cannot omit the alert or invent another event.
Notification delivery/retry and offline-device catch-up need actual acceptance;
do not claim notification delivery from an outbox write alone. These messages
must not contain recovery codes, authenticator secrets or wallet material.

## Wallet boundary

Account recovery codes are separate from D70's 24 wallet words and SLUG recovery
file. This procedure never accepts or uploads those wallet backups and does not
import or manage an independently held backup file. D48/D70/D72's existing
hosted-wallet recovery authority is unchanged: recovered login can regain access
to the same retained hosted wallet under the security hold. It must not create a
replacement wallet, change the gym identity or grant access by email alone.
As already specified in D72, exporting a copy does not by itself revoke hosted
recovery authority or establish exclusive self-custody.

## Engineering contract and acceptance still required

Before enabling real accounts, freeze and independently verify the exact proof,
challenge, factor/session epoch and durable recovery-event formats. The trusted
server must bind both proofs to the same account and original recovery attempt,
rate-limit verification, consume the saved code atomically, and preserve exact
recovery across interruption, duplicate requests and restart. A changed recovery
email, an existing session or a refreshed assertion cannot replace either proof.
Do not infer that an upstream provider's recovery flag satisfies D85.

Qualify code save-confirmation, both-proof refusal cases, one-time use, concurrent
attempts, lost responses, replacement-factor verification, immediate revocation,
all hold restrictions and email/device alerts on the real selected integration.
No code, credential, provider or notification has been provisioned by this record.

The two-proof design is consistent with the combination of saved and issued
recovery methods described in [NIST's account recovery guidance](https://pages.nist.gov/800-63-4/sp800-63b.html#account-recovery).
This policy decision alone is not a claim of NIST assurance-level compliance.
See also [OWASP's MFA recovery guidance](https://cheatsheetseries.owasp.org/cheatsheets/Multifactor_Authentication_Cheat_Sheet.html#resetting-mfa).
These sources inform the implementation; Mike's D85 decision sets SLUG's policy.
