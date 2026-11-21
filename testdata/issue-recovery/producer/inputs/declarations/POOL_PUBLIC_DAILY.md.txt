# Explicit public DAILY pool

Set79 adds an explicitly selected DAILY operator, authenticated transport and
headless worker path around the existing daily source, rounds, schema16 wallet,
indexed settlement and UTC scheduler. Defaults remain V1. Existing roots,
source pins, wallet guards, credentials and outboxes are not adopted or rewritten.

The source is implemented. Core/history, transport, worker and desktop/operator
normal/race/profile checks pass for the scopes listed in
[Set79 notes](CHANGESET_79_NOTES.md). Completed-day accounting refresh and exact
source/round contention handling also pass full accounting regression. The
subsequent bounded intake-contention retry passes its focused normal/race and
cheap controls. Normal-node smoke passes. Focused DAILY cancellation provenance
and original-output cleanup controls pass normally, under race and in their
selected cheap scope; the actual headless-process acceptance also passes.
The 17-module build, full operator and selected desktop closeout also passes
for its recorded source before the new DAILY ledger-admission contention change.
The installed successor now passes focused controls and the full affected
accounting/operator normal, race and cheap suites.

[Public Path B attempt 5](docs/evidence/set79/public-actual.json) now passes the
complete normal-node/TLS/schema16 proof: true derived full intake, real-expiry
late recovery, exact receipt retry through two fresh operator processes, and
exactly one B credit at 60 descendants. A's immutable signed body stays identical
and is broadcast only once. All original owners, processes and nodes join;
source, copied inputs and normal-profile bindings remain unchanged. All four
failed Path B attempts remain preserved.

[Automatic-wallet Path A attempt 2](docs/evidence/set79/attachment-actual.json)
also passes: three original vault signatures and three accepted fighters, two
pool rewards, one scheduled payment of 1,980,000,000 atoms with a 209-atom fee,
and the exact original-wallet increase. Stopped reopen and the final closed
ledger audit pass. The original 268-file fixture catalog stays unchanged; all
226 immutable copied paths stay exact. The first failure remains preserved.

[Headless Path C attempt 1](docs/evidence/set79/headless-actual.json) also passes
through the actual DAILY CLI: ten original signatures and ten accepted fighters,
two rewards, one 19.8-SLUG scheduled payment with a 209-atom fee, and exact wallet
receipt. Its original CLI exits 0 after one SIGTERM with joined output and no
forced kill; stopped reopen retains the same outbox and issuance hold. Final
17-module builds, vet, formatting and diff checks pass. Exact-tip CI remains
pending. This document is not deployment or product-release approval.

The actual reports retain their exact original source catalogs. Final vet found
one test-cleanup warning after all 17 builds passed. The successor adds only two
deferred cancellations in that test; its focused normal/race checks and final
static run pass. Captured normal dependency records place the file in
`TestGoFiles`, outside the actual binaries' `GoFiles`/`CgoFiles`. The linked
[source-delta evidence](docs/evidence/set79/actual-test-only-source-successor.json)
preserves this distinction without relabeling historical inventories.

## Explicit selection and ownership

| Surface | DAILY selection | Retained boundary |
| --- | --- | --- |
| Offline operator preparation | `slugpool-provision create\|resume\|verify --daily` with Policy2 | Original protected inputs, OS randomness on fresh create, exact original material on resume |
| Operator service | `slugpoold COMMAND --daily` with Setup2; commands remain `init`, `serve`, `status`, `rotate`, `revoke`, `checkpoint` | Original wallet, online authority owners, runtime root and joined cleanup |
| Public pool HTTP | `/pool/v2/`, `NewDailyServer`, `NewDailyClient` and `DailyClientConfig` | Exact source, settlement configuration, gym, native recipient and TLS origin |
| Worker and outbox | `NewDailyPool` and explicit daily outbox create/open | Original job/nonce, complete selected configuration, synchronous evidence before delivery |
| Headless existing wallet | `slugpilot --candidate-remote-pool --daily` | Original audited local wallet/session, recipient, Mining lease and vault signer |

