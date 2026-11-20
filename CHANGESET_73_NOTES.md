# Change set73 — fighter ownership and direct transfer

Branch: `slug-fighter-ownership`, stacked from completed Set72 chain
`04ef85385d170272afd4cb28a88d2011168f1e2e` and wallet
`21f8d65494a6193f3071705cfad86f7c0e942eb2`. Both Set72 topic heads were pushed and
read back from origin; both master heads were unchanged. GitHub returned no
checks, statuses or workflow runs at those exact commits: absence, not a pass.

Functional transfer/recovery qualification passed on the recorded source
composition, including both ordinary noon ownership cases. Final clean release
packaging, promotion/provider verification and visible native/D74 product gates
remain open. The chronology below preserves failures and the exact scope of
each successor; the final checkpoint records the completed outcomes.

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
did not replace the later actual ownership/wallet acceptance recorded below.

The carrier and wallet contracts are `FIGHTER_TRANSFER_CARRIER.md` and
`FIGHTER_CARRIER_WALLET.md`. Independent references precede their product
implementation. At that component checkpoint, public admission/finality history,
replay, wallet schemas14/15, reviewed transfer, owned stable/event/career
integration and the packaged independent restored-gym command were still being
implemented. Those component checks did not establish full-set completion or
promotion; later qualification is recorded below.

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
node recovery and final set-wide checks were separate requirements, with their
later outcomes recorded below.

## Current integration and work order

Mike's November19 order is explicit: finish fighter transfers and independent
gym/fighter recovery; then complete the public mining pool and scheduled payouts;
then sign-in and onboarding. Existing fixture authentication supports tests only.
The final joint D74 design review remains before product release.

At this integration checkpoint, wallet schema14/15, shared carrier/withdrawal
exclusions, ownership-aware stable/event consumers and public replay were being
integrated. The noon cutoff
review found that freezing admission/checkpoint prefixes alone omitted the
durable finality-frame prefix. The narrow contract/reference successor is now
implemented so later frames cannot backdate ownership after a reorganization.
This implements D76/D83 without changing their timing or community doctrine.
Full reviewed transfer, competing-send/finality/noon checks and packaged
independent restored-gym use were the acceptance milestone completed by the
later reports below.

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

At that checkpoint, the desktop source and schema14 wallet adapters compiled in
the explicit new route; full controller/native review, public-history recovery,
end-to-end acceptance, package checks and rollout were separate later work. Nothing here
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
watch/READY and funded transfer acceptance were separate later work.

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
adapter was qualified separately later; no native end-to-end claim follows.

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
That observation qualification did not cover packaged native transfer or
independent commit-only recovery, and did not close Set73.

## Independent recovery and commit discovery

`restored-carrier-actual-attempt5` passed in215.782s with4,898 inputs unchanged.
It starts from the export2 file alone, resumes an interrupted schema12 scan,
activates schema15, and retains the same gym and fighter. It recovers the exact
lost preparation acknowledgment, reopens the signed pair before delivery, and
completes one transfer: commit102/reveal103, sender at59 descendants, recipient
at60 (height163), and fresh reconstruction through public evidence only. The
original hosted tree/export stayed unchanged and both actual nodes joined.
This used independently owned components in one test process; it did not test
the packaged restore command, native UI, or commit-only reveal/refund acceptance.
Earlier schema routing, signer, child-session logout and peer-relay harness
failures remain preserved with their narrow diagnosed successors.

The public replay now offers one opaque, fixed-head recovery-commit capture,
at most4,096 compact indices and eight rows per page. It filters by authenticated
historical predecessor ownership; candidate metadata cannot sign. Changed
head/source/node identity invalidates the capture. `ErrEscrowSpent` distinguishes
current exact escrow absence only after canonical commit and original node
checks; it includes mempool spending and does not assert permanent spend proof.
Missing/malformed evidence remains unavailable. `Store.Empty` requires the
audited pristine metadata-only store, not merely absence of a ready head.

`recovery-discovery-focused-attempt3` passed normal3.862/race9.153/tagged1.472/
vet0.288s with574 inputs/profile unchanged and all processes joined. The first
two attempts preserve sandbox preflight and a modeled fixture's missing map
initialization. These are scoped public-read/lifetime checks, not automatic
recovery from actual commit-only chain history. The wallet's separate intent-free
match scan passed its focused checks, including more than128 lookup candidates
while the journal stayed empty/held and the locked wallet DB stayed identical.

