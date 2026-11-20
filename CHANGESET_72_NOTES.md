# Change set 72 — recovery covers newly issued addresses

Implementation and local qualification complete on the stacked
`slug-hosted-allocation-recovery` branch. Publication and final source bindings
are recorded in `../artifacts/set72/final-binding`. No compiled seed input changed,
so this desktop/wallet set requires no master promotion or seed deployment.

The explicit `--candidate-hosted-recovery` mode reserves address ranges in a
durable recovery inventory before deriving or disclosing them. Receive addresses
and reviewed withdrawal change therefore remain covered when the local wallet
disappears, without requiring a manual export before that loss. The same fixture
account can export a current file and verify it in an independent scratch wallet.

The frozen contract is `HOSTED_ALLOCATION_RECOVERY.md`, SHA256
`8e6036d0bb1dac60a0e513427fe71bcc42cb5909bfdc67fb0c42a61de6790307`.
This uses a separate `LocalHostedRecoveryPilot` namespace, schema13 and the
existing version1 Phase-A recovery format. The two published fixture identities
remain the only supported accounts. Ordinary Pool startup, old namespaces and
the approved production profile retain their existing meanings.

## Behavior and boundaries

The pool recipient stays at native84 external index0. New receive addresses
begin at index1; reviewed withdrawal change uses reserved internal paths. The
complete reserved prefix includes uncertain, burned, unused and abandoned ranges.
Plan, Reserve, Consume, Ack and Finalize retain their exact operation and concrete
owner identities. A lost reply cannot allocate a replacement range or adopt a
different action's success. Lookup is read-only; process replacement requires an
explicit fresh Resume or Redisplay action for the audited durable operation.

Existing wallet controls gain receive, resume and explicit redisplay without a
navigation redesign. Final address disclosure requires stopped mining, committed
inventory/local linkage, successful watch registration and the original account
View. Wallet, database and network work and joins stay outside account leases.
The bounded final publication rechecks that original authority.

Only an absent exact wallet database or its whole directory selects recovery-only
mode. All other required identities, inventories and journals must survive.
Corruption, wrong types, symlinks, foreign schemas and missing established stores
refuse. If either fixture wallet is missing, normal services remain stopped; the
intact fixture cannot pretend to be healthy or missing. The missing fixture gets
an export/inspection owner, never an installed replacement payment wallet.

Physical read-only archive opens audit surviving fighter, league, pool and
withdrawal stores without repair, metadata merging or payment replay. Verification
requires the current exact inventory manifest, a separate schema12 restore,
original gym proof and stable chain observation. D72 permits verification during
the reset hold while blocking export and withdrawals. Failed cleanup retains
the exact owner instead of permitting an unsafe replacement instance.

## Qualification

Go qualification uses the recorded Go 1.27.4 no-CSE candidate profile. Normal
release binaries retain 256 MiB Argon2id; tagged checks prove profile refusal.
Reports, exact source inventories and failed attempts remain in
`../artifacts/set72`. The portable independent reference is in the paired wallet's
`wallet/testdata/hosted-allocation` directory.

- Independent Python 3.14.3 and 3.9.6 producers reproduced the same 4,004,018-byte
  corpus, covering 37 states, two bindings, negative storage/framing cases,
  replays and the 1,024-operation boundary. Portable reproduction passed in
  81.771s. Expected values were not copied from new Go output.
- Manager, inventory, vault, allocator, held-session, read-only lookup, watch
  and archive checks passed their recorded focused normal/race/profile checks.
  All 379 frontend tests passed. Earlier focused runs retain their original
  source scope; exact later corrections are accounted for by named successors.
  Early core runs captured wrapper/settings and sources, not contemporaneous
  compiler binary hashes, and are not retrospectively labelled final-source
  race qualification.
- All ten chain modules built. All six wallet modules built and passed vet,
  then passed their full normal suites in 404.762s with 1,197 inputs unchanged.
  A separate final tagged check passed all five intended refusal tests in
  1.596s, including the loader case omitted by an earlier selector.
