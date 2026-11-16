# Change set 9: complete blockchain and integration verification

Branch `slug-test-pow`, stacked on set 8 (`c8176de5`). The complete blockchain
suite and integration suite now finish and pass. No tests are skipped to
obtain these results. Production proof-of-work parameters and genesis are
unchanged.

## Test profile and safeguards

The explicit `slugtestpow` build tag selects a compile-time Argon2 memory
constant of 64 KiB. Default builds retain the literal 256*1024 KiB value.
Both profiles use the same 80-byte header as password and salt, one pass,
one lane, and 32-byte result. There is no runtime override. Hash-cache keys
remain valid because the profile cannot change within a process.

Cheap hashes necessarily change block identities, including genesis. A
tagged-only initializer recomputes the six test genesis identifiers from
unchanged headers; default genesis constants are untouched. Independent
reference tests cover both profiles. The default-only proof test now pins
nonce 1 and the exact handover digest
`5f42cf87f3acc67ff351adc85f0df64b4cc19e3e278fd46ce63c76511dc8d719`, derived
independently from fixed serialized bytes using literal production Argon2
parameters. Production SLUG and testnet4 genesis pins remain default-only.

Tagged daemons identify themselves as `0.27.0-slugtestpow`, use a separate
`slug-testpow` default home/config/data/log namespace, and reject default
mainnet, testnet3, testnet4, and signet before database startup. Explicit
regtest or simnet is required. Normal home/version behavior is unchanged.
The release Dockerfile never selects this tag.

The RPC harness explicitly passes its own profile to nested `go build`,
including an empty `-tags=` in production to override conflicting inherited
GOFLAGS. Build failures return captured diagnostics; they no longer fall
back to a possibly unrelated `btcd` on PATH.

## Restored and repaired coverage

- HaveBlock and notification tests use native mined SLUG blocks. The short
  fork retains main-chain, side-chain, header-only, absent and orphan cases.
  Fixture header bytes and hashes are independently constructed/derived.
- Both set 2 skips are removed. Template connection and block sanity are
  tested with valid native blocks; the invalid-nonce case deliberately finds
  a failing proof instead of assuming that incrementing a nonce must fail.
- Pruning and crash replay retain 14,131-block chains. A mature coinbase is
  spent and respent through later blocks. Replay flushes at height 9,000,
  crashes at 12,000, starts an empty cache, verifies spent/unspent outputs,
  and continues to the final tip. Test parameters keep regtest's fixed easy
  target and a long subsidy interval; they are not deployed parameters.
- The old prune assertion fetched genesis for every requested hash. It now
  checks the actual block, requires a nonempty pruned prefix including a
  valid stale branch, and verifies a contiguous retained suffix containing
  every required replay block. Whole-file pruning can legitimately retain
  some blocks older than the last UTXO flush.
- Database test cleanup removes the shared root only if empty, preserving
  other temporary test databases. The example uses its own temporary path
  and checks the stable duplicate-block rule code instead of a Bitcoin hash.
- The retained Bitcoin script-validation fixture is read using explicit
  Bitcoin framing magic, separate from SLUG database/network magic. The
  loader now rejects empty, wrong-magic, truncated and invalid-length files
  instead of silently returning no blocks. Transactions remain unmodified.
- Test block-solver goroutines select cancellation when sending results,
  avoiding stranded winners after another worker solves a header.
- Integration chain-tip fixtures preserve transactions and fork topology;
  tagged fixtures derive new links/nonces with a direct Argon2 oracle.
  Default production fixture bytes are unchanged.
- Harness balance expectations use SLUG's subsidy; the large-spend case
  still consumes multiple coinbases. Three existing goroutine Fatalf calls
  found by vet now return errors to the test goroutine through channels.

## Verification

Go 1.27.4, darwin/arm64. All commands run in the committed Go workspace;
dependency versions and module files did not change.

```sh
go test -tags=slugtestpow -count=1 -timeout=30m -v ./blockchain/...
go test -tags=slugtestpow,rpctest -count=1 -timeout=30m -v ./integration/...
```

`make test-chain` provides both commands without verbose output.

- Complete blockchain command: **45.62 seconds wall time**, package 45.212 s,
  including full-block validation/reorg scenarios, restored tests, pruning,
  crash replay, examples, indexers and work-math subpackages. No skips.
- Complete integration command: **21.68 seconds wall time**, integration
  package 21.245 s and rpctest 3.359 s. Both tags and the `./integration/...`
  pattern matter: they include the full RPC and harness suites. No skips.
- Default full wire suite: 5.686 s; default full chaincfg: 1.610 s; tagged
  chaincfg: 0.227 s; tagged cache/reference tests: 1.671 s. Default production
  proof tests passed, including the independently pinned handover vector.
- Default-profile restored short scenarios, both proof tests and the duplicate
  genesis example passed together in 7.720 s using production Argon2 memory.
- Profile safety and harness no-fallback/build-argument tests passed under
  both profiles. An actual tagged binary rejected every public network
  before creating the requested data directory. Tagged root, blockchain and
  integration vet passed. All ten modules build normally; gofmt/diff checks
  are clean.
- Required normal-build smoke passed: version 0.27.0, unchanged genesis,
  two mined blocks, exactly 10 SLUG subsidy, correct seed subversion,
  convergence to height 3 and tip
  `42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`,
  report-only/unconfigured checkpoint authority, and clean stop.

The first complete tagged blockchain run exposed one remaining Bitcoin
genesis literal in an example. After adapting that example, the entire
command was rerun to completion. Passing individual tests was not substituted
for a full suite result.

## Limits and next steps

The full scenario suites use the deliberately incompatible cheap profile;
they do not measure production memory/DoS behavior. Production parameters
are checked by independent vectors, normal short scenarios and the normal
seed smoke. The set 8 residual security risks remain documented in its notes.
Broad tagged tests for unrelated modules with fixed production block-hash
fixtures are not the intended command; those modules retain default-profile
coverage. Existing general CI/release maintenance items recorded in set 6
are not claimed resolved here.

Set 7 authority enrollment is still pending Mike's public key, followed by
an eligible signed checkpoint (60 descendant blocks). The seed is still at
height 3, so no positive checkpoint height is eligible. No operator private
key was generated or accessed. No master merge, live seed push, deployment,
or production genesis change was performed. Phase A closeout requires Mike's
explicit go-ahead as specified in HANDOVER.md.
