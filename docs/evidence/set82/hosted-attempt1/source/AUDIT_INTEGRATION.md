# Root execution integration — source-only proposal

This adds the reviewed stopped public audit to the existing disposable fixture
admin image. No product source, shipping image policy, runtime, or network work
has been performed. The audit's original source selection is
`audit/source-manifest.json` SHA256
`7db6c1c4d9f1932edd45e4b0efbbedabe52e40c021b1cc9c56563a12fc8af73c`.
Root may preserve preimages and bind a format-only successor before compiling.

## Minimal context/build delta

`compose-context.py` currently hardcodes the already-created
`operator-context-attempt2` and copies only the preparer. Do not rerun it in
place. Use a new explicitly named context; retain old contexts/reports. Reuse
the same pinned chain/wallet archives and the **current** package/admin sources
(the older context predates the reviewed mount0755/lost+found correction).
Keep the original qualified operator build/stage byte-for-byte unchanged.

Add only these five audit build inputs, with their exact hash/length checked
against the selected audit manifest before copying:

```
audit/go.mod             -> audit-src/go.mod
audit/main.go            -> audit-src/main.go
audit/support.go         -> audit-src/support.go
audit/financial.go       -> audit-src/financial.go
audit/operator_audit.go  -> audit-src/operator_audit.go
```

Add their destination hashes to `fixture-inputs.sha256` using absolute in-image
paths `/src/slug/desktop/hostedaudit/<name>`. The existing manifest's pinned
`/fixture/reference-v1.json` already supplies the audit's sole independent
reference; no private file or runtime tree enters the build context. Add only
`!audit-src/` and `!audit-src/*` to the current closed `.dockerignore`. Recompute
the fixture-manifest hash and context inventory in the fresh context.

In the existing `fixture-build` stage, before workspace/source snapshots:

```dockerfile
COPY audit-src/go.mod audit-src/main.go audit-src/support.go audit-src/financial.go audit-src/operator_audit.go /src/slug/desktop/hostedaudit/
```

Extend the current workspace construction in `/src/slug/desktop`:

```sh
cp go.sum hostedaudit/go.sum
go work edit -use ./hostedfixture -use ./hostedaudit \
  -replace github.com/taytr-llc/slug/desktop@v0.0.0=/src/slug/desktop \
  -replace github.com/taytr-llc/slugwallet@v0.0.0=/src/slugwallet \
  -replace github.com/taytr-llc/slug@v0.27.0=/src/slug
```

This extends the already selected17-module paired workspace with the preparer
and audit modules. It uses the inherited local module graph and pinned archive
Go sums, not a remote replacement. Preserve the current exact native Linux
amd64/Go1.27.4 image, CGO0, normal PoW, candidate-go profile, original archive
checks, fixture-manifest verification, and complete source-before/source-after
comparison. Capture workspace/profile after both `-use` additions. Do not alter
or reuse the product `desktop/go.work` itself; use `hosted-fixture.work` as now.

Add these build/evidence commands beside the preparer build, inside the same
existing source bracket (shown for root to execute, not executed here):

```sh
go list -mod=readonly -deps -json ./hostedaudit > /build-evidence/audit-dependencies.json
sh ../scripts/candidate-go.sh build -mod=readonly -p=1 -trimpath -buildvcs=false -tags= \
  -o /out/slug-host-audit ./hostedaudit
sh ../scripts/candidate-go.sh verify-binary /out/slug-host-audit > /build-evidence/audit-build-info.txt
sha256sum /out/slug-host-audit > /build-evidence/audit-binary-sha256.txt
```

Add to **admin**, without changing its bootstrap behavior or the serve image:

```dockerfile
COPY --from=fixture-build /out/slug-host-audit /usr/local/bin/
```

The existing admin copies `/fixture/` and `/build-evidence/`, so it already
carries the pinned reference and will include the new audit build evidence.
No observer/CLI module is needed to build this audit. Native local compile and
image compile must bind their own full dependency/profile/source closure.

## Stopped invocation and repeat

Stop and join the original operator process first; retain its result. Invoke
the audit as UID/GID10001 with original `/data` mounted. Do not run bootstrap
`stage` or `prepare` again, copy/chown/rename any owner, or open the custody wallet.
The helper acquires the original root flock, so a live operator/restart refuses
rather than allowing a concurrent reader. Root preserves command/exit/output
and child joins. Keep each output fresh0600, bounded16MiB, and hash it afterward.
The example uses shell noclobber in the existing admin environment:

```sh
su -s /bin/sh -c 'umask 077; set -C; exec /usr/local/bin/slug-host-audit audit --root /data/hosted-test --expected-setup-id EXACT_CHECKED_ID --expect-delivery confirmed > /data/hosted-test/public/audit-before.json' slug
```

`EXACT_CHECKED_ID` must be the selected lowercase64-hex ID already verified by
`check-selection.py`, not an ID adopted from a newly opened runtime. The helper
uses the public setup at `/data/hosted-test/public/setup.json`; its reference
default is `/usr/share/slug/fixture/reference-v1.json`.

Restart the same original operator with the same protected inputs and root,
observe the bounded same-day recovery, then stop/join again. Repeat to a new
output, adding:

