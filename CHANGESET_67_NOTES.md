# Set67: authenticated own-payout history

Status: implemented and locally qualified. Branch `slug-payout-history` is
stacked from completed Set66 `247edc868a620dd39ac3bfdf678241066fa4f844`.
The normal Mac bundle is version 0.5.6/build 67. Commit and exact remote-push
bindings are retained outside the repository under `artifacts/set67/final-binding`.

## What changed

The existing pool balance panel now captures the signed-in gym's recorded pool
payments and presents fixed eight-row pages. A separate action observes the
selected original transaction output in the already-owned wallet. Both readings
retain their own timestamps: pool-recorded state and wallet-reported confirmation
are different facts. Neither reading authorizes another payment or proves that
an output is unspent.

The service verifies the existing journal and source before filtering by the
original gym. It checks the same-state balance, exact recipient, output index
and amount. The desktop retains the original view, source, recipient and native
action throughout asynchronous work. A failed refresh keeps the older dated
capture; a successful refresh invalidates its old selectors. Unknown replies
recover the exact action, never an older successful action. Stop, backup, reset
and Close cancel and join the independent payout task before wallet reuse.

The wallet adapter uses only the retained wallet and a narrowly bounded read.
It verifies transaction identity, selected output script and integer amount,
and requires the wallet's synced hash and height to remain unchanged around
that observation. Strict response and transaction parsers reject duplicate or
unexpected fields, malformed framing, invalid witness forms and excessive sizes.
Verified absence is distinct from unavailable, busy, malformed or unsynchronized
responses. The public history limit is 366 rows and 128 KiB; the conservative
independent body-size calculation is 90,468 bytes. Desktop scratch accounting is
bounded separately; it is not a claim about total Go heap usage.

## Deliberately unchanged

D58/D79 accounting, fee and payment rules; the worker and solo paths; source and
recipient pins; normal chain selection; mining controls; app logos and navigation.
No new payout, withdrawal, transfer, production custody, chain or league policy
is introduced. D72 View permits held accounts to read their own history without
granting a restricted capability. The final joint D74 design review remains a
release gate.

All 1,770 compiled node/control inputs are identical to Set66. The required
current-source smoke passed, so no redundant seed rollout was performed. The
qualified live release remains Set50 `592e143c`; this set is a topic build.

## Independent reference and actual evidence

The exact contracts are `POOL_PAYOUT_HISTORY.md` (SHA-256
`d219a14449b84aed9552e2679e9c9ecd15930349ae117455aeb36e86a4b36fc4`)
and `POOL_PAYOUT_DESKTOP.md` (SHA-256
`88c8ba350e44869b9c22f90cb2e168c011f351625c3ba750d70a5203ea91e9e5`).
Their original proposal headings are preserved; the separate preimplementation
freeze record binds the adopted bytes.

Public inputs were extracted from a read-only copy of Set61's qualified journal.
Original files and the extraction copy remained byte-identical. The independently
produced core/wallet corpus contains 34 valid histories, 43 negative values,
12 HTTP frames, 36 transactions and 37 modeled wallet observations. Its SHA-256
is `7aaf45191dc4ac5d34d5ab930c2e6461d50f389ac22611278a897da938cc1825`.
The independent desktop wire corpus contains 72 positive cases, 61 negative
cases and eight raw frames, plus lifecycle and amount-format models. Its SHA-256
is `4b3eafc010d515557097357a968bd007571c7126f7c0f15a2cd4184bcc88f802`.
Both reproduce with Python 3.14.3 and 3.9.6 and their portable producers.
Modeled observations are explicitly distinct from actual wallet reads.

