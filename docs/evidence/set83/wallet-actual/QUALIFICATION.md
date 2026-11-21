# Set83 packaged wallet acceptance

**Result:** PASS for the bounded synthetic-wallet read flow, using separately
qualified machine-helper and native-window gates on the same normal-PoW package.
This is not real account custody, notarization, public hosting capacity or D74
release approval. No new mining, issuance, transfer or payment was requested.

Product commit: `7e1f6b7982a4af1ce765d9883417a20d17f46dcf`.
Paired wallet: `dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`.
The normal Mac bundle is version 0.6.1/build83, ad-hoc signed. Its app SHA256 is
`9ffffcd47239522e1e0960192d3423267d096187325c16b5bf09ca72c5c10cfd`;
its helper SHA256 is
`8c61109cd45107c5cc565769d9a2b414b8ab39c8e6774f3bfaf9e948e4e50f82`.
[Package provenance](package/BUILD.txt) records the clean sources and unchanged
Go 1.27.4 no-CSE build profile. All six [GitHub jobs](github-jobs.json) pass on
that exact product commit, including the full race job:
[run 44942648233](https://github.com/taytr-llc/slug/actions/runs/44942648233).

## Machine gate

[Attempt3](../ui-actual-attempt3-review/actual3/machine-report.json) passes its
machine-helper scope in 62.411 seconds, inside its original 299.776-second
window. Two actual packaged helper processes log in, read the original paid
wallet, log out, receive EOF and exit0 without signals. Reopening rejects the
previous View for both activity and payout reads. The original wallet at
height165 shows 1,980,000,000 spendable atoms and exactly one incoming payment:

`2a475d6318ff194115024169265271221aea644a95c7d89e2782ec7492a1afa9`, output0.

Both activity captures and transaction details reproduce raw SHA256
`83889f90b7ffcee9d5ae8f283f1a9b74b214da27037549ac1f861abe8466eb8c`.
Closed logical audits show an empty new attachment, and the original mining
attachment remains unopened and byte-identical. See the preserved
[partial qualification](actual3-partial-qualification.md): this machine pass
does not convert attempt3's separate native or host deadline failures to passes.

## Native gate and integrity

Attempt5 starts at 20:18:48.118 UTC on November21 with an absolute deadline of
20:23:48 UTC. [Root-observed native accessibility states](actual5/public/native-observations.json)
verify the actual packaged window: stopped mining, height165, 19.80000000 test
SLUG spendable, one dated confirmed pool payment, a separately dated wallet
observation of output0 with one confirmation, one incoming wallet activity row,
and its raw transaction detail. Signing out clears wallet identity, balances
and captures. Application-menu Quit ends the original app normally:
[exit0, no signals, original Wait joined](actual5/public/native-window-process.json).
Its native owner closes and joins its original helper before allowing Quit;
this is not an inference from a vanished window alone.

The [host operator](actual5/public/host-lifecycle/result.json) stops by request
with original child and monitor exit0, both joined; the original SSH launch
[also joins successfully](actual5/public/host-lifecycle/launch-wait.json).
The [closed financial audit](actual5/public/audit-after.json) preserves both
credits and the one original signed payment, including its reservations and
funding allocation. This checks the existing payment; it does not claim another
scheduled payout. The [wallet comparison](actual5/paid-roots-comparison.json)
preserves all 226 protected files and the unopened original mining attachment.
Permitted wallet/account database lifecycle changes are listed, not called
whole-tree byte identity. The separately selected attachment remains
[logically empty](actual5/empty-outbox-native.json).

The original local node and peer relay [join with exit0](actual5/public/local-infrastructure-joins.json).
The hosted fixture node's original PID1 receives TERM only after its executable
and UID/GID checks. Its structured [final logs](actual5/public/host-node-cleanup.json)
confirm the node child and relay joined and clean shutdown at
20:21:22.448635914 UTC. All named native5 runtime and integrity gates finish
before its original deadline; the last joined-node record is 154.331 seconds
after its start. The administrative fixture container is separate from the
stopped operator. The live Phase-A seed and master were not changed.

## Retained failures and limits

- Attempt1 retains its guard and public-report parser preparation failures.
  Attempt2 selected local nameV2 for the unchanged nameV1 wallet and failed
  before its first helper snapshot. Narrow harness successors correct those
  inputs without altering the remote nameV2 source or product checks.
- Attempt3's native watchdog sends TERM/KILL and joins exit-9. Its host owner
  reaches the deadline with child/monitor exit0 but `clean:false`; the failed
  marker remains. Post-deadline integrity audits do not repair that result.
- Attempt4 omits the original local peer relay. RPC proves zero peers and
  [template error -9](actual4/public/getblocktemplate-diagnostic.json). The
  helper reports an uncertain login outcome and exits1; normal native Quit
  refuses a clean-close claim, and the watchdog eventually kills the app.
  Its host nevertheless stops cleanly by request. Restoring the relay and
  restarting that local node yields [one peer and a valid template](actual4/public/peer-ready.json).
  Unchanged product bytes then pass attempt5. No wallet corruption was inferred
  and no data, template, currentness or cleanup check was relaxed.
- The direct helper diagnostic under attempt4 ran after the operator stopped.
  Its generic login refusal is retained, not treated as independent proof of
  the missing-peer cause. The named RPC evidence supplies that observation.
- These are separate finite gates, not one extended clock. The fixed prepared
  transport selection and its original expiry remain unchanged across UI
  retries. No general certificate/credential migration is qualified.

[EF-20](../../../EDGE_CASE_FOLLOW_UPS.md#ef-20--failed-wallet-attachment-obscures-a-missing-node-peer)
records the remaining failed-attachment diagnostics. Other public-launch
follow-ups retain their existing scope. Sign-in/onboarding is next; marketplace
stays on hold and the full joint D74 review remains last.

[Archive manifest](archive-manifest.json) enumerates the public evidence and
source copies by exact SHA256. It excludes credential contents, private keys,
wallet databases and live runtime trees. CUA observations are explicitly
identified as root-observed UI facts, separate from machine-generated records.
