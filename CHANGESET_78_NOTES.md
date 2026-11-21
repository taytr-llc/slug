# Change set 78: finite daily intake reservation

Set78 connects the independent daily liability bound to one original Source2
pool controller. Ordinary capacity is derived from selected original source,
round and settlement limits, with headroom for an unseen canonical winner.
No wallet, source pin, journal encoding or payout policy changes.

`AttachDailyOperatorAdmission` owns the exact configured gym set and starts
paused. Complete source/index/window audits precede capacity classification;
nonmember requests cannot alter drain state. Issuance reserves receipt bytes
before touching the issuer. After drain, new retention requires fresh canonical
proof and a whole-history budget check, while exact saved-row recovery stays
available. The existing single synchronized commit, session/clock fences and
unknown-outcome rules remain authoritative.

The maximum ordinary envelope is 4,095, not an unlimited source or a throughput
claim. Source2's physical/work/byte limits, signed authority limits and selected
financial ceilings still apply. Paid and pending obligations remain counted.
D84 whole-payout deferral and the original Store/backend signing capability are
unchanged. See [the contract](POOL_DAILY_ADMISSION.md).

## Qualification status

- Independent capacity oracle: **PASS**, identical Python 3.14/3.9 output,
  SHA256 `2db086115f51b0c7e22d2fd7688ad5b46c56e238fa1346d709c9a19bebb7aa78`.
- Independent actual identity/economics oracle: **PASS**, identical Python
  3.14/3.9 output, SHA256
  `1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f`.
- Local build, normal/race, cheap refusal, vet and formatting: **PASS**,
  composing the preserved first run with the affected test successor below.
- Normal-PoW seed smoke/restart: **PASS**, including unchanged source/profile,
  height61/checkpoint60, clean restart and no public-chain mining.
- Normal-node/schema16 fresh-process actual: **PASS**, composing the retained
  execution with the source-bound checker3 correction below.
- Set78 exact-tip chain/wallet GitHub checks and Linux evidence: **PASS**,
  all seven jobs at chain `1f086601647c00e33fad1dca9275aa63b18caa07` and
  wallet `dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`. The downloaded Linux
  report passed in 547.351 seconds with identical before/after source heads,
  mandatory daily tests present and clean process joins. See
  [remote evidence](docs/evidence/set78/ci.json).

Closeout attempt1 passed all 17 builds and the pure-capacity, poolshare, round
and scheduler normal suites. The new cross-owner kernel test failed because its
controlled node reported a different tip hash at zero descendants; the reader
correctly refused that observation. The failed run and original fixture are
preserved. Only the fixture tip at equal height was corrected; production,
economics and independent expected values did not change.

Attempt2 passed the affected settlement normal suite (30.304 seconds), full
accounting race (315.094), desktop operator normal/race (42.586/272.355), cheap
refusals, vet, formatting and diff checks. Each attempt bound 7,431 source inputs;
source/profile comparisons and process joins were clean. Earlier unaffected
passes were retained rather than relabeled as a wholly passing first run.
These controlled-node results do not replace the actual acceptance below.

The direct-owner actual passed in 429.479 seconds, including a separate recovery
process (47.78 seconds; cold source open 0.740 seconds). It filled the true derived
ordinary ceiling at two receipts, then recovered a third original canonical
winner after real expiry. Pending A's exact indexed body, inputs, claims, outputs,
fee, raw bytes and txid survived the single owner reopen. Lost-reply recovery
needed no second send; B added exactly one credit at60 descendants and none at59.
One original A broadcast, the occupied same-day slot and immature B funding were
preserved. All original owners and both nodes joined cleanly.

The actual execution passed its Go assertions, but the original outer checker
and checker-attempt2 failed on evidence-shape assumptions. Both failed reports
remain preserved. Checker3 accepts the declared empty pre-forward txid, optional
empty Report.reason and AccountingPage's intentionally cleared mutable block
observations; full receipt equality across restart and exact payout bytes remain
required. It verified all 2,486 source inputs, profile, binaries and public
evidence unchanged, without rerunning the actual. This is a bounded financial/
restart proof, not continuous mining, arbitrary future recovery, public transport
or real-UTC scheduler acceptance.

The smoke built the captured source and synced the existing seed at height61,
verified the signed height1 checkpoint with60 descendants, chain/activation and
stale-template checks, then restarted cleanly. Only two isolated blocks were
mined before connection; no public-chain mining occurred. Source/profile/binary
checks passed and both process groups joined without signals.

Committed summaries: [local qualification](docs/evidence/set78/local.json),
[actual composite](docs/evidence/set78/actual.json) and
[seed smoke](docs/evidence/set78/smoke.json).

The predecessor Set77 has all seven GitHub checks green at chain `21da8ea51de2`
and wallet `dfc5d9dd7e0f`; those results do not qualify Set78. Controller test
cleanup was tightened before execution to report original Close failures; its
predecessor is preserved. Production review alone is not runtime evidence.

Explicit V2 public TLS/client/worker and operator/native composition remain
separate, as do signed-authority capacity, continuous retention, sustained
ordinary traffic and hosted scheduled payouts. No login or UI organization is
added. Public pool operation and payouts come first; the joint D74 review is last.
