# Change set 14: gym-bound prospect work proofs

Branch: `slug-prospect-proof`, based on released master `0f44da5f`.
Mike requested promotion/verification of set 13 and continuation to set 14,
then explicitly allowed set 14 to begin during the rollout observation.

Set 13 is deployed as `16ac3a82-2b8b-4ada-a358-cd9480299a7d`. Its fresh public
observer verified the same 61-block chain and finalized checkpoint, stayed
connected for 900.052 seconds and passed restart. Railway reported one
healthy replica with no new runtime errors or active issues. Set 14 is a
separate topic branch; no set-14 deployment is included here.

## What changed

- Added the off-chain `prospect` package and `slugprospect` commitment/verify
  CLI. The exact versioned contract is in `PROSPECT_PROOFS.md`.
- A canonical, zero-value `OP_RETURN` output with visible `SLUGPR01` marker
  binds a gym identifier to chain identity, DNA version and an explicit
  prospect-target policy before work is performed. Exactly one reserved
  family output is permitted; multiple gym commitments are rejected.
- Verification checks an index-zero coinbase Merkle path and production
  Argon2id work against separately supplied chain/policy assertions. Success
  returns a stable header-based prospect ID, gym/policy binding and versioned
  DNA, explicitly labeled `verification: supplied-context`.
- Added strict bounded JSON, stripped coinbase preflight before decoder
  allocations, target/time/parent checks, serialized hashing and rejection
  of `slugtestpow` builds. There is no attacker-controlled Argon parameter.
- Added two independently derived fixtures. Python constructs the protocol,
  transaction and Merkle bytes; a standalone oracle imports only
  `golang.org/x/crypto/argon2` and uses literal production parameters. No
  project encoder or verifier output was copied into expected values.

The marked output is an engineering format for new mining templates, not a
chain consensus change. It allows both future solo and pool integrations to
prove the same kind of pre-work gym binding. Relaying a published proof does
not change its committed gym. Prospect target and block target are distinct;
the test fixtures deliberately meet the former and fail an artificial block
target of one. These are synthetic contexts, not actual accepted pool shares.

## Deliberately unchanged and remaining work

No node consensus, genesis, production PoW parameters, RPC exposure, signed
checkpoint behavior, database format, wallet or payout flow changed. The
normal node does not import the new package; the CLI runs offline. It needs
no signing key and does not mine or publish anything.

This is a verifier prerequisite for registration, not a completed ownership
registry. Context files are trust inputs, not authenticated chain snapshots.
The proof does not authenticate account control, establish discovery/birth
time, prove all transactions valid, or prove pool acceptance. Hidden duplicate
Merkle subtrees cannot be ruled out from the supplied branch. The stable ID
supports future deduplication but does not itself persist a replay database.

Before actual registration, implement trusted node/policy resolution,
prospect-target calibration to D30, freshness rules, an immutable birth
anchor, persistent IDs/tombstones across restart/reorg, account and transfer
authorization, and portable publication/checkpoint records. A late or repeated
submission must never reset aging. No default production prospect target is
shipped. The package limits active Argon calls to one; callers must still
bound admission and queues. This is not a global node memory/CPU quota.

## Verification

- Independent fixture regeneration: passed both complete records with actual
  256 MiB Argon2id, including commitment, policy ID, prospect ID and DNA.
- `go test -race ./prospect ./cmd/slugprospect ./fighter ./cmd/slugfighter
  -count=1 -timeout=120s`: passed. Package times: prospect 6.585s, prospect CLI
  3.367s, fighter 1.850s, fighter CLI 2.090s.
- `go test -tags slugtestpow ./prospect ./cmd/slugprospect -count=1
  -timeout=60s`: passed (0.482s / 0.253s). This tests explicit refusal of
  production verification, not acceptance of cheap work. Production-vector
  tests run in the normal build.
- Covered competing commitments and nonminimal pushes, unknown marker
  versions, null/duplicate/unknown/missing JSON fields, truncated and huge
  declared lengths, canonical coinbase heights, output-value bounds,
  parent/time/bits/target mismatches, Merkle mutations, target equality and
  adjacent values, zero results on rejection, stable IDs under hex case
  changes, rejection before expensive hashing, and concurrent use.
- All ten normal module builds passed. Vet of the new package/CLI passed.
  Formatting and diff checks were clean. No dependency changes.
- Built CLI commands matched independent public fixtures; invalid proof
  invocation exited nonzero with no verified record.
- Required normal node smoke passed: version 0.27.0, two isolated blocks,
  first coinbase paying 10 SLUG, report-only conflict, normal adoption of the
  seed's 61-block chain, finalized/no-conflict with 60 descendants,
  `verifychain 1 61`, clean restart and retained finality.
- Smoke result: `/private/tmp/slug-set14-smoke.json`; temporary data/logs:
  `/private/tmp/slug-set14-smoke-ctnmsq6b`.
- Independent security review found no actionable discrepancy against the
  specified contract. It confirmed the limitations above; this is not a
  whole-system security certification.

Earlier complete blockchain/integration suite evidence remains in set 9.
The known CI/address-vector maintenance debt remains; CI is not claimed green.