Use the same protected, absolute path selections documented for
[operator provisioning](POOL_OPERATOR_PROVISIONING.md) and
[operator runtime](POOL_OPERATOR_RUNTIME.md), together with independently
selected Policy2/Setup2. The daily provisioner publishes V2 intent, READY and
operator-wallet backup records. Its backup is not a service-journal restore.
The runtime's `init` still requires the existing dedicated schema16 wallet and
the three online role files; it does not generate keys. Only `serve` starts the
listeners and configured scheduler. Checkpoint import accepts an already-signed
public envelope, never the offline checkpoint signing key.

The headless command requires `--data-dir`, `--remote-attachment-root`,
`--remote-config`, `--remote-node-rpc`, `--remote-pool-credential` and
`--remote-admission-credential`. It rejects other explicit modes and any
`--node-binary`. It opens an existing local session through the supported signed
fixture login, starts that selected worker and reports bounded dated status.
It creates neither a node nor a wallet. This is not a hosted-login or MFA flow.
There is no new helper protocol or native DAILY UI; the V1 helper wire refuses
a daily controller.

The local schema14 wallet's original source and recipient remain separate from
the remote Source2. Original READY and recipient controls are checked without
repair. Signing remains purpose-bound to retained work under the original
Mining lease; no key getter or replacement signer is exposed. Credential/RPC
contents stay in protected files and concrete owners, not command arguments or
status output. Failed cleanup retains the original owners and reports failure. Requested parent cancellation can report clean shutdown only after the
original owner/output joins and all observed causes are cancellation-only.
Independent write, partial-frame, deadline, integrity and cleanup failures remain
failures. This shutdown status grants no fresh authorization; a pre-effect
canceled launch still refuses.

## Authentication and payout reads

Pool and admission credentials retain separate audiences, generations, expiry,
revocation and original-session fences. Daily credentials and transport trust
have explicit V2 identities. Admission HTTP remains v3, and embedded manifest
jobs remain Version2; this selection does not change job or submission bytes.

Every successful daily work, submission, balance and payout-page response binds
the selected source, settlement configuration and recipient. The client checks
those bindings before exposing work or accepting a receipt, even if no balance
has been requested. A structurally valid response from a different settlement
or payee is not interchangeable with the selected one.

`DailyBalance` and `PayoutsPage` preserve exact reduced fractions. Cumulative
`paid_atoms` and `reserved_atoms` are canonical nonnegative int64 decimal strings,
not floating-point values or a single-transaction money limit. Individual
payments retain their existing transaction amount limits. Observations retain
the existing bounded timestamp grammar and are dated captures, not a promise
of continuously current funds.

Payout history scans at most eight descending **batch slots** per page, returning
zero to eight matching payments. Each read fully audits the original ledger and
rechecks the original session. `revision`, `high_batch`, `before_batch` and
`next_before` are decimal strings. An empty page is not necessarily the end;
follow the returned cursor until `done`. The full authenticated balance is not
the sum of the visible page. A changed revision returns definite
`stale_capture` (HTTP 409); restart only the read capture. No history is silently
truncated to the old V1 complete-history response limit.

## Financial and uncertainty rules

The 99% miner allocation, operator 1%, minimum payout, confirmation/maturity
checks, UTC daily slot and fee rules are unchanged. D84 defers the **entire**
ready payout when its transaction cannot fit. It retains all earnings, signs
and sends nothing, and leaves the daily slot unused. No funding subset, split
payout, miner surcharge, advance or consolidation is authorized. Waiting alone
does not guarantee an oversized candidate will shrink.

Original signed pending payout recovery runs before new source import or new
payment planning. Setup2 binds the same concrete source, round, ledger, backend
and scheduler owners before daily admission is enabled. Intake reservation is
derived from the selected limits and exact enrolled gym set, never a caller's
ordinary quota. `status` exposes the derived `daily_reservation`.

Intake coordination retains the same invocation's readiness and capacity cause
across exact original-source Busy responses. One 30-second budget starts before
the intake mutex; retries wait 10 milliseconds after the concrete pool/issuer
gates return. Waiting for the intake mutex still joins its current owner, then
checks the original budget. Cancellation, expiry and any mixed or unknown error
remain failures. A failed Status does not discard a permanent drain request,
and these retries perform no financial work.

