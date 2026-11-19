# Set63 — shared fixture entries and noon snapshots

Branch: `slug-shared-pool-entries`, stacked from Set62 `163324e1`.

## Change and reason

Connect the Pool Pilot's actual mined fighters to Saturday entries and a shared
noon snapshot. The parent owns the event coordinator; each original-view child
owns its entry store and retains its Pin2 stable. Immutable setup intent and
completion records preserve the exact participants across interrupted setup,
reset and restart. Missing established stores fail closed. Nothing repins solo
histories or copies shared receipts into a different authority.

Existing native and browser entry/event controls now use this shared route,
with strict original-view and complete shared-source checks. Backup pauses new
lane work and joins active work; polling and cancellation remain available.
The Mac package is version 0.5.2/build63, with the existing layout and logos.
See `SHARED_POOL_LEAGUE.md` for the contract.

## Verification

Evidence is retained under `../artifacts/set63` using the exact Go 1.27.4 no-CSE
profile. This is finite candidate evidence using published fixture accounts.

- `runtime-parent-attempt3`: focused normal and race tests passed in 8.62 and
  12.07 seconds. Covers exact setup/reopen, lost completion acknowledgment,
  established data loss, orphan/repin refusal, parent/borrower shutdown ordering,
  original-session forwarding and allowed actions during a reset hold.
- `child-attempt3`: normal and race lifecycle/backup/league checks passed in
  13.86 and 53.52 seconds. Covers original participants, child-owned entry handles,
  borrowed event lifetime and lane draining without an account lease.
- `protocol-attempt2`: focused normal and race checks passed in 4.26 and
  32.23 seconds. Closed wire families, independently frozen legacy corpus,
  shared-source substitution refusal, native pool contexts and final original-view
  dispatch gates retain their bounds. Normal and tagged compile checks passed.
- `frontend-attempt1`: all 303 browser tests passed. Both fixtures retain exact
  shared source pins; held sessions can enter, backups postpone new work without
  losing drafts or consuming the automatic capture attempt, cancellation remains
  available, and replaced-view responses are discarded.
- `actual-attempt5`: normal-PoW shared mining and wallet runtime acceptance
  passed in 103.70 test seconds (109.37 seconds including build). Actual A/B/A
  admissions selected original global sequences 8/15/22. Both gyms saved their
  original assignment entries, then produced the November 21 noon snapshot.
  Only the tagged calendar/store clock advanced; account, mining-job, wallet,
  reset-hold and settlement clocks remained real. A post-noon roster removal
  did not alter the historical snapshot. Reset revoked the old mutation view
  while preserving the same gym/source and hold. Replay and full restart
  retained the exact participant captures, combined record, entry receipts,
  source identity and height12 without implicit mining. The public result
  records snapshot digest
  `4736c55bfe516badef06a51762b0660032378b5dd407368edbf321f502d8aa05`.
- `package-attempt1`: normal node/helper/GUI built in 12.28 seconds with strict
  ad hoc signatures. `package-helper-attempt1` passed in 53.11 seconds: both
  exact bundled fixture wallets exported owner-only files and verified fresh
  restores, both gyms exposed the same source/event authority and upcoming
  Saturday, all four before-noon capture attempts failed with `event_not_due`,
  and clean EOF/reopen retained stopped zero-work state. No fake clock or
  completed-noon claim applies to this normal package check.
- `static-attempt1`: repository formatting, diff checks and affected-package
  vet passed. Later edits only strengthen the tagged integration test; final
  qualification binds those exact test bytes separately from the unchanged
  package's compiled inputs.

- `desktop-full-attempt1`: complete normal accountpilot, pilot, bridge,
  recoveryfile, helper and native suites passed in 156.41 seconds. All local
  Go/build/testdata, native and embedded frontend source bindings stayed exact;
  opt-in actual nodes were disabled. Native unit linking retains the existing
  macOS26-versus11 object warnings; no older-OS runtime claim is made. The
  packaged build uses its separately recorded macOS target and signatures.
- `smoke-attempt1`: two isolated normal-PoW blocks, zero mining peers, 10-SLUG
  subsidy and report-only checkpoint conflict passed. Read-only public-seed sync
  reached height61 with sixty checkpoint descendants; height62 activation,
  verifychain, stale-template refusal, restart and clean shutdown passed.
  No public block was mined.

## Preserved failures and corrections

Actual attempt1 passed before additional exact final-record and old-view
mutation assertions. Attempts2/3 reached restart but the harness incorrectly
treated a typed temporary busy response on entry/event reads as an evidence
mismatch. Positive read checks now retry only typed busy errors within their
bounded deadline, with unchanged original view and arguments. Mutation outcomes
and all other errors still fail. Actual attempt4 omitted complete shared-source
synchronization before a roster change: Recent shows the consumed local tail,
which does not prove all global receipts were checked. The final harness runs
bounded explicit synchronization to caught-up before mutating. None of these
changes weakens the production lane or its synchronization requirement.

Child attempt1 and the overbroad protocol attempt1 encountered a noncanonical
`/var` alias in new test fixtures. Child attempt2 omitted the fixture admission
transport needed for strict DTO validation; its fake lease also needed to model
the real serialized lease, and its checkpoint wait needed a finite deadline.
The corrected isolated fixtures passed. Protocol attempt1's own cases passed,
but its selector also ran unfinished runtime tests; only the scoped successor
is the passing qualification. Runtime-parent attempts1/2 stopped at runner
metadata preflight errors before testing; attempt3 excludes generated external
test-main paths from local source inspection. Actual compile attempt1 had an
unnecessary integer type conversion; attempt2 compiled. All attempts remain
preserved, with no relabeling of failures as passes.

## Remaining work and deployment impact

Next connect the shared publication/card/bout/result/career pipeline. Activate
career accounting before the next event's lock and first result finality;
preserve older snapshots without backdating authority. Event-carrier funding
must remain separate from share-accounted pool earnings. Real accounts/custody,
ownership/transfers/marketplace, production operations and permanent genesis
remain open. The thorough joint D74 design review stays last before friends,
testers or public release. No native chooser rendering or GUI review is claimed.

The node/control dependency assessment found all 1770 inputs unchanged, with
unchanged deployment configuration. No redundant Railway rollout is needed;
master and the live seed remain Set50 `592e143c`. Final commit, source, reports
and bundle hashes are bound under `final-binding` after qualification and commit.
