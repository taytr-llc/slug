# Fresh hosted client flow

This is unexecuted external source for the bounded published-fixture test. Root
owns the native builds, source/profile selection, host commands and final checks.
It makes no deployment or acceptance claim. Original Set81 A2 evidence stays
unchanged. The selected flow uses normal validation, without `localwalletpilot`,
fault relays, replacement jobs, clock changes or new economic expectations.

`credentials.py` is offline encoding only. `run-client.py` owns one original
native miner **or** one stopped observer, captures bounded output, forwards one
requested SIGTERM and waits. It never starts nodes or changes operator policy.
The separate unchanged `/Users/taytrrip/slug/set82-prep/hosted-actual/proofclient`
reader is the execution seam for original receipt
replay, validated public money and local-chain checks; its source/build still
needs root admission. Raw HTTP bodies alone are not validated payment evidence.

## Select inputs once

Root first admits `../check-selection.py` output against the exact public
174293-byte reference SHA256
`17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a`.
The selected origin is `https://turntable.proxy.rlwy.net:26354`; preserve its CA
and the complete config/profile/money binding. The original fixed UTC due is
not moved if setup or acceptance fails. Credential expiry is the prepared due
plus one hour. This runner accepts a single original scenario deadline at most
900seconds away; every generation uses that same absolute deadline. Root's
outer bound and source/profile finalization remain authoritative.

Use a **fresh complete copy** of the closed published baseline6 owned-gym. Root
checks the original fixture catalog and immutable226-file history before/after.
Do not copy A2's post-payment wallet or create another recipient. Keep the same
absolute owned-gym and remote-attachment roots through miner, replay and observer.
The `incrementing256` account alone logs in; original name version is1. All
protected inputs reside in original current-user-owned0700 directories and are
single-link0600 files. All output paths are fresh; failures retain files.

The native wallet RPC file uses the existing strict Version1 grammar:

```json
{"version":1,"url":"http://127.0.0.1:29582","username":"SELECTED_EXISTING_LOCAL_RPC_USER","password":"SELECTED_EXISTING_LOCAL_RPC_PASSWORD","ca_certificate":""}
```

Use the actual selected local node credentials, not those placeholder values.
The host private HTTPS RPC belongs to the operator only. Do not put that URL
into the attachment or relax its literal-loopback policy. The local normal peer
connects through the separately admitted mTLS bridge to the hosted normal node.
Both must show the original public baseline blocks followed by actual generated
blocks up to62, current matching tips and a connected peer before work issuance.

## Original rotations and exact wrappers

With the original operator stopped, root runs its real `init` and three
`rotate` commands: pool audience for each of the two enrolled gyms, admission
audience for the first gym. Use the existing selected `slugpoold` common options
and `--daily-recoverable-issue`; each rotate adds:

```text
rotate --gym-id ORIGINAL_GYM --audience pool|admission
  --expected-generation 0 --token-file ORIGINAL_PUBLISHED_TOKEN_FILE
  --expires-at-ms PREPARED_CREDENTIAL_EXPIRES_AT_MS
```

Do not rotate while serve owns the root. Preserve the complete command
stdout/stderr/exit and original process cleanup. Require exit0, no stderr and
exact one `version:2,command:"rotate",code:"complete"` frame emitted only after
the command's original owners close. Do not repeat an ambiguous rotation or
increment generations to make a fixture fit.

The original published raw tokens are exactly64 ASCII characters, no LF:
`SHA256("SLUG Set79 Path A pool " + GymID)` for each pool gym and
`SHA256("SLUG Set79 Path A admission " + firstGymID)` for admission. These are
isolated engineering credentials, never user keys. Use the preparer's existing
`pool-token-0`, `pool-token-1`, and `admission-token` files unchanged.

For the first gym, run the offline encoder twice after root checks command
exit/cleanup. Here `PUBLIC` is the admitted six-file public selection directory,
`PRIVATE` the local protected input directory and `EVIDENCE` its separate public
evidence directory; all are absolute paths. The setup/remote/prepared files may
be copied locally only with exact checker-bound bytes.