## Desktop integration checkpoint

The explicit owned mode is installed in the topic checkout. The native bridge
retains immutable request correlation before preparation and distinguishes
signing consent, broadcasting consent, delivery observations and public ownership
finality. Cancelled/revoked/duplicate callbacks cannot sign or send. Menu clicks
capture the original view before dispatch. Pending reset bootstrap exposes only
the original childless source until recovery can load both complete scopes.
Failed descriptor cleanup retains the actual owner and sticky uncertainty.

`owned-child-focused-attempt2` passed normal16.837/race23.134/vet6.390s;
`owned-native-focused-attempt1` passed native/bridge normal and race plus vet;
`owned-checkout-focused-attempt1` passed bootstrap normal/race, native consent,
all helper selectors and vet with6,125 inputs unchanged. The frontend's12
behavior cases pass in `owned-ui-attempt3`, including account-switch clearing,
disabled unavailable controls, exact saved requests and delivery/finality labels.
These use mocked native boundaries and do not claim a visible GUI acceptance.

The tagged calendar seam compiled with explicit ordinary-build exclusion. It
uses a fixed offset only on the idle fixture's owned/calendar/source-authority
clocks, with no later job/admission work; account, wallet and node clocks stay
real. At that checkpoint, whole-mode actual constructor/mining/recovery, funded
transfer/noon, commit-only reveal/refund, packaged CLI/native app and set-wide
smoke were separate acceptance requirements. The prepared owned bundle keeps the existing fist artwork; D74's
joint product/design review remains last. No Set73 release or deployment yet.

The standalone restored-session setup is now installed. It retains exact
INIT/ACTIVATE/READY boundaries, permits explicit schema12-to15 activation only
after the recorded guard, and opens established components without replacement.
Each refresh captures fresh public history from the same pinned reader and
abandons only the exact closed stale replay before retry. `restored-setup-attempt1`
passed compile11.475/normal11.909/race101.252/profile11.384/vet1.247s with1,958
inputs unchanged. Its independent marker corpus covers24 positive,54 semantic
negative and48 parser-negative cases. That setup qualification did not cover the
command-line facade or actual-chain discovery/reveal/refund.

Recovery file constructors now preserve an opaque cleanup owner when a first
descriptor close fails, including before a normal Selection/Handle is returned.
The error retains every failed owner and the original cause; cleanup never
retries a possibly reused OS descriptor. Native dialogs, the owned child and
the restored session retain that uncertainty and block replacement work.
`recovery-cleanup-focused-attempt1` passed the complete file package normal/race,
native consent/cleanup normal/race, session/child cleanup race and vet with6,140
bound inputs unchanged and exact source copies. No actual-node or visible GUI
claim follows from these checks.

Whole-mode startup now refreshes the exact concrete ownership source before
league construction. Stored rows alone do not establish a ready public head.
`owned-startup-focused-attempt1` passed normal12.188/race15.345/vet4.522s with6,783
inputs unchanged, covering fresh/reopened readiness and cancellation during a
blocked node read. The subsequent actual baseline attempt1 still failed at its
fresh constructor; both nodes joined cleanly and6,774 inputs stayed unchanged.
The valid owned league intent is48,686 bytes including its newline: it fits the
declared64KiB owned limit but the reused legacy writer incorrectly reapplied
its32KiB limit. A bounded writer now preserves each route's existing cap through
fresh publication, matching-existing retry and readback. The original-writer
measurement and failed baseline remain preserved. The boundary and existing
transfer-action checks passed normal9.940/race29.040/vet4.522s with6,786 inputs
unchanged. The same source also preserves wallet preparation errors that were
previously shadowed inside a branch. Whole-mode actual acceptance was then rerun;
these focused checks alone did not close the remaining Set73 milestones.

The standalone recovery command and its transfer/discovery facade are installed.
`restored-cli-focused-attempt1` passed compile8.566/normal17.321/race118.094/
profile12.939/vet1.080s with1,970 bound inputs unchanged and all process groups
joined. The command validates options before file access, preserves unknown
publication outcomes, separates signing from broadcasting consent, and joins
input/output cancellation even when the final error output is blocked. Discovery
keeps an independent retained copy of its public candidate rather than borrowing
the displayed response. That focused qualification did not cover the separate
process commit-only reveal/refund acceptance later recorded below.

