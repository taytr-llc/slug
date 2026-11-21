# Set79 public DAILY test and freeze plan — proposed

Source-only, 2026-11-20. No generated oracle or execution is claimed. Freeze the
companion byte/API contract first; preserve old corpora and V1 inputs unchanged.
Root owns implementation selection, producer execution and qualification.

## Three disjoint implementation lanes

1. Public bytes/read projection: Policy2/Setup2/provision markers + pure existing
   schedule-binding helper; new daily ledger payout-page reader and wire validators.
2. Credential/transport/worker: fresh poolminer2, DailyClientConfig/trust, selected
   service+TLS ingress catalog, daily durable worker/outbox and fixture tests.
3. Production composition: explicit existing-vault DAILY attachment/headless
   launcher plus original daily Runtime/command/provision selection,
   source/ledger/wallet/schedule binding, paused daily intake and joined lifecycle.

Test helpers must use original concrete owners for authority claims. Modeled page
bodies/counters and HTTP failures are explicitly conditional codec/transport
controls; they do not claim valid high-volume source or spendable funds. Keep
production code and independent expected-byte producer separate.

## Independent reference before Go expectations

Copy published inputs with identities, not live/private artifacts: existing
operator Policy1/Setup1 corpus and certificate; daily source/round/ledger/wallet/
scheduler corpora; Set78 capacity and actual models; original manifest/job/work
and worker receipt literals' independent sources. Producer implements selected
new bytes without reading Go outputs. Freeze source/input lock, then reproduce
under two Python interpreters before consumers are staged.

Require the following cases and their exact IDs/canonical bytes:

- Policy2/Setup2 one/two/64 enrollment positives with independent sorted GymIDs,
  payees and every derived namespace/component ID. Same public request in V1/V2
  has deliberately distinct namespace/Policy/Setup/credential/trust IDs. Exact
  unchanged public-material/certificate and schema16 guard grammar. Compare the
  pure scheduler digest to the already-frozen daily scheduler oracle, not a new
  domain. Missing/unknown/duplicate/raw-order/null/LF and cross-mode refusals;
  wrong genesis/checkpoint/profile/source/settlement/recipient/schedule bindings.
- Full bounds: existing min/max daily fields, MaxRounds/MaxWorks relation,
  64/65 enrollments, duplicate key/payee/GymID, malformed or nonnative script,
  wrong budget membership, unsupported fees and timing. No caller quota. Keep
  maximum-frame sizes independently measured; the useful selected fixture must
  have nonzero derived intake, without claiming every valid config is usable.
- Distinct DailyClientConfig: source, settlement ID, recipient, gym, CA and origin
  changes alter trust; token rotation does not. V1 trust bytes remain exact.
  Numeric/empty/uppercase/length-invalid IDs or script refuse, not normalize.
  Exact daily WorkResponse/SubmissionResponse bytes include settlement ID and
  recipient. Test wrong/missing money binding on the first Issue/Work/Submit/
  Lookup success before any Balance call; no work/receipt is disclosed. Wrong
  Issue/Submit success stays unknown. Embedded original job/receipt bytes match.
- Credential2 enrollment/state/token/reuse domain bytes, both audiences, issuance
  lifetime30days, exact replay, generation/history caps, expiration and revoked
  history across reopen. Original V1 outputs stay unchanged.
- Outbox2 config/marker/meta/scan/entry modeled bytes with empty/nonempty Merkle
  branches and separate pool/fighter states. Reopen changing either limit in
  either direction refuses daily; retain V1's intentional limit behavior.
  Daily metadata appends required issue_pending bool: false/true positives,
  missing/null/string/numeric field and true+scan/cursor refusals, true with
  original pending e/h/p rows permitted. Existing counters remain exact.
  Daily protected local credential2 canonical record/final-LF/2048-byte frames
  and both audiences; V1 record refuses daily and daily refuses V1.
- Money strings: 0, 2^53-1, 2^53, MaxInt64 and refusal at MaxInt64+1; negative,
  leading-zero, exponent, float, whitespace and numeric JSON refuse. Per-payment
  monetary cap stays exact. Reduced/nonreduced fractions (including valid MaxMoney+1 outstanding shapes),
  zero/unknown reason,
  available>confirmed-reserved and report0 edge cases. Observation time above
  253402300799999 refuses independently of valid full-int64 paid/reserved totals. Long allowed fraction
  components and maximum item/response frames must remain within unchanged caps.
