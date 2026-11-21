# Public DAILY Path B actual source

This external snapshot adapts the qualified Set74/Set78 late-winner harness for the fresh, explicitly selected Set79 DAILY headless runtime. This readiness successor has not been compiled or executed; prior failed attempts and exact source snapshots are preserved. Root owns the sole build/runtime lane and the outer runner.

Executable interface:

```
publicdailyactual --node /abs/slugd --operator-binary /abs/slugpoold \
  --artifacts /abs/fresh-owned-root \
  --setup-reference /abs/public-daily/reference-v1.json \
  --economics-reference /abs/daily-admission-actual/reference-v1.json
```

The selected inputs are pinned in the source: public DAILY reference `a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3` (3,176,583 bytes), and Set78 actual economics `1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f` (29,980 bytes). The two-enrollment Setup is selected by name and its original canonical bytes and ID are checked. The synthetic wallet mnemonic comes only from that published reference. No user wallet, private checkpoint key or existing runtime root is read.

The source changes only ephemeral loopback transport/certificate selections and the future real UTC minute before constructing the new root. Authority, source, funding scripts, enrollment identities, selected economic limits, maintenance intervals, custody intervals and retries are preserved. Normal PoW, the original 900-second context, the outer 1,080-second allowance and maximum 180 normal blocks remain; the scenario requires final height 163. No test clock or manual settlement call is used.

The actual flow keeps A as the first pool reward, C as the sole ordinary nonblock row, and B as the previously unseen canonical winner recovered after real expiry and ordinary intake 2. Three issued jobs survive restart; retained rows are 1→2→2→3. B includes the exact A payout transaction, so its independently expected reward is 1,000,000,141 atoms. A has one input, payment 990,000,000, fee 141, and change 9,999,859. B's two net claims are each 99,000,013,959/200; these are outstanding amounts after A is paid, not lifetime earnings. No fighter admission or automatic worker/outbox claim is made.

The original node relay loses the first successful A send response. It then refuses only `getrawtransaction(A_txid, 1)` until the original parent runtime, wallet and online authority have joined. Every other RPC is forwarded unchanged. This explicit observation-only fault preserves the synchronized A reservation while B becomes canonical; it is released before the first production CLI. All attempted sends are recorded before forwarding, and final evidence requires exactly one original raw send and one acceptance.

Two separate `slugpoold serve --daily` lifetimes reopen the same roots and selected files. The first performs exact B duplicate/lookup/work recovery and original A recovery, then joins at B59 for a full closed-store audit. After one final block, the second imports B at B60 and joins. Audits open the original issuer→DAILY pool→DAILY round→DAILY settlement read-only chain and fully authenticate it before bounded public `head`, `report`, `b/1`, `bs/1`, `c/1..2`, `e/1..3` projections. No runtime or CLI remains open while those projections run. The complete immutable signed body SHA/bytes and original A credit are compared at every phase; only delivery state may advance.

Public phase files are `01-runtime-listening.json`, `02-A-block.json`, `03-A-pending.json`, `04-full-intake.json`, `05-expired-absent.json`, `06-B-pending.json`, `07-B59-restart.json`, and `08-B60-final.json`. `actual-report.json` contains completion, exact counts, source reference identities, all attempted/accepted broadcasts, explicit relay fault, two original CLI process cleanup rows, two node cleanup rows and the final pass conjunction. A failure retains the owned tree; it is never deleted by this executable.

The source uses the root-qualified DAILY scheduler accounting refresh after its same-day payment cursor is complete. It retains the demanded B60 assertion and does not bypass the scheduler with a direct ledger call.

Attempt2 reached the real scheduled A broadcast and immutable reserved audit, then failed the reopened B/C readiness phase. Its runtime and nodes joined cleanly. The combined readiness/Issue allowance was 20s although the selected maintenance interval is 30000ms; one legitimate maintenance retry can outlast that allowance. The final paused reason was not recorded, so the precise production cause is not claimed. This successor separates a 90s read-only readiness wait on the same runtime from the unchanged 20s Issue operation. Each readiness observation is retained in the original runtime's public completion evidence, and premature Run completion remains fatal. Issue still retries only exact definite busy/rate_limited responses; no unknown retry, replacement owner, request deadline, selected configuration, 900s parent or 1080s outer bound changes. The complete predecessor manifest/files are preserved under `pre-startup-readiness/`.

This is a finite two-gym transport/runtime proof. It does not establish physical-full recovery, continuous 64-miner operation, deployment, account login, or wallet history migration. The unchanged `node_helpers.go` is copied from the committed late-winner harness; main/flow/audit adapt only this selected scenario. Compilation, race behavior and runtime evidence remain pending until the root executes the frozen snapshot.

Balance-wait diagnostic successor: the same four original waits now retain their names, selected budgets, start/end/context deadline, total existing reads and last 32 public outcomes in `balance-wait-evidence.json`. Each outcome is only a closed code/unknown flag or the complete authenticated daily balance. Deferred persistence includes bounded-wait failure. No request, wait predicate, polling interval, credential, economic assertion or deadline changes. The runner binds this artifact on success and retains it on failure; a refusal is an observation, not proof of its internal cause. Earlier actual failures and their exact source remain preserved.