```sh
python3 credentials.py \
  --selection-report "$EVIDENCE/selection-check.json" \
  --setup "$PUBLIC/setup.json" --remote "$PUBLIC/remote.json" \
  --prepared "$PUBLIC/prepared.json" \
  --audience pool --rotate-result "$EVIDENCE/rotate-pool-0.stdout.json" \
  --token-file "$PRIVATE/pool-token-0" --output "$PRIVATE/pool.credential"

python3 credentials.py \
  --selection-report "$EVIDENCE/selection-check.json" \
  --setup "$PUBLIC/setup.json" --remote "$PUBLIC/remote.json" \
  --prepared "$PUBLIC/prepared.json" \
  --audience admission --rotate-result "$EVIDENCE/rotate-admission.stdout.json" \
  --token-file "$PRIVATE/admission-token" --output "$PRIVATE/admission.credential"
```

The exact ordered JSON+LF wrapper is:

```text
{version:1,base:{version:2,config_id:BASE_REMOTE_ID,gym_id:FIRST_GYM,
 audience:AUDIENCE,generation:1,expires_at_ms:ORIGINAL_ROTATE_EXPIRY,
 token:PUBLISHED_TOKEN},issue_profile_id:PROFILE_ID} + LF
```

`BASE_REMOTE_ID` is `DailyRemotePoolConfigID(remote.base)`, **not** the outer
recoverable remote ID, setup ID or miner enrollment ID. The encoder separately
recomputes the latter enrollment's v2 token/reuse hashes and checks them against
the real rotation result. Every gym/audience/generation/timestamp/profile and
all selected input bytes are checked. No secret/token is printed. A successful
encoding is not authority to alter the original rotation or its evidence.

For second-gym balance/history only, use the checked second pool rotate result
and original `pool-token-1`. Do not build a second account attachment. The
proof client must verify its credential result against that exact enrollment.

## One automatic miner, then stopped wallet observation

Root sets one original `DEADLINE_MS`, admits exact native binary identities,
and invokes the launcher as a child in its existing outer process group:

```sh
python3 run-client.py --mode miner --binary "$BIN/slugpilot" \
  --config-id "$REMOTE_ID" --deadline-ms "$DEADLINE_MS" \
  --owned-root "$FIXTURE/owned-gym" \
  --attachment-root "$FIXTURE/remote-attachment" \
  --remote-config "$PRIVATE/remote-config.json" \
  --node-rpc "$PRIVATE/remote-rpc.json" \
  --pool-credential "$PRIVATE/pool.credential" \
  --admission-credential "$PRIVATE/admission.credential" \
  --evidence-dir "$EVIDENCE/miner-1"
```

The launcher invokes exactly the original `slugpilot
--daily-recoverable-issue --candidate-name-version=1` plus its six explicit
path flags. Its original LoginFixture/Start owns the Mining lease and vault.
Do not add `--daily`, `--candidate-remote-pool`, login bypasses or another source.
The launcher does not infer accepted rewards from status `records` or `pending`.
Root independently observes the original canonical reward blocks63 and64 and
two matching source receipts, then sends **one SIGTERM to this launcher PID**.
It forwards one SIGTERM to its exact child, drains output and joins. Thirty
seconds without exit triggers a forced kill/failure, with five seconds to reap;
no forced cleanup can pass. A premature exit, malformed/truncated output,
nonzero exit, stderr, changed selected input or bound expiry also fails.

Do not SIGTERM only the child, kill the whole group to simulate clean success,
detach/restart a process, or discard a third/unknown outcome. Preserve exact
status rows, pending q/e/fighter/receipt state and any extra already-started
request. At source capacity, a terminal q may consume a remaining request slot;
that does not authorize another job or clearing a hold. Stopped replay must use
the original completed request/work/nonce and original selected client. Never
send a fresh Issue or mutate q/scan/e metadata to obtain the desired count.

After CLI join, the proof reader's original selected outbox audit/Close precedes
read-only projection and exact saved Work/Nonce replay. Require two accepted
canonical pool rows and the original fighter outcomes; preserve all other
retained records and exact q/e/h/p/meta/scan/cursor hashes. A stopped reopen
must not call Start. New-mode full-intake/reservation and prior A2 loss/expiry
proofs remain separate evidence; this normal hosted flow does not recreate them.

Generate exactly100 normal descendants to164 before the fixed due. No miner
is running during this maturity step. Original scheduled settlement must fund
one transaction from the reward63/64 coinbases. Start the observer on the same
closed client roots before payment:

