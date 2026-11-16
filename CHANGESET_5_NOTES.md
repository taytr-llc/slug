# Change set 5: rebrand + test-vector sweep (branch slug-rebrand, stacked on slug-deploy)

Two parts: (A) fix every test suite that change sets 1 and 2 left red, with
expectations re-derived rather than copied from the code's own output;
(B) operator-visible rebrand from btcd to slugd/slugctl.

## ACTION REQUIRED when this branch is deployed to Railway

The Docker image's daemon binary is now named slugd. The Railway service
start command must change at the same moment from:

  btcd --notls --rpcuser=u --rpcpass=p --datadir=/data

to:

  slugd --notls --rpcuser=u --rpcpass=p --datadir=/data

If the branch deploys without the start-command change, the service will
fail to start ("executable btcd could not be found" style error). The
currently deployed branch (slug-deploy) is unaffected until then.

## Part A: test fixes

Derivation rule for all string vectors: decode the Bitcoin expected string,
swap ONLY the version bytes (or bech32 HRP), re-encode with a fresh checksum.
Payloads (keys, hash160s, witness programs) are byte-identical to the
canonical Bitcoin/BIP32 vectors, so each corrected expectation is proven by
construction, not trusted from the code under test. 96 replacements:

- btcutil/hdkeychain (extendedkey_test.go, example_test.go, bench_test.go):
  xprv/xpub expectations re-encoded with SLUG HD version bytes (priv
  0x736c7567, pub 0x534c5547; serializations start RWC8zh / JgduaQ), P2PKH
  addresses with version 0x53. Two examples (defaultWalletLayout, audits)
  hardcode the BIP32 master key as a string split across two literals; both
  halves replaced with the SLUG encoding of the same key.
- TestCloneWithVersion: two expectations deliberately KEPT as Bitcoin
  encodings. That test clones keys to explicitly passed version bytes
  (0x0488b21e / 0x0488ade4), so its correct outputs are the Bitcoin strings.
- btcutil/wif_test.go: mainnet WIF expectations re-encoded with version
  0xd3. The intentionally invalid WIF case (bad checksum) untouched.
- btcutil/block_test.go TestBlock: expected hash is now the Argon2id PoW
  hash of the unchanged Bitcoin block 100000 header,
  1bc3113253d9ead82ae75318b4efa6fddd783bb33da0de8fc13c2675bcf38547.
  Verified independently: a throwaway program hashed the serialized header
  with golang.org/x/crypto/argon2 directly and got the identical value
  before the expectation was written.
- btcutil/gcs/builder: test P2SH address re-encoded with version 0x47
  (same hash160, so the filter expectations are unchanged).
- txscript (p2a_test.go, taproot_test.go, example_test.go): bech32m
  expectations re-encoded with HRP "slug" (same witness programs);
  ExamplePayToAddrScript's input address and ExampleExtractPkScriptAddrs'
  expected address re-encoded with version 0x53.
- database/example_test.go: expected genesis serialized size 285 -> 218
  bytes (the SLUG genesis; verified by serializing
  chaincfg.MainNetParams.GenesisBlock, hash 7893676099cf...).
- database/testdata/blocks1-256.bz2: ffldb fixture retagged. Each record's
  4-byte network prefix was Bitcoin's magic; wire.MainNet is now SLUG's
  0x534c5547, so loadBlocks rejected every block. All 255 records retagged
  to the SLUG magic; block bytes untouched.
- config_test.go: sample config filename updated (see rebrand).
- netsync (manager_test.go + testdata/blockheaders-mainnet-1-11.txt): the
  fixture held Bitcoin mainnet headers 1-11 rooted at the Bitcoin genesis;
  both tests using it (TestCheckHeadersList,
  TestBuildBlockRequestSkipsInflightBlocks) failed with "previous block
  000000000019d668... is not known". Regenerated: 12 headers chained on the
  SLUG genesis at genesis difficulty (bits 0x207fffff, Argon2id ground per
  header, timestamps genesis +120s each). The file holds headers 1-11; the
  test's checkpoint (height 11), block 10, and block 12 hash strings were
  updated to the generated chain (block 11:
  306ffe041d1b468735ad5e85de448a83bcc4e629feed2c1ee8da58ae46b725fb).
- integration (getchaintips_test.go): the 9 pre-baked regtest blocks
  (main chain 1-2-3-4, branch 2a-3a-4a-5a, and 4b competing on 3a) were
  SHA256d-mined and rejected under Argon2id. Regenerated with identical
  topology: coinbase-only blocks on the regtest genesis, subsidy from
  blockchain.CalcBlockSubsidy, BIP34 coinbase height in the OP_N form
  btcd's compareScript requires, Argon2id ground to the regtest target.
  All expected hashes in the test are computed from the blocks at runtime,
  so only the hex constants changed.

