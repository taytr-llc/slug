# Change set 7: report-only signed checkpoints and offline tooling

Branch `slug-signed-checkpoints`, stacked on set 6 (`df655087`).
Implementation is ready; operator enrollment and the first official signed
testnet checkpoint remain pending. Mike has no checkpoint key yet and
explicitly chose to run key generation himself on his Mac.

## Decisions and changes

Mike selected report-only finality: a signature declares the official league
record, a conflicting local branch is flagged, and normal chain selection
and validation continue. The signing key cannot veto the chain. This is
recorded in `docs/adr/0001-report-only-signed-checkpoints.md`; earlier
fast-sync-anchor language in planning documents is superseded.

- `chaincfg.SignedCheckpoint` authenticates a versioned, domain-separated
  binary message containing network magic, genesis hash, positive height,
  and block hash. It uses standard Go Ed25519. JSON is strict transport,
  with exact field/hex lengths and a 4096-byte file limit.
- `slugcheckpoint keygen`, `sign`, and `verify` work offline. Key creation
  writes a private raw seed with mode 0600 and exclusive creation; only
  public hex is printed. Signing takes a private file, never key material
  in command-line arguments. The operator tool is included in Makefile
  build/install; root Docker builds all command packages as before.
- `chaincfg.Params.CheckpointPublicKey` holds the compiled authority.
  Mainnet remains explicitly unconfigured until Mike supplies his public
  key. There is no generated authority or runtime key override.
- Repeatable `--signedcheckpoint` files are verified and sorted at startup.
  Invalid files, signatures, chains, or duplicate heights are rejected.
  The blockchain copies all records and key bytes before retaining them.
- `getcheckpointinfo` and `rpcclient.GetCheckpointInfo` expose a coherent
  best-chain snapshot, authority state, signed records, descendant counts,
  and pending/provisional/finalized/conflict status. A conflicting hash is
  shown alongside the official signed hash and signature. Limited RPC
  users can read this state. Reloading files requires a restart.
- Sixty descendants means `tip height - checkpoint height >= 60`, excluding
  the checkpoint itself. Local status can change after a reorg without
  revoking the signed declaration or choosing the community's branch.

## Deliberate non-changes

Signed records never enter `Config.Checkpoints`, checkpoint fast-add,
script-verification shortcuts, proof-of-work checks, or reorg decisions.
Legacy `--addcheckpoint` and `--nocheckpoints` retain their old behavior;
they are not the signed league mechanism. Consensus, genesis, stored-chain
format, mining parameters, and the seed deployment are unchanged.

No operator private key was generated or read. Tests use separate public
test fixtures and disposable test keys. No official checkpoint was signed.
At seed height 3, no positive height has 60 descendants; the first eligible
height is 1 at tip 61. No extra blocks were mined on the public seed chain.

## Verification

Go 1.27.4, Mac arm64, workspace mode. All ten module builds passed and
`gofmt -l .` was empty. No module/dependency changes were needed.

- Full root, btcjson, rpcclient, offline CLI, chaincfg, and wire suites:
  passed uncached, 12.42 s combined.
- Signed-checkpoint blockchain tests plus the existing PoW, subsidy,
  retarget, and regtest subset: passed uncached, 4.16 s.
- The new snapshot tests passed under the race detector (1.408 s). They
  cover 59/60 descendants, pending height, conflicting reorg and recovery,
  caller mutation isolation, concurrent tip changes, and unchanged
  consensus-checkpoint/chain-selection state.
- The envelope tests independently sign literal canonical bytes and cover
  every signed-field tamper, key/chain replay, endian encoding, duplicate
  and unknown JSON fields, malformed sizes, and atomic decode failure.
- Offline tool tests cover permission/key lengths, no overwrite/symlink
  clobber, tampering, wrong key/network, argument validation, and limits.
- Independent review found and fixed a loader boundary bug: encoding/json
  trims outer whitespace before custom decoding. The loader now checks
  the entire bounded read before unmarshalling; regression cases include
  a valid signed JSON prefix with oversized padding and hidden trailing
  data. The relevant root tests were rerun successfully.
- Netsync fixture tests passed (53.26 s); integration TestGetChainTips
  passed (15.14 s). Full blockchain/integration debt remains set 9.
- Required smoke passed: fresh node, correct genesis and version, two
  mined blocks, exactly 10 SLUG in block 1, seed subversion verified,
  convergence to height 3 and tip
  `42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`,
  then clean `slugd stopping.` and exit 0. The new RPC/help also passed,
  reporting `authorityconfigured: false`, report-only mode, and coherent
  heights/hashes before and after sync. Authenticated imports/reporting
  were exercised in process with test-only keys, not a fabricated live key.

## Remaining operator steps and limits

1. Mike runs `slugcheckpoint keygen` himself and backs up the private file
   offline; only the printed public hex is supplied for client enrollment.
2. Compile that public key, then verify/sign/publish an eligible testnet
   block once it has 60 descendants. These are unfinished set 7 activation
   steps, not permission for an automatic deploy or seed-chain mining.
3. File distribution, live reload, multi-key rotation, and automatic
   checkpoint publication are not included. A signature authenticates a
   statement, not proof that the operator followed the burial policy.

See `docs/signed-checkpoints.md` and `cmd/slugcheckpoint/README.md` for the
reviewable commands and policy. Security review and test-suite work can
continue on this implementation while operator enrollment is pending.