Actual acceptance opened owned copies of the original four source/accounting
journals and chain with two owned loopback nodes. Fresh published-fixture wallets
located the original transaction
`eb3bd019318cb8ce5635794caae4b970fbb0480f905ea61e1033289a9ab3ed26`:
output 0 pays 49.5 SLUG to incrementing256 and output 1 pays 29.7 SLUG to zero256.
Legacy wallet RPC baselines were saved before invoking the new adapter. Both
wallets reported one confirmation at the original height-109 tip, before and
after full reopen. Authenticated HTTP results matched the independent expected
bytes. Original source trees, copied public markers and copied journal logical
keys/values remained unchanged. Writable LevelDB open/close may change copied
physical metadata; no physical invariance is claimed for that acceptance copy.
No scheduler, new settlement payment or mining ran. The existing published-fixture
account enrollment signatures are not payment signatures. All owned processes
joined normally. This is a historical test observation, not a current network
confirmation claim.

## Qualification

Evidence is retained under `/Users/taytrrip/slug/artifacts/set67`:

- `core-focused-attempt2`: normal, race, cheap-profile refusal and vet pass.
  `core-reference-attempt2/race.log` passes the independent history cases. That
  standalone log has no separate contemporaneous source manifest; later exact
  source bindings cover its current test files without retroactively claiming one.
- `wallet-focused-attempt1`: adapter, parser, strict RPC and independent models
  pass normally and under the race detector.
- `desktop-focused-attempt3` and `desktop-retry-final-attempt2`: original-view,
  lifecycle, wire, exact-action retry and concurrent dispatch regressions pass
  normally and under the race detector; final affected-package vet/format pass.
- `desktop-full-composite.json`: all six desktop packages pass. Four unchanged
  packages retain the full-suite result; the two affected bridge/native packages
  use their complete later rerun. The exact three changed files and both source
  bindings are recorded rather than calling an earlier run unchanged.
- `frontend-attempt3`: all 346 frontend tests pass.
- `static-attempt1`: all ten chain modules build, root/desktop vet, formatting
  and diff checks pass. Subsequent three bridge/native changes are covered by
  their final rerun; the revised integration harness is covered by actual
  acceptance. The original static record is preserved.
- `actual-payout-attempt3`: both original outputs, exact baseline comparisons,
  restart, source invariance and normal cleanup pass.
- `smoke-attempt1`: isolated two-block/10-SLUG reward, report-only checkpoint
  conflict, read-only seed synchronization at 61, height-62 activation checks,
  chain verification, stale-template refusal and clean restart pass. No public
  mining occurred.
- `package-attempt3`: exact normal-PoW Mac app and four executable build/signature
  checks pass. `package-helper-attempt1` passes both name routes, both fixture
  accounts, initial/reopened sessions, empty own-history capture, replacement,
  stale-selector refusal, logout isolation and retained recovery checks in
  117.05 seconds. Those checks exercise the actual bundled helper; they do not
  claim interactive GUI acceptance or a funded payout in that helper run.

## Preserved failures and corrections

The initial core Close test cancelled before proving source work was active;
its successor fixes test ordering. A reference consumer incorrectly required a
configured recipient for a valid empty history; only that test was corrected.
The initial producer lock named a mutable current reference file; exact Set66
references were frozen before producing expectations. The wire model's initial
zero-hash restriction was corrected before execution. Its first generated corpus
then exposed a synthetic transport-trust identity inconsistent with the inherited
public hash formula. The failed corpus and run remain archived; an independently
reviewed successor fixes the fixture without weakening production validation.

Actual attempt 1 used a read-only copied issuer where the share store requires a
writable handle; attempt 2 lacked the second loopback peer required for wallet
synchronization. Both failures and cleanup evidence remain. Attempt 3 adds the
required copied handle/peer without changing synchronization rules or mining.

Two exact-action retry defects were fixed: a definitive not-found response now
retires only its unresolved action, and pre-dispatch bridge contention retires
only a never-dispatched action. Dispatch reservation is atomic; possibly sent
requests retain recovery identity. Earlier passing packages 1 and 2 remain
archived as superseded by final package 3.

## Remaining scope

The app is locally ad-hoc signed, not notarized or distributed. This set supplies
no production issuance, new hosted provider, Windows runtime acceptance, current
ownership proof or final design approval. Continue the settled wallet/lifecycle
work next; fighter-transfer timing and general league matching remain separate
owner decisions.
