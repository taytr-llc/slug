# Set80 finite issuance recovery acceptance

Source-only plan against approved contract
`f3e019a840d6b69d853350763364a3fc41f10ea899f00ed6f164db5d58a95610`.
No harness implementation, producer execution or runtime acceptance is claimed.
The independent reference remains a prerequisite. Existing Set79 A/B/C evidence
stays unchanged; this plan adds exact-request recovery, not another payout policy.

## Concrete fault seam

The current source coordinator in `slug/poolshare/pool.go:144` commits the original
issuer job, records its durable effect, then calls `Node.Proposal`; `saveWork`
occurs only after that call and the original context/expiry/session checks.
`slug/prospectnode/template.go:41` sends a real `getblocktemplate` RPC with
`mode:"proposal"` and the exact candidate block. The selected implementation must
retain that ordering when it substitutes request-aware issuer commits.

Use a parent-owned RPC relay derived from the bounded forwarding/join owner in
`set79-prep/public-daily-attachment/relay.go`. Arm one proposal only while one
identified Resume request is active. Forward its exact bytes to the original
normal node, require the real successful proposal response, then hold those
response bytes before returning them to the operator. Publish a public barrier
acknowledgment and immediately kill the exact operator child, within five seconds
of that acknowledgment and before its unchanged operation deadline. The relay
never invents a node result or caches a result as issuance authority.

The barrier alone is timing evidence. After the operator is reaped and all relay
handlers for that connection are joined, open the original issuer/source through
their full selected read-only audits. Require exactly `pending/issued`, one
source request/template, one issuer request/job/reverse mapping, and **zero source
work rows, work reverse rows, submissions and accepted receipts**. Reconstruct
the proposal from that exact audited job/template and compare its bytes with the
captured RPC candidate. These assertions prove the intended gap rather than a
saved-but-unaccepted result or an unrelated RPC. Close the audit owners before
starting the replacement process. An unexpected state fails the case; it is not
an invitation to kill another attempt until the desired state appears.

## Two actual cases and one narrow pre-dispatch control

| Case | Controlled event | Required retained evidence |
|---|---|---|
| A: committed job, lost successful response | Proposal barrier, intentional operator kill/reopen, then one TLS response loss and joined worker restart | Same request, intent, job, canonical template, challenge, issued/expiry times and original owner selections; one job/work for that request, actual receipt and scheduled wallet payment |
| B: never-published expiry | Same proposal barrier/kill, retain the original pending request through actual expiry, then resume | `issued_unused/issued`, original job and template, `expired_before_work`, zero work/submission/birth/credit/payment |
| L: local intent before network | Original outbox's existing private post-sync commit checkpoint in a test-only child | Exact active request and null result survive kill/reopen; upstream request count remains zero until a later explicit Start |

### A — recover the original job, then recover a lost reply

Run the actual selected headless CLI with the original schema14 account/vault,
audited recipient, Mining lease and protected credentials, against a fresh
selected operator with its original concrete issuer/source/round/ledger/schema16
owners. Reuse the public Set79 fixture catalog and normal two-node machinery;
do not adopt an old V2 root or unidentified `issue_pending` hold. Freeze all new
wrappers/IDs independently before execution. Reuse the qualified Path A public
block-only pool target, original prospect policy, two-enrollment intake bound and
adequate original worker outbox limit. The original independent one/two-reward
economics remain unchanged; the new wrapper identities must be independently
derived. Exact bytes/selection are immutable after construction.

Reach and audit the gap above. Stop/join the original CLI if it is still alive,
recording its real cancellation/unknown result and retained request. Restart the
operator with the identical paths, setup, profile and protected selections, then
explicitly restart the CLI on the same original outbox. A TLS front relay at the
independently selected origin forwards the first Resume of that exact ID to the
real selected server. It receives and validates the entire successful work
response, but closes the downstream connection before delivering any response
bytes. This is response loss after real source completion, not a fabricated HTTP
result or a replay cache. Forward bearer headers only to that original server;
never put them in evidence.

Prevent another downstream Resume from being forwarded until that original CLI
is stopped and joined. Preserve the attempted request IDs; do not translate the
loss into a supposedly definite safe error. Full closed audits must show one
completed source work and original issuer job, while the local request still has
a null result and active ID. Reopen the same worker explicitly. Release normal
forwarding and require a fully authenticated response identical to the withheld
one, including request/profile/gym/key/source/money binding and exact work.
The local sync must retain that full response, clear only its active ID, and
link the scan to that request. Allow the actual miner's normal PoW under those
unchanged targets until its original recovered work yields a canonical pool
receipt. Reuse Path A's explicit Stop observation and exact one-or-two-reward
branches for the real stop race, with every retained pool/fighter pending outcome
reconciled or explicitly retained. Any later legitimate new request must have a
different ID and its own completed predecessor; record every request/job/work,
and prove specifically that replay of the original ID created none. No ignored
errors, nonce substitution or retry-until-lucky process runs are allowed.

Stop by the actual CLI signal path, require its precise successful joined exit,
and perform a stopped reopen with no Start. Compare the complete q/e/h/p state,
scan/cursor and active ID, not just a pending count. A later authenticated lookup
or replay of completed work remains the exact historical work even after its
expiry; it must not authorize renewed hashing of expired work. Record the
original request's job/work count before and after each replay. No second job,
new template, new challenge or renewed expiry is allowed.

