# Remote pool desktop attachment

Set74's first desktop slice adds a public configuration codec and a protected
two-audience credential owner. It does not yet select a native application mode,
open a wallet, attach a node or start a worker. The existing applications retain
their original routes and histories.

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
it does not claim client SPKI pinning. The future concrete node adapter remains
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

The next slice must borrow the actual existing wallet/session, attach its
selected local node once, retain a separate outbox and require explicit Start.
Opening or refreshing must never mine or deliver pending work automatically.
OperatorV2 fighter/history integration remains separate; remote receipts must
not be presented as entries in an old local stable. Navigation and the final
joint D74 design review remain deferred.
