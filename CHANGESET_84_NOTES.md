# Set84: trusted sign-in assurance

Branch: `slug-auth-assurance`, stacked on completed Set83 `046afb22`.
Status: D85 recovery policy recorded; implementation and actual sign-in pending.

Mike approved separate single-use saved account codes together with fresh proof
to the already-verified email, replacement-factor enrollment, session revocation
and the same D72 24-hour hold. Enrollment must confirm that codes were saved,
and use must notify the verified email and all enrolled devices. This completes
the recovery-policy choice while preserving the same hosted wallet and the
separate 24-word wallet backup. See `ACCOUNT_FACTOR_RECOVERY.md`.

The next useful delivery is actual qualifying authentication in the system
browser leading to the original stopped wallet, then sign-out and reopen.
The existing coordinator is explicitly fixture-only; its old assertion and
account session do not attest MFA or passkey user verification. A new selected
trusted assurance path must retain those facts through coordinator consumption,
session authorization, credential/factor epochs and original-wallet fences.
Old fixture assertions and profiles remain strict and unchanged.

[Provider qualification](AUTH_PROVIDER_QUALIFICATION.md) records the concrete
Auth0 reset-hook findings and remaining assurance/recovery gates. Auth0 is the
first evaluation candidate, not a selected or deployed integration. The owned
login/RP domain and any existing SLUG auth/email provider are awaiting Mike's
answer; real passkey registration must not guess that domain.

Provider selection must qualify all enabled credential/factor/reset paths,
including administrative/API routes, against the existing durable reset barrier.
A post-change webhook or an email-only provider recovery path cannot satisfy
that ordering. Pin the selected provider, relying-party domain, origin, callback
and trust keys before real credential registration. Freeze exact contracts and
independent reference derivations before implementation; synthetic tests alone
will not be called a real sign-in feature.

No new authentication behavior, credentials, wallet issuance or notification
delivery is implemented by this policy commit. The full joint D74 review remains
last; marketplace remains on hold. Nonblocking findings belong in
`docs/EDGE_CASE_FOLLOW_UPS.md`.
