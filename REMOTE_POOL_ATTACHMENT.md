# Remote pool desktop attachment

Set74 adds a public configuration codec, protected two-audience credentials and
an explicit attachment to an existing schema14 wallet. Its concrete controller
opens only an established owned-gym root and verifies the original enrolled
account, vault, wallet and recipient. A separate explicit native route now
connects that existing wallet through its own helper process. Its normal/race,
frontend and packaging checks pass; visible native acceptance is still pending.
The existing applications retain their original routes and histories.

The configuration binds the complete selected OperatorV2 profile, pool source,
enrollments, settlement destinations, HTTPS origin and CA trust. It also binds
the existing schema14 wallet's original source, owner, allocation identity and
audited native84 recipient. The selected gym must match the existing gym public
key, and its remote destination must equal that original recipient. The remote
policy must equal the retained local policy. Public identifiers and valid JSON
alone grant no wallet or signing authority.

Remote pool and admission credentials occupy separate protected files, bound
to the exact configuration, gym, audience, generation and expiry. The private
owner retains the original file descriptors and admits one live lease per
audience. Shutdown cancels and joins those leases before closing files. A
replaced or changed file invalidates access. Replacing credentials requires
stop, join and explicit reopen; it cannot rebind pending work. No credential,
client or general signing getter is exposed.

Both audiences preserve their own typed unknown-outcome response when a
mutation is interrupted. The worker can then retain the exact pending share or
fighter submission. Remote credentials never substitute for the original local
Mining capability. Admission cannot issue mining jobs.

Darwin credential custody requires CGO for ACL checks; a no-CGO build refuses
custody. Client trust uses the selected CA and existing transport identities;
it does not claim client SPKI pinning. The concrete node adapter stays
inside the existing wallet boundary: private HTTP on literal `127.0.0.1` with an
explicit port. Remote public services use HTTPS.

## Qualification and next boundary

`remote-credentials-attempt2` passed normal checks (10.429s), race checks
(21.620s), unsupported-platform refusal (14.742s), cheap-profile controls
(14.350s) and vet (1.512s). All 4,579 selected inputs stayed unchanged and every
process group joined. Tests cover exact independent configuration/marker bytes,
32 cross-binding refusals, ten raw refusals, original credential files, both
audiences' cancellation and uncertain-result handling, and sticky cleanup.

The first attempt correctly rejected a reference fixture that reused the
operator's change address as a miner destination. The preserved diagnostic
isolated the settlement validator. An independently derived successor selects
the existing third published operator wallet, keeping both local recipient
objects unchanged and adding an explicit collision refusal. Python 3.14 and
3.9 produced identical 563,867-byte output, SHA256
`5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177`.
Production destination checks were not relaxed.

The concrete attachment borrows the original session, attaches its selected
local node once and retains a separate outbox. Both recipient markers must exist
before wallet opening and still match after synchronization. This audit cannot
allocate another address or repair a missing READY marker. Original View and
Mining capabilities remain required; signing is restricted to the exact saved
fighter proof through the retained vault. Empty Merkle branches retain their
canonical representation when copied for exact matching.

Open and Login return stopped. Explicit Start alone mines or delivers retained
work. Constructor and Login request cancellation cannot cancel a successfully
retained owner; Stop, logout, expiry and Close cancel and join it explicitly.
The controller opens no old source/history service and refuses missing existing
vault or wallet controls. Failed cleanup retains its exact owner and root lock.

`remote-attachment-attempt1` passed normal (25.722s), race (61.726s), no-CGO
refusal (11.174s), cheap-profile refusal (9.428s) and vet (1.105s). Seventeen
ordinary new controls passed normally and under race, alongside existing
recipient and wallet-startup regressions; the cheap-only case also passed.
Its 4,606 selected inputs stayed unchanged. `remote-controller-attempt1` passed
normal (11.553s), race (15.035s), no-CGO compilation (9.028s) and vet (0.938s),
with 4,613 unchanged inputs. All process groups joined. These are focused
boundary checks. See `docs/evidence/set74/remote-attachment.json`.

The subsequent actual rehearsal also passed: the existing incrementing256
schema14 wallet connected to a separate TLS operator, mined one normal-PoW block
and obtained both an accepted share and a separately signed fighter admission.
Stop/logout joined all owners; the same controller and outbox reopened stopped
with identical receipts. Independent operator audit matched those receipts.
The runtime took 13.894 seconds, all 2,967 selected inputs stayed unchanged,
and the original fixture plus 226 protected copied history/control files stayed
unchanged. See `docs/evidence/set74/remote-attachment-actual.json`. The native candidate is qualified below; remote fighter ownership replay and
payout presentation remain separate work.

OperatorV2 fighter/history integration remains separate; remote receipts must
not be presented as entries in an old local stable. Navigation and the final
joint D74 design review remain deferred.

## Explicit native candidate

The separate `SLUG Remote Pool Pilot.app` requires `--candidate-remote-pool`
and six absolute selected paths: `--data-dir`, `--remote-attachment-root`,
`--remote-config`, `--remote-node-rpc`, `--remote-pool-credential` and
`--remote-admission-credential`. Select name v2 explicitly with
`--candidate-name-version=2` where the original wallet requires it. Opening
this bundle without its selection refuses; it never creates a default wallet
or launches a node. The existing local node remains a separate retained owner.

Native retains and independently parses the public config file. RPC and bearer
values stay in their protected files; only their paths reach the helper. Five
fixed native methods expose Snapshot, LoginFixture, Start, Stop and Logout.
Legacy wallet/export/transfer/league methods are not registered in this route.
This remains a published-fixture login, separate from real sign-in/onboarding.

The strict bounded protocol seals the selected config and original opaque view.
A dispatched mutation with an uncertain result retires its original helper;
there is no automatic replacement or replay. Stop/logout/expiry/Close join the
original controller. Quit also joins the native calls, helper process and
selected config; incomplete cleanup remains visible and retained. Signal
ownership begins before helper construction and hands off to the same quit
path once the window starts. Both bundled executables require Darwin CGO1.

The real process rehearsal passed in 13.608 seconds: explicit mining obtained
one accepted share and a separate signed fighter receipt; two successive real
helpers reopened the same stopped outbox and closed cleanly, including signed-in
EOF shutdown. The chain stayed at height 2; no block win is claimed by this run.
All 3,041 input files, the original fixture, and 226 protected copied-history
paths stayed unchanged. The operator audit matched both receipts.

The native package passed normal/race tests, public DOM fixtures, vet and a
separate ad hoc signed two-binary build. The limited no-CGO check verifies only
the selected protected-input refusal leaf; it does not compile or qualify the
Wails app without CGO. Exact evidence is in
`docs/evidence/set74/remote-native.json`.

Visible native acceptance passed on the same packaged binary in 241.213 seconds:
original fixture login, Start, native-menu Stop, sign out and Quit. The visible
capacity notice followed one attempt with no pending delivery; closed audits
then proved one accepted share and separate signed fighter receipt. Every owner
joined without signals. Source/package identities, original fixture and 226
protected copied-history paths stayed unchanged. The chain remained at height 2.
Two failed harness attempts remain recorded: an unsupported build-metadata
assumption and a name-v2 selection against the original v1 fixture. Neither is
relabeled as passed. Public hosting, remote ownership/history and payout display,
D80 continuation-backed credit, and the final joint D74 review remain open.
