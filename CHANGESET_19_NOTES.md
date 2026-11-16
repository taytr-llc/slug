# Change set 19: bounded local prospect authority

Branch: `slug-prospect-service`, stacked on set-18 implementation `a666da6b`
(released on master as `c88abe11`). Mike requested continuation after sets
17/18 were deployed and verified. This set builds the next client/service
boundary; it does not promote a new release or activate a public authority.

## What changed

- Added `prospectservice`, a loopback-only HTTP authority with fixed operator
  policy and node configuration, retaining one existing schema-3 registry.
  It issues durable D68 jobs and accepts proof-v2 gym authorizations.
- Bound API credentials to compressed gym public keys before work. Credentials
  grant API access only; the gym signature still authorizes each proof.
- Bound connections (16), active operations (one, no queue), body/header sizes,
  timeouts, per-key request rates and durable job/receipt counts. Shutdown waits
  for actual handlers before storage closes, even after HTTP connections close.
- Preserved final expiry, original birth and historical duplicate recovery.
  Capacity and current node/policy changes cannot prevent historical recovery.
  Uncertain writes fail closed; transport errors retain an uncertain outcome.
- Added a validating client with verified TLS or literal-loopback HTTP, no
  proxy/redirect/retry, strict nested response validation and full work/DNA
  verification. Automatic signing uses an in-memory callback and returns the
  generated public authorization on submission failure for exact replay.
- Added `slugregistryd`, `issue-job-service` and `record-job-service`. CLI inputs
  contain public evidence and API configuration, never private signing keys.
  Daemon startup requires a writable, matching schema-3 store already created
  explicitly; no auto-creation, migration or recovery is introduced.
- Added locked `registry.Info` metadata snapshots for startup validation and
  durable quotas, without hashing or reading the authority clock.
- Recorded the implementation decision in `ADR_001_PROSPECT_SERVICE.md` and
  the exact service, configuration and retry contract in `PROSPECT_SERVICE.md`.

Independent review caught an implicit-port mismatch between accepted HTTP
client URLs and the daemon's strict Host check, and unbounded pre-validation
allocation for typed public-key inputs. HTTP port 80 is now explicit in the
canonical client origin; public keys are length-checked before decoding.

## Deliberately unchanged and known debt

No consensus, genesis, production PoW, proof/job/signature encoding, registry
schema, checkpoint policy, node startup, Railway configuration, persistent
chain volume, public TCP endpoint, Dockerfile or dependency changed. The seed
does not run this authority simply because its binary exists in an image.
No real private key, operator seed or production API credential was accessed.

These are private integration primitives, not a public enrollment deployment.
The operator's wall clock and durable history remain trusted. Configured
storage capacities are engineering bounds, not a new product prospect cap.
Jobs/receipts are not pruned; capacity must be deliberately managed. Automatic
retries cannot silently replace jobs or reset birth.

The next miner slice needs full payable templates (coinbase value/auxiliary
data, transaction bytes and witness commitment), automatic renewal and both
prospect-share and block-target handling. Submitting a winning block changes
the current parent and requires explicit admission ordering. Context-only jobs
are not complete mining templates. Wallet/login/export, public hosting,
calibrated prospect targets, ownership/transfers and publication remain later
work. No new product decision beyond existing D44/D68 was invented.

The Phase A chain remains disposable. Existing CI/platform debt remains;
CI is not claimed green and this set does not rerun every unrelated slow suite.

## Verification

- All ten workspace modules built; Linux/amd64 command binaries built with
  production PoW. `gofmt -l .`, `git diff --check` and affected-package `go vet`
  were clean.
- Normal full affected suites passed: `prospectservice`, `registry`,
  `prospectnode`, `prospectjob`, `gymauth`, `prospect`, `fighter`,
  `cmd/slugregistry`, `cmd/slugregistryd`.
- `go test -race -p 1 ./prospectservice ./registry -count=1` passed (7.842s and
  24.412s respectively). Final command-package race tests passed for
  `cmd/slugregistry` (15.233s) and `cmd/slugregistryd` (1.555s), including the
  regular-file preflight guard. All affected packages passed with `slugtestpow`,
  including refusal before authority startup and client use.
- Adversarial tests cover auth before body/RPC work, both-route key binding,
  duplicate/unknown/null JSON, malformed/oversized bodies and receipts, token
  redaction, Host/Origin/method/path restrictions, rate and TCP connection
  limits, busy rejection after cancellation, shutdown draining, frozen policy,
  read-only/incorrect stores, poison latch and durable quotas across reopen.
- Client tests cover verified/untrusted TLS, rejected redirects, disabled
  environment proxy, lost response with retained authorization, foreign-chain
  rejection before signing, malformed error flags/status, full receipt
  verification and equivalent valid retry signatures retaining first evidence.
- Normal-node smoke mined two local blocks with exactly 10 SLUG subsidy,
  reported checkpoint conflict without vetoing mining or chain selection,
  synchronized to public seed height 61, verified the chain, reported 60
  checkpoint descendants, and preserved the chain/checkpoint on clean restart.
- Fixture-only end-to-end service smoke independently derived job ID,
  commitment, signature, production Argon work, prospect ID and DNA in Python.
  It deliberately discarded a submitted proof's HTTP acknowledgment, then
  recovered the original historical receipt through the validating CLI.
  A second valid proof was rejected at record capacity. Daemon restart retained
  the job capacity and original birth, even with the node unavailable and a
  changed current target. All test processes shut down cleanly.

Smoke evidence: `/Users/taytrrip/slug/set19-smoke.json`; original run directory
`/private/tmp/slug-set19-smoke-c1twnu_n`. Test target `ff…ff`, private scalar 1
and deterministic API token were public fixtures confined to disposable local
tests; none was installed on Railway.