```sh
python3 run-client.py --mode observe --binary "$BIN/slug-host-client" \
  --config-id "$REMOTE_ID" --deadline-ms "$DEADLINE_MS" \
  --owned-root "$FIXTURE/owned-gym" \
  --attachment-root "$FIXTURE/remote-attachment" \
  --reference "$REFERENCE" --remote-config "$PRIVATE/remote-config.json" \
  --node-rpc "$PRIVATE/remote-rpc.json" \
  --pool-credential "$PRIVATE/pool.credential" \
  --admission-credential "$PRIVATE/admission.credential" \
  --evidence-dir "$EVIDENCE/observer-1"
```

This invokes the already reviewed `slug-host-client observe`, rebuilt against
the selected product source from the unchanged module at
`/Users/taytrrip/slug/set82-prep/hosted-actual/client`; it opens/logs in
but never Start/Issue/mines. Require a ready original wallet snapshot at164
before the actual scheduled send. Preserve that baseline first. When the exact
transaction is in the selected mempool, root generates one block165 and verifies
its canonical inclusion. Require ready wallet165 with spendable atom delta
**1,980,000,000**, unchanged other wallet categories as selected, stopped state,
attempts/hashrate0. Then SIGTERM the observer launcher and require its own joined
Logout/Close report **and** process report, exact source/input identities and
the immutable fixture catalog. Neither report's `passed` flag substitutes for
these values. Stop observers before another CLI or any closed client audit.

## Exact read-only wire routes and financial acceptance

The concrete `poolservice.NewRecoverableDailyClient` uses selected TLS origin/CA,
original pool token, public key, full Source2, settlement ID, recipient and issue
profile. It supplies `Authorization: Bearer TOKEN`, content-type JSON, no proxy,
redirect or compression. The wire routes are POST:

| Operation | Path | Ordered request |
|---|---|---|
| Balance | `/pool/v3/balance` | `{}` |
| First payout page | `/pool/v3/payouts` | `{"revision":"0","high_batch":"0","before_batch":"0"}` |
| Next page | `/pool/v3/payouts` | captured revision/high_batch, previous next_before, all decimal strings |
| Exact saved receipt lookup | `/pool/v3/lookup` | `{"job_id":"ORIGINAL_JOB","nonce":ORIGINAL_NONCE}` |
| Original saved work read | `/pool/v3/work` | `{"job_id":"ORIGINAL_JOB"}` |
| Existing exact request read | `/pool/v3/issue-lookup` | `{"request_id":"ORIGINAL_REQUEST"}` |

Do not confuse read-only lookup with `/pool/v3/issue` resume or `/submit`.
Read wrappers from the original client validate wireVersion3, full source,
profile, gym/payee and financial bindings before their existing Version2 typed
projection. Keep cumulative `paid_atoms`/`reserved_atoms` and page cursors
lossless decimal strings at JSON boundaries. Empty Items is not EOF; use Done
and captured NextBefore. Never restart a stale history capture to pretend a
single complete snapshot.

Reuse A2 `waitMoney` policy: at most120seconds under the original scenario
context, only definite `busy`, `rate_limited` or `unavailable` for these pure
money reads, bounded refusal log and last failure, no unknown/mutation replay.
Exact receipt Lookup retains its separate20second/one-second pacing and only
definite Busy/rate retries. A failed complete read is not accepted from a
partial response. Premining no-token/wrong-token controls belong to the proof
client and must neither create work nor change any source/ledger counter.

At confirmed165, both gyms need fresh validated balance/history. First gym:
paid1,980,000,000, reserved0, confirmed/available0/1; exactly one confirmed batch1
payment output0 with original txid/amount. Other gym: paid/reserved0 and empty
complete payout page. Both capture high_batch1, before_batch2, done true,
next_before0. Closed original operator audit then requires two credits, one
immutable batch body/raw/claims, exact inputs from63/64, miner output1,980,000,000,
fee209, original change19,999,791 and original delivery progression. Root's
stopped audit comparisons and canonical chain proof supply what public money
DTOs cannot: exact raw/body/funding and same-root financial integrity.

The minimum remaining execution seams are the proofclient build/qualification,
actual stopped root audits, and root's joined host/node lifecycle plus final
source/catalog checks. This launcher does not claim to complete those actions.
