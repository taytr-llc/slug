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

## Completed component: supplied carrier validation

The pure `fightertransfer` package implements the frozen compact carrier,
authorization digest, canonical funding tuple and public transaction discovery.
It validates complete supplied parent/commit/spend bytes, including the actual
TxIDs and signatures, and distinguishes invalid, unavailable and capacity
failures. Its results do not claim canonical inclusion, ownership or finality.
Public discovery finds carriers without a saved local transaction ID; refunds
expose the envelope without becoming ownership transfers.

The independently reviewed Python reference contains12 positive cases,31 invalid
cases,2 missing-evidence cases and4 funding-tuple refusals. Its exact output SHA256
is `365e2997171b5f06706d628866f2cbc64cf0d7a3ac4205015def259a5340f4af`.
Python3.14, Python3.9 and the portable checked-in producer reproduce identical
bytes. Tests include third-party funding,16 inputs, actual full parent bytes,
nonminimal outer pushes, high-S outer signatures, harmless extra stack items,
and a valid non-RFC6979 gym signature. The internal gym signature remains low-S;
the parser cannot infer a signer's nonce-selection algorithm.

Focused normal/race/profile-neutral/vet qualification passed in
0.771/2.441/0.871/0.234 seconds, respectively:
`../artifacts/set73/carrier-focused-attempt2/report.json`.
The go-list dependency source closure, test data and exact compiler profile were
unchanged throughout; all process groups joined cleanly. The tagged run proves
only that this supplied-byte parser has no proof-of-work dependency. The first
attempt failed before tests because its inventory script wrongly required an
optional absent `go.work.sum`; that failure and runner are retained. These checks
do not replace the actual ownership/wallet acceptance still required below.

The carrier and wallet contracts are `FIGHTER_TRANSFER_CARRIER.md` and
`FIGHTER_CARRIER_WALLET.md`. Independent references precede their product
implementation. Public admission/finality history, replay, wallet schemas14/15,
reviewed transfer, owned stable/event/career integration and the packaged
independent restored-gym command remain in progress. Component checks below do
not complete the full set, which is not promoted, deployed or a release candidate.

The new observation changes compiled node inputs. A later completed Set73
promotion therefore requires the standing full checks, smoke and authorized
deployment verification; the earlier unchanged-node Set72 smoke is insufficient
for this new RPC. Master and the live seed remain unchanged at this checkpoint.

## Completed component: carrier journal and wallet codecs

The separate `fightercarrier` package defines strict versioned preparation,
signed-package and wallet bookkeeping codecs. Its private application journal
retains exact intent, reviewed preparation, both signed legs and delivery
attempts before broadcast. A dispatch ticket is single-use; retry retains the
same bytes. Only an unsigned operation can be abandoned. These are delivery and
recovery records, not proof that a fighter changed owners.

The independent wallet corpus SHA256 is
`8648ac23d5e077df1a4dd1e3b8f07157922e650e92e9eea99c233a6993283495`.
The original corpus and the narrow successor adding refusal cases remain in
the artifact history. Python3.14, Python3.9 and the installed portable producer
reproduce the successor. Shared codec normal/race/tag-neutral/vet checks passed
in1.254/9.999/1.618/1.125s, scoped to189 positives,113 negatives and12 raw cases:
`../artifacts/set73/wallet-codec-attempt2/report.json`.
Concrete manager/inventory cases are qualified separately; they are not counted
as passing merely because these structural codecs reject malformed data.

The journal's latest focused normal/race/profile-refusal/vet run passed in
2.599/18.320/0.990/0.445s:
`../artifacts/set73/carrier-journal-focused-attempt2/report.json`.
It covers exact independently derived transactions; one-use dispatch; same-action
replay; reopen without automatic sending; explicit byte-identical retry;
commit-before-spend observation; unsigned-only abandonment; and both restored
single-leg recovery modes. It also covers interrupted file publication,
preservation of foreign/malformed stages, alias/identity refusal, original-view
fences, joined cancellation and strict mutation-clock progression. All source
closures and the compiler profile stayed unchanged; process groups joined.

## Fresh recovery spendability checks

`OwnedChainSource` retains one original local node lifetime and rechecks its
network/genesis. Reads are bounded and complete traversals use the node's
instance/revision observation to detect restarts, advancing tips and same-hash
reorganizations. `CheckUnspent` now checks the exact confirmed output amount and
script between equal observations. Decimal RPC amounts are converted exactly
to atoms; fractional atoms and floating-point rounding are refused. This is a
fresh observation, never a fund reservation or ownership proof.

The existing node `gettxout` implementation previously included a chain output
even when a mempool transaction spent it. With `include_mempool=true` it now
checks pending spends before and after lookup, covering confirmed and unconfirmed
parents. Chain-only reads retain their existing behavior. Consensus is unchanged.

Focused transport normal/race/profile tests passed in1.462/3.506/1.349s; RPC
normal/race passed in2.550/5.344s and combined vet in0.827s:
`../artifacts/set73/unspent-focused-attempt1/report.json`.
These use bounded HTTP and RPC fixtures, not an actual transfer. The complete
source/profile binding stayed unchanged and all process groups joined. Actual
node recovery and the final set-wide checks remain required.

## Current integration and work order

Mike's November19 order is explicit: finish fighter transfers and independent
gym/fighter recovery; then complete the public mining pool and scheduled payouts;
then sign-in and onboarding. Existing fixture authentication supports tests only.
The final joint D74 design review remains before product release.

Wallet schema14/15, shared carrier/withdrawal exclusions, ownership-aware
stable/event consumers and public replay are being integrated. The noon cutoff
review found that freezing admission/checkpoint prefixes alone omitted the
durable finality-frame prefix. A narrow contract/reference successor is being
prepared so later frames cannot backdate ownership after a reorganization.
This implements D76/D83 without changing their timing or community doctrine.
Full reviewed transfer, competing-send/finality/noon checks and packaged
independent restored-gym use remain the acceptance milestone.
