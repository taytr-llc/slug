# Retained fighter admission at daily pool capacity

Set 81 separates mining intake from delivery of already-issued fighter work in
the explicitly selected DAILY operator, including recoverable issuance. A full
pool source still stops new mining work. Once both original public and round
readiness checks succeed, the same issuer may accept a valid fighter proof from
an unexpired original job. Pool receipts do not confer fighter admission.

The default/V1 operator and existing admission constructors retain their prior
behavior. The selected operator uses
`NewPausedDailyFencedManifestAdmissionServer`; it starts paused on every open.
There is no new wire format, journal schema, signed profile, or persisted latch.

## Holds and readiness

Only an exact pool-capacity hold permits retained admission. Public publication,
admission, round, missing-reservation, over-bound, and unknown hold causes remain
hard holds. A later hard cause supersedes an earlier pool-capacity status and is
retained across contention and readiness retries within that runtime. Restart
does not erase the underlying issuer or signed-authority limits; every new
admission still passes those original checks.

Mining never reopens merely because fighter delivery becomes available. A full
source reopened from disk must pass fresh readiness before its initially paused
admission service is enabled. The existing transition deadline is unchanged.

Selected DAILY round maintenance also retries exact original-store `busy`
results within its existing 30-second step. Sync, List, individual Refresh and
the permitted live-handle reanchor each keep their own retry boundary, so a
completed earlier phase is not repeated. A short cancellable pause separates
these attempts from the shared maintenance cadence. Unknown, mixed, wrapped,
corrupt and uncertain-write failures are not converted to contention. Legacy
maintenance remains a single call; settlement and signing are outside this retry.

## Expiry and recovery

Historical receipt recovery comes first and preserves the exact original record,
birth, ownership publication, and sequence. It does not sample the current clock
or consult the node again.

For a never-admitted proof, the selected service checks the original issuer's
clock before applying its intake hold. `PreflightManifestAdmission` validates
the original prepared proof, issuer, job, and session, then synchronizes the
existing clock high-water mark. This can report an expired job even while
mining is drained. It grants no reservation: the unchanged final admission
commit checks time, authority, and capacity again.

Only the exact issuer expiry result, complete success of the original account
lease, and a subsequent successful check of that same session authorize a
definite expired response. The preflight callback captures its result privately
and returns nil so an outer authorization failure cannot be hidden by an expiry
error. Uncertain clock writes remain unknown and poison the service; foreign or
mixed expiry-looking errors cannot clear the worker's retained proof.

The worker's existing durable delivery behavior records a definite expiry as
terminal. Unknown or unavailable outcomes retain the original signed pending
work for recovery. No new signature, job, or birth time is substituted.

## Accounting boundary

Retained fighter admission creates no pool share, round credit, quote, signature,
or payment. Distinct valid proofs from the same original job are not restricted
to one fighter merely because pool intake is full. A prospect-only proof does
not need a pool receipt.

D80 still grants no fighter from an expired job, including a canonical late
pool winner. D79/D81 accounting and D84 whole-payout deferral remain unchanged.
An already reserved payment retains its exact claims and signed transaction.

This change does not provide continuous source history, signed-authority
continuation, hosted operation, or production custody. Under Mike's November 21
priority, general capacity expansion is a public-launch follow-up; the bounded
hosted pool/payout test comes first, then wallet balances/history, sign-in and
the final joint D74 review. Marketplace is on hold.
