# Change set 70 — own-wallet activity

Status: implementation and candidate qualification complete. Paired commits and
remote topic verification are recorded separately in `artifacts/set70/final-binding`.
No live-seed rollout or product-release approval is implied.

## Result

The Pool Pilot's existing wallet section shows incoming, outgoing and
self-payment records from one immutable local capture. Eight-row pages and
selected transaction/output details retain that capture's wallet/node tip and
observation time. Explicit Refresh replaces it only after a successful capture;
a failed refresh retains the previous dated view. Confirmations describe the
observed chain, not permanent finality or pool accounting.

The wallet reads public root/gym/profile identity, persisted synchronization
stamp, raw transactions and integer credit/debit records in one database read
transaction. It reads no private recovery material, unlocks no keys and allocates
no addresses. Reverse block-index and unmined metadata traversal are bounded;
there is no whole transaction-bucket scan or floating-point history RPC adapter.
Transaction parsing enforces canonical framing, bounded input/output/witness
counts, raw-byte limits and checked amount totals before generic decoding.

Captures contain at most 256 distinct transactions plus evidence of an older
record. Raw data is limited to 256 KiB per transaction and 8 MiB aggregate.
Stored index/record limits, a shared page/detail projection slot and logical
32 MiB overlap budget bound accepted work. These are logical accounting limits,
not a bound on the whole Go process's resident memory.

Six strictly typed commands carry full original View/source/root/gym binding.
The native app creates a job ID before Begin, retains its original helper and
resolves an uncertain reply only by querying that exact job. A previous capture
never proves admission of a new action. Cancellation, Stop, backup and Close
join actual capture/detail work before releasing the wallet. Terminal cancellation
or an exact missing-job response also retires the native reservation, allowing a
later explicit refresh. No network request holds an account lease.

Existing RPCs, wallet schemas, pool earnings, withdrawal authorization, fees,
consensus, custody, navigation and logos remain unchanged. The read does not
infer spendability or choose among competing unmined transactions. Fees are
reported only when every non-coinbase input is owned; unknown fees are null.
Five direct standard script forms produce addresses; unsupported scripts remain
empty rather than substituting a different payment script.

## Independent references

`WALLET_ACTIVITY.md` was reviewed and frozen before dependent implementation:
SHA256 `71bff8b11210aab89e324fc96d296a41e3a0248c5ae6e2293face8e51de51138`.
Eighteen public source inputs, including old funded transaction bytes, were
locked before outputs. Reviewed Python producer sources were locked separately
before running them. The new Go implementation never supplies expected values.

- Core corpus: 44 positive, 18 negative, 18 raw-framing, 27 storage and four
  budget cases. SHA256
  `4b3880f7facb9286502aa6a3bdcf38db7821a96e1d1b622f4d27a898a057598e`.
  Three negatives concern JSON/hex fixture parsing, whose information no longer
  exists in typed binary input; the Go semantic adapter tests the other 15.
- Wire corpus: 70 positive, 120 negative, 20 raw-framing, six lifecycle and nine
  formatting cases. SHA256
  `fd0f28ff06b4cb0502e7822ad401330941cdb71e9a4d4b5e31d5a017f911dc2b`.
- Both reproduce byte-for-byte with Python 3.9 and from their portable repository
  bundles: wallet `wallet/testdata/own-activity` and desktop
  `frontend/testdata/wallet-activity-wire`.
- Independent synthetic sizing established request/page/detail upper bounds of
  322/8,039/47,539 bytes, summaries 154,881 bytes, and conservative logical
  overlapping data/buffers 26,579,280 bytes before implementation. Sizing alone
  is not a valid transaction/wire-verdict corpus or a process-memory claim.

The original recipient first-seen time of one incoming payment was not retained
by the old independent evidence. It is explicitly unknown in that reference;
actual qualification compares its stored timestamp as a new bounded observation,
not as an independently predicted time. All transaction bytes and monetary facts
remain independently fixed.

## Qualification

Candidate checks use Go 1.27.4 with
`all=-d=ssa/generic_cse/off,ssa/lowered_cse/off`, bound by
`build-profile.json`. Artifacts are under `../artifacts/set70/`.

