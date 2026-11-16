# Change set 22 — native desktop mining pilot

Branch: `slug-desktop-pilot`, stacked on set 21 implementation `6b593fbe`.
Status: implemented; macOS pilot verified, with platform/UI limits below.

## What changed

The gym shell is new Go work using Wails and embedded assets. Mike confirmed
that Paymetheus is not the base; btcwallet is a separate repository to fork
when the wallet workstream starts. The pilot integrates the existing node,
automatic worker, authority and evidence storage through a small supervisor.
Its separate module/workspace leaves the ten-module chain build independent.

The supervisor owns two normal-profile nodes restricted to explicit loopback
peers, a v2 authority and a public fixture gym. Start is explicit; opening the
app does not mine. It retains both chain databases, authority identity, jobs,
outbox and receipts across stop/start and app restarts. Missing established
stores or chain files fail closed. The control protocol is bounded JSON lines;
EOF, cancellation and output failure stop and drain owned work and processes.

The interface presents mining activity and the latest saved prospects. Public
receipt export returns the original accepted record without a new proof hash
or claim of current canonical membership. The local training chain and fixture
identity are labelled throughout. See `ADR_004_DESKTOP_PILOT.md`.

## Deliberately unchanged

Consensus, genesis, PoW cost, D68 birth/expiry, D69 broadcast ordering and
report-only checkpoints are unchanged. No public-chain mining, real wallet,
hosted login, seed export or public authority is introduced. The Railway seed
does not start a desktop supervisor, wallet, authority or automatic worker.
The wallet and native release-signing workstreams remain separate.

## Verification

- All ten chain modules build. The required normal-PoW chain smoke passed:
  two local 10 SLUG blocks, checkpoint conflict reporting without veto, seed
  synchronization to height 61, finalized checkpoint, chain verification and
  restart. Existing context, authenticated admission, birth and v1 service
  checks passed. A valid payable proposal was accepted and a one-satoshi
  overpayment rejected. The seed was not mined by this smoke.
- Six frontend behavioral tests pass: no autostart, serialized polling and
  export, cancellation, recovery retries, coherent malformed-update handling
  and text-only bounded display values. User actions wait for an in-flight
  snapshot, matching the native bridge's single-request protocol.
- Supervisor focused tests (4.973 s), final race run (65.497 s), vet and
  test-profile refusal (0.308 s) pass. Regression coverage includes concurrent
  first initialization, established store/chain deletion, ambiguous metadata,
  blocked output during EOF and shutdown storage errors.
- Real production-PoW supervisor smoke passed in 9.728 s: no autostart,
  explicit start/stop/start, stable authority origin, preserved height,
  immutable receipt/birth and EOF draining every owned listener. A separate
  1.843 s test interrupted first startup with EOF and SIGTERM; both exited
  cleanly and reopened partial first-start data safely.
- Shell and bridge race tests pass (1.498 s and 6.933 s); vet, whole-repository
  gofmt and diff checks are clean. Bridge tests cover malformed/oversized
  responses, correlation, cancellation and EOF drain. Atomic export and symlink
  refusal tests pass.
- The native arm64 Mac bundle builds with production PoW and passes plist and
  ad hoc signature validation. The real interface starts idle, mines and
  renders actual prospects/attributes, stops, starts through the native menu,
  cancels a save dialog and exports a public receipt with mode 0600. The saved
  JSON exactly matches the retained receipt, including its original birth.
- Native Quit during mining exited 0 and closed every owned listener. Reopening
  the same data showed 48 saved prospects, 48 attempts and height 20 with mining
  stopped. A second native Quit exited 0; app inventory confirmed it was closed.
- The helper cross-compiles for Windows amd64; runtime remains unverified.

Deployment observation and final artifact hashes are recorded separately after
promotion; this file does not claim an unobserved rollout result.

## Known debt

Hosted login, btcwallet adaptation/custody, independently tested full export
under D48, calibrated prospect policy and public authority operations remain
integration gates. A local fixture receipt is not a wallet export or an account.
Windows runtime/tray verification and release signing/notarization require
their platform/release work; compilation alone does not establish runtime
support. Existing repository CI debt remains; CI is not claimed green.

The native close-button automation timed out, and subsequent app observations
automatically reopened its window. It also relaunched the app after Quit, which
was independently confirmed by process exit and a fresh idle instance. Source
review verified Wails' documented hide-and-cancel hook, but hidden-window
visibility and the system-tray Show action remain unverified interactively.
Native application-menu actions, Start/Stop, export and actual Quit/reopen were
verified. This limitation is recorded rather than treated as a passed tray test.