Continue with the actual original scheduler/schema16 backend and real UTC due
time: mature the retained canonical rewards on the normal node, observe one
durably saved payout before its real broadcast, confirm it and require the
original schema14 wallet to observe the exact recipient output. Reuse Path A's
validated payment artifact, bounded authenticated DAILY money/page reads and full
closed source/round/ledger audit, adapted only to the new selected response/profile
binding. Require the independently fixed one/two-reward amount, fee, raw/txid,
paid/reserved values and change; no caller-clock change or direct replacement
signer. This financial continuation is required for the new composition's actual
acceptance. If it cannot fit the root-selected finite due/budget, report that
unmet gate; an issuance-only pass cannot silently replace it.

### B — expiry before any source work was published

Use a separate fresh selected operator/outbox case with the same independently
bound public inputs. Reach the same exact gap and kill/join once. Capture the
original issued/expiry times from the closed full audit. Wait until real
`UnixMilli >= ExpiresAtMS`, rechecking actual wall time after every context-bound
wake. No fake clock, changed expiry or new job is allowed. The nodes remain
parent-owned; no mining or payout action is needed for this case.

Restart the same original stores and make one bounded typed Resume for the same
request under legitimate same-gym authentication. Require `issued_unused`, null
work, exact old job ID/intent, the fixed reason and a resolution time at or after
the original expiry. Read-only Lookup and repeated Resume must return that same
terminal without count growth. A focused worker test consumes this exact kind of
authenticated terminal into the original request and proves active-ID clearing
without a scan; it stops at that iteration rather than treating a later automatic
new request as part of the terminal proof. Full closed original-owner audit
proves zero work, receipt, fighter birth, round credit or payment for this case.
The terminal wire frame alone is not proof of nonpublication.

### L — distinguish pre-network from pre-server-dispatch

A TLS relay can hold before forwarding to the server and prove local intent
precedes **server dispatch**. It cannot prove that no client network call began.
For the stricter point, use a package-local opt-in test child with the existing
`PoolOutbox.checkpoint("commit")` seam (`prospectworker/pool_outbox.go:323`),
armed only for the new active-request/null-result sync. Block there, report the
public request ID, kill/reap that child, and full-audit/reopen the original outbox.
Use real clients and an upstream counter; require zero outbound Issue/Resume
before the checkpoint and zero server requests. Only a subsequent explicit Run
may dispatch that same ID. This is a test-only checkpoint, never a shipping hook,
alternate authority, entropy override or exported credential getter.

Focused original-owner tests also cover source-intent/issuer-absent and
pending/sealed recovery, write acknowledgment uncertainty before/after each sync,
delayed same-ID replies, exact result persistence, wrong gym/profile/money,
revocation/cancellation, shared slot/byte exhaustion and all reverse-index
corruption. The frozen independent state/bound corpus supplies expectations;
these tests do not substitute for actual case A's real process gap.

Run meaningful normal/race D80 controls **on the new selected mode**: fill the
derived ordinary intake, retain an original unseen winning job, make that exact
block canonical after expiry, close/reopen the complete original owner chain,
recover it twice and import one credit after the unchanged confirmation lock.
An earlier reserved payout's entire body, inputs/claims, outputs, fee, raw bytes,
txid, creation/day and payment rows remain identical. Include noncanonical,
capacity, poisoned-write and wrong-session refusals. This extends the existing
combined real-store fixture to new constructors/bindings; old-mode tests alone
are regression evidence, not new-mode coverage. D84 entire-payment deferral and
all original financial audits remain required in those controls.

## Process ownership, bounds and evidence

Use the existing 900-second scenario, 1080-second outer watchdog and 180-block
ceiling as the starting proposal; root selects the exact runtime budget/lock
after implementation, accounting for the real scheduled payment and one real
five-minute expiry wait. No execution is authorized by this plan. Each product
operation remains 30 seconds; startup readiness stays under the original
scenario deadline. Each relay barrier has a finite parent
deadline, cancels with the scenario and joins every handler. No deadline renewal
on retry, automatic process replacement, public mining or seed access.

Reuse Set79 C's exact child/PID/one-Wait/manual-pipe owner and original inherited
process group. The parent owns both nodes and relays throughout; restart is an
explicit harness action only after the previous child is reaped and original
audit owners are closed. Expected SIGKILL events are labeled intentional crashes
with PID, signal, barrier and wait evidence. They are not clean Close results.
All other CLI/operator exits, relays and nodes must join normally. Preserve every
unexpected exit/cleanup failure and the original synthetic tree; never delete an
uncertain wallet/DB tree. An outer timeout cleans the whole original group and
fails the qualification.

Public evidence includes exact selected IDs and binaries, parent/child PID and
generation, barrier timestamps, request/intent/job/template hashes, original
issued/expiry times, full audited request/work/job/receipt counters, withheld and
replayed public response identity, local q/scan/cursor/control hashes, intentional
crash classification and final joined-owner status. Bound event lists and logs;
exclude bearer/RPC credentials, wallet DB contents and seeds. Bind source,
profile, binaries, public fixture catalog and immutable history paths before and
after every outcome. Emit failure evidence as well as success evidence.

This qualification proves finite exact issuance recovery on selected local TLS
and real original owners. It does not claim continuous capacity, a public host,
new provider authentication, altered payout rules or D74 completion. Existing
financial/D80/D84 regression suites must remain green, and old V1/DAILY modes
must still reject the new selected issuance owners while original financial
consumers retain their unchanged Base Config2 bindings.
