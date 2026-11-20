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
durable finality-frame prefix. The narrow contract/reference successor is now
implemented so later frames cannot backdate ownership after a reorganization.
This implements D76/D83 without changing their timing or community doctrine.
Full reviewed transfer, competing-send/finality/noon checks and packaged
independent restored-gym use remain the acceptance milestone.

## Concrete wallet and export qualification

Schema14 starts the new typed allocation2 wallet; explicit restored schema15
activation retains the original exported recovery bounds and watches issued
paths in all four existing scopes. New spending remains native84. Carrier and
withdrawal owners share permanent signed-input exclusions. Unsigned abandonment
releases inputs without rewinding allocated return addresses. Reconciliation
uses the original View or FighterTransfer capability before and after IO;
post-commit authority loss preserves the write and poisons the original owner
with an uncertain-outcome error. InspectPrepared recovers the exact lost-ack
projection without minting a chain-authorized signing capability.

Qualification is deliberately composed: carrier-owner-attempt3 passed normal
and race (16.753/119.327s), but its tagged phase failed a test expectation.
carrier-owner-tagged-attempt1 passed the corrected top-level refusal and vet
(2.212/1.895s). Earlier schema placeholder and test-fixture failures are retained.
The later carrier-funding-reconcile-attempt1 passed normal20.778/race106.083/
profile4.707/vet0.709s with1,223 inputs unchanged. It verifies exact independent
16-input signed bytes, insufficient15-input funding, lost-ack inspection and
original-session reconciliation, not actual live-node acceptance.

The typed inventory2 recovery export passed normal6.913/race16.984/vet4.784s
in export2-focused-attempt1. Preparation does not publish; original RecoveryExport
authority gates the retained destination. Reset/close clears prepared bytes and
joins publication. Cross-version same-account exports remain mutually excluded.
withdrawal2-focused-attempt1 passed normal23.838/race124.814/vet3.945s, including
older allocation withdrawal regressions, exact journal reconciliation and
reservations retained across reopen. These scopes do not replace file-only
restored-gym and actual spending acceptance.

## Public evidence reconstruction

The public read transport serves only committed admission/publication rows,
signed finality frames and retained cutoff records. Its reader pins the complete
independently supplied ownership configuration and original endpoint/lifetime;
HTTP is literal-loopback only, HTTPS requires normal certificate validation plus
the expected SPKI hash. Requests, responses, work, deadlines and concurrency are
bounded. Close joins actual calls, including decoding and signature checks.
No private source database, account or operation journal is a recovery input.

Remote replay imports signed frames as untrusted evidence and verifies their
complete historical branches with normal work. It cannot sign as the source.
Newly observed finality lacking required portable source evidence is reported
unavailable for complete reconstruction, never as the former owner's ready
state. A source publication followed by fresh replay completes that evidence.
Exact abandoned work can be retired without deleting immutable records; a new
frontier must be fully verified again.

public-read-focused-attempt1 passed normal3.029/race8.400/profile2.013/vet0.829s.
A peer review then tightened unsupported HTTP deadline refusal and typed
cancellation. remote-abandon-attempt1 passed their successors with exact work
restart and remote genesis replay in4.920/12.567/1.844/0.294s,625 inputs unchanged.
Both are bounded local HTTP/genesis/model evidence, not funded transfer tests.
The subsequent shared prospect/header hash gate and historical cutoff readiness
correction passed public-core-hash-gate-attempt1: normal5.387/race14.930/
profile2.477/vet0.384s,633 inputs unchanged and all groups joined. Core header
work now uses the existing prospect verification mutex, preventing overlapping
256MiB Argon work with prospect verification. Old Verify behavior is preserved.

The desktop source and schema14 wallet adapters now compile in the explicit new
route. Full controller/native review, public-history recovery, end-to-end
acceptance, package checks and rollout are still outstanding. Nothing here
authorizes or claims production genesis, public sign-up or completion of D74.

