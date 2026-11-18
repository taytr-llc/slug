# Change set 45 — candidate event seed evidence

Branch `slug-event-seed`, stacked from set 44 implementation
`a00ada0d2d3c5e903a4160e598905c667c10ee32`. Final implementation
`ca84a62d178386646aef119cbe2edbb6ad38f508` passed exact packaged acceptance and
was promoted to master `dcc448290d4b6f1ff76459a5c0044deeaefb7aa5`.
Deployment and the full live observation/restart passed. This is engineering
closeout; the product-release limits below remain open.

Connect a complete immutable event snapshot to authenticated candidate
publication and retained seed evidence. The intended slice records the first
qualifying anchor, a fixed future target, explicit finality evidence, conflicts
and following-Saturday escalation without substituting a seed or cancelling.
It does not execute fights, award results or replace production ownership.

D76/D77 remain controlling. The exact publication, scan, checkpoint and
capability contracts were frozen under approval
`c02f51d028a1cbd6d047901c0a229b0094498ec8fc9088038abba4b363ad8116`.
Core contract SHA256 is
`51ddfc57931b1937f918f990a36c050dc9874b2d1a60b069273e314e317523df`;
helper contract SHA256 is
`0abdcfa84e2213bd644ad2bfe43789961c3231ef10841121fa2f039776fb8f7e`.
The independent reference was frozen before implementation comparisons under
`decbef0a5a69aac9827817994307b65a82f4a0daa20e654574072fe3f480b5db`.
The producer read no new set45 runtime source or output before that freeze.
After freeze, the same agent separately reviewed implementation source; the
reference bytes remain immutable.
A block inclusion proof alone does not establish
firstness, a transaction digest is not publication authority, and a future
block hash is not proof of unbiased randomness or universal prior ignorance.

The candidate design uses only published fixture material and isolated owned
nodes. General wallet spending, real operator keys, public-chain publication
and public-chain mining remain outside this slice. One fixed publication chain
must survive account switching and restart; another fixture's chain cannot be
used as a fallback. Preserve D72 restrictions and explicit mining controls.

Set 44's exact release is deployed at master
`1fc307e07eec1be7f2466e0aabedb505f8844b0d`; its full 900.044-second live
observation and clean restart passed using frozen binaries independently of
this new topic branch. See `CHANGESET_44_NOTES.md` for the completed closeout.
Mike's complete joint D74 review remains required once major flows are ready,
before friends/tester or public release. The portrait-render check remains
pending at his request.

The implementation now includes the journal and source/account seams, the
owned-node publication adapter, typed helper family, explicit v5 installation,
native correlation and candidate seed panel. The panel separates historical
first finality from eligibility at the last verified chain observation and
shows its chain timestamp. Reading metadata never refreshes that timestamp or
automatically publishes, signs, funds or mines.

Source checks exercise exact-decimal RPC values, fixed transaction/witness
binding, RPC envelope and redirect refusal, process/config refusals, bounded
normal header work and cancellation joins. Core, helper, adapter and native
normal/race checks pass, as do independent corpus comparisons. The isolated
normal-PoW composition now passes through publication, target finality, rollback,
conflict retention and restart. Final desktop normal/race regression and the narrow shared-store correction
now pass. The final package and live rollout closeout are recorded below.


The frozen reference contains 125 DTOs, 1,420 codec recipes, 194 protocol cases,
3,861 Saturday rows, 78 retained model states and 69 full wire block fixtures.
Both Python 3.9 and 3.14 under UTC and Los Angeles reproduced exact bytes. The
standalone normal-Argon helper independently rechecked 145 retained header
outputs. The corpus is SHA256
`2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0`.
Original source-review candidates and first runtime mismatch logs are preserved;
expected values were not replaced with Go output.

Pre-freeze review corrected late first finality erasing an already-due incident,
Begin ACK/job state monotonicity, helper framing's final LF and authenticated
alternate-policy equivocation evidence. Runtime comparison then caught the
helper's missing LF reservation. The independently frozen Info transport cap is
20 KiB, separately from 40 KiB internal metadata; that narrow engineering
clarification was adopted without changing the reference. Internal disk shapes
are not advertised as tolerant public parser APIs.

Independent adapter/native review found and closed two implementation issues:
native record polling repeated full condition projection, and attestation's
last membership bracket omitted the captured tip used to establish depth.
Native reads now retain cheap exact source/authority binding on top of the
closed transport parser. Attestation rechecks anchor, target and captured tip,
then checks process ownership before signing. Ordinary tip extension remains
valid; a replacement strictly above the target invalidates that captured depth
witness. Focused regression and exact source captures accompany the review.

Independent core review also found and closed certificate replacement on an
attestation replay and missing cancellation checks between History rows. An
already valid exact-target certificate is now preserved byte-for-byte, and
History checks cancellation between records. Both regressions were reproduced
before the fixes; the alternate valid signature was independently derived after
the original reference freeze, without changing that reference.

The first actual composition timed out at its inherited ten-minute budget
while waiting for normal coinbase maturity. The second reached height 103, then
its test harness treated a legitimate busy polling response as fatal. The third
passed publication through finality and recovery. A fourth added actual reorgs
and full block exports; it reached height 177, then a read-only reconcile job
was cancelled at the queue-inclusive 30-second deadline. Its logs do not
establish whether it was queued or running, or why it used the full budget.
All attempts and owned-process cleanup are retained as diagnostics.