- All six wallet modules pass full normal tests, build and candidate-flag vet;
  1,076 inputs remain unchanged. The root module takes 352.269 seconds; the five
  nested modules pass in 1.787/0.233/0.660/1.630/4.609 seconds. Explicit recovery
  and withdrawal cheap-profile refusal checks pass in 1.304 seconds. Focused
  public-state/history normal/race/cheap checks retain 992 unchanged inputs.
  See `wallet-full-attempt1/result.json` and `wallet-build-vet-attempt1/result.json`.
- Typed protocol/bridge checks pass normal/race in 12.97/22.05 seconds.
  Final controller/runtime/projection race/cheap/candidate-flag vet pass in
  12.312/2.709/1.021 seconds with 4,163 inputs unchanged and exact compiler
  hashes recorded (`controller-final-attempt1`). Full normal qualification of
  all six desktop packages passes in 197.915 seconds (`desktop-full-attempt1`).
  Coverage includes original-view publication, failed refresh, resource overlap,
  exact paging and joined capture/detail cancellation through Stop, backup and Close.
- Final native normal/race checks pass in 3.405/5.616 seconds on the final
  cancellation/reservation implementation (`native-attempt2`). The original-helper
  guards also received finite source review. The exact macOS 0.5.9/build70
  package builds in 17.816 seconds with all paired source files unchanged.
- The exact packaged helper passes in 127.719 seconds across both name versions
  and fixture identities, initially and after reopening. It verifies empty
  genesis captures, whole-job equality on completed Begin replay, explicit
  replacement, prior-capture and old-account refusals, recovery and existing
  payout/withdrawal/league flows. Every owned process exits cleanly. Native file
  selection is emulated with held descriptors; this is not a visible-dialog check.
  See `package-helper-attempt1/report.json`.
- All 363 frontend tests pass in 1.957 seconds with 88 bound inputs unchanged.
  Exact signed amounts, old-view refusal, explicit replacement, uncertain Begin,
  terminal cancellation and selected details are covered.
- Concrete normal-PoW activity qualification passes in 11.233 seconds using
  exact copies of both Set68 funded wallets and their retained node chains.
  Each yields three records: payout, own withdrawal and the other wallet's
  payment. Raw bytes, credit/debit metadata and all projected monetary/output
  facts match the independent reference. Exact tip 110 is unchanged, repeated
  capture and complete semantic wallet state match, and original sources remain
  byte-identical. No new mining, payment, export or address allocation occurs.
  Owned processes close cleanly. See `actual-activity-attempt1/report.json`.
- All ten chain module builds pass with 2,217 inputs unchanged. The required
  normal-PoW HANDOVER smoke passes, including public-seed synchronization,
  report-only checkpoints, activation, stale-template refusal and restart.
  All 1,770 compiled node inputs remain identical to Set69; no redundant seed
  rollout is required. The live master remains Set50.

## Preserved attempts and limits

The first controller run found a test expected-value pointer alias and missing
early output-total validation. The full projection already refused invalid
amounts; the raw prewalk now enforces the same guard before generic decoding.
The test takes its own expected-value copy. Neither correction changes the
independent corpus. The failed attempt and successful successor are retained.

Source-collector setup failures (including an absolute generated Go test-main
path outside the repository), initial compile failures, and a report serializer
that treated the profile check's successful nil return as JSON null are retained.
They are not candidate passes; fresh bound successor runs establish qualification.
The earlier `controller-focused-attempt2` normal/race/cheap commands used the
frozen wrapper, but its report did not record compiler binary hashes. Its plain
`go vet` result is diagnostic, not candidate-profile qualification. The final
bound controller race/cheap/vet successor and full desktop normal pass above
close that evidence gap without relabeling the old report. Initial native
normal/race evidence predates the cancellation/helper fix; final native and full
desktop runs now cover the final source. Earlier pilot/bridge dependency deltas
are explicitly linked to final wallet normal/race and controller/desktop results.

This is bounded candidate evidence for the two published fixture wallets. It
does not provide real accounts, hosted evolving recovery, production issuance,
notarized distribution, a visible native-dialog check or the joint D74 review.
The next functional work is current-wallet to hosted-recovery integration.
