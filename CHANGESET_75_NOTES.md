# Change set 75: stream pool accounting and defer oversized daily payouts

Branch `slug-pool-capacity`, stacked from Set74 `e0e71ac0`.
This completes the first integrated capacity prerequisite; durable continuation
and public-host qualification remain open.

## What changed and why

Round synchronization and verification now stream the original accepted prefix
in bounded pages. The accountant retains the open window and duplicate guards,
checks every expected stored round/index plus the complete namespace, and only
then commits the one staged batch. It no longer builds a full history of jobs,
proofs and completed round projections before doing that work.

The concrete `WalkSettlement` read audits all source/storage evidence before
calling a visitor, holds one original gate through owned pages, and returns no
source config on failure. Settlement uses it for initial binding and credit
validation/import. New credits are staged until all old credits and the complete
source binding have been checked. Pending payout recovery retains its original
financial owner, raw bytes, claims, inputs, outputs and occupied daily slot.

Mike chose whole-batch deferral for transaction overflow, recorded as D84.
Funding observation pages preserve the entire ready set; a set that cannot fit
one daily transaction produces a visible `transaction-capacity` hold before
signing. Every balance carries forward, with no partial subset, extra fee, debit
or consumed daily slot. The reason survives service and desktop validation and
has a plain-language balance message.

A source review caught early capacity refusal masking old namespace corruption.
The implementation now finishes source/old-row/prefix/namespace validation before
reporting capacity. A regression pairs an otherwise identical full-byte-limit
case with and without an unexpected stored key.

## Deliberately unchanged

V1 config/source identity, receipt and round encoding, exact allocation/fee
arithmetic, duplicate guards, limits, source/custody ownership, consensus and
checkpoint behavior remain unchanged. No archive deletion, implicit migration,
repinning, production key generation or live deployment occurs. The 60-block
earnings lock, 100-block maturity, 1 SLUG minimum and no-reserve-advance rule
remain intact. D74's full joint design review stays last.

## Verification

Passed locally under the candidate Go 1.27.4 no-CSE profile: all six pool
packages in normal and race modes (130 top-level tests per mode), thirteen
explicit cheap-profile refusal tests, six focused desktop balance tests in
normal and race modes, scoped vet, JavaScript syntax and formatting checks.
The full-intake late-winner regression remains in the passing settlement suite;
the separate two-node actual proof at Set74 is retained, not relabeled as a new
Set75 actual run. GitHub qualification runs on the exact pushed branch commit.

The frozen V1 rational corpus remains the allocation oracle. A separate Python
producer derives native-SegWit full-byte/count boundaries without importing
product code; reproduction must match the retained fixture byte-for-byte.
Concrete normal-PoW regressions cover late winner/reopen, unchanged pending
payout, complete exactly-once new-credit import, cancellation and late refusal
without partial state, unknown/missing storage, page ownership and joined Close.
Conditional large-input inspection tests preserve complete selection and reject
later-page errors; they do not create an impossible V1 ledger.

Final commands, source identities and outcomes are recorded in
`docs/evidence/set75/accounting-stream.json`. The initial dependency run is
retained: local HTTP tests encountered sandbox bind refusal. The unchanged
loopback-enabled successor is recorded separately; the failed run is not a pass.
That successor also retained a harness invocation error: `candidate-go.sh` does
not implement a `vet` subcommand. The remaining checks use the existing explicit
`go vet` compiler profile after `candidate-go.sh check`; no product/test
assertion or timeout changed for either correction.

## Scheduler integration follow-up

Before Set75 closeout, source review found one omitted closed-reason grammar
in `poolschedule`: it rejected `transaction-capacity` instead of retaining the
D84 deferral. A new conditional scheduler-state regression failed against
`6539b3fb` before the one-reason fix. Complete scheduler normal/race tests and
vet then passed. The regression checks retained state round-trip, unchanged
daily cursor, recovery-first retry and refusal of an unknown reason. See
`docs/evidence/set75/scheduler-capacity-reason.json`; the original failed
reproduction is retained. Other accounting source is unchanged from the
initial local qualification. This is not a claim of a beyond-V1 actual payout.

## Remaining debt

The first slice crosses no lifetime cap. Global duplicate sets, per-round share
arrays, full-prefix work, monolithic settlement history, exact residual claims,
client sequence/history limits and signed publication limits still need the
explicit continuation work in `POOL_CAPACITY_CONTINUATION.md`. Current maximum
transaction shape is 256 inputs/256 payments with change: 46,383 full signed
bytes, below 100,000. Its overflow arithmetic tests are not end-to-end proof of
continuous capacity. Waiting alone need not clear a D84 size hold.

Next: explicit durable history/adoption preserving original jobs and pending
payments, then actual capacity-boundary restart and host/payout qualification.
Sign-in/onboarding follows that milestone; this change does not release a
friends build or approve production genesis.