Whole-mode baseline attempts2 and3 remain preserved failures with clean node
shutdowns and unchanged source bindings. Attempt2 encountered a legitimate busy
refresh during Start; its bounded successor retries only that exact refusal.
Attempt3 exposed a login/mining capability mismatch: the original login may
publish run state under View, while work and admission require the separately
minted Mining token. The narrow correction follows the existing pool-child
split and does not grant Mining to a login token. The later focused qualification
and baseline6 result are recorded below.

The actual desktop bridge attempt2 built the normal node, helper and probe, then
failed during scope bootstrap before login, mining or transfer. Its cleanly
joined three-request diagnostic shows successful startup followed by both scope
responses being refused during strict self-validation. The carrier Identity's
two exported fields lacked the JSON tags required by the strict owned decoder;
the correction preserves the already emitted field names and keeps decoding
strict. Failed bridge attempts and diagnostics remain available; that diagnostic
was not a passing packaged desktop flow.


## Final engineering evidence checkpoint (draft, not promotion)

This section records later completed evidence; every failed run remains
preserved. No Set73 release commit,
provider deployment or post-rollout result has yet been recorded.

The whole-mode baseline6 passed 59.796s with 6,798 inputs/profile unchanged and
clean process joins. It exercises both published fixtures, Start/Stop, admissions,
owned capture/entry, actual recovery export/Verify, exact reopen, reset-held
refusal and original-view behavior. The actual bridge4 passed 28.859s with 2,913
inputs unchanged: real normal helper processes, both fixture scopes, original
view correlation, logout and exact-root restart. It is a nonmining pipe test,
not visible GUI or funded-transfer evidence.

`restored-cli-commit-actual-attempt4` passed 776.383s with 5,172 inputs/profile and
binaries unchanged; all 33 CLI processes and both nodes joined cleanly. Actual
file-only schema 12 scan and explicit 15 activation retained the same gym/fighters;
automatic bounded commit discovery, reviewed reveal/refund, signing restart,
separate delivery approval, exact return/retry and checkpoint +59/+60 checks passed.
Original one/zero exported bounds remained controls. The earlier failed timeout
was preserved; bounded RefreshStep composition was separately reviewed and
qualified before this successor. Original private authority/journal trees were
not copied as recovery authority.

Full wallet attempt1 passed all six normal modules plus explicit cheap-profile
refusals, 525.041s with 1,346 unchanged inputs. Full desktop attempt1 passed all
eight normal targets, 243.372s with 4,372 unchanged inputs. That desktop result is
predecessor evidence: two later terminal-Observe adapter leaves are covered by
`carrier-complete-observe-attempt2` normal/race/profile/vet and 4,774 unchanged
inputs. Completed Observe now returns the same dated receipt under the original
View/selected-root fence, without node sampling or wallet/journal mutation.
Independent complete-state fixtures test returned-copy ownership, exact durable
bytes, revocation/held View, cancellation/Close and substituted-root refusal.
Public replay supplies current ownership. The tagged actual harness corrections
are excluded from normal builds.

Complete chain attempt2 passed the normal 256 MiB profile guard and full
`blockchain/...` / `integration/...` tagged suites, 85.315s total commands, 572
unchanged inputs and clean process groups. Both actual nested harness daemons
recorded the exact no-CSE compiler profile and explicit slugtestpow tag. Attempt1
had three passing commands but failed required nested-build provenance because
Go prepended GOROOT/bin; its launcher-only successor restored the locked shim
before test execution. Assertions, package lists and 30-minute Go deadlines were
unchanged. The two skip events were packages with no tests; no test case skipped.

The node smoke's 1,776 compiled inputs remain unchanged through these desktop
adapter changes. Its original height 61/history/checkpoint/activation/conflict/
verifychain/local-restart evidence is not a new rollout result. Normal package5
passed 18.744s with 7,529 paired inputs unchanged, five binary profiles and strict
codesign. It predates the final startup-readiness correction; the clean final
release bundle is still to be built once from its exact candidate release tree.
No intervening dirty bundle rebuild is necessary merely to update metadata.

