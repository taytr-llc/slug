# Change set 80: exact-request issuance recovery

Status: implementation is on `slug-pool-issue-recovery`, stacked from Set79
`fdd40d8edd9a685613f1f667f0074e56ec4e7da5`. Focused checks and actual
expiry case B and payment case A pass. Complete affected normal/race regression
coverage passes. Exact-tip CI remains pending. No deployment or public-host approval is claimed.
Set79's seven exact-tip CI passes are recorded in `docs/evidence/set79/ci.json`.

Set80 adds an explicitly selected recoverable issuance mode over the unchanged
DAILY economic configuration. A durable request ID and exact template intent
precede issuer effects; the original issuer atomically binds that request to one
job or an immutable negative seal. A lost response or restart resumes that same
request. It cannot adopt the latest job, generate replacement entropy or extend
the old job's expiry. Fresh selected markers and wrappers prevent implicit
adoption of V1 or plain DAILY roots.

## Implemented scope

- The original selected issuer/source owners retain complete request/job/work
  reverse references and audited counters. Opening performs validation, not
  network repair or issuance. Authenticated Resume advances legitimate retained
  states. A never-published job that expires resolves to `issued_unused`; work
  already committed remains exact historical work after expiry.
- Existing signed limits and finite source/outbox budgets reserve request and
  job growth before effects. Old unsigned-admission reserve remains additive.
  Source and issuer census checks detect orphaned records; no trusted persisted
  cache, raised economic ceiling or replacement authority is introduced.
- `/pool/v3/` adds exact Resume/Lookup request/result envelopes and selected
  profile binding while preserving embedded jobs, submissions and DAILY money
  bodies. The concrete client mints private response provenance only after full
  validation. An original protected Mining/session wrapper remains in the call
  path; a decoded disk response cannot mint live client authority.
- The selected outbox retains its request before dispatch and atomically records
  an authenticated exact result with scan linkage or terminal completion. Stop,
  restart and changed account/attempt generations cannot revive an old result.
  Existing pending pool/fighter delivery remains recoverable.
- Explicit `--daily-recoverable-issue` composition retains the original wallet,
  account, recipient, session and schema16 signing capability. V1 and plain
  DAILY defaults remain strict. No new helper/native UI protocol is added.
- D80 original-job canonical recovery, original pending-payout recovery and D84
  whole-ready-payment deferral remain unchanged. No subset, surcharge, reserve
  advance, additional daily payment or changed share allocation is authorized.

## Qualified checks

The archived component reports and copy manifests retain each original report
by length/hash, with its exact test pattern, source/profile identities, elapsed
commands, skips and cleanup result. These are focused selected tests;
these are not a claim that every historical package test was selected.

| Scope | Normal | Race | Selected cheap |
|---|---:|---:|---:|
| Issuer/source core | 5.650 s | 14.870 s | 4.680 s |
| Transport/ingress | 4.490 s | 11.112 s | 1.649 s |
| Worker/outbox | 2.045 s | 10.315 s | 2.039 s |
| Desktop attachment/controller/headless | 15.047 s | 51.074 s | 11.384 s |
| Operator composition/provisioning | 5.559 s | 46.859 s | 9.549 s |
| Financial/D80/D84 controls | 307.192 s | 322.461 s | Separate existing refusal scope; no new financial cheap pass asserted |

- Independent main reference: PASS under Python 3.14.3 and direct CLT Python
  3.9.6, byte-identical 6,134,980-byte output `4bbe4eb9…dac91`. The producer,
  allowlisted public inputs, failed predecessors and qualification are already
  installed under `testdata/issue-recovery`. The modeled byte/state/bound cases
  are not independent live cryptographic or runtime authority.
- Independent actual wrapper selection: PASS under both interpreters,
  173,644-byte output `0f0890e5…f3d10`, retaining the original Set79 reference
  economics. This qualifies the expected selection, not the actual A/B runs.
- The financial fixture uses original selected stores, normal PoW and real
  five-minute expiry, fills the derived ordinary bound, reopens the original
  owners, credits the canonical late winner once and retains the reserved A
  body. Its controlled node fixture is distinct from the standalone actuals.
- Case L: PASS, 3.021 seconds. The opt-in child is killed after original outbox
  intent sync and before network dispatch; the same durable null request survives
  full reopen and later dispatch. This does not prove the operator proposal gap.
- All 17 module builds, accounting/operator vet, gofmt and diff checks: PASS
  with 7,940 bound sources, unchanged profile and joined command groups. This
  precedes the separate test-only cheap-profile guard. Full selected desktop
  cheap attempt 3 passes afterward with three top-level controls and four
  normal-profile skips; attempt 2 was only the single explicit refusal control.
- Normal-node smoke: PASS. Two isolated blocks check the 10-SLUG reward and
  report-only checkpoint conflict; read-only public synchronization reaches
  height 61/60 checkpoint descendants and survives joined restart. No public
  chain mining occurred.
- Actual helper normal compile: PASS. Case B passes with the independent public
  checker in 342.741 seconds: real five-minute expiry, the same original request
  and job after a process restart, terminal `issued_unused`, no work/share/birth/
  credit/payment, and unchanged original stores. All processes join.
