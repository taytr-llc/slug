# Change set 41 — candidate stable and ten-slot roster

Branch: `slug-gym-stable`, stacked on set40 implementation
`78d04c9c26f8a015929c03a2b028b2372de78c45`.

The existing Candidate Pilot now retains each verified discovery in a separate
candidate assignment store and lets its account select up to ten distinct
fighters. Inventory paging, roster membership and visual cache retention are
separate. Each view projects original retained admission birth and current condition
at one authority time, with zero XP until a trusted results authority exists.

The store pins the exact account, fixture identity, admission authority and
candidate descriptor. Bounded catch-up verifies at most eight receipts in a
serial operation and synchronizes assignments with the consumed source position.
Clock rollback, changed source records, missing established data and uncertain
writes fail closed. Prepare/apply slot operations retain a durable identity,
revision and original view for replay, abandonment and lost-ACK reconciliation.
An acknowledged no-op records its outcome without increasing roster revision.

The helper uses one serialized proof lane with the miner, original captured
account leases and bounded result/cache storage. Fixed inventory pages stay
selected through new arrivals; Follow Recent is an explicit operation. Native
command-specific codecs and background-job correlation preserve exact request
identity through polling and cancellation. The frontend keeps account controls
available independently of stable availability and presents reconciliation
states rather than guessing whether a mutation committed.

The same bundle and LocalManifestPilot root receive a recoverable explicit v2
upgrade. Existing retained wallets and published fixture identities stay intact.
The older manifest route refuses an upgraded root. Installation checks preserve
valid old pre-Prepare and complete READY-assisted recovery while refusing partial
or mismatched established components. Current authenticated sessions under D72's
hold may make reversible roster edits; all enumerated hold restrictions remain.

## Verification and scope

The frozen Python corpus independently derives stable namespace IDs, complete
source-record bindings and 48 condition boundaries from verified public records.
It reuses the previously frozen independent proof, manifest and condition
algorithms; Go output is not used to generate expected values. Core tests cover
8+3 receipt catch-up, ten slots, replay/CAS/no-op, revoked views, clock rollback,
capacity, source drift, uncertain commits and ten actual process-kill boundaries.

Independent reviews cover the core, parent leases, serial lane, upgrade,
protocol, native bridge and frontend. The DOM suite covers page/roster behavior,
held sessions, stale responses and uncertain mutations. Actual process-kill
upgrade tests use owned normal-PoW fixture nodes. Final integrated normal/race/
tagged checks and exact packaged two-wallet acceptance are recorded separately
before rollout, together with independent proof/SVG/payout and stable-condition
checks. Every started private node and helper must drain and close its listeners.
The usual per-set smoke, fresh height61 preflight and full 15-minute seed
observation gate deployment completion.

`GYM_STABLE.md` records the full contract. `GYM_STABLE_RECENT.md` defines explicit
Follow Recent selection and the private upgrade validator boundary;
`GYM_STABLE_INTEGRATION.md` records recovery and integration details.

No chain consensus, wallet derivation, signed-checkpoint behavior or Railway
service configuration changes. No public-chain mining, official descriptor,
transfer/discard, entry lock, fight result, XP award, real signup/custody or
permanent genesis is enabled. The Mac bundle remains ad-hoc signed. Pixel
rendering, native chooser interaction, actual accessibility and Mike's thorough
joint D74 review remain uncompleted release gates.
