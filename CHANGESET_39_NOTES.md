# Change set 39 — candidate manifest service and durable miner

Branch: `slug-manifest-miner`, stacked on set 38 implementation
`401f45bfed914aa402f3b3b7240fdfa8f8ec0793`.

## What changed

Protocol 3 connects an explicitly pinned schema-5 candidate authority to typed
job issuance and admission. Static per-gym credentials and revocable fixture
account sessions are distinct constructor choices. Requests, responses, nested
objects, certificates, connections and active work have explicit bounds. Session
leases surround authority operations; an error after possible mutation reports
an unknown outcome. Historical disclosure still needs a valid session.

The client checks the complete returned job, normal proof of work, authorization,
DNA, immutable birth, eligibility and static appearance binding. Its origin,
authority pin and transport trust identity are immutable. Custom certificate
sets have strict parsing and canonical identities; credentials do not enter the
public outbox. A configured pin is trust supplied by the caller, not a signature
authenticating arbitrary receipt files.

Worker/outbox version 2 is separately typed. It synchronizes found work before
broadcast/signing, authorization before admission and checked receipts before
acknowledgment. Block wins broadcast immediately. Retained evidence survives
lost acknowledgments and retries after expiry; accepted historical receipts keep
their original birth and appearance. Capacity accounts for remaining growth so
a full journal can still save signatures, receipts and block reconciliation.
Unknown writes poison the handle. Cancellation joins actual hashing, callbacks
and writes and prevents subsequent transport; caller-owned resources remain
the caller's responsibility.

`MANIFEST_MINING.md` preserves the exact approved engineering contract (SHA256
`c25e2fb7f7361a0207acea952c44d62e7fcc9a7ebeeba5887fb2cf3d2b9ebfdf`).
`MANIFEST_MINING_RESERVATION.md` separately records the corrected remaining-growth
formula (SHA256 `d656049b06a4daafba2ddb3a0e06f98312e02e5e7708631eb142bd55816d5815`).
Original contract inputs and independent expected values remain unchanged.

## Verification

Independent fixtures cover 102 configurations, 130 issuance cases, 339 receipt
cases and 39 error envelopes on both routes. Two Python versions reproduce the
frozen outputs without production Go encoders supplying expected values. Service
normal/race/tagged checks and 265,246 bounded fuzz executions pass. Worker normal,
race, tagged-refusal and vet gates pass, including exact-capacity recovery, real
process termination, uncertain writes, index corruption and cancellation. All
12 pre-existing service files and eight pre-existing worker files are unchanged.

The two-wallet normal-PoW composition uses owned loopback nodes and published
wallet fixtures. It covers both D69 admission sources, durable lost-ACK recovery,
reset revocation, same-wallet recovery and gym-only signing during the hold.
After real five-minute job expiry, it stops both nodes, changes the service's
current target, refreshes held mining grants and recovers the exact original
receipts without signing again or consulting a node. An independent Python
checker re-verifies every public proof, signature, DNA and appearance binding.
Final results and source captures are recorded in workspace set39 evidence.

Root integration additionally requires all ten chain modules to build, complete
service/worker regression checks, desktop account tests, formatting, exact copy
hashes and the per-set normal-PoW smoke. Release binaries are frozen before
promotion. A fresh unchanged-height-61 check precedes deployment, followed by
the full 15-minute live observation. Deployment results are recorded separately.

## Preserved behavior and remaining gates

Old protocols, jobs, stores and outboxes retain their contracts. There is no
implicit migration, official generator activation, public receipt import,
ownership/transfer authority, new CLI or default desktop route. Opening/listing
an outbox checks local structural consistency without Argon and does not prove
portable receipt provenance. Coherent administrator rewrites and whole-store
rollback remain outside local integrity guarantees.

The seed deploy starts only slugd, with the existing volume and RPC exposure.
No public block is mined. The Mac GUI remains the fixture Account Pilot. Mike's
portrait rendering check remains pending by request; actual screen-reader
acceptance and the thorough joint D74 review remain open before product release.