## Part B: rebrand

- cmd/btcctl renamed to cmd/slugctl (main file now slugctl.go). The Docker
  build names the daemon explicitly: slugd. `slugd --version` prints
  "slugd version 0.27.0".
- Daemon defaults: home dir AppDataDir("slug") (was "btcd"), config file
  slug.conf, log file slug.log, sample config sample-slug.conf (file
  renamed, contents updated). Nodes run with explicit --datadir (Railway,
  Mike's Mac, Ren's VM) are unaffected.
- slugctl defaults: home AppDataDir("slugctl"), config slugctl.conf, and it
  looks for the server's rpc.cert and slug.conf under the slug home, so
  slugctl finds a default slugd install with no flags.
- Wire user agent: server userAgentName "slug"; peers now see
  /btcwire:0.5.0/slug:0.27.0/. The /btcwire:0.5.0/ protocol prefix is
  unchanged (it names the wire protocol implementation, not the chain).
- RPC stop now returns "slugd stopping." (help text updated to match).
- upgrade.go: the pre-0.3.3 legacy migration now points at slug paths
  (~/.slug, slug.db). SLUG has no such installs, so it is inert, and slugd
  can never migrate a real btcd installation's data by accident.
- Dev tools (addblock, findcheckpoint, dbtool): default home now the slug
  home; help text says slugd.
- doc.go and flag help: program name slugd, config path ~/.slug/slug.conf,
  currency wording SLUG instead of bitcoins/BTC in the generate and
  minrelaytxfee descriptions.
- Dockerfile: builds slugd (ENTRYPOINT ["slugd"]) plus the cmd tools
  (slugctl, addblock, findcheckpoint, gencerts); header notes the Railway
  start-command change above.

Deliberately NOT renamed (internal or protocol, renaming buys nothing and
risks breakage): Go module path github.com/btcsuite/btcd (proposed as a
separate change set 6, Mike's call: it is a huge mechanical diff and makes
future upstream btcd merges harder), package and variable names (btcdLog
etc.), the log subsystem tag printed in log lines (still "BTCD:"), the
btcjson "btcdconnected" notification method (RPC wire protocol string),
and btcwallet paths in slugctl (separate app, not in this repo).

## Verification

- Full build of all modules; gofmt clean.
- Green suites: btcutil (all packages), txscript, chaincfg, chainhash,
  wire, btcec, mempool, mining, peer, rpcclient, psbt, addrmgr, connmgr,
  btcjson, root package (including TestCreateDefaultConfigFile with the
  renamed sample config), database (including ffldb with the retagged
  fixture; the module takes about 4.5 minutes, the ffldb interface tests
  store and reload all 255 fixture blocks). netsync's two fixture tests
  and integration's TestGetChainTips pass on the regenerated fixtures.
- Live smoke test of the rebranded binaries: a fresh slugd mined 2 blocks
  on an isolated datadir; the startup warning confirmed the new defaults
  (/root/.slug/slug.conf). It then peered with the pre-rebrand node: the
  old node reported its subver as /btcwire:0.5.0/slug:0.27.0/, and the
  rebranded node reorged onto the shared chain and synced to height 3
  with block 3 hash 42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188,
  identical to the old nodes' (the block Mike's Mac mined).
- The full netsync package also passes end to end (368s; it is slow
  because its helpers grind Argon2id blocks, not because anything fails).
- Known remaining debt: the blockchain suite grinds large numbers of
  blocks through Argon2id in its test helpers and does not finish in
  reasonable time (still hashing at timeout, not failing assertions). It
  needs a dedicated pass, likely a test-only reduced-cost PoW knob. The
  two Bitcoin-mainnet fixture tests in blockchain remain skipped with
  explanatory comments. The rest of the integration module (the rpctest
  suites beyond TestGetChainTips) was not run.

## Fetch / build / push (Mike's Mac)

  cd ~/slug/slug
  git fetch /path/to/rebrand.bundle slug-rebrand:slug-rebrand
  git checkout slug-rebrand
  go build ./...
  cd btcutil && go test ./... && cd ../txscript && go test ./... && cd ..
  go test -run 'TestCheckHeadersList|TestBuildBlockRequestSkipsInflightBlocks' ./netsync/
  git push origin slug-rebrand