Pending attempt3 passed canonical commit/reveal, saved complete observation, checkpoint,
+59 pending owner and sender capture, then recipient login refused with the
closed public error. Source/timing supports a coherent-balance Snapshot race
while wallet catch-up changes cached height inside a transaction, but the
original cause was collapsed and is not asserted proven. The private readiness
predicate waits without balance sampling while retaining original connected
backend/lifetime/session requirements, the 60-second startup bound and final
coherent Snapshot/publication fences. The focused startup successor passed
normal 24.830s, race 91.491s,
cheap-profile checks 8.110s and vet 0.980s. All 4,766 imported inputs/profile
remained unchanged and all eight process groups joined without signals/errors.
The real-manager regression proves the narrow old readiness question could fail
while catch-up commits; it does not recover the discarded cause of attempt3.
Report SHA256: afbbff9b4a4a5878cca469cd67a530bd3386ff2830fe100e4a3f0c7cdd5a9aeb.
The annotated actual below subsequently passed recipient startup; its noon
capture remained incomplete.

The post-startup source map confirms all 1,346 wallet, 572 chain and 1,776
node-smoke inputs unchanged. Full desktop remains a predecessor pass: exactly
three normal production files changed (the two delivery adapters and readiness
caller), one tagged actual harness changed, and three source/test files were
added. Focused successor evidence covers those deltas; no full desktop rerun is
claimed. Native consent/file-cleanup and retained-scope leaves, plus the 12-case
DOM inputs, are explicitly mapped. Bridge4 remains dated nonmining evidence;
its readiness dependency changed and was covered by the focused test, with
whole-mode actual qualification recorded later below. See the external post-startup validation map;
its JSON SHA256 is
21e4406c4528c97c4104a972b78e62d6f32ee1642e44f2d9117a8426fdd9d8a8.

The annotated pending-noon actual failed in 465.437s on a reviewed private
desktop copy with phase-only login diagnostics. Recipient login and complete
capture at +59 passed, followed by the real clock transition; the original
sender event still reported awaiting_capture at cutoff +82s. All 7,410 bound
source/artifact inputs and profile stayed unchanged; all three groups joined
cleanly. Normal node/PoW, scenario, assertions, deadlines and cleanup were
unchanged. This remains `shipping_qualification=false` and no visible native
dialog qualification. The ordinary final-before case did not run at that
checkpoint; neither noon gate had passed. Report SHA256:
5d382485ec2de036265f6952ce3b71f06454b4c78b618e8fe0250ccd25ea2d89.

A separate closed-artifact public audit used the original pins with physical
read-only registry/core opens. It found the authentic cutoff intent: same node
instance/revision 163/hash at height 162 before and after noon, accepted/signed
admissions 2 and publication prefix 31. No cutoff was published; retained replay
was still in its chain phase with next block 11. All 13 public-store files and
2,190 source hashes stayed unchanged; build/profile/audit processes joined
without forced cleanup. A second bounded read of only the exact public work
row and event-cutoff key
confirmed the same original boundary target at height 162 and no stored cutoff.
The replay had progressed through ancestry/header verification into block 11;
it was still historical work, not a second current replay. Both public trees
remained byte-identical. No private wallet/account data, node/network operation
or mutation API was used. The scheduler-only successor now yields 10 ms outside all gates after each
successful incomplete slice, then reacquires the original child/View fence.
Idle/error work keeps the one-second cadence; wallet refresh remains on five
seconds. All Step/batch/calendar/child/pre-noon limits and error/Close semantics
are unchanged. Focused normal 12.027s, race 22.667s, real tagged boundary 13.248s
and vet 1.357s passed with 6,819 inputs/profile unchanged and all four groups
clean. Report SHA256:
b2627ae48d6a25275b6619b9dc1488c412099ddb6669ce769833c18307198d92.
Ordinary pending4 then failed in 398.040s before any clock offset, while its
unchanged 120-second +59 Finality wait received empty state/height zero and
required-evidence-unavailable. All 6,812 source/profile inputs stayed unchanged;
all three groups joined without signals/errors. Original normal PoW, assertions
and limits were unchanged. Final-before did not run at that checkpoint, and
neither noon gate had passed. The focused startup and scheduler results retain their scoped passes;
no Set73 completion or diagnosis is inferred from the failure. Report SHA256:
d05ef4ba6be9474d3e5c86d3253eaac6b2b92411cff0d9da76646a5a07a41ed7.

