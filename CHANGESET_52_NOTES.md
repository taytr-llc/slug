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
preserved. Runtime implementation and focused checks are next.