owned-wallet-focused-attempt1 preserved three failures: the root public-source
test used a temporary path containing macOS's /var symlink, and the wallet
adapter tried initialization-only Resume on an inventory that already contained
a committed recipient. The successor canonicalizes the test path (production
path refusal stays strict) and opens a nonempty inventory using its independently
reconstructed exact binding. It also fixes same-tip instance/revision cache
reuse found by peer review. owned-wallet-focused-attempt2 passed normal15.178/
race54.212/profile10.196/vet1.237s,1,880 inputs unchanged. It includes actual
schema14 fixture setup/reopen and loopback normal-genesis replay; live wallet
watch/READY and funded transfer acceptance remain outstanding.

## Actual public transfer order and finality

The completed replay now classifies authenticated canonical competing sends as
pending/void, and identifies finalized transfers only through the effective
state's verified ancestry. An orphan-only witness or unrelated historical state
cannot appear finalized. Same semantic representations do not void their own
winner. `public-core-transfer-status-attempt2` passed normal3.601/race12.744/
profile2.116/vet0.285s with635 bound inputs unchanged. Attempt1 preserves an
inherited hash-cancellation test that could finish from cache before its polling
probe; its successor synchronizes at the real gate and uses a fresh nonce,
without changing production hashing or weakening cancellation/join assertions.

`public-core-actual-attempt2` passed in208.259 seconds with1,897 inputs and the
candidate profile/approved lock unchanged. Two fresh normal-PoW loopback nodes
mined164 blocks. Actual admission, two independently funded competing reveals,
local and public-only pending/void classification, sender ownership at59,
winner finality at60, and exact public HTTP/node reconstruction all passed.
Missing signed finality evidence correctly prevented a ready remote owner.
Explicit owned-node invalidation of reveal block104 returned the nodes to103;
local and fresh reopened public replay retained the same official final owner
and reported the same conflict. This is an administrative canonical-switch
rehearsal, not an organically propagated fork. Both nodes joined cleanly, with
no forced kills or remaining process group.

Attempt1 remains failed: its copied mechanical fixture writer authored version2
transactions, while the frozen carrier contract requires version1. Production
recognition correctly rejected them. The successor changes only the external
fixture writer, adds the exact version1 preflight and bounded typed diagnostics,
and retains the independent semantic-ID derivation and all finality assertions.
This completes the actual public-core scenario, not restored-wallet signing,
noon attachment, native dialogs, the normal app package or deployment.

Native delivery component checks passed normal8.617/race34.008/profile19.875/
vet10.367s in `owned-transfer-actions-attempt1`, with bound sources/profile
unchanged and clean process groups. The checks cover fixed signed-leg selection,
exact node bytes and canonical inclusion, changed instance/revision refusal,
missing evidence and joined cancellation using independent carrier bytes and
explicitly synthetic transport framing. The later unsigned-reservation burn
adapter has its own pending qualification; no native end-to-end claim follows.

## Exact node transaction observations

The node now implements the bounded single-transaction `getmempoolentry` read.
It returns the eight supported fields from one original mempool lock, with
distinct absent/busy/capacity failures and no whole-pool or ancestor traversal.
The existing broader JSON type remains unchanged. `VerifyBlockTransaction`
checks canonical transaction framing, unique transaction IDs, the Merkle root
and exact saved bytes; it makes no independent header-work or canonicality claim.

`owned-observation-focused-attempt2` passed the scoped node/core and desktop
normal, race, tagged and vet checks with4,723 inputs/profile unchanged.
`owned-observation-actual-attempt3` passed in26.159s with4,640 inputs unchanged:
two actual nodes reopened isolated copies of the previous closed acceptance
chain, confirmed exact commit inclusion, accepted one retained reveal into the
mempool, and verified both reveal transactions after reconsidering existing
blocks104–164. This used no new mining or signing and no private authority tree.
Both nodes and all process groups joined without forced cleanup.

Attempts1 and2 remain preserved failures: the administrative reconsider call
needed more than the inherited five-second observation timeout, then the test
helper incorrectly required non-null JSON result/error fields for a valid null
success. The successor changes only that bounded administrative test call;
production observation deadlines and exact transaction assertions stay intact.
The packaged native transfer and independent commit-only recovery milestone
remain outstanding; this observation qualification does not close Set73.
