# A2 receipt Lookup — diagnostic-only successor proposal

A2 failed at `work_evidence.go:138`, `exact interrupted pool result only`.
The public report records all original owners/processes joined, one stopped CLI
row and zero pending deliveries, no payout attempts/sends, and completion of the
original request's crash-gap/reply-loss/replay stages. It does not retain the
failed Lookup error, closed outbox row or returned receipt, so it cannot establish
which rejection branch occurred. A2 remains failed.

The last CLI status is at 1795264873496 ms; the scenario ends about
1795264873875 ms, while the original job expires at 1795265161973 ms. These
timestamps do not indicate a 20-second Lookup timeout or original-job expiry.
They do not identify a unique service/transport cause either.

## Current exact error paths

`readLookup` gives the complete operation one 20-second context and retries only
an exact `*poolservice.ClientError` with `OutcomeUnknown=false` and code `busy`
or `rate_limited`, with the existing one-second cancellable pause. All other
errors return immediately. The caller accepts only exact definite `not_found`
for a retained row whose pool state is still `pending`; it never credits that row.

The current client can return definite `unavailable` for a failed HTTP/TLS read,
unexpected status/header/body framing, selected response binding or receipt
validation failure, normal nonce/context failure or a server's own unavailable
response. It also returns typed `invalid_request`, `rejected`, `not_found`,
`unauthorized`, `busy`, and other closed wire codes where applicable. A false
OutcomeUnknown flag means this was a read-only call; it does not make every
unavailable error a safe-to-ignore success or prove ordinary gate contention.
The current evidence does not distinguish these cases.

The server's original `Pool.Lookup` is read-only: it authenticates the original
session, loads exact work/submission, validates the retained submission and
rechecks the session. Its error mapping keeps Busy/not-found/invalid/session
classes and maps other read failures to unavailable. There is no source-backed
reason to change that product classification from the omitted A2 error alone.

## Proposed observation only

The two `after/` leaves add a public `receipt-lookup-evidence.json` after the
original CLI has joined. It captures the full closed audited outbox rows/control
before the live read, every Lookup attempt's exact error type/client code/unknown
flag, original call deadline, start/end times, context result and successful
returned submission. It is written on failure as well as success; `completed`
is true only after all original assertions finish. The public report is capped
at 16 MiB; attempt growth retains the original one-second pause and 20-second
per-row deadline. No token, header, credential,
wallet, private fixture, HTTP response body or new authority is recorded.

The deadline, one-second pause, Busy/rate-limited predicate, accepted not-found
condition and every receipt/canonicality/financial assertion remain unchanged.
The panic message also includes the typed error code and original row state.
There is no new unavailable retry, broad errors.Is rule, replacement client,
new process, changed financial expectation or retry-until-lucky flow.

Root must review/compile/select this source before any execution, and separately
extend the independent checker/root public-evidence inventory for this new file.
The current active helper and checker are untouched. The separate original-A2
read-only diagnostic can test retained structure without running this successor;
its result cannot reconstruct the lost live transport error.
