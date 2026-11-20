# Change set73 — fighter ownership and direct transfer (in progress)

Branch: `slug-fighter-ownership`, stacked from completed Set72 chain
`04ef85385d170272afd4cb28a88d2011168f1e2e` and wallet
`21f8d65494a6193f3071705cfad86f7c0e942eb2`. Both Set72 topic heads were pushed and
read back from origin; both master heads were unchanged. GitHub returned no
checks, statuses or workflow runs at those exact commits: absence, not a pass.

D83 is recorded in `FIGHTER_TRANSFER_POLICY.md`: canonical chain order chooses
the first valid competing send, ownership remains pending until its checkpoint
and60 descendants, and noon locks use the effective owner. This set must include
reviewed actual transfer and independently restored same-gym ownership/use.
Initial ownership or a parser alone does not complete that milestone. Real login
and MFA implementation remain deferred; the joint D74 review remains last.

## Completed component: atomic node observations

`CHAIN_OBSERVATION.md` defines the additive read-only `getchainobservation` RPC.
It combines a public RPC-lifetime identity with the best hash/height and a
nonwrapping in-memory revision. Every connect/disconnect changes revision,
including A→B→A. Startup gets a new identity; exhaustion disables observations
without stopping chain processing. Fail-fast lock acquisition avoids waiting
behind hashing or writes. Existing block bytes, consensus and persisted chain
schema are unchanged.

The strict client preserves full uint64 precision as decimal text, refuses
ambiguous responses, and identifies only the exact busy error as retryable.
It supplies no ownership/finality or wall-clock authority on its own. The later
cutoff coordinator must retain the exact two-sided boundary and source prefixes.

Focused normal/race tests exercise real connect, duplicate refusal, disconnect,
reconsider, same-hash return, concurrent reads, overflow, command/result grammar,
public lifetime identities and help. Normal/race/vet plus a normal node build
passed under the exact Go1.27.4 no-CSE profile. Evidence:
`../artifacts/set73/node-observation-focused-attempt3/report.json`.
Normal9.587s, race30.337s, vet4.180s, node build2.391s; source/profile unchanged
during that run and owned process groups empty. This is scoped qualification,
not the whole Set73 build/test matrix.

The actual two-node normal-PoW check passed in4.189s:
`../artifacts/set73/node-observation-actual-attempt1/report.json`.
It mined two isolated blocks, checked a stable boundary, invalidated/reconsidered
one block, observed the original hash with a changed revision, rejected an extra
RPC argument, and restarted the original database at the same tip with a new
instance and revision1. All three node lifetimes exited normally. No public
network mining or production key was used.

Strict client normal/race/vet checks also passed (3.261/3.771/0.669s), with their
explicit limited named-file binding:
`../artifacts/set73/node-observation-client-attempt1/report.json`.
The initial client command was blocked from the default cache before tests;
the successor used the existing writable task cache. The initial root handler
compile failure (RPCError pointer versus code) and inventory-runner failures
(tracked documentation symlink and incorrect presumed target) are preserved in
the diagnostic/attempt1/attempt2 evidence; no failed report became a pass.

## Carrier feasibility and remaining work

The separately reviewed mechanical carrier probe passed six cases on109 actual
normal-PoW blocks in99.154s, with clean joins:
`../artifacts/set73/carrier-feasibility-attempt4/report.json`.
It proved standard relay, the520-byte redeem boundary, deterministic refund
reconstruction, and actual canonical inclusion of nonminimal pushes, high-S
outer signatures and harmless extra stack items. The last scriptSig was9390
bytes. A local521-byte element refusal is labelled local, not node inclusion.
Its temporary domains/economics are probe inputs; this is not a full exported
wallet restore, ownership or finality test.

Earlier feasibility attempts remain failed: unpublished module graph lookup;
missing required RPC argument; and a checker treating omitted `allowed:false`
as an unexpected response after the first three real cases had passed. Each
successor fixed only the identified harness issue without changing consensus,
relay flags, expected acceptance or proof-of-work.

The frozen carrier and wallet contracts are `FIGHTER_TRANSFER_CARRIER.md` and
`FIGHTER_CARRIER_WALLET.md`. Independent references precede their new product
implementation. Public admission/finality history, replay, wallet schemas14/15,
reviewed transfer, owned stable/event/career integration and the packaged
independent restored-gym command still require implementation and qualification.
The full set is not complete, promoted, deployed or a release candidate.

The new observation changes compiled node inputs. A later completed Set73
promotion therefore requires the standing full checks, smoke and authorized
deployment verification; the earlier unchanged-node Set72 smoke is insufficient
for this new RPC. Master and the live seed remain unchanged at this checkpoint.