- Full affected normal regressions pass: 993 top-level tests, with documented
  natural skips. Financial, core, transport, worker and operator full race
  regressions also pass. The initial full desktop race attempt reaches its
  cumulative 600-second package alarm before all tests finish. Four subsequent
  fixed pilot partitions pass, retaining 600 seconds per process and every
  top-level Test/Fuzz seed target. Combined with the same source-bound complete
  accountpilot/CLI results, all 595 race targets finish: 572 PASS, 23 natural
  top-level skips, and three documented subcase skips. No test is omitted; the
  initial failed aggregate stays failed. The real-expiry financial
  case is separately passed once per mode and is explicitly excluded from the
  full financial rerun.

## Preserved failures and corrections

Every attempt remains failed where it failed; later results are separate records.

- Reference attempts 1–3 stopped before output on an omitted names-v2 descriptor,
  an omitted remote-source profile in the money-envelope selection map, and an
  assumed HTTP status on deliberately invalid inherited error frames. Exact
  independent inputs/schema corrections preserve all inherited values, labels,
  economic limits and case counts. Attempt 4 produced the correct 3.14 output but
  stopped at the system Python launcher diagnostics; attempt 5 used the exact
  installed CLT Python 3.9.6 directly and passed both interpreters.
- Core compile attempt 1 compiled successfully but failed overall because the
  runner wrote an exclusive final source file twice. Core normal attempt 1
  exposed a test consumer's four-input/six-input corpus-shape assumption. The
  finalization and test-consumer corrections are preserved separately. A narrow
  selected-constructor failed-Close retention change was also source-reviewed;
  it does not alter stored bytes or authority.
- Transport normal attempt 1 selected clients from missing auxiliary metadata
  on deliberate wrong-profile vectors. The test now uses their unchanged base
  binding, with no parser relaxation or oracle change. Cheap attempt 1 could
  not bind the local TLS test socket; the subsequent same selected pattern
  passed without a product change.
- Desktop normal attempt 1 used an expired published credential in a live-owner
  test and HTTP/empty CA where the selected client requires HTTPS/trust. The
  live test fixtures now use a future expiry and real test TLS certificate;
  pure canonical reference cases remain exact. Cheap attempt 1 ran a normal-only
  config positive; the explicit test guard and separate refusal are preserved.
- Financial normal attempt 1 accidentally included the existing mutable `report`
  row in an immutable-row comparison. The one-line test correction excludes it
  alongside the prior head/delivery-state exclusions. Whole batch/body/claim/
  payment comparisons remain; no product or economic expected value changed.

- Actual A attempt 1 completed the physical scenario and one 9.9-SLUG payment,
  but is explicitly disqualified: a checker source was edited and restored
  during the run. The root disposition overrides the mechanical PASS. Its
  complete evidence and coordination record remain archived as rejected.
- Actual A attempt 2 fails at a live receipt lookup before payment. The original
  generic assertion omitted the error code. A separate read-only diagnostic
  passes all saved-store audits and finds exactly one valid accepted source
  receipt matching the original outbox. It does not recover the omitted live
  error or qualify A. The separate successor adds observation without relaxing
  receipt validation, accepted errors or deadlines.

## Standalone payment recovery

Actual A attempt 3 passes in 440.362 seconds under a separate frozen diagnostic
helper. It proves the real standalone operator's pending/issued crash gap,
successful TLS reply loss, original outbox replay and original-wallet scheduled
payout. One accepted reward becomes one credit and one 9.9-SLUG payment at the
original UTC due, after real coinbase maturity. The fee is 141 atoms from the
operator cut; transaction `4184893ba689eaa908996c5b9e8d8fddcbc7b722e3ecf069a7298d6b86107393`
is confirmed, the original wallet increases by exactly 990,000,000 atoms, and
the isolated chain ends at height 104. Stopped reopen preserves the complete
outbox state, and historical request replay preserves the original work after
expiry. All original owners/processes join; 39 public evidence files, source,
profile, binaries, original fixtures and old histories remain unchanged.

The original checker remains mandatory; the supplementary checker verifies
closed-outbox client lookup traces against exact public relay responses. A3
returns the original accepted receipt and a definite `not_found` for a separate
pending row. The helper adds observation only: no product, retry, deadline,
receipt, financial or expected-output change. A2's omitted live error remains
unexplained; its retained valid rows and this separate passing run do not
retroactively qualify it.

## Continuing boundaries

Exact-tip CI remains pending. Actual B independently proves real expiry before
source publication, an unchanged `issued_unused` terminal and zero work/credit/
payment. Both actuals prove one intentional operator kill, complete original-
owner audits, precise child/relay/node joins and unchanged source/profile/public
identities. Case B's stopped local null request remains retained; local terminal
consumption is separately covered by the focused worker tests.

The next functional slice is retained fighter admission when only pool intake
is full: preserve new-mining drain, independent authority limits and authoritative
expiry. Continuous capacity and real hosted operation follow before onboarding.
The compiled node closure remains unchanged; no redundant seed rollout is needed.

These finite cases do not establish continuous retention, unlimited issuance,
sustained population load, public hosting or revised signed authority capacity.
Original finite limits, key/session custody and payout rules remain. D74's final
joint product organization/design review is still a separate release gate.
