# Change set 52 — pool work validation

Work is on `slug-pool-shares`, stacked from Set 51 maintenance `03f9af14`.
This is functional pool preparation. App organization/navigation remains
deferred to Mike's final joint D74 review; its external draft is unapplied.

`POOL_ACCOUNTING_POLICY.md` records D79's per-block proportional rounds,
difficulty weighting, winning-share inclusion, daily settlement, 1% fee and
1-SLUG minimum. Earnings lock at sixty descendants; payment waits for the
original coinbase's consensus maturity. Reserve advances are forbidden.
Accounting, funding incidents and money movement are not implemented here.

`POOL_SHARE_VALIDATION.md` defines the small immutable `prospectwork` adapter.
Before runtime work, five independently derived normal-Argon headers and
thirteen target checks were frozen under
`prospectwork/testdata/pool_share_reference`. Expected bytes come from the
existing independent manifest-candidate reference and a standalone x/crypto
oracle, not the adapter. One supplied-context model covers all three misses.
The portable producer reproduced the exact corpus; external originals remain
preserved. Contract and reference were committed at `29a2f2f1` before runtime.

The adapter now prepares immutable manifest work, checks only a submitted
nonce, classifies pool/prospect/block targets independently, and produces a
target-independent duplicate key. It bounds its own checks to one synchronous
normal-Argon invocation and rejects the cheap test profile. Returned candidates
cannot mutate later checks. This is work verification, not durable acceptance,
fighter admission, account credit or payment.

Normal, race and cheap-profile-refusal `prospectwork` tests and `go vet` pass
under the recorded Go 1.27.4 no-CSE candidate profile. Evidence is in
`../artifacts/set52/validation-attempt1/report.json`; formatting is clean.
The tests cover frozen byte/hash/target boundaries, candidate ownership,
all-miss and pool-only outcomes, cancellation and hash contention. Independent
source review found no concrete defects.

There is no new caller, listener or deployment configuration. This library
slice needs no desktop rebuild or seed rollout; master/live remain Set 50.
The next functional step is durable issued-work and share acceptance, before
round accounting and settlement. The remaining D79 incident/rounding/fee
decisions must be settled before money movement.
