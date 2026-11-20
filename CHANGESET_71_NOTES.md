# Change set 71 — current recovery through signed fixture accounts

Status: implementation and qualification complete on the stacked
`slug-hosted-current-recovery` topic branch. Independent references, focused
normal/race checks, full wallet/desktop suites, actual recovery, normal packaged
helper and the required smoke passed. The live seed remains at Set50; no linked
node or deployment input changed.

This set connects an explicit hosted-fixture Pool mode to schema-2 signed login
and reset, purpose-separated vault signing, current public wallet metadata and
authorized recovery-file export. The normal packaged helper will expose the
route through existing fixture login/reset and backup controls in a separate
data directory. The legacy Pool route remains available.

`HOSTED_CURRENT_RECOVERY.md` is the adopted contract, SHA256
`92b247a5c3f47e637ed0dbf09d08c8f580a881cc80ff114a4c22d2e119572a86`.
Its external freeze and finite review closure are in `../set71-prep/frozen`.
The two published fixture identities and previously published provider/wrapping
keys are the only identities permitted. This is local simulated hosted custody,
not a live provider, real signup, production issuance or product release.

Reset must preserve the existing wallet and gym, revoke old sessions before
joining work and retain the approved 24-hour hold. Interrupted reset must remain
resumable through a reachable signed-out controller. Current exports capture the
installed wallet's committed cursors; they do not silently rewrite the old vault
snapshot or claim continuous disaster-recovery coverage. Recovery plaintext
stays in memory until the original export authority admits its bounded final
file transaction.

The earlier draft review corrected initializer identity pinning, process-local
versus durable capture recovery, pending-reset startup, held enrolled reopening,
exact pending-reset lookup, lock ordering and canonical state encodings before
any implementation. Expected values come from separately locked Python producers
using preserved public inputs, not new Go output.

## Observed qualification so far

- All three independently reviewed producers passed, and Python 3.9.6 reproduced
  Python 3.14.3 byte-for-byte. The portable source/input bundle and expected-file
  locations are documented in `testdata/hosted-current-recovery/README.md`.
  Preserve the superseded input-copy race and source-only review corrections;
  no expected output was generated before the consistent input/source locks.
- Additive pinned authority/coordinator constructors: complete `gymaccount` and
  `gymprovider` normal/race tests and profile-bound vet passed (19.616s, 22.999s,
  0.291s), with 2240 inputs unchanged. Exact-ID mismatch refuses before LevelDB
  open; old random-ID APIs remain available.
- Exact schema-2 account/barrier lookups: focused normal/race tests and vet
  passed (1.792s, 2.897s, 0.554s), with 2242 inputs unchanged. These read-only
  methods grant no session, reset permit or clock/hold mutation.
- Public wallet current-capture plus affected activity tests passed normal,
  race and cheap-profile refusal (15.375s, 93.740s, 6.959s), with 940 inputs
  unchanged and process groups clean. The earlier compile-only unused-import
  failure is preserved; the corrected compilation passed before runtime tests.
- All 365 frontend tests passed in 2.039s with 88 inputs unchanged. New cases
  pin hosted versus old Pool snapshots and allow the exact pending fixture to
  resume reset while signed out, without login or implicit mining.
- Focused hosted/controller/native/selection checks passed normal (11.196s),
  race (48.954s) and vet (1.280s), with 1624 inputs unchanged. The first attempt
  is preserved: one fixture supplied macOS's `/var` alias where the constructor
  requires a canonical path, and two assertions expected the completed-reset
  rejection during the earlier pending barrier. Test-only corrections retain
  exact phase-specific rejection checks; no production check was relaxed.
  All seven desktop packages then passed in 224.986s, with no source exclusions
  or changes during qualification.
- All ten chain modules built. All six wallet modules built and passed vet.
  The exact node build retains the same 1770 compiled/checkpoint/profile inputs
  as Set70; no live seed rollout is required.
- Normal macOS package 0.5.10/build71 built in 22.627s with unchanged paired
  sources and strict ad hoc signature verification. The first build's icon
  conversion failure is preserved: the identical artwork and icon script
  succeeded outside the sandbox, followed by the complete successful build.
  The bundle is not notarized, installed or a product release.
- Vault focused attempt1 exposed a fixture-consumer problem: installing a
  duplicate serialized LevelDB key through Batch.Put collapses it before audit.
  The independent corpus remains unchanged; the successor consumer rejects that
  serialized framing before installation. Attempt2 passed normal/race/cheap/vet
  and the affected legacy vault regressions. The final attempt4 passed normal
  (12.86s), race (45.82s), cheap refusal (2.02s) and vet (0.34s), with 3610 inputs
  unchanged. It also covers replay of an older committed capture after newer
  captures, changed-intent refusal and rejection of historical captures for a
  new export. The source-only accounting correction bounds those retained public
  manifests separately; frozen reference bytes remain unchanged. Attempt3's
  intermediate compile error is preserved. See `../artifacts/set71/vault-closeout.json`.

The actual hosted rehearsal passed in 317.062s with 4281 source inputs unchanged.
Both fresh signed fixture accounts received mature native84 funds, completed
reviewed withdrawals with the independently calculated 144-atom fee, exported
the exact current 1/1 cursors and passed the production schema12 file verifier.
An independent file-only restore recovered the first wallet's exact change coin
and confirmed a controlled spend while the installed database was unavailable
and stayed byte-identical. Verified reset retained the same account, wallet,
gym and source while enforcing the hold and denying old sessions/views. Exact
reopen retained the original withdrawal operations; missing established current
storage refused without recreation. Both owned nodes shut down cleanly, with
no leftover process group or forced kill.

The exact normal packaged helper passed in 176.307s: both naming versions and
both fixture accounts, four current exports, ten production file-verification
successes, eight held export refusals, exact restart/marker preservation and
missing-store refusal/restoration. Five negative CLI probes refused before
creating data. The legacy Pool login/export/verify/restart regression passed,
and the native executable's flag probe confirms that legacy Pool remains the
default. All eight helper process groups exited cleanly, without forced signals.
This is helper/flag acceptance, not native visual or dialog automation.
All six wallet modules passed their complete normal suites (377.462s in total),
followed by the existing cheap-profile refusal checks (1.197s), with 1102 inputs
unchanged. The required smoke passed: an isolated two-block branch paid the
10-SLUG subsidy and reported its checkpoint conflict without rejecting the
chain. Read-only seed synchronization then reached height61 with 60 checkpoint
descendants; chain verification, stale-template refusal and clean restart/shut
down passed. No public-chain mining occurred. Source/profile inputs remained
unchanged. Qualification reports and preserved attempts are in
`../artifacts/set71`.
No live seed rollout or public-provider deployment has occurred for this set.

The proposed release sign-in experience is ordinary account creation and return
login, same-wallet verified reset and a separate backup flow. Email/password is
a proposed default, not a newly approved provider or frozen screen design.
The thorough joint D74 design review remains last. No app reorganization or
real authentication-provider deployment is claimed by this set.