A fresh-login continuation of that exact fourth journal passed, retaining the
original transaction with dispatch count one and never republishing. It proved
that loss of depth strictly above the target preserves first finality without
creating a conflict, while rollback of the exact target latches a conflict.
Restoring that branch does not erase the conflict. Held and foreign accounts
cannot publish or attest; held reconciliation does not redispatch. Restart
retains the transaction, target, first finality and conflict, with no job cache.
The independent checker verified the complete exported 178-block branch:
first carrier at height 116, fixed target at 117 and 60 descendants. It checked
normal Argon hashes, transaction and Merkle/witness commitments, the funding
outpoint and maturity, exact publication signature and fixture checkpoint.
It does not turn captured local time/process observations into portable facts.

The following-Saturday continuation durably records the D77 incident at the
exact boundary and preserves it across fresh login and reopen without starting
nodes. Its initial harness incorrectly assumed historical mining counters
would be zero; that failed report is retained. The corrected assertion checks
stopped/offline state and unchanged prior owned-process log bytes. Native
private-pipe composition separately passes declaration preparation, shared-view
correlation and held/foreign refusals; it makes no new publication claim.

All mining in these compositions is explicit, test-only and confined to owned
fixture chains. Coinbase maturity 100, activation at height 62, normal Argon
parameters and the 60-descendant requirement remain intact. Release builds
exclude both the cheap-PoW and scoped-clock tags. The packaged normal-clock
flow cannot claim a successful closed event before an eligible real Saturday.

Independent frontend review corrected stale selected-record data hiding a newer
journaled incident, incomplete rollback of a newly learned authority when the
shared cache was full, and ambiguous read-only Reconcile uncertainty copy.
Reads now present a coherent selected observation, failed cache admission
restores both snapshot and authority, and known Reconcile uncertainty never
claims a transaction was sent. All 163 frontend model/DOM tests pass; this is
not a rendering, accessibility or joint design-review acceptance.

The full desktop race run exposed a predecessor shared-store error race:
a request after completed Close could nondeterministically return cancellation
instead of the fatal closed-dependency error. A focused before-fix probe
reproduced it on request 130. New requests now check the stable store lifetime
before and after acquiring its gate, while caller cancellation on a live store
and in-flight shutdown draining retain their behavior. The original broad
failure and before-fix probe remain preserved; final affected normal/race
validation and independent review bind this narrow dependency correction.
The first cheap-profile continuation was blocked by sandbox loopback permissions;
with local listeners permitted, the full cheap-profile and vet checks passed.

Mike added the native icon update during final verification. After comparing the variants, he chose the
original detailed SLUG fist for the app icon; the menu bar uses the separate
letterless simplified template fist for system light/dark tint. Both PNG sources are
committed, and both Mac bundle scripts build a standard ICNS resource and name
it in Info.plist. Existing menu actions and Wails lifecycle remain intact.
The original detailed artwork is preserved outside the checkout. Asset and
bundle checks remain separate from the pending full joint design review.

The first exact committed package at `dc01ac01` passed its 122.360-second
acceptance before Mike restored the original detailed app-icon choice. That
bundle, binary set and source/evidence records are preserved; they were never
promoted. The follow-up changes only the app image and its description, retains
the simplified menu template and rebuilds the final package from its new commit.


## Final package and deployment closeout

The bundle with the original detailed app icon at `ca84a62d` passed in
121.083 seconds.
Its source and three binary hashes remained unchanged, and five independent
proof/stable/entry/event/seed reports passed against the exact public result.
Both fixture wallets, six previews, entry/event/seed flows and process restart
were checked; all captured listeners and owned configuration commands were
absent after cleanup. The retained package report and independent review are
`../set45-package-acceptance.json` (SHA256
`02ddebcb425ba19180c927b7e5a93c45f803e459fe8c9da5e3481381add801d6`)
and `../set45-package-review.json` (SHA256
`82d7008e7e5409de5d45bc82f42e635136c8e8d3328da7279c773f0276c1bf54`).
The earlier `dc01ac01` bundle and every diagnostic remain preserved separately.

Normal-PoW smoke and fresh public-seed preflight passed. Railway deployment
`636c34ad-4549-4584-8d72-ecd916e92a85` succeeded with the exact release tree and
unchanged seed configuration. The observer ran 900.0442289580001 seconds with
31 successful samples and a clean restart, retaining public height 61 and its expected hash;
no public block was mined. Final observation/closeout evidence is
`../set45-closeout.md` (SHA256
`cfc4d9c6006f20485fed45c096733f972ed4cc7018e8e6ec7798bdee219bf589`).
Remote CI returned no runs/checks for the
implementation, so no remote-CI pass is claimed.

The packaged test uses real time and verifies setup, absence, typed refusals
and restart; it does not demonstrate a successful real-clock closed event,
publication or finality before an eligible Saturday. Successful closed-chain
composition is the separate tagged-clock, normal-PoW evidence described above.
The package checks icon sources, conversion and embedded bytes, not Dock/menu
rendering, native chooser behavior or screen-reader acceptance. The limited
signed-out app inspection did not establish icon visual acceptance. Mike's
portrait-render check and one full joint D74 review after the major flows are
ready remain required. No official league/production ownership, awards,
public-chain publication or automatic mining is enabled by this closeout.
