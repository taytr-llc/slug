# Opt-in actual daily financial integration

This source is a fresh 721-input/64-payee source2 → round2 → indexed ledger →
original encrypted schema16 wallet → normal node qualification. Authentication
alone uses64 immutable published fixture identities. It does not qualify the
V1 public service/worker,64 custody wallets, deployment, or a real-day scheduler.
All old default builds remain excluded by `slugdailyactual`; there is no tagged
production API. Two exported test helpers hold the original ledger gate: an idle
settlement-only clock selector and a full audited snapshot limited to721 credits,
two batches and their owned frontier. Node/work/expiry clocks remain real.

The independent reference was generated before Go test authoring in Python3.14
and3.9 with identical85,070-byte SHA256
9bcb4b64505e38de193411dcf51fb19f4d1332cbb2af53d21904663848cb4955.
Reproduce into a fresh path with `python3 reference/model.py --inputs reference/inputs --recipe reference/recipe.json --out /tmp/fresh-daily-reference.json`.
The existing input/source-ready hashes remain unchanged.

The root runner compiles a normal CGO0 node and CGO1 tagged test with the exact
candidate profile, offline cache and GOMAXPROCS=2, using an explicit GOWORK for
all chain and wallet modules. From the chain module the selected test is
`./poolsettle -run '^TestDailyActual721Inputs64Gyms$' -tags slugdailyactual`.
The bound compiled test gets `-test.timeout=35m`; outer group guard40minutes.
Env inputs are `SLUG_DAILY_ACTUAL=1`, `SLUG_DAILY_NODE=/absolute/slugd`,
`SLUG_DAILY_REFERENCE=/absolute/reference-v1.json`, and
`SLUG_DAILY_OUTPUT=/absolute/existing-public-artifact-dir`, and
`SLUG_DAILY_ROOT=/absolute/new-owned-fixture-dir`. The fixture path must not
exist; its existing parent must be canonical. It is created0700 and never
automatically deleted by the test, even after a failed owner close or node join.
TMPDIR must be an owned runner scratch directory. The outer owner retains the
exact fixture tree on both success and failure; only public reports are publishable. No ordinary CI command imports
the wallet. The actual test has its own30-minute context, individual operations
remain30seconds, cold source reopen5minutes, nonce search≤64/4minutes, and at most
924 canonical blocks. No runtime parameters change proof targets or consensus.

Sequence: one canonical pool reward → mature signed A → confirm A;720 distinct
canonical rewards cycling64 accounts → mature721-input B → lose B's actual RPC
response after forwarding → close every original owner → one fresh recovery process
opening the exact original roots once → recover B without resend → confirm B
→ Inspect one change with721 origins.
Same-day and repeated recovery cannot prepare a third batch. The complete A/B
input/output/claim/raw/txid/day/fee identity remains fixed. Fees141/51,236,
change9,999,859/7,209,948,623 and cumulative miner payments713,790,000,000 are
independent expected integers. Every source row is a real original job/winner;
there are no ordinary or fighter submissions and no manually inserted credit.

The public output contains actual-report.json and01-A.json,02-B-pending.json,
03-complete.json. All generated public synthetic seed/passphrase/wallet files
and normal nodes live only under the explicit fresh owned fixture root. Original owners close in dependency
order before relay/node joins; failed cleanup is qualification failure, not a
replacement owner. The root runner additionally binds full source/profile and
binary metadata, caps logs, owns the entire process group and checks it empty
on every path. Source-only authors did not execute this stage; the root-owned
qualification results below record actual execution.

## Separate-process reopen successor

Passed attempt3 remains valid in-process owner-reopen evidence. It did not
prove a cold process: wire.BlockHash retains pure Argon2 header hashes globally,
independent of the source owner's lifetime. This successor leaves the normal
cache unchanged and executes the reopen/recovery/final stage in a new address
space of the same compiled test binary. No expected financial values change.

The parent launches only `-test.run=^TestDailyActualReopenChild$` after all
original financial owners close. A bounded public `recovery-handoff.json` pins
the original fixture root, frozen reference, phase A/B hashes, original loopback
node/relay endpoints, parent PID/group and original scenario deadline. The child
opens only existing owners, creates no fixture entropy/jobs/nodes and never
starts a second runtime. Its same-day clock remains test-only bookkeeping.

The child inherits the original outer process group. The parent retains its
monotonic 30-minute scenario deadline and owns exactly one command Wait; the
child also receives that original absolute deadline, not another 30-minute
budget. Signal cancellation joins owner cleanup; parent cancellation requests
termination, then allows only bounded cleanup before killing/joining. Any
forced termination or cleanup failure fails qualification. Logs are capped at
16 MiB. The existing 35-minute test/40-minute outer guards, 5-minute source
reopen and 30-second operations remain.

`recovery-child-report.json` records distinct child/parent PID, inherited PGID,
original deadline, handoff hash, completion, one reopen, owner cleanup and
`03-complete.json` hash. The parent's `actual-report.json.recovery_process`
records the exact launched PID, matching child identities, exit code, joined/
forced flags and child-report hash. The outer runner must independently retain
and verify these public files in addition to the original financial reports.
Parent verifies exactly original A/B relay sends and attempts only after child
exit. Relay and both parent-owned nodes remain alive during recovery; they close
after child join. A child that cannot join leaves ancestor cleanup to the outer
process-group guard and cannot produce acceptance.

## Qualified result

Attempt4 passed the fresh-process scenario in 1,255.733 seconds. Cold source
validation took 142.534 seconds; the distinct recovery child exited successfully
with original owners closed. Two original A/B broadcasts only, exact reserved
economic bodies, all balances, 721 inputs, 64 payees and 721 change origins passed.
Both nodes joined cleanly. B contained 109,102 raw bytes, paid 7,128 SLUG and used
a 51,236-atom fee. Source/profile/public-evidence bindings stayed unchanged.
The paired evidence index retains the full report hashes. Attempt3 remains
preserved as in-process acceptance, and attempts1–2 remain preserved failures.
