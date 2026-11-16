# Change set 18: job-bound birth and freshness

Branch: `slug-prospect-birth`, based on set-17 implementation `9cc49685`
(released on master as `7968e0a0`). Mike authorized building, deploying and
verifying sets 17 and 18 in sequence. Set 18 was developed while the deployed
set 17 underwent its public observation window.

Set 17 deployment `8d98e53c-ded4-4c1e-9b21-38f853229dc1` completed its
900.054-second, 31-sample observation and persisted restart at 06:40 UTC.
Railway then reported one healthy replica, no failures/alerts or pending work.

## Decision and behavior

Mike explicitly approved **D68**: automatic five-minute mining jobs, age starts
at issuance, and new admissions at or after expiry are rejected without grace.
The new decision and reasoning are recorded in `BRAWLCHAIN_SPEC.md`, section
10. Exact discovery time cannot be inferred from a miner's header or first
receipt. Issuance is a conservative bound that can start age five minutes
early but prevents withheld work from becoming younger at submission.

- Added `prospectjob`, with canonical immutable job bytes binding every field,
  strict JSON, positive millisecond timestamps, checked five-minute expiry,
  gym/chain/policy/context binding and a pre-work random challenge.
- Added proof format 2 (`SLUGPR02`) and authorization version 2, both committing
  the full job ID before work/signing. V1 bytes, gym identity, policy ID, DNA,
  and stable chain/work prospect IDs remain unchanged.
- Added explicitly created registry schema 3. It generates challenge/time
  internally, synchronizes issuance before output, looks up its own stored
  job, verifies key authorization and production work, checks current node and
  policy, then atomically records immutable birth, receipt, index and metadata.
- Added persistent clock high-water handling, including rejected expiry
  observations. New time-dependent writes fail closed on rollback. Failed
  admission may advance clock metadata but never creates a birth or sequence.
  Uncertain writes poison the handle until close/reopen.
- Historical duplicate recovery verifies evidence and returns original birth
  without current clock/node checks. It is explicitly labelled historical,
  never renewed eligibility. Multiple distinct prospects can share one job.
- Added `init-birth`, `issue-job-node`, `get-job`, `record-job-node` and
  `commitment-job`; public-input authorization preparation supports version 2.
  All conditional v2 verification commands explicitly report no birth claim.
  Exact protocols and trust boundaries are in `PROSPECT_BIRTH.md`.

## Deliberately unchanged and limits

Older registries are not migrated and old evidence receives no retrospective
birth. Schema-1/2 admission remains proof-v1 only. No player signing ceremony
or prospect cap is introduced. The job authority trusts its wall clock and
durable local storage; unsigned exports do not independently attest time.
Whole-store administrator rollback and reorg after a node observation remain
outside this guarantee. A future status journal must preserve original births.

These are library and command foundations. Automatic renewal/mining in the
end-user client, hosted wallet/login with tested export, public admission
service, production target calibration, ownership/transfers, status history
and signed publication remain subsequent slices. Deploying binaries creates
no live job authority or public service. No real private key was accessed.

No chain consensus, genesis, production PoW, seed startup, persistent chain
volume, public TCP endpoint, private RPC, checkpoint policy, Dockerfile or
dependency was changed. Existing CI/platform debt remains; CI is not claimed
green. The Phase A chain remains disposable, not the permanent genesis.

## Verification

- Independent Python job preimages, commitments, deterministic ECDSA v2
  signatures, full 256 MiB Argon work and DNA regenerated both fixture records
  exactly. No production output was used to create golden values.
- Seven-package production race suite passed: prospect 6.654s, gymauth 5.338s,
  prospectjob 5.530s, prospectnode 4.100s, registry 22.574s, prospect CLI 6.717s,
  and registry CLI 13.560s. Normal/tagged protocol and command suites passed.
  The final complete tagged registry suite passed in 0.503s.
- Tests cover every job field, strict nested JSON, overflow boundaries, zero
  challenge, exact expiry and delayed final check, old-work rebinding even
  with a fresh valid signature, multiple prospects, schema bypass, denied new
  admission under changed policy/reorg, persistent history during outages,
  output failures, clock rollback/reopen and uncertain writes before/after
  commit. Tagged builds refuse issuance, admission and production evidence
  output while still permitting public job inspection.
- Real subprocess kills after synchronized issuance, birth, and rejected
  expiry observations all recover correctly. Seventeen structural/evidence
  corruption cases fail closed. Review caught and fixed a guard callback's
  mutable Merkle slice alias; its regression passed with race detection in
  3.237s. Combined new crash/corruption/alias tests passed in 3.383s.
- Final review also aligned plain v2 verification with the other commands'
  `birth_verified:false` label. Focused normal/tagged tests passed 0.784s/
  0.354s and confirm byte-identical v1 output. No actionable review item remains.
- All ten normal module builds, Linux/amd64 CLI builds, vet, gofmt and diff
  checks passed. Final targeted CLI builds include the review fixes.
- Normal-node smoke passed: two isolated blocks and first 10 SLUG subsidy,
  report-only conflict, seed adoption to height 61, finalized checkpoint with
  60 descendants, full-depth level-1 chain verification, clean restart/exit.
- Extended smoke issued a fresh real-clock five-minute job, independently
  matched its ID/script/signature/work/DNA, admitted an immutable schema-3
  birth, recovered it across process reopen and unavailable-node configuration,
  and rejected old work attached to another fresh job even after re-signing.
  Public fixture key 1 and explicit all-ones test policy only; no proof was
  submitted as a block and the seed tip stayed unchanged. Evidence:
  `/private/tmp/slug-set18-smoke.json`, run directory
  `/private/tmp/slug-set18-smoke-typ3rm1p`.

Post-deployment evidence belongs outside the repository at
`/Users/taytrrip/slug/set18-deployment-report.md`, avoiding extra live deploys
for observation notes. Set 17's prior rollout report is alongside it.
