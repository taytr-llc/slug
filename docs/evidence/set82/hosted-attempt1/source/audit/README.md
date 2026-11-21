# Stopped hosted operator public audit

Source-only external helper; root owns formatting, builds, runtime, and evidence
admission. No execution has qualified this source yet. It adapts the already
qualified Set81 A2 `operator_audit.go` by copying that leaf byte-for-byte and
adding only hosted selection, root exclusion, fixed economics, and comparison.

```
slug-host-audit audit \
  --root /data/hosted-test \
  --expected-setup-id EXACT_INDEPENDENTLY_CHECKED_HOSTED_SETUP_ID \
  --expect-delivery confirmed \
  [--compare /data/hosted-test/public/audit-before.json] \
  [--reference /usr/share/slug/fixture/reference-v1.json]
```

`--expect-delivery` must be `prepared`, `broadcast`, or `confirmed`. A normal
host test can take two confirmed audits around the small same-root restart.
It need not add a crash/response-loss relay or force an observable prepared
window. If a prepared/broadcast audit is available, its exact reservation is
checked, and comparison permits its legitimate forward transition to confirmed.
No statement that a reservation remains pending is made after confirmation.

The operator must first stop and join. This helper acquires a nonblocking
exclusive flock on the **same operator directory** used by the original runtime,
then keeps it until every original read-only owner and projection has closed.
A running original operator refuses the audit; a restart cannot enter during
it. The selected root must be canonical, nonsymlink, current-owner0700. This is
a public inspection helper, not a substitute for the production protected-file,
ACL, deployment-volume, process-join, or original-session checks.

It reads these selected public inputs only:

- `root/public/setup.json`, canonical recoverable DAILY setup, matching the
  independently supplied expected ID;
- `root/operator/SLUG-POOL-OPERATOR-ISSUE1.json` and
  `SLUG-POOL-OPERATOR-READY-ISSUE1.json`, exact existing completed selections;
- the exact174293-byte independent reference, SHA256
  `17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a`;
- optionally one canonical prior helper frame directly under `root/public/`;
- only the four original public issuer/source/round/settlement stores.

It never opens `root/custody/wallet16`, any miner/account/vault wallet tree,
credentials, seeds, private TLS keys, node RPC configuration, or any network
connection. It does not Create, sign, issue, admit, mine, broadcast, or mutate
any store. Root captures stdout as a fresh public artifact; no output path is
silently overwritten by the helper.

The full inherited audit opens the selected original registry, source, round,
and ledger owners read-only, validates their crossbindings and full logical
contents, and closes them in reverse order. Only afterward does it open bounded
StrictAll public row projections. All four public trees are hashed before and
after, and must be byte-identical within the audit. Failed-close handles are
retained, not retried. The helper uses an original30second context; the invoking
process owner must retain its finite outer bound and joined cleanup as usual.
A signal, deadline, failed read/close, or changed selected input is a failure.

The host-specific expected facts are fixed independently by reference17557 plus
the approved genuine height2→62 bootstrap: reward blocks63/64, exactly2 accepted
source entries,2 confirmed rounds,2 original1,000,000,000-atom credits, one batch,
one1,980,000,000-atom miner payment,209-atom transaction fee,19,999,791-atom change.
Every credit retains exact99/1 claims. The full audited signed raw transaction
must use precisely those two original outpoints and exactly the configured
miner/change outputs. No maximum-sized DER signature is assumed. The complete
balance inventory must match reserved/paid/confirmed amounts for the selected
delivery state. Existing product audits validate signatures/replay; this helper
adds no signing backend or new authority.

A prior comparison requires the same root/setup/reference, a successful closed
prior audit, and nonregressing delivery. It requires identical full signed batch
body/hash/size/raw/txid, exact credits, and **all** settlement logical row hashes
except the existing mutable `head`, `report`, and `bs/<sequence>` rows. Current
physical database files need not equal the older runtime snapshot: legitimate
runtime writes may have occurred between the two stopped audits. Within each
audit they must remain unchanged. Root must separately preserve and authenticate
the prior public artifact identity; the helper cannot turn a supplied JSON file
into runtime authority.

Output is one compact public JSON frame (at most16MiB), only after all readers
and the root lock close. `passed` means this closed public inspection succeeded;
it does not establish a current chain height, broadcast count, TLS authenticity,
wallet ownership, or original wallet balance. Hosted acceptance must independently
observe the one real scheduled transaction, node confirmation at165, original
stopped/reopened miner wallet delta, and no repeated batch/broadcast after restart.
The report's `compared` and `immutable_batch_and_credits_unchanged` are false when
no prior frame is supplied. Command exit status, stdout completion, and original
process joins remain mandatory even if a report was emitted.

Build as `github.com/taytr-llc/slug/desktop/hostedaudit` in root's exact composed
paired chain/desktop/wallet/nested-module workspace. Preserve the normal PoW
profile and original source/dependency closure; no product module or shipping
flag changes. Cheap PoW refuses before any input/output access. Root may apply a
format-only successor with preserved source preimages before compiling.