At the post-calendar checkpoint, the map preserved the post-startup predecessor
and verified unchanged wallet/chain/node-smoke closures. Full desktop now composes
with five changed normal production leaves, one changed ordinary test, one
changed tagged harness and four additions; all differences are explicit.
The scheduler's complete source closure was unchanged at that checkpoint; earlier native
cleanup/protocol/DOM leaves retain their qualified scopes. Package5 remains
predecessor evidence. See the external post-scheduler validation map, SHA256:
32a65ebf048c51f7bf062dec200f5036d93405d0b3bbe90bb775fbdad1d14509.

Two further read-only public audits localized the ordinary pending4 obstruction.
Current replay still targeted height132/publication29, while the original registry
retained unsigned sequence30 for height133 and the node reached height162.
A different latest-tip payload was refused before stale-work retirement. All
13 public-store files remained unchanged. The precise post-stage error that
left the old row unsigned was not logged: a moving-tip trigger is source/timeline
supported, not directly established. The obstruction itself is confirmed.

The narrow chain-publication successor preserves the exact old body/time and
checks its height/hash through the original concrete node's fresh canonical
observation before signing/committing. It runs before admission PublishNext,
including accepted-but-unsigned admissions; newer current observation remains
separate. Missing/orphaned evidence fails closed, with no signed abandon or
skipped sequence. The focused registry/core/pilot normal/race/profile-refusal/vet
run passed all twelve commands, 6,418 unchanged source/profile inputs and 24
clean process groups. Report SHA256:
d312ebec5ecd71664f9af8ece5d6ad794d2285f67065aa7a980df5bf90c848fc.

A separate reviewed six-file cutoff addon is installed and qualified. Its focused
registry/core/pilot normal/race/profile-refusal/vet run passed all twelve checks,
including five new cutoff and six retained chain regressions under normal/race.
All 6,431 source/profile inputs remained unchanged and all 24 groups joined
without signals/errors. Report SHA256:
55667865095cead5dab36415c8992893ba0c160a3a8357f943bc91c003adf471. It drains an interrupted exact reference to an already signed,
stored cutoff before ordinary startup publication, including before league
construction. It checks original signed body/ID/boundary/registry intent and
pending/head CAS. It creates no new cutoff signature/time or readiness; normal
replay still gates Head/event reads. This is an adjacent interruption recovery
finding, not the observed pending4 cause. Ordinary pending5 then failed in 417.339s at the unchanged 120-second noon
participant wait (main582). It had passed the +59 and recipient checks and
recorded phase07 with a nonzero clock offset. Its last sample was a zero
EventStatus with Busy; this may overwrite earlier statuses and does not prove
all polls were Busy. The runner's clock_offset_used=false is PASS-only summary
bookkeeping, not evidence that the failed case stayed on real time. All 6,819
inputs/profile/binaries were unchanged and all three groups joined without
signals/errors. Report SHA256:
235a4ae9407b51776fdf38b2379d2640c45ef685b8080f72ecc1f089b247bcf1.
The failure is preserved. The closed public audit then found the exact stored
cutoff also published by the issuer, no pending issuer row, and no core work row.
Issuer publication sequence53 recorded clock high water at cutoff+14.045s; the
stored current head was height162/publication53 with clock high water at
cutoff+27.388s. All 14 public files and 2,201 source inputs remained unchanged.
These raw public records distinguish the failed event capture from the earlier
unfinished replay/publication cases; they do not grant fresh current readiness
or event authority. Audit report SHA256:
16fda166c84d761bf72217d365989789d84296ae59838bb44796a7a8af3a2c51.
At that audit checkpoint, final-before had not run and the capture diagnosis
was unresolved. Both publication focused passes retained their scope; neither
was successful noon acceptance or Set73 completion.

A second read-only consumer audit found the event reserved at cutoff+14.107s
and its clock at cutoff+80.348s, with neither participant capture present. The
active Result career ClockMS remained its activation time 1795192762371, below
the cutoff 1795291200000. Source review confirmed that all three owned calendar
capture branches omitted the existing Result.FenceCareer prerequisite; the tagged
calendar offset also omitted the Result clock. The frozen career contract requires
a durable original-View fence before capture and refuses an unfenced cutoff.
The exact six-file coordinator/tagged-clock correction is installed and
independently reviewed; focus attempt1 failed after eight ordinary checks passed;
no schema, award rule, ownership readiness or noon acceptance changes are inferred.
Consumer audit report SHA256:
d3471340c16d25393cb6ce2f1320a0ed8745021e3973e7386d4be69126791c0d.
All 14 public files and 2,201 source inputs remained unchanged.

