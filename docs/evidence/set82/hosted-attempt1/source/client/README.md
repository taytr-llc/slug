# Stopped original-wallet observer

Unexecuted external source; root owns builds, execution and evidence admission.
This helper uses `accountpilot.OpenRecoverableDailyRemotePoolController` and
`LoginFixture("incrementing256")` on the original closed baseline6 copy. It never
calls `Start`, `Issue`, mining, signing or wallet creation. The ordinary
`slugpilot --daily-recoverable-issue` remains the separate automatic miner.

```text
slug-host-client observe \
  --owned-root ABSOLUTE_EXISTING_BASELINE6_COPY \
  --attachment-root ABSOLUTE_ORIGINAL_REMOTE_ATTACHMENT \
  --reference ABSOLUTE_REFERENCE_V1_JSON \
  --remote-config ABSOLUTE_PROTECTED_REMOTE_CONFIG \
  --node-rpc ABSOLUTE_PROTECTED_LOCAL_WALLET_RPC \
  --pool-credential ABSOLUTE_PROTECTED_POOL_CREDENTIAL \
  --admission-credential ABSOLUTE_PROTECTED_ADMISSION_CREDENTIAL \
  --snapshots ABSOLUTE_FRESH_PUBLIC_JSONL \
  --report ABSOLUTE_FRESH_PUBLIC_REPORT_JSON
```

Use the exact174293-byte independent reference with SHA256
`17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a`.
ExpectedLocal/options/fixture and all remote financial fields remain equal to
that reference; only the independently admitted Origin/CA and dependent issue
profile differ. The original controller enforces protected credential files,
local loopback node RPC, schema14 account/vault/wallet/recipient and original
attachment binding. Input files are held by identity/hash and checked again
after cleanup; credential contents and hashes are never public outputs.

All selected paths are distinct canonical absolute paths. Output parents must
already be current-user-owned0700; each file is exclusively created0600 and
never overwritten. Use different snapshot/report paths for before/after runs.
The owned-gym and attachment paths remain the same across miner and observer;
they cannot be open by two processes simultaneously. Root's existing fixture
catalog checks remain responsible for original/copy history preservation.

Open60seconds, Login90seconds and each Snapshot20seconds preserve the A2
operation budgets. Only exact `pilot.ErrRemotePoolBusy` repeats within a
Snapshot's same20second context. The overall observer window is900seconds;
root sends SIGTERM after collecting the required original-wallet observation.
No automatic retries/replacements of controllers or sessions occur.

`--snapshots` receives at most1024 JSON lines of at most8192bytes, flushed once
per observation. Every line binds ConfigID, original gym/source/recipient,
sequence/time, stopped worker with attempts/hashrate0, retained records/pending,
wallet state/height/spendable/immature/unconfirmed atoms and its public last code.
The helper permits syncing/unavailable wallet observations; only root's final
acceptance checks can require a ready wallet at the selected height and exact
payout delta. It does not infer payment from one observation or a success flag.

SIGTERM cancels the original active call. After it returns, the same controller
receives bounded Logout then Close; all outputs and immutable selected inputs
are checked/closed. The final report records original PID/parent PID, start/end/
deadline, reference/config identity, sample count/last observation, zero Start
calls, requested-stop/logout/close/input/cleanup outcomes and closed errors.
Only a requested stop after at least one sample with successful Logout/Close
can pass. Precise canceled-only trees may represent requested stop; unrelated
errors and every cleanup/write error remain failures. Timeout is a failure.
Partial output and failed reports remain. No source/runtime claim is granted by
the report's booleans alone.

Build the two source files in an explicitly composed paired workspace, adding
this module (`github.com/taytr-llc/slug/desktop/hostedclient`) to the existing17
modules. As established by the preparer build, the external composed workspace
must explicitly replace desktopv0.0.0 and rootv0.27.0 with the exact selected
local modules (and walletv0.0.0 if present in that composed graph), without
editing product go.mod files. Use `scripts/candidate-go.sh`, normal PoW and the
platform's original CGO selection; root binds the exact source/dependency/profile
closure. A cheap-PoW build refuses before input/output access.

Credential wrappers are intentionally not fabricated here. Root first checks
the original stopped `rotate` result's selected gym/audience/generation/expiry,
then encodes its protected client wrapper using the already selected base remote
ID and issue-profile ID plus the corresponding published test token file.