- A copy of a genuinely interrupted setup completed both fixture logins and
  READY states in 18.107s, preserving original source/account/recipient pins,
  with one allocation each, no mining/payment and unchanged original files.
- The funded rehearsal passed in 423.781s with 4,336 inputs unchanged. Both
  fixtures received funds and performed two reviewed withdrawals with the
  independently calculated 144-atom fee. No manual export preceded loss.
  Database loss and whole-directory loss both yielded current external 3/internal 2
  recovery manifests; older files refused. An independent file-only restore
  recovered the latest change and confirmed a controlled spend. Reset holds,
  session revocation, unchanged original wallet files, healthy reopening and
  no second debit on submitted-operation retry all passed.
- Desktop qualification is an explicit composite: full attempt1 passed six
  packages and failed only the pilot package's legacy setup cases. After the
  narrow correction below, existing setup tests passed normal 19.803s,
  race 114.875s and vet 0.820s; the complete pilot package passed in 156.707s.
  The correction runs retained 3,102 unchanged inputs and clean process groups.
- Normal macOS package 0.5.11/build72 rebuilt in 13.444s and passed strict ad hoc
  signature verification. Package-helper attempt4 completed both new naming
  routes, both fixtures, receive/replay, healthy/missing-wallet verification,
  reset holds and restarts, with fourteen clean helper groups. Its later legacy
  startup failed. The corrected package5 separately passed legacy initial/reopen
  acceptance in 33.269s with unchanged identities/markers and two clean groups.
  The sole production delta is the reviewed legacy assignment below. These
  are explicit composite results; neither failed overall attempt is relabelled
  passed or claimed to have executed the final bytes.
- Required smoke passed isolated mining/subsidy, report-only checkpoint conflict,
  read-only seed sync to height 61 with 60 checkpoint descendants, chain verification,
  stale-template refusal and clean restart/shutdown. No public-chain mining
  occurred. All 1,770 compiled node inputs match Set71.

The bundle is not notarized, installed or a product release. Helper and flag
acceptance does not claim native visual/dialog automation. Final source/profile
checks bind the above composite to the paired commits; remote CI absence, if
observed, is assessed separately and is never counted as a pass.

## Corrections and preserved failures

Real defects found and corrected were a missing-namespace dereference in the
wallet recovery audit, omitted namespace byte accounting, directory-kind checks
during fresh setup, watch access through the retained concrete RPC wrapper,
rebinding an initialized inventory during signed login, and a typed nil legacy
withdrawal handle boxed into a nonnil interface. That last bug refused first-time
legacy setup. The fix boxes only a successful concrete handle; the hosted
allocator branch is untouched. The funded allocated-path acceptance is retained
under this exact reviewed delta, not relabelled as a new execution.

Test/harness corrections included canonical RawMessage framing, a shadowed test
error, fixture cursor comparisons, missing second-principal enrollment, a timing
assertion, an archive fixture clock, a never-connected RPC assumption, flag-help
stderr capture and canonical receive-request field order. Frozen expected bytes
and product checks were not relaxed. Sandbox listener failures and the deliberately
stopped combined race command remain preserved too. Individual reports retain
each failure, diagnosis, correction and successor scope.

## Remaining work

Coverage is the supported reserved prefix, not arbitrary addresses independently
derived beyond it. The co-located fixture vault is not an off-device recovery
service. A file does not recreate missing signed-payment bytes, reservations or
application journals, and local hashes alone do not detect whole-filesystem
rollback. Verification does not install a wallet or establish exclusive custody.

D83 records Mike's approved transfer policy in `FIGHTER_TRANSFER_POLICY.md`:
first valid competing chain-ordered send wins; ownership stays pending until its
checkpoint and sixty descendants; noon uses the effective owner at lock; later
checkpoint conflict retains official history under D62. Set72 records this
decision but does not implement transfers.

Set73 combines authoritative ownership, actual direct transfers and recovery of
the same gym's owned fighters with a transfer from an independently restored key.
Login/provider/MFA work remains deferred until these foundations are complete.
D82 stays required for launch, and the full joint D74 design/navigation review
remains last. No PR or seed rollout is part of this closeout.