The post-publication map recorded two changed imported production files in
the full-wallet predecessor closure and three added production/four test leaves
in those imported packages. Wallet repository files are unchanged, but its
compiled closure is no longer called unchanged. Wallet has no calls into these
publication functions, and the new leaves have no init/global behavior. The prior
full pass is composed with the focused publication successor; actual wallet/core
acceptance is reported separately, with no current full rerun claimed. Full chain572 and smoke1776
remain exact, with no new publication leaves in their bound packages. Full
desktop is predecessor evidence with ten existing differences/thirteen additions.
The later cutoff runtime/additions are explicit successors to chain focus and
are included in the exact cutoff focused source closure.
See the external post-publication validation map, SHA256:
99f8ca85f078eb0e15ff7bbbbcd7577ee53c3b69391df6928e6e8a0a494f4a4f.

Neither native attempt produced validated original-root GUI acceptance.
Attempt1 stalled during CUA attachment and reached its unchanged 1,800-second
process bound. Attempt2 selected default-root instances instead of the original
funded process and ended by manual interruption. Their failed reports retain
forced termination and cleanup-probe uncertainty; later empty exact-group
censuses do not convert them to clean-join passes. Default fixture logins and
public empty refresh did occur, without signing, mining or export. They do not
qualify the original funded flow, real chooser/modal behavior or visible
end-to-end acceptance. Real bridge/controller flows, native consent/cleanup tests, 12 frontend behavior cases and normal package
evidence form an explicit finite engineering composition; visual/native product
acceptance and the final joint D74 review remain open. No further default-root
mining or indefinite tool-attachment loop is required by this engineering check.

After actual gates and exact source documentation, root will push paired topic
commits, construct a candidate release commit with the exact topic tree and prior
master/topic parents without moving master, cleanly detach it, and build the
final normal bundle once. Prepromotion provider/seed/CI checks precede CAS master
promotion and ordinary push. Exact provider SUCCESS/source/volume/endpoint and
900-second read-only observation with the same clean release binary are required
before recording rollout completion. Those identifiers and outcomes are unknown
here; no public mining, checkpoint change or new volume is part of the sequence.

The next area remains the public mining pool and scheduled payouts. A scheduler
already exists; public operator lifetime, external-miner access and nonfixture
signer/backend custody/operation are the concrete missing integration. Existing
fixture service/journal tests do not qualify that deployment. Preserve payout
policy and exact-byte recovery; sign-in/provider/MFA remains later. D74 stays last
before friends/tester or public product release.

The external post-career source comparison retains prior full-suite scope. This
latest correction adds no further wallet, chain or node-smoke input changes; the
earlier wallet imported publication leaves remain explicitly composed through
their focused checks. Full chain572 and smoke1776 inputs still match. The full
desktop predecessor now has eleven changed existing inputs and seventeen added
files in its original nonignored selection; the latest delta is one ordinary
calendar helper change, one ordinary test, and tightly tagged Result-clock
wiring/setter/tests. The qualified cutoff-resume selected closure now differs at
that calendar helper, so it is not relabeled current/unchanged. At that map checkpoint, final packaging and both noon acceptance gates were
open; the later actual outcomes are recorded below.

Career-fence focus attempt1 passed result/pilot ordinary normal, race, cheap-profile
refusal and vet (eight checks), then failed the first tagged Result clock test at
initialization with outcome_unknown. The test put its DB under one temporary
directory and passed a different directory to initialize, so durable tree sync
failed. No production change is proposed; a same-directory tagged fixture
successor corrected that layout. The original aggregate remains FAILED, with all 6,518
inputs/profile unchanged and all 21 groups clean. The six tagged successor checks subsequently passed and compose explicitly
with the unchanged eight ordinary passes. Report:
335d6cc3147fab566be6fd253858a1f9da31ef56e30f4af8ebb41b9fef7da7d3.
No successor actual or final-before result was supplied by that failed focus.

