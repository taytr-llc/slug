# Change set 17: gym-key authorization

Branch: `slug-gym-identity`, based on the completed set-16 implementation
`25884a6a` (released on master as `e94c1cfa`). Mike authorized building sets
17 and 18 and deploying and verifying both. Each remains a separate stacked
change set and is promoted only after its local checks pass.

## What changed

- Added `gymauth`: chain-bound self-certifying gym IDs, action-specific full
  proof digests, secp256k1 ECDSA compressed keys, canonical strict DER low-S
  signatures, bounded JSON parsing, and an automatic in-memory client signing
  helper. Exact bytes are in `GYM_AUTHORIZATION.md`.
- Added public-input gym-ID, digest-preparation, authorization-verification
  and authenticated-admission commands. Online paths retain the trusted node
  checks and final pre-commit observation from set 16. Signature verification
  precedes production Argon work. CLI commands never read private keys.
- Added explicit registry schema 2, initialized in a new directory. The
  original proof, context, verified result, authorization and sequence/index
  metadata are synchronized atomically. Evidence-only methods cannot bypass
  the schema's authorization requirement. Replays and restarts preserve first
  evidence; uncertain writes still require close/reopen before retry.
- Added independent Python secp256k1/RFC6979 signature and new gym-bound work
  fixtures, using the independent Argon oracle and DNA reference. Public test
  scalars 1 and 2 are reproducible fixtures only.

## Deliberately unchanged and remaining work

Schema-1 receipts retain their bytes and meaning; no automatic migration or
retrospective association of an arbitrary old gym ID with a new key occurs.
The authentication label means control of the specific gym key for a specific
admission. It is not proof of an account, a person, wallet balances, transfer
authority, fighter birth, or permanent chain canonicality. Wallet/login and
export/recovery integration remain ahead; signing introduces no player-facing
prospect selection or signing step under D44.

There is no public admission server, calibrated default target, live registry
installation, new production key, or remote command execution on Railway.
Deploying ships the tested binaries only. No consensus, genesis, PoW settings,
seed configuration, persistent chain volume, private RPC exposure, checkpoint
behavior, or dependencies changed. Existing CI/platform debt remains.

Set 18 will separately implement Mike's approved five-minute pre-work job
rule and record its reasoning as D68: age begins at issuance, with no grace
for a new admission after expiry. Set 17 does not assign birth metadata.

## Verification

- Independent Python fixture regeneration passed, including production
  Argon2id work and DNA. Both exact canonical signatures match the Go helper.
- Production race tests passed for `gymauth` (5.385s), `registry` (12.959s),
  `cmd/slugprospect` (5.197s), and `cmd/slugregistry` (8.606s).
- Normal and tagged command tests and authorization tests passed. A dedicated
  tagged schema-2 test confirms production receipts can be structurally opened
  but cannot be admitted or emitted through Get/List/Export under cheap PoW.
- Tests cover malformed/duplicate JSON, uncompressed/wrong keys, strict DER
  suffix rejection, high-S twins, wrong policy/action/proof fields, independent
  digest order/length boundaries, schema bypass, atomic authorization storage,
  first and replay guard rejection, restart, defensive copies, output failures,
  before/after-commit uncertainty, and corrupt stored signature rejection.
- All ten normal module builds, Linux/amd64 CLI builds, vet, full-repository
  gofmt and diff checks passed. Independent reviews found no actionable issue.
- Required normal-node smoke passed: two mined blocks, first subsidy 10 SLUG,
  report-only checkpoint conflict, public seed adoption to height 61, finalized
  checkpoint with 60 descendants, `verifychain 1 61`, clean restart/exit.
- Real-node integration independently constructed fresh gym-bound proof bytes,
  signature, Argon work and DNA; public preparation output, online authorized
  verification, schema-2 atomic admission and persisted replay matched. A
  different wallet key was rejected without a second receipt. Seed tip stayed
  unchanged. Only explicit all-ones test policy and public fixture key 1 were
  used. Evidence: `/private/tmp/slug-set17-smoke.json`; run directory:
  `/private/tmp/slug-set17-smoke-3okadwa7`.

Post-deployment evidence is recorded outside the repository in
`/Users/taytrrip/slug/set17-deployment-report.md` to avoid an extra live deploy
merely for observation notes. CI is not claimed green.
