# Change set 30 — approved production recovery profile foundation

Branch: `slug-production-profile`, stacked on set 29 `slug-recovery-vault`.
Paired wallet: `e9e578c3bb948f0b7284429bb81af9bb339e319d`.

Mike approved D73's separate production recovery profile: dedicated coin type,
standard payment scopes 44/49/84/86, uniform nested49, native84 defaults and a
permanent-chain-bound gym key. The wallet implements the exact candidate
descriptor, strict version-2 recovery format and explicit manager schema 10
using only two published test fixtures. Its production descriptor table remains
empty. Final coin allocation, permanent genesis and full recovery activation
remain separate gates.

The paired implementation includes independently derived payment and SLIP13/
SLIP10 gym vectors, retained export/fresh restore, per-wallet discovery/balance
and spending behavior, bounded fixture scanning, corruption/rollback refusal
and unchanged old profiles. This chain set records D73, pins that wallet in the
desktop module, and builds bundle version 30. Node consensus and live seed
configuration are unchanged. Existing desktop accounts remain the old public
fixtures; this does not enable real signup or production custody.

Mike also required D74: a thorough joint design review before friends/tester or
public product release. The spec, standing rules and `LAUNCH_READINESS.md` now
track that gate. The approved logo handoff is preserved in workspace brand
assets for the forthcoming desktop design work, without recreating the mark.

## Validation

- Independent profile standards/oracle, strict metadata, rare derivation retries,
  cancellation and malformed public keys pass normal/race/tagged checks.
- Schema10 corruption, unsupported mutation, capacity/cursor boundaries and
  creation rollback pass; actual RPC receive/change defaults match independent
  vectors and preserve v1 behavior.
- Normal-PoW candidate acceptance passed 75.86s with unchanged sources through
  height 103. A file-only fresh restore recovered exact root/gym, balance and
  history, signed all four funded script types together, and spent its native
  SegWit change afterward. The original wallet path was unavailable.
- All ten chain and six wallet module builds, relevant vet and normal host/Linux
  node builds pass with stable runtime hashes. All six wallet-module normal
  suites pass; the final root run took 166.511s with 251 source/fixture hashes
  unchanged.
- Four existing desktop/account/identity/vault flows passed 124.229s with 311
  captured files unchanged, all 15 listeners closed and no owned processes.

The required chain smoke, clean paired Mac artifact, packaged helper acceptance,
exact-source wallet CI and live rollout observation are recorded in the workspace
set30 release evidence as they complete. Promotion remains gated on those checks.
No PR, real wallet key, public-chain mining or public wallet endpoint is part of
this set. The candidate gRPC service and production resumable recovery scanner
are not claimed as completed interfaces.