```
--compare /data/hosted-test/public/audit-before.json
```

Use `audit-after.json` for the second output. Require successful command exits,
passed reports, selected inputs/public files unchanged within each call, and
second `compared=true`/`immutable_batch_and_credits_unchanged=true`. Compare
confirmed→confirmed in the normal hosted flow. `prepared`/`broadcast` are optional
observations only; do not manufacture a fault to obtain one. Whole physical DB
file equality is required within an audit, not across legitimate runtime writes.

## Reusable original normal-flow seams

All paths below are under `/Users/taytrrip/slug/`; they are source references,
not a request to copy or run the full A2 scenario.

| Need | Exact existing source/function | Narrow hosted use |
|---|---|---|
| Authenticated v3 client | `set81-prep/retained-admission-actual/selection.go`: `clientConfig`, `recoveryClientConfig`; `slug/poolservice/recoverable_config.go`: `NewRecoverableDailyClient` | Original selected host Origin/CA/profile/payee and actual protected credential. No dummy credential or replacement client authority. |
| Original retained work | `set81-prep/retained-admission-actual/outbox_audit.go`: `readOutbox`, `readSelectedOutbox`; `work_evidence.go`: `auditRows`, `readLookup` | After miner/controller joins, full selected read-only outbox audit; preserve exact Work/Nonce/receipt, original q/e/h/p/control and signatures. Read-only Lookup alone is not a mutation replay. |
| Exact receipt replay | `set79-prep/public-daily-actual/late_flow.go`: `lateSubmit`, `lateLookup`; exact equality at fresh CLI restart in `runDailyLate`; `slug/poolservice/client.go`: `Submit`, `Lookup` | Reuse only the20second exact-original Work/Nonce call pattern with1second pacing and definite busy/rate_limited retries. The v3 concrete client chooses its own route. Compare the complete returned receipt; no new Issue or nonce. Unknown and all other errors stop. Do not copy late-winner orchestration. |
| Receipt versus accounting projection | `set81-prep/retained-admission-actual/work_evidence.go`: `sourceReceipt` | Only clears the two mutable observation fields when comparing an AccountingPage projection; never weaken complete saved Submit/Lookup receipt equality. |
| Original reward outpoints | `set81-prep/retained-admission-actual/payment.go`: `funding` | Independently fetch canonical blocks63/64 through the selected isolated node, require original target and exactly one1e9-atom operator coinbase output per block. |
| Exact signed transaction | same `payment.go`: `validatePayment`; `node_helpers.go`: `block`, `minedTx` | Verify2 original inputs, configured miner1980000000/change19999791 outputs, fee209; retrieve canonical block165 and exact txid/raw. No signature-size equality assumption. |
| Whole money/history | same `payment.go`: `moneyReadEvidence.refused`, `waitMoney` | Original120second pure-read bound; only definite busy/rate_limited/unavailable retries. Both gyms' complete balance and captured payout page must match source/config/payee, one high batch, original txid/output, paid1980000000 versus0, reserved0 and exact zero confirmed/available. |
| Stopped original wallet | `set82-prep/hosted-actual/client/main.go`: `run`, `snapshot`, `observed`; inherited `set81-prep/retained-admission-actual/work_evidence.go`: `openController` | Existing-only owner/Login, no Start; zero attempts/hashrate. Require ready wallet at164 before payout, at least165 after, same binding, exact spendable delta1980000000; retain fresh public snapshots and joined Logout/Close. |
| Full closed accounting | `set82-prep/hosted-actual/audit/operator_audit.go`: `auditOperator`; `financial.go`: `verifyFinancial` | The new helper already wraps this unchanged A2 audit, including original stores/full row census and immutable history comparison. |
| Process ownership | `set81-prep/retained-admission-actual/operator_process.go`: `launchOperator`, `stop`, `evidence`; `cli_process.go`: `launchCLI`, `stop`, `evidence` | Use lifecycle/one-Wait/cleanup evidence concepts; do not reuse hardcoded old wallet paths or automatic fault kills. The host wallet path is custody/wallet16, not A2 wallet16. |

The inherited `node_helpers.go:startNode` includes old local fixture flags and
loopback assumptions; **do not reuse its launch arguments**. Use the selected
hosted normal node/mTLS isolation. Preserve normal genuine bootstrap60 blocks
before timed issuance, rewards63/64, maturity164, confirmation165, max180,
original900/1080 windows, immutable original due, and the small same-root restart.
No admission hold/loss relay, late-winner proof, expired-unused case, or renewed
deadline is needed in this normal hosted flow.

`payRetained` shows the original independent checks, but its full orchestration
is A2-specific and must not be invoked wholesale. In particular its RPC relay
`sends()` supplied a broadcast-call count. The hosted node currently exposes
ordinary RPC without that counter. A closed batch, canonical tx, payout page and
wallet delta prove one retained payment and no duplicate payout; they cannot
by themselves count repeated identical `sendrawtransaction` invocations. Do not
invent a send count from one on-chain txid. If root retains a literal one-RPC-send
assertion, it needs actual bounded RPC-call evidence; no fault injection or
payment-policy change follows from this distinction.