- Captured pages: empty history;1/8/9 slots; zero matches in a middle page;
  pending at global high_batch versus a forged first-on-later-page pending item;
  nonzero-revision/zero-batch empty capture; same gym absent/present; lifetime sequence beyond366;
  beginning/middle/end valid cursors; skipped cursor is merely selection;
  mixed-zero/out-of-range/overflow counters; duplicate/reordered items and
  equal-day/non-descending CreatedAtMS; wrong
  source/config/gym/recipient; stale revision/high-batch; unknown error literal.
  Compare exact full balance independently, never infer it from the current page.
- Daily HTTP permits only bounded outer SP/TAB/CR/LF around an exact canonical
  request/response/error body. Check total size before trimming; internal spacing,
  extra values, other Unicode whitespace and cap-plus-one framing refuse. Keep
  strict config bytes and protected one-final-LF files as separate parsers.
  Balance/error envelopes retain65536; payout pages retain131072. Literal daily
  ErrorResponse cases bind stale_capture=false/HTTP409 and unchanged inherited
  outcome_unknown=true and busy=false mappings; wrong flags/unknown codes refuse.

## Meaningful concrete regression groups

**Original owners and setup.** Fresh daily constructors reject V1 before file or
network effects; all V1 constructors reject daily. Real schema16 daily creation/
open still uses the original owner and existing guard. Prefix interruption and
READY replay preserve exact selected files, no incomplete-wallet repair or
missing established marker recreation. Owner-bound scheduler binding refuses
foreign/audit-only/closed Store even when the pure digest matches. Listener/intake
cannot start after a failed child audit/bind. Cancel/Close fail retains the exact
runtime/backend/store/file owner; tests must report unexpected cleanup failures.

**Session and transport.** Use real poolminer daily owner and TLS server/client:
wrong audience/gym, expired/revoked credential, mid-call rotation/logout and final
active recheck, close during work/disclosure. Validate actual TLS CA/SPKI/host,
strict routes/content/body bounds and no bearer on ownership. No nested account
lease around a concrete call. Lost Submit acknowledgment only recovers the same
nonce. First-success money-binding mismatch never reaches hashing, receipt
persistence or signer. Issue unknown is not blindly retried. Capacity/nonmember requests cannot
turn healthy global intake into an integrity incident. Public endpoints never
expose original backend or signing handles.

**History authority.** Real daily ledger read uses full source/index audit before
any page, original session at disclosure and immutable b/bs ordering. Alter a
late/unrelated key or credit and require total refusal, not a partial page.
Between pages, a legitimate commit changes Revision and returns stale_capture;
fresh capture gives a coherent result. Multiple empty matching pages still
advance through at most8 slots. Complete traversal reconciles exact current
paid/reserved totals with recorded items. Exercise pending delivery state changes
without rewriting the immutable batch body. No writes/node/clock advancement from
balance/history reads. A synthetic >366 sequence codec case is not a financial
provenance claim; use independently constructed valid ledger history if claiming
that operational boundary.

**Worker and runtime.** Actual daily attachment selects the original existing
wallet/session/recipient: unchanged local source markers, no allocation when READY
is missing, same remote enrolled payee, exact saved outbox candidate and Mining
lease/vault epoch before signing, final original view check, logout/rotation/Close
while signing. No generic key getter or fixture fallback. Remote daily Config2/
marker2 and IDs get independent old-local/new-remote binding vectors, including
wrong OriginalSourceBindingID, equal-looking foreign session and changed payee.
The real headless launcher must exercise this adapter rather than an unused API.
Original daily worker/outbox pending-first reopen, nonce
reservation before hashing, pool-only/prospect-only/block outcomes, no automatic
hashing after expiry, exact unknown recovery, capacity-paused pending recovery,
and failed cleanup. Runtime daily budget uses complete original destination set,
full constructor audits before paused attachment enables, exact derived maximum,
issuer byte holdback, ordinary drain with ongoing round refresh/financial recovery.
Whole Sync remains atomic; no partial tail becomes an early daily payment.

