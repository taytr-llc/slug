# Change set 15: persistent prospect evidence registry

Branch: `slug-prospect-registry`, based on released master `fdeebae2`.
Mike explicitly authorized deploying set 14 and starting set 15 during its
verification window. Set 15 remains a separate topic branch.

Set 14 deployed successfully as `a69fbf95-1ad9-44a0-9644-39b564562e3f`.
Its fresh public observer verified the existing 61-block chain and finalized
checkpoint, remained connected through all 31 samples over 900.034 seconds,
and passed restart with clean exit. The seed remains on master `fdeebae2`;
this set does not promote or deploy the registry.

## What changed

- Added the offline `registry` package and `slugregistry` command for an
  explicitly initialized, chain-specific local evidence database. The exact
  contract and commands are in `PROSPECT_REGISTRY.md`.
- Every admission re-verifies the raw context and proof with production
  Argon2id. Stable prospect IDs survive restart, and repeated submissions
  return the first complete evidence and receipt sequence unchanged.
- Reused LevelDB with synchronous atomic batches, process locking, a mutex
  around admission, format/chain guards, bounded records/pages, and strict
  corruption handling. Uncertain write outcomes disable the handle until
  reopening; a retry then reconciles by stable ID.
- Reads re-verify stored evidence. Read-only lookup, paginated listing, and
  streaming NDJSON export expose portable conditional records.

## Deliberately unchanged and remaining work

This is persistent evidence storage, not completed prospect enrollment.
Context/policy authentication, account and transfer authority, trustworthy
birth anchors, freshness, target calibration, canonicality/status assertions,
and signed publication/checkpoint integration remain ahead. Sequence numbers
are local receipt order, not discovery time or age. No timestamps or age
values are fabricated from receipt time.

There are no record deletion, status reset, or pruning commands. Future
invalidation must retain original IDs and evidence. The store does not
authenticate its administrator or detect replacement of the entire database
with an old copy. No public admission service or automatic registry startup
is added. Node consensus, genesis, PoW parameters, checkpoint behavior,
existing node database, seed startup, network exposure and wallets are
unchanged. No operator key was accessed or needed. No new dependencies.

## Verification

- Combined production-PoW race tests passed:
  `go test -race ./registry ./cmd/slugregistry ./prospect ./cmd/slugprospect
  ./fighter ./cmd/slugfighter -count=1 -timeout=120s`. Package times were
  11.193s, 6.439s, 8.193s, 4.369s, 1.352s and 1.299s respectively.
- Tagged refusal tests passed for registry, registry CLI, prospect and
  prospect CLI (0.338s, 0.566s, 0.668s and 0.955s). Tagged builds cannot admit
  evidence or emit stored production receipts through get/list/export.
- Tested first-evidence preservation across reopen and case/time-bound
  variants; eight concurrent duplicate submissions; invalid and foreign-chain
  inputs; defensive copies; page bounds and index gaps; corrupt/unknown
  schemas; cross-process locks; an acknowledged synchronous write followed
  by process kill; and injected failures both before and after actual commit.
  Both uncertain outcomes require reopening and then reconcile correctly.
- Read validation checks altered stored DNA against the independent set-14
  fixtures. Boundary review corrected generic digest handling so a
  mathematically possible zero digest is not confused with a forbidden zero
  genesis. No new cryptographic vectors were derived from implementation output.
- All ten normal module builds passed, as did vet and repository-wide
  formatting/diff checks. Linux/amd64 registry CLI cross-build passed.
  Runtime tests ran on macOS. Windows initialization remains unvalidated;
  it must support the required directory synchronization to succeed.
- Built CLI execution matched an independent fixture, preserved the original
  receipt on replay, exported the record, and exited 1 with empty stdout for
  invalid evidence. Evidence: `/private/tmp/slug-set15-cli.json`.
- Required normal node smoke passed: version 0.27.0, two isolated mined
  blocks, 10 SLUG first subsidy, report-only checkpoint conflict, ordinary
  adoption of the seed's 61-block chain, finalized/no-conflict checkpoint
  with 60 descendants, `verifychain 1 61`, and clean restart/exit.
  Evidence: `/private/tmp/slug-set15-smoke.json`; data/logs:
  `/private/tmp/slug-set15-smoke-mx0s_om6`.
- Independent implementation review found no remaining actionable issue
  against this local evidence-store contract after the boundary fixes.

Earlier complete blockchain/integration suite evidence remains in set 9.
The known CI/address-vector maintenance debt remains; CI is not claimed green.
