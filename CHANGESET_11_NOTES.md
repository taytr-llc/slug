# Change set 11: first signed Phase A checkpoint

Branch `slug-first-checkpoint`, stacked on set 10 (`0ca7466a`). Mike signed
the eligible height-1 record offline. Its public JSON is archived unchanged
at `checkpoints/phase-a/height-1.json`, independently verified, and loaded
successfully into the enrolled client. This completes set 7's signed
testnet checkpoint deliverable.

## Signed record and preparation

- Network: disposable SLUG Phase A (`mainnet`, magic 1397511495).
- Genesis: `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`
- Height: **1**
- Block hash: `729d92f2de29fa6415f4920df49164a8a9824971ff972d42037ee7c9d41f470f`
- Public authority: `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`
- Public JSON SHA-256: `8a21dc887e9107aef878c121c94cac9e02e5b5c99f67958c0e913526a5a9cebd`

Before signing, a normal production-profile node synchronized the existing
height-3 chain and mined 58 further blocks. A fresh observer connected only
to the seed downloaded and validated the same height-61 chain. Canonical
heights 0, 1, 3 and 61 were checked, preserving the original height-3 tip.
A separate direct Argon2id computation over the raw height-1 header with
literal production parameters confirmed its identity and target.

The observed height-61 tip is
`1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0`.
Height 1 therefore has 60 descendants; the checkpoint itself is not counted.
The seed now serves this extended chain without a software deployment.

## Verification and import

- The offline tool verified the 365-byte public document against the exact
  enrolled public key, expected network and genesis.
- Independent verification manually assembled the 99 signing bytes from
  the documented domain and fields and used standard Ed25519, without the
  project's signing codec. The signature passed; a changed message failed.
- A fresh normal-build node loaded the signed record explicitly. At genesis
  it reported `pending`. It then mined two valid blocks on a conflicting
  local branch, paying 10 SLUG per block, and reported `conflict` while
  preserving the official signed hash. Mining and chain validation continued.
- Connecting that node to the seed caused ordinary chain selection to adopt
  the height-61 chain. `getcheckpointinfo` then reported `mode: report-only`,
  the enrolled key, `conflict: false`, matching official/local height-1
  hashes, `status: finalized`, and exactly 60 descendants.
- An explicit `verifychain 1 61` returned true. This is supplemental block
  sanity checking, not a complete snapshot/UTXO audit; fresh normal sync
  supplies ordinary chain validation. Depth 0 was not used because the
  current RPC implementation checks zero blocks at that depth.
- The node stopped cleanly, restarted with the same public record and
  persisted chain, and again reported finalized. The final stop exited 0.
- The required smoke also verified version 0.27.0, unchanged genesis,
  expected seed subversion, normal subsidy, and original height-3 ancestry.
- All ten modules build normally; gofmt/diff checks are clean. No Go code,
  dependency, signing format, or production consensus parameter changed.

## Distribution and boundaries

The Dockerfile now copies public records to `/usr/share/slug/checkpoints/`.
They are not enabled automatically: the proposed seed startup flag is
`--signedcheckpoint=/usr/share/slug/checkpoints/phase-a/height-1.json`.
Local container build/run was unavailable because Docker's daemon was not
running; the packaging path and source file were checked, and actual Go
binaries/import behavior were verified as described above.

No private seed was opened, copied or used by the agent. Only Mike ran the
signing operation. No master merge, seed software deployment, Railway
configuration change, enforced checkpoint, or production-genesis change
occurred. Public record distribution and signed finality reporting do not
give the signing key a chain-selection veto.

Phase A closeout is now ready for Mike's explicit merge/deployment decision
under HANDOVER.md. The residual security and CI/release limitations in sets
8 and 9 remain applicable; this record does not erase them.