**Unknown Issue across restart.** Use the original daily outbox and counted client:
prove premark sync precedes the sole outbound Issue, then drop the response and
require issue_pending=true with no scan and no second Issue/hash/nonce reservation.
Close and reopen all worker/outbox owners; repeated Start stays paused/issue_unknown
while a pre-existing pending entry still reconciles by exact identity. Its daily
attachment projects paused/outcome_unknown without changing the V1 sanitizer.
Prove the outbox gate is available during a blocked network call, and completion
rechecks the original live attempt before commit. Cover crash
before dispatch, after dispatch, after valid response but before scan commit, and
before/after-sync commit uncertainty; existing poison semantics and exact reopened
valid state decide recovery, not an error return alone. A bound success atomically
stores scan/cursor and clears; a valid already-expired success is persisted before
normal retirement. Only original-live-attempt exact trusted busy/rate_limited
OutcomeUnknown=false may sync-clear/backoff; unknown busy, wrapped/mixed error,
wrong binding, expired attempt token and cancellation never clear. Failure of the
clear write must not dispatch again. No latest-work adoption, manual clear, fresh
outbox fallback or automatic retry on a different owner. These are original-owner
fault/lifecycle controls, not a new server idempotency claim. V1 retry behavior and
metadata remain unchanged.

**Scheduling and D84.** Original real scheduler starts with pending recovery even
before due; original UTC cursor and restart do not create another day slot or
signature. Whole ready candidate that exceeds shape yields transaction-capacity
in authenticated daily balance/history, no subset, no fee charge/debit/sign/day
occupation. Unknown reason still refuses. Existing source-tamper, raw-body,
credit/funding, immutable-A and legacy tests remain active.

## Two actual paths, neither replaced by fixture passes

A. **Ordinary public work → scheduled on-chain payment.** Original headless
command/Setup2, genuine local TLS ingress and poolminer2 credentials, daily
workers/outboxes, original normal nodes and schema16 backend. Observe real
work/fighter receipt separation, exact miner payees, stopped same-outbox reopen,
rotation/revocation and original pending recovery. Reach60 confirmation and
actual coinbase maturity, wait for an independently selected real UTC due minute,
and observe one complete scheduled payout. Lose its broadcast response and
restart in a fresh process: exact original signed raw/txid/inputs/outputs/fee/day,
no second signature/payment. Read balance and all captured payout pages through
actual TLS and reconcile. Prove current30-second dispatch budget at selected
workload; do not substitute clocks, increase deadlines or weaken full audits.

B. **Full intake → original late canonical winner through the same transport.**
Reuse Set78's independent tiny true-derived ceiling, original pending A and three
original jobs. An explicit delayed client holds a normally mined original B;
it does not ask the ordinary worker to continue an expired scan. Fill the derived
ordinary limit through authenticated public calls, prove rejected issue/share
has no effects, and preserve exact physically absent B evidence. Wait real expiry,
make B canonical, submit via TLS, then fresh-process reopen. One exact B receipt,
round and credit at60, none at59, unchanged A body/claims/raw/day and no expired
fighter. Preserve the separate immature B/no-advance and whole-payout constraints.
Use exact unknown-outcome recovery; do not change the server's conservative Issue
classification merely to simplify an assertion. Root freezes timings/identities/
economics and process ownership before execution.

Both actuals use only published synthetic custody in isolation, never production
keys/host deployment. Retain every failed attempt, source/profile/evidence identity
and original owner/process group cleanup result. Passing Set78 direct-owner actual
is prerequisite evidence, not proof of these new TLS/runtime paths. The prior
721-input/64-payee payout is financial-shape evidence, not sustained64-miner load.

## Closeout decision

Run finite normal/race, explicit cheap/no-CGO profile refusals where supported,
vet, paired builds and old mode regressions; require named positives so skips do
not become passes. Then actual A/B and exact-tip CI. Report selected limits,
derived intake, measured source/job/share/outbox consumption, real due/maturity
and joined restart evidence. A bounded invited headless pilot may be useful before
retirement exists, but no continuous/physical-full/all-future-nonce guarantee.
Native route, deployment configuration and D74 remain separately visible gates.
