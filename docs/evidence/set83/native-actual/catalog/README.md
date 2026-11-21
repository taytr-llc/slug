# Original paid-root catalog reuse

Source/public-index review only. No actual owned-root, wallet DB, credential or
outbox tree was read by this reviewer. Root executes all capture/audit actions
only after the relevant original controller and attachment owners have joined.

Reuse `set79-prep/public-daily-headless/run.py` functions `fixture_tree` (line244)
and `fixture_after` (line311), with `runner_support.identity` and the identical
Set81 `fixture-catalog.json` SHA2637978c. The catalog binds268 original baseline
files and226 immutable copied files. `fixture_tree` limits the inventory to2000
entries and64MiB. `fixture_after` preserves the exact directory/mode inventory,
checks all226 immutable files, and permits file changes only in these existing
account/wallet lifecycle stores:

- Exact file: `fixture-provider/state.json`.
- Prefixes: `authority/db/`, `coordinator/db/`,
  `fixture-provider/instance-lock/`, `hosted-vault/db/`, `instance-lock/`,
  `shared/gyms/incrementing256/owned-wallet/inventory/db/`, and
  `shared/gyms/incrementing256/owned-wallet/wallet/`.

Capture the current closed paid root before and after the new execution:
`/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5/owned/owned-gym`.
Compare both captures to the original immutable catalog and compare each other
with the same permitted changes. Retain the original root device/inode/owner/
mode and refuse symlinks or directory inventory changes. This does not assert
byte equality for mutable account/session/vault/wallet database files.

Do not call `fixture_after` with attempt5's old artifact destination: its final
write targets the already-retained `fixture-after.json`. The inherited writer
uses exclusive creation, so that call refuses rather than overwriting. Adapt
only the evidence destination into the fresh Set83 artifact directory. Do not
rerun fixture preparation or copy/reset any paid owner tree.

The remote attachment is separate:
`/Users/taytrrip/slug/artifacts/set82/hosted-actual-attempt5/remote-attachment`.
Its only permitted physical file churn is under `outbox/db/`, where LevelDB
Open/Close can change journals and manifests even without a new logical entry.
All other files, complete directory inventory/modes and original root identity
must stay exact. In particular preserve `SLUG-REMOTE-POOL-INTENT.json`,
`SLUG-REMOTE-POOL-READY.json`, and `outbox/SLUG-POOL-WORKER.json`. No blanket
attachment prefix allowance is justified.

Physical DB churn is allowed only together with a separate full logical audit.
The already qualified `hosted-actual-5/proofclient/replay.go` uses the same
original attachment directory flock and `readOutbox` from `outbox_audit.go`:
complete selected owner audit/Close, then bounded readonly StrictAll projection
of every q/e/h/p/meta/scan/cursor key/value. The resulting scalar `outboxState`
includes all-row and request hashes, active request, scan/cursor, marker,
sequence/count/logical/reserved-byte controls and readonly unchanged evidence.

Use the exact already-built proofclient5 `replay --phase after --sequence 1`
with original attempt5 selected public config and protected credentials, under
the new root execution's remaining deadline. Its internal60s/individual20s
bounds remain; root must supervise it inside the common300s window. Do not reuse
the expired attempt5 proof-control wrapper or start another Issue. The host must
still be running for its historical Lookup/exact Submit replay. The native
controller must already be fully closed before that replay obtains the flock.

Require new replay `passed`, `clean_join`, `selected_inputs_unchanged`,
`same_original_receipt` and `new_issue_calls == 0`, then exact equality of both
new `outbox_before` and `outbox_after` to the retained attempt5
`proof-replay-after.json` `outbox_after`. Compare complete `rows`, `work`, `nonce`
and `receipt` as well. The retained public report is26855bytes,
SHA65f48981afe3f86bd5a3e3ca00c7bb7a81b117ba1b9388d6140908ad21ac8095.
Its records2/pending0/requests3 and complete rows SHA
cc273bbb94ca68fd5300da96ca18b10b13e18dbc3988b5f9c9957be02bbbe720
are supporting cross-checks, not substitutes for the full comparison.

Financial host audits and original process/infra joins remain separate. These
catalog checks do not establish live wallet readiness, payment authority or a
new successful runtime result.