The installed DAILY scheduler/mutation path now queues on the same original
ledger gate within a bounded contention budget; public money reads and V1 stay
nonwaiting. Exact Busy-only initial audits and read-only source checks retry
within that invocation, including a post-sign check. Planning, quoting, signing
and broadcasting are not retried as a whole. The first contention starts one
shared 30-second waiting budget, bounded further by the original caller;
independent fatal errors, cancellation and expiry remain failures. Six focused
normal/race controls and two cheap controls pass, including original-gate
contention and unchanged financial effects after signing. The full affected
accounting/operator suites and all three complete public actual paths pass.
Final build/static qualification passes; exact-tip CI remains separate.

For DAILY only, an occupied scheduler cursor also permits accounting refresh
after the configured due minute. `RefreshCompletedDaily` fully audits the
original ledger/source and samples its original clock. Before any backend call,
credit import or write, it requires that the sampled day equal the durable
attempt's admitted day, that the original payment slot is already occupied,
and that no batch remains pending. The existing completed-day path can then
import later confirmations and refresh balances without quoting, signing,
broadcasting or occupying another day. Pending recovery still runs first.
An expired admitted day is a no-effect retryable refusal, not permission to
prepare the next day's payment. V1 scheduling, cursor bytes and payment-due
decisions remain unchanged.

An unseen canonical winner from an original issued job can use the existing
post-drain path only while complete-history financial and physical headroom
remains. The original session, real expiry/canonical checks and one synchronized
receipt commit remain required. Exact saved receipts and pending payments remain
recoverable without replacement issuance or duplicate credit. See
[finite daily admission](POOL_DAILY_ADMISSION.md) for the D80 boundary.

Before a daily worker calls Issue it synchronizes `issue_pending=true` in the
original outbox. Network work does not hold the outbox mutex. A valid response
must match the same live attempt and commit its exact scan plus hold-clear in
one sync. Unknown, malformed, wrong-binding or canceled responses retain the
hold across Stop, Run and process reopen. Pending receipts and fighter delivery
can still reconcile. Only an exact same-attempt response already classified by
the trusted API as definite no-effect permits the corresponding clear.

The current Issue request has no recoverable request identifier. A crash after
the durable intent but before dispatch can therefore also leave a permanent
issuance hold. The worker never adopts a latest job, clears the hold on reopen,
or blindly issues replacement work. Daily headless status exposes the existing
`paused` / `outcome_unknown` vocabulary. Recovery from that ambiguous issuance
is an explicit remaining availability boundary.

## Finite scope and remaining gates

Source2 still has selected lifetime limits of at most 8,192 retained works,
16,384 receipts and 8 GiB. Ordinary admission is derived at at most 4,095 rows;
the per-round window remains 4,096. Existing round, credit, batch, claim, report,
transaction, outbox and signed-authority bounds also remain. Job renewal and tip
changes consume work capacity, so a block target alone cannot predict operating
duration. No retirement, rollover, journal deletion or repinning is implemented.
Physical-full recovery and unlimited future conflicting winners are not promised.

The delayed-client Path B now qualifies original expired-job recovery at
derived full intake, fresh-process retry, exactly one late credit and unchanged
reserved payout bytes on normal-PoW nodes with TLS and original schema16 custody.
It does not prove that the automatic worker scans expired work. Original local
wallet/vault and automatic-worker Path A now passes its complete receipt,
scheduled-payment, authenticated balance, wallet-delta and closed-ledger checks.
Its direct controller composition does not itself qualify the headless command;
the separate Path C actual now covers that process/argument path and clean
SIGTERM shutdown. Earlier direct-owner proofs retain their own scope.

At ordinary intake exhaustion, a pool result can be accepted before its later
fighter admission is refused at capacity. The original signed or interrupted
outbox row remains pending; a pool receipt or scheduled payment does not prove
a fighter birth. Stop/reopen must preserve those rows and any issuance hold.
Path A2 records three accepted fighters and zero pending delivery rows, while
its ambiguous issuance hold remains set. Complete fighter delivery for every
capacity/Stop race remains a separate integration boundary; no admission-guard bypass, journal reset or larger capacity is implied.

Normal-node seed smoke passed with two isolated blocks before read-only public
synchronization, checkpoint descendants and restart checked; no public-chain
mining occurred. Final package/command static qualification passes. Exact-tip
paired CI remains pending; hosted operation and sustained realistic miner
traffic remain separate readiness boundaries. This
selection adds no onboarding, native navigation or production-genesis decision.
Public pool operation and payouts precede onboarding; the joint D74 review
remains the final product-release gate.
