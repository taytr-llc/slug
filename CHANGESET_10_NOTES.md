# Change set 10: enroll the checkpoint authority

Branch `slug-checkpoint-authority`, stacked on set 9 (`273b5e17`). This
completes public-key enrollment for set 7; an operator-signed checkpoint is
still pending.

## Change

Mike supplied this 32-byte Ed25519 public key on November 15, 2026:

```text
811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3
```

The exact bytes are compiled into `chaincfg.MainNetParams.CheckpointPublicKey`
for the disposable Phase A SLUG network. Other network authorities remain
empty. Updated the current operator guide, ADR action status, and AGENTS.
Historical change-set notes retain their original verification state.

The loader regression now explicitly clears a local parameter copy for the
unconfigured-authority case. It separately verifies that a fixture signer is
rejected by the enrolled operator authority, avoiding a false-positive test
after MainNetParams stopped having a nil key.

## Deliberately unchanged

Signed checkpoints remain report-only. No consensus checkpoints, chain
selection, block validation, genesis, PoW parameters or signature format
changed. There is no runtime authority override. No private key was opened,
generated, copied, or used by the agent. At Mike's request, only existence,
size and permissions of his private file were checked; its contents were
not read.

Enrollment does not claim proof of possession or an official checkpoint.
Verification of an actual operator signature awaits Mike's offline signing
of an eligible public height/hash. No signed record was fabricated or
imported, and no master merge or Railway deployment was performed.

## Verification

- All ten modules build; gofmt and diff checks are clean.
- Complete chaincfg (1.689 s), root (0.712 s), offline tool (0.610 s),
  btcjson (0.766 s), and rpcclient (4.131 s) suites passed. Blockchain
  signed-checkpoint tests passed (0.407 s).
- Required production smoke passed: version 0.27.0, unchanged genesis,
  two mined blocks, 10 SLUG subsidy, seed subversion, and convergence to
  height 3 with tip
  `42d92f37dd5d5f1e52aa374d00635ecd4d5983a092ea502fe4d0e9b2f7105188`.
  Both the fresh and synchronized node reported `mode: report-only`,
  `authorityconfigured: true`, the exact supplied public hex, and an empty
  checkpoint list. The node stopped cleanly with exit 0.

## Remaining activation

The first positive checkpoint at height 1 needs tip height 61 (60
descendants). Once an eligible validated height/hash is available, Mike
signs it offline and supplies only the public JSON document. The enrolled
client can then verify and report that record. The live seed still tracks
`slug-rebrand`; it does not receive this enrollment until an explicitly
authorized deployment.
