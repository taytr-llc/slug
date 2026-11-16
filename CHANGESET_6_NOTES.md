# Change set 6: SLUG module paths

Branch `slug-module-rename`, stacked on `slug-rebrand` at `0574b8ba`.
The supplied handover and standing rules were recorded in a separate
documentation commit before the mechanical rename. No deployment is part
of this change set.

## What changed

- Renamed all ten modules from `github.com/btcsuite/btcd` to
  `github.com/taytr-llc/slug`, retaining the existing module suffixes and
  intra-repository version requirements.
- Updated Go imports, package documentation, import examples, module
  documentation links, and the rpctest harness's runtime `go build` target.
- Updated Makefile and release-script package targets to the new module
  paths and `cmd/slugctl`. Explicit output names preserve `slugd` rather
  than Go's new implicit `slug` name; Windows output retains `.exe`.
  Build/install targets and the GitHub workflow Dockerfile use these names.
  The Railway root Dockerfile already names its outputs explicitly.
- Added relative replacements for each module's transitive sibling-module
  dependencies. The renamed SLUG version tags do not exist yet. A plain
  path substitution made both builds and `go work sync` attempt to resolve
  those unpublished versions. Local replacements keep graph loading and
  per-module tidy/sync on this checkout without publishing release tags.
  Remove this scaffolding when the fork's module releases are established;
  these modules are not yet independently consumable published releases.
- Ran `go work sync` and `go mod tidy` for all ten modules. Updated the
  workspace explanation and build/install examples accordingly.

## Dependency changes examined

No new external dependency version was introduced. A before/after
`go list -m all` comparison against `0574b8ba` found only removal of unused
`github.com/kcalvinalvin/anet` from psbt's graph (the local btcutil already
uses its internal copy).

Sync reconciled btcutil's stale testify/objx/yaml requirements with the
versions already selected by the root workspace: testify v1.12.1,
objx v0.5.3, and go.yaml.in/yaml/v3 v3.0.5. Sums for obsolete btcd modules
were removed. Every added external go.sum line already existed in another
go.sum at the baseline commit; no checksum was invented or rewritten.

## Deliberately unchanged

- Production behavior, PoW parameters, genesis, chain parameters, and all
  test vectors and binary fixtures. An independent comparison of the 333
  changed Go files found only module-path substitutions and import ordering.
- Internal btcd identifiers and log tags, the `btcdconnected` notification,
  and the rpctest harness's internal temporary executable name.
- External btcsuite dependencies such as btclog and btcwallet references.
- Historical upstream issue/commit links, upstream attribution and badges,
  and the supplied handover/history documents.
- `master`, `slug-rebrand`, Railway configuration, and the public seed's
  deployed source. No PR was opened and no release tag was created.

## Verification

Mac arm64, Go 1.27.4. Workspace mode throughout builds/tests. Test runs used
`-count=1 -p 1 -timeout=15m`, with GOMAXPROCS=4 to bound parallel hashing.
Times below are wall-clock seconds including the Go command's overhead.

- All ten modules: `go build ./...` passed. `gofmt -l .` is empty.
- All ten per-module tidy commands and `go work sync` passed.
- `make build` passed (2.38 s); temporary-GOBIN `make install` and
  `make release-install` passed (8.48 s combined). An explicit Makefile
  GOBIN override also passed (1.85 s). Both binaries report 0.27.0.
  Windows output-name dry runs and release-script shell syntax passed.
- Full module suites passed: btcutil (8.89 s), txscript (2.05 s),
  chaincfg (2.03 s), chainhash (0.55 s), wire (3.11 s), btcec (11.90 s),
  psbt (1.13 s), and v2transport (0.83 s).
- Root, mempool, mining, peer, rpcclient, addrmgr, connmgr, and btcjson
  passed (35.15 s combined).
- Full `./database/...` passed (105.48 s), including ffldb and treap.
- Netsync fixture tests `TestCheckHeadersList` and
  `TestBuildBlockRequestSkipsInflightBlocks` passed (54.57 s).
- Blockchain subset `TestMainNetRetargetWindow`,
  `TestCheckProofOfWorkArgon2id`, `TestSlugBlockSubsidy`, and the
  `TestRegtest*` tests passed (4.30 s).
- Integration `TestGetChainTips` passed (14.54 s); this exercises the renamed
  runtime build target in rpctest.

### Live smoke proof

Built separate slugd/slugctl binaries and used an isolated temporary
datadir, loopback P2P port 29433, and loopback RPC port 29432.

- Version: `slugd version 0.27.0`.
- Started at height 0 with genesis
  `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`.
- `generate 2` reached height 2; block 1's coinbase paid exactly 10 SLUG.
- Added `junction.proxy.rlwy.net:16023`; the seed advertised
  `/btcwire:0.5.0/slug:0.27.0/` and height 3. The temporary node reorged
  onto the seed's chain and reached height 3, tip
  `42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`.
- `stop` returned `slugd stopping.` and the process exited 0.

The first smoke script incorrectly expected verbose transactions under
`tx`; this btcd RPC returns them under `rawtx`. That temporary script was
corrected and the complete smoke test passed; no node code changed.

## Known debt and limits

- The full blockchain and remaining integration suites remain unverified;
  the dedicated reduced-cost test PoW work is change set 9. This change
  set makes no full-suite claim. Only the named netsync fixture tests were
  rerun here, not its entire suite.
- The address module was built but its full suite was not rerun; the older
  address-vector debt reported in set 2 is not claimed resolved here.
- No Docker image or complete cross-platform release was built. The old
  release script still uses `go mod vendor` in workspace mode; CI's
  workspace job still calls `go work init` despite committed go.work.
  The auxiliary GitHub Dockerfile still exposes inherited Bitcoin ports.
  These pre-existing release/CI issues need a separate focused cleanup;
  Railway uses the root Dockerfile with SLUG ports.
- Signed checkpoints (set 7), the PoW security review (set 8), and Phase A
  closeout remain separate work. Master and the live seed branch stay at
  their handover tips until explicitly authorized otherwise.
