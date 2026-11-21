# Change set 77: selected live round maintenance

Set77 adds an explicit live read path for one original, fully audited round
Store. It removes repeated full audits from rotating maintenance reads while
keeping full `Sync`, financial validation and all selected limits intact.
Local component qualification passes; public runtime acceptance remains separate.

`poolround.Store.BeginLive` returns an opaque handle for that original owner
and verification epoch. `Live.List` reads at most eight committed records;
`Live.Refresh` checks the exact winning source entry before and after a fresh
node observation. Metadata and touched index/record bytes must match the
verified directory. Neither canonicality nor spending authority is cached.
The first 60-descendant confirmation and immutable round allocation survive
a later funding-conflict observation.

Full `Sync` still validates one complete captured source tail, the old prefix,
rounds, indexes and namespace before its single synchronous commit. Capacity,
cancellation or validation failure leaves tentative additions uncommitted.
Successful known commits update the directory under the same gate; uncertain
writes poison the Store and revoke its live handle. Close joins original work
and revokes the epoch. Full reads and `WalkSettlement` keep their complete
audits, including corruption outside a live read's touched records.

`Runtime.EnableLiveRoundMaintenance` is an explicit local selection before
first `Run`. The default path remains unchanged. Selected maintenance still
performs full `Sync` before its rotating refresh and retains the existing
capacity behavior for old rounds. An exact stale handle permits one explicit
full reanchor on the same Store, within the same operation deadline. Selection
uses the original action/Close lifetime; no replacement owner is introduced.
There is no setup, CLI, GUI or wire selector in this change.

D84 whole-payout deferral, exact pending-payment recovery, allocation, fees,
maturity and signing authority are unchanged. No source, wallet, settlement or
scheduler format changes, migration or capacity increases are included.

## Qualification status

All 17 module/build targets passed. Full round, settlement and scheduler suites
passed normal (51.112 seconds) and race (247.459 seconds) checks. Full operator
normal (42.447 seconds) and race (285.779 seconds) checks, the cheap-PoW refusal,
vet, formatting and diff checks passed. Each run bound 7,407 source inputs and
joined its owned processes cleanly. The independent Python 3.14/3.9 model outputs
match exactly; its projected arithmetic/pages do not grant source authority.

The first operator run failed a new test's one-balance assumption: the original
fixture configures two gyms. The corrected test verifies both exact identities,
earned credit only for the mining gym, zero for the other, and no reserved,
paid or available amounts. Every production input remained identical; the
failed attempt and original test are preserved. Passing accounting/build gates
were retained and the complete affected operator suite rerun in both modes.
[The evidence index](docs/evidence/set77/local-qualification.json) binds that
composite result. Controlled node observations and the immature funding backend
remain separate from actual node/payment or public operator acceptance.

Normal 256 MiB PoW smoke passed: two isolated 10-SLUG blocks, report-only
checkpoint conflict, read-only synchronization to the height-61 seed, finalized
height-1 checkpoint with 60 descendants, activation-height checks and a clean
restart. No public-chain mining occurred. The initial no-argument smoke command
was refused before effects and retained separately; the explicit build/run passed.
Exact-tip remote checks are reported separately.

See [the live-round contract](POOL_LIVE_ROUNDS.md) for touched-record limits.
Cold audit and full Sync remain history-dependent. Finite Source2 and financial
ceilings, explicit V2 public transport/operator integration, signed authority
capacity, V2 late-winner restart acceptance and sustained ordinary mining load
remain separate gates. Public pool operation and payouts precede onboarding;
D74's full joint product organization/design review remains last.