The career-fence tagged successor passed result/pilot tagged normal, race and
vet (six checks). Its 6,516 current source/profile inputs remained unchanged and
all 18 groups joined cleanly. It checked the original 6,518-input inventory with
only the exact tagged fixture replacement, excluded from ordinary/cheap builds;
the eight prior ordinary passes therefore remain applicable. This is an explicit
14-check focused composition, not a rewrite of the failed first aggregate or a
current full-suite/actual-noon pass. Successor report SHA256:
5315be80e09af10caffd064a5f0ae4f6bc2f82edf2e15968fa5349ec376ab199.
Neither ordinary successor had run at that focused-check checkpoint.

Ordinary pending-at-noon attempt6 passed in331.921s with all6,820 source/profile/
binary inputs unchanged and three clean process groups. The real controller
two-leg transfer reached checkpoint+59 before the authentic noon bracket; the
sender remained owner of record in the exact two-participant capture, with its
original birth and independently checked condition. After+60 finalized recipient
ownership, the saved noon event/cutoff remained immutable. Exact component reopen
and frozen authority counters passed. This uses normal node/PoW with a scoped
calendar offset; native dialogs and full ordinary controller reopen after a
future clock offset are not qualified. Report SHA256:
8500dd197dbec05835d5785df9ba9f9f352d7be2fd082db9cfbc23b77b309d68.

Finalized-before-noon attempt1 then failed in245.886s after+60 and fresh recipient
login, before clock transition/noon. Its addReceivedEntry fixture requested both
independently born welter fighters; the unchanged one-per-class rule correctly
returned entry_class_conflict. All6,820 source/profile/binary inputs stayed exact
and all three groups joined cleanly. The reviewed tagged-only helper successor
selects the received fighter alone while retaining two-item owned inventory and
roster, both participants/original receipts, outgoing exclusion, exactly-once
target owner/birth/condition, saved history, component audit and original limits.
It makes no production or policy change. The helper is reached only when
finalBefore=true, so pending6 retains its valid prior false-path scope and is not
relabeled as a successor rerun. Final-before acceptance was still outstanding
after this failed first attempt.
Failed report SHA256:
84036caf9a99765b7a8d76ea85f650bdf2ddb7a26e4a81205001d625c3c8e7d9.

Finalized-before-noon attempt2 passed in358.172s with all6,820 source/profile/
binary inputs unchanged and three clean process groups. It exercised the same
actual two-leg carrier and +59/+60 checks, finalized recipient ownership before
the authentic noon bracket, retained both owned/rostered fighters while selecting
only the received fighter for that event, and verified both exact participants
and original entry receipts. The sender was excluded at cutoff; the target
appeared exactly once under its recipient owner with unchanged birth and the
full independently checked condition. Saved event/cutoff identity, frozen
authority counters and public-core/event component reopen all passed. Report:
26bc3f9baf30dc8a5f53a31ecfb8d7c4fda7785eef5340fe235ae3392f9ab705.

Both ordinary noon variants are now functionally qualified. Their6,820-input
inventories differ in one checkout file: the reviewed tagged helper reached only
when finalBefore=true. The other seven differences are its external mirror, plan,
source metadata and case-specific execution-lock/lineage substitutions. Production
bytes and every ownership/noon/condition/history assertion and deadline are
unchanged. Pending6 remains a valid predecessor pass for its unaffected false
path, not a claim that it reran the successor harness. The complete final-before2
inventory was rechecked against the current filesystem when preparing the final
external evidence map and matched.

This functional closeout composes the recorded full wallet/desktop suites with
the explicit focused successors, unchanged full-chain/node-smoke source checks,
independent recovery/CLI actuals and both ordinary noon cases. It does not claim a
new full current-wallet/full-desktop rerun. The actual reopen scope is public-core/
event components with normal component cleanup; native dialogs and a full
ordinary controller reopen after future clock offset are excluded. Visible
original-root GUI acceptance and D74 remain product gates. Final clean release
packaging, exact promotion/provider identity and900-second post-rollout observation
remain unperformed at this documentation checkpoint. Root records those actual
release facts only after they occur; none is inferred from functional passes.

Final hygiene checks found all Go files formatted. Staged diff checking reported
one inherited trailing blank line at
`gymstable/testdata/owned-reference/inputs/condition/inputs/lifecycle_rating_reference.py.txt:46`.
Those frozen reference bytes are identical to the committed fightercondition
original and its catalog, SHA256
`aa97b6f197d78d0f6245e5c122b482c5e5b1d6e34a74db5224e5f535cbda8d64`,
and were deliberately preserved. Every other staged diff check was clean; this
is a documented reference-input hygiene exception, not a production defect.
