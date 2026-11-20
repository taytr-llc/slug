# Change set 74 — public pool runtime and scheduled payouts

Branch `slug-public-pool-runtime`, stacked from completed Set73 maintenance
`1aba70ca36dfbd8e492f64b33c8d329f682de56c`. The live seed remains release
`e3eb4d495455383121bc1138f2cc010c90c6fb72`. This set is in progress. Isolated
normal-PoW scheduled-payout and restart acceptance pass; public deployment,
operator provisioning and the remote desktop attachment remain ahead.

The work order is public mining service and scheduled payouts, then sign-in and
onboarding. D74 remains the final joint product review. Preserve D58/D79/D80/D81:
per-block difficulty-weighted rounds, confirmation at 60 descendants, payment
only after coinbase maturity, one daily batch, 1 SLUG minimum, exact carry,
network fees from the pool's 1% only, and exact-byte recovery without advances.

## Completed component: recover without preparing a new payment

`poolsettle.Store.RecoverPending` uses the original transaction-recovery prefix
but returns before importing new credits, selecting funding, quoting a fee,
signing or occupying a new daily slot. It can observe or resend only the exact
already-retained transaction. Successful `complete` with no reason describes
recovery, not today's payment; uninspected available funding is conservatively
zero. Existing RunDaily behavior and the authoritative Batch.Day remain intact.

`SchedulerBinding` audits the original writable settlement and returns owned
configuration copies. It refuses read-only handles and performs no backend
operation or financial mutation. Its logical configuration digest is not a
unique database identity; the runtime must retain the original paths/owners.

Qualification: `artifacts/set74/settlement-attempt1/report.json` passed with
3,792 source/reference inputs unchanged under the existing Go 1.27.4 no-CSE
candidate profile. Focused normal tests passed in 14.814 seconds, race tests in
77.154 seconds, cheap-PoW refusal tests in 2.922 seconds and vet in 0.445 seconds.
Every owned process exited cleanly. Coverage includes no pre-due preparation,
lost-ack exact-byte retry, next-day new-credit separation, unresolved/conflicted
funding, cancellation, capacity, uncertain writes/reopen, original-owner binding
and the existing transaction/maturity/history controls. No node or public chain
was mined by these library checks. This is not full service or wallet acceptance.

## Completed component: explicit operator key owner

The internal operator owner pins separate public admission, publication, cutoff
and checkpoint roles. Online construction accepts only the first three private
file selections; public checkpoint imports need no checkpoint custody. It
retains exact original file descriptors and identity, refuses permissions/ACLs,
links and replacements, and joins signing/commit operations before Close.

Owner qualification passed on macOS: normal 1.364 seconds, race 3.094 seconds,
vet 0.185 seconds and the no-CGO refusal control 0.668 seconds. All 1,986 bound
inputs/profile remained unchanged and six process groups joined cleanly. Tests
include 32 independent signatures plus actual added-file/parent ACL refusal.
Evidence: `artifacts/set74/operatorauthority-focused-attempt4/report.json`.

The first attempt found that macOS denies the security-xattr query even without
an ACL. The corrected implementation uses the retained-descriptor ACL API;
Darwin operator custody requires CGO and explicitly refuses without it. Preserve
the first defect, a harness-only SDK symlink admission failure, and two corrected
test expectations separately. Existing node/helper settings remain unchanged.
Linux execution and downstream operator integration are still required.

## Completed component: operator admission/publication stores

Explicit version-2 profiles select independently supplied operator public keys
and fresh V2 file/database guards. Legacy V1 constructors and validation remain
closed to those profiles. Writable stores retain the original concrete owner;
audit readers need no signer. Admission, checkpoint-publication and exact
pending chain/cutoff commits retain the original operation through durable
publication. A shaped chain reference alone cannot obtain a signature.
Checkpoint import accepts already-signed public evidence, never private custody.

The independent public corpus reproduced identically under Python 3.14 and 3.9.
Registry tests consume 48 positive and 92 refusal cases, 12 canonical raw cases,
12 guard projections and four checkpoint-role cases; the remaining six finality
frames and ten frame refusals belong to the later ownership-core consumer.

`artifacts/set74/registry-attempt2/report.json` passed with 3,846 source/reference
inputs unchanged: normal 3.278 seconds, race 9.477 seconds, cheap-profile controls
1.435 seconds, no-CGO pure codecs 1.655 seconds and vet 0.404 seconds. All owned
processes joined cleanly. Concrete store tests include normal-PoW admission,
exact pending reopen, wrong original-owner refusal, audit/mixed-guard refusal,
checkpoint import without custody, and cancellation during synchronous commit.

Attempt1 passed normal/race but wrongly ran normal-chain checkpoint positives
under the incompatible cheap genesis. The successor preserves those inputs and
asserts all 12 checkpoint/publication refusals there; no production validator was
weakened. Darwin lifecycle tests select CGO as required by the qualified owner;
pure codecs remain supported without it. This is local component qualification,
not a public operator deployment or complete core/remote-reader acceptance.

## Completed component: recovery-first daily scheduler

`poolschedule` retains one original concrete settlement store and a separate
bounded schedule journal. Every start/retry recovers retained payments before
considering a new daily batch. Only a completed daily result advances the UTC
cursor; recovery, deferral, uncertainty and missing funding never pretend a
payment completed. An uncertain completion write poisons the handle and requires
reopen/recovery. Close cancels and joins active work before releasing storage.

The independent scheduler corpus contains 37 positive, 35 refusal and six
completion-guard cases. `artifacts/set74/scheduler-attempt2/report.json` passed:
compile 1.554 seconds, normal 5.925 seconds, race 22.472 seconds, cheap-profile
codec/refusal 3.281 seconds and vet 0.491 seconds. All 4,443 paired-repository
inputs and the profile remained unchanged; five process groups joined cleanly.
Concrete tests cover before-due recovery, due dispatch, same-day suppression,
uncertain completion/reopen, original-owner binding and joined cancellation.
Attempt1 was a test-only return-arity compilation error, retained separately.
The directory is trusted operator storage; pre-open checks do not promise
protection from a hostile local writer racing LevelDB's background path access.

## Completed component: concrete payout-node connection

`poolsettlenode` supplies funding inspection, fee quotation, transaction
observation and exact-byte broadcast. It has no key or signer. It requires
authenticated HTTPS with selected trust or literal-loopback HTTP, verifies the
Phase A identity and activation, checks original canonical mature funding even
through change, and distinguishes definite indexed absence from unavailable
evidence. It sends once; the settlement journal owns uncertain-delivery recovery.

Fee policy explicitly selects a node estimate or fixed atoms per kilobyte.
Independent rational arithmetic and serialized-size vectors cover decimal units,
conservative native SegWit witness size and CompactSize boundaries. Unavailable
estimation and configured caps cause deferral, never silent fallback or clipping.
Peer review found and fixed contradictory error/result envelopes and omitted
critical zero-valued fields before execution; negative controls preserve both.

`artifacts/set74/settlement-node-attempt2/report.json` passed: normal 1.197
seconds, race 3.680 seconds, cheap-profile controls 1.150 seconds and vet 0.291
seconds. All 3,869 source/reference inputs and the profile stayed unchanged;
four process groups joined cleanly. Tests exercise local HTTP/TLS models,
maturity, orphaned original funding, authentication/trust, malformed replies,
fee caps, lost broadcast acknowledgment and joined cancellation. Attempt1
compiled but the sandbox denied its local listener; unchanged sources passed
with local-listener permission. This is not an actual funded-wallet payout.
See `POOL_PAYOUT_RUNTIME.md` for the component contract and remaining integration.

## Completed component: operator ownership and public reader

Explicit operator issuer/replica/audit constructors select the V2 profile and
persist the expected role in both marker and metadata. Issuer operations retain
the same concrete online owner as the original registry; replicas and audit
readers have no signer. An immediate original-owner operation spans signing and
durable commit, including exact pending chain/cutoff recovery. The offline
checkpoint key is not loaded. Existing V1 APIs and record bytes remain intact.

The separate `/slug/ownership/v2` route uses independently expected profile and
version selection. Remote reconstruction verifies signed public evidence and
normal work; it cannot create a missing finality frame. Changed persisted roles,
substituted equal-key owners, mixed guards and incomplete public evidence refuse.
No constructor migrates an old history or changes chain order/finality.

`artifacts/set74/ownership-core-attempt1/report.json` passed with all 3,879
source/reference inputs and the profile unchanged: complete fighterownership
normal tests 9.120 seconds, complete race tests 18.126 seconds, operator
cheap-profile controls 2.334 seconds, no-CGO public/replica checks 2.810 seconds,
and fighterownership/registry vet 0.458 seconds. Five process groups joined
cleanly. The independent corpus supplies six frame positives, ten frame refusals
and six conditional compositions. Concrete tests cover public normal-work
admission reconstruction, pending reopen, boundary/cutoff, owner cancellation
during commit, ownerless serving and remote refusal to sign. Synthetic branches
in the pure corpus are not claimed as normal-work chain evidence.

Pre-execution source review corrected the new final operation check to occur
before successful cutoff completion cancels its own replay context; the original
stage is preserved externally. This component qualification does not establish
an actual funded operator transfer, remote service deployment or native V2 gym
integration.

## Completed component: immediate settlement signing authority

The original settlement now attaches a private, active capability only around
its synchronous backend Sign call. It binds the exact store/backend pointers,
source/configuration, verified inputs/claims, plan and unsigned bytes. The
dedicated wallet can check this without reentering the already-held settlement
gate. Retained or cancellation-stripped contexts cannot revive it; it expires
before signature validation, durable batch publication or broadcast. Existing
backends still receive the same inputs and the financial ledger is unchanged.

`CheckBackendBinding` audits the original store during constructor binding;
`ConfigBindingID` provides only public configuration correlation. No DTO or
matching digest grants signing authority. The independent wallet16 corpus
reproduced byte-identically under Python 3.14 and 3.9 and supplies three config
positives and six refusals here; wallet-specific vectors await their consumer.

`artifacts/set74/signing-capability-attempt1/report.json` passed: normal 8.836
seconds, race 29.072 seconds, cheap-profile/config controls 2.752 seconds and vet
0.451 seconds. All 3,883 inputs/profile remained unchanged; four process groups
joined cleanly. Controls include active/expired/foreign capability, cancellation,
noncomparable backend refusal, no reentrant gate, and the existing durable-before-
broadcast, exact retry, midnight, uncertain-write and signer-mutation tests.
This capability is not the dedicated wallet implementation or an actual payout.

## Completed component: bounded public TLS ingress

`poolingress` owns a TLS 1.3 listener pinned to the selected HTTPS origin,
certificate identity and eight exact existing routes. It forwards only bounded
requests to three original private loopback services. It rejects forwarded
identity, alternate routes, ambiguous framing and redirects; uncertain upstream
responses never trigger an automatic retry. Connection/request caps and joined
shutdown cover partial bodies and active upstream work. It has no wallet or
administrative route.

`artifacts/set74/pool-ingress-attempt3/report.json` passed all eleven tests,
race checks and vet: 1.003, 3.105 and 0.288 seconds respectively, with 3,889
source/reference inputs unchanged and all process groups joined. Fifty literal
protocol cases independently specify accepted/refused request shapes.

Attempts 1 and 2 remain failed, preserved evidence. Two cancellation fixtures
waited for server request cancellation without first consuming their request
bodies. Local Go 1.27.4 HTTP source confirms disconnect monitoring starts after
body consumption. The successor fixtures assert the exact body before waiting,
and add cleanup-only release paths. The original three-second cancellation
assertions and production ingress source remain unchanged. This component pass
does not establish public deployment or an actual payout.

## Completed component: fixed miner enrollment and credentials

`poolminer` binds a bounded, sorted miner enrollment to the complete operator
profile, pool source and settlement destinations. Credentials are scoped to
pool or admission use; only hashes enter the private durable journal. Rotation
uses exact generations, refuses token reuse across audiences and caps a new
credential at thirty days. Exact lost-response recovery returns the original
expiry without extending it. Revocation, expiry and backward-clock detection
remain effective across reopen. An original session callback and its temporary
recheck cannot survive cancellation, callback exit or joined Close.

`artifacts/set74/poolminer-attempt1/report.json` passed normal tests 2.116 seconds,
race 3.960 seconds, cheap-profile refusal 1.471 seconds and vet 0.343 seconds;
3,904 inputs/profile stayed unchanged and all four process groups joined. The
independent corpus covers eight positive states/configurations, thirty-two
refusals and three credential hash vectors, reproduced identically. Concrete
controls cover two-audience rotation, expiry/revocation, unchanged recipients,
uncertain writes, escaped rechecks, callback panic, original file identity and
close while an original operation is active. This is pre-enrolled mining access,
not hosted login, wallet ownership or public signup.

## Dedicated wallet and operator process

The paired wallet topic at `40959793` adds an explicit schema16 encrypted fixed-path operator
wallet. It retains the original owner/guard and exposes settlement signing only
through the immediate original-store capability. Generic wallet APIs refuse this
purpose; explicit unlock/expiry and cleanup retain their original custody. The
independent signing corpus includes exact signatures through 256 inputs. See the
paired `CHANGESET_74_NOTES.md` for the three preserved failed attempts and the
successful composite: normal 16.443s, race 117.595s, legacy 1.689s, supplemental
cheap refusals 3.329s and vet 1.956s. The cheap successor changes only its test body
and selection; production and executed normal test bodies remain unchanged.

`desktop/pooloperator` now composes the concrete authority, dedicated wallet,
issuer, credential store, pool, rounds, settlement, scheduler, ownership replay
and TLS ingress. It binds the original settlement backend before READY, owns
bounded maintenance and unlock renewal, and joins all work before custody release.
Missing established children fail closed. The new headless `slugpoold` command
supports init, serve, stopped-root status/credential management and public signed
checkpoint import. It neither creates a wallet nor handles the checkpoint key.
See `POOL_OPERATOR_RUNTIME.md` for the configuration and finite-run boundary.

`artifacts/set74/runtime-focused-attempt1/report.json` passed compile 6.905s,
normal 7.383s, race 15.527s, unsupported no-CGO refusal 7.243s, cheap controls 7.108s
and vet 0.779s. Its 4,515 source/profile inputs stayed unchanged and all process
groups joined. Tests cover independent setup bytes/IDs, 21 cross-field refusals,
five raw refusals, private-file/ACL identity, original-root lock/partial restart,
local actions, signed checkpoint import without chain-authority grant, canceled
work joining and bounded command output. Action tests use real partial owners;
they do not claim full funded Runtime.Run acceptance. The portable independent
reference producer and its public inputs reproduce the exact setup corpus.

Final composition review identified a normal contention path: admission and
public maintenance share the original nonwaiting signing authority. Its raw
`operatorauthority.ErrBusy` now reports busy and retries without discarding the
retained replay. Retry requires every joined error to be a recognized retryable
cause; busy cannot hide closed custody, integrity or cleanup failure. The
successor `runtime-contention-attempt1` passed normal 8.589s, race 10.680s and
vet 0.386s with 4,524 unchanged inputs and clean process joins. Twelve classification
controls cover this correction. This is a source-review finding, not a previously
observed deployed outage.

`public-pool-actual-attempt1` built the exact normal node, headless command and
two-miner probe, then stopped before miner startup: standalone wallet creation
could not find the bdb driver. Both executable dependency inventories omitted
that driver, while wallet-package test imports had registered it. The created
fixture wallet directory remained empty. Both nodes and the process group joined
cleanly; all 2,409 source inputs, profile and binaries stayed unchanged. This
failed actual attempt is preserved. The concrete operator host now explicitly
registers bdb, matching the existing desktop host convention; registration does
no key or database I/O. `runtime-driver-attempt1` passed normal 9.695s,
race 11.481s and vet 0.453s, including a standalone driver regression check.

## Actual scheduled payout and restart

Attempt2 passed standalone wallet creation and correctly refused the rehearsal's
CA certificate as a server leaf. The fixture now uses a separate non-CA IP-SAN
server certificate, CA-only client trust and leaf SPKI pinning. Product TLS
validation stayed unchanged; the failed run and its clean shutdown are retained.

`public-pool-actual-attempt3` passed in 382.080s on chain source `4a3f20da` and
wallet `40959793`, using normal 256 MiB PoW and two isolated nodes. Two independent
automatic miner processes used the actual TLS ingress, found canonical blocks 2
and 3, resolved every saved pool/fighter receipt and reopened identical outboxes
in separate processes. The bootstrap reward paid a different fixture address
and never entered pool accounting.

At the real UTC due time, both miners showed 9.9 SLUG confirmed, zero available,
reason `immature`, and no attempted broadcast. The scheduler was cleanly stopped
while the fixture mined both original coinbases to maturity; reopening the same
runtime then prepared the daily transaction. The node accepted it, and the
controlled relay discarded that response. Runtime, wallet and authority closed
and reopened on their exact original paths. Recovery observed the retained
transaction, which confirmed in block 104. It paid 990,000,000 atoms to each fixed
payee, with 240 atoms of network fee taken only from the pool's 20,000,000-atom cut.

Only one transaction was attempted/accepted: `c76ea513f07c9e0eba9c5aa91529345c3931743135294d7cb0c6bee51c545255`.
After two more retry intervals, each miner still had exactly one same-day
payment. The actual `slugpoold status` and `serve` processes reopened the stopped
root, served the same confirmed histories and joined cleanly on SIGTERM. Both
nodes, both miners, both outbox-reopen processes and both CLI processes joined
without forced cleanup. All 2,434 selected inputs, profile, binaries and public
evidence remained unchanged. This proves lost-response reconciliation without
a second payment; this scenario did not need to retransmit the observed transaction.
The source-bound summary is `docs/evidence/set74/operator-actual.json`.

`all-module-builds-attempt1` also passed all six wallet modules, ten chain modules
and the headless operator command (25.162s aggregate build time). Its 2,687
repository inputs, 3,169 selected inputs, profile and output stayed unchanged.
This is Darwin build/runtime evidence; Linux/container execution is not inferred.

`operator-linux-build-attempt1` cross-built the untagged operator for Linux
amd64 (13.972s) and arm64 (34.688s), with CGO disabled and the same Go 1.27.4
no-CSE profile. Embedded settings match both targets; 2,708 inputs, the profile
and both binaries remained unchanged. This is cross-build evidence only. The
local Docker client has no running daemon; Linux process, filesystem/ACL and
container qualification remain outstanding. The explicit fork's topic branch
has no Actions runs, so remote CI is not counted as a pass.

## Remote desktop configuration and credentials

The first remote desktop slice validates a complete OperatorV2 source, fixed
payees, HTTPS/CA selection and the existing schema14 wallet's original recipient.
Two separately protected audience credentials retain their original descriptors,
bounded leases, cancellation and sticky cleanup. Interrupted mutations retain
the audience-specific unknown-outcome error used by exact worker recovery.
There is no native route, attached wallet or automatic miner in this slice.

`remote-credentials-attempt2` passed normal (10.429s), race (21.620s), no-CGO
refusal (14.742s), cheap-profile controls (14.350s) and vet (1.512s), with 4,579
unchanged inputs and clean process joins. The first failed attempt exposed a
reference collision between operator change and miner payout. The independent
successor uses an already published third operator wallet, preserves both local
recipient vectors and adds a collision refusal; production validation is intact.
Python 3.14 and 3.9 produced identical bytes; the portable committed producer
also reproduced them exactly. See `REMOTE_POOL_ATTACHMENT.md` and
`docs/evidence/set74/remote-credentials.json`.

## Native Linux qualification workflow

The topic-only Linux job checks the exact pushed chain revision with wallet
`6a69ecfc`, uses the same Go 1.27.4 no-CSE profile, builds normal Linux binaries
and runs focused authority, wallet16 and operator normal/race checks. Its five
actual POSIX ACL controls test refusal before and after descriptor selection.
It retains bounded public logs and source/binary bindings, joins process groups
and removes synthetic test custody before uploading evidence. It neither deploys
nor accesses production secrets. Provisioning and deployed-host acceptance are
explicitly excluded from this initial job. GitHub rejected its installation:
the existing OAuth login lacks the `workflow` scope. The reviewed file is staged
as `docs/ci/operator-linux.workflow.yml`; it is not an active Actions workflow.
The rejected unpublished commits are preserved on local
`archive/set74-workflow-scope-815334e0`; the code-only topic push remains a normal
fast-forward from `2b40847b`. Linux execution is pending authentication scope,
not counted as a pass. No broader credential was requested or read.

## Offline operator provisioning

The separate operator-run `slugpool-provision` command creates a dedicated
schema16 wallet, three online signing roles and their matching public Setup.
Its public entry point uses only OS randomness and selected protected files;
it has no seed argument, fixture default or checkpoint-private-key input.
The original capsule and operator-specific 24-word backup stay offline.
Neither backup recreates shares, earnings, pending transactions or the scheduler.
Interrupted setup preserves original identities, opens an existing wallet rather
than replacing it, and refuses missing established files. See
`POOL_OPERATOR_PROVISIONING.md` for the concrete outputs and recovery procedure.

The portable independent reference contains six positive derivations, 61
structured refusals and 18 raw cases. Python 3.14 and 3.9 produced identical
543,515-byte output before Go used those expected values. Test entropy consists
only of these published synthetic capsules; no real key has been generated.

Preserved failed checks found a bounded schedule conversion that needed an
explicit uint32 cast, then an inventory mismatch: the actual wallet basename is
`wallet.WalletDBName` (`slugwallet.db`), not `wallet.db`. The implementation and
current documentation use that existing constant; the frozen reference contract
is preserved with a separate erratum. Subsequent stopped-runtime checks exposed
an incomplete test RPC fixture. Runtime construction requires ownership identity
and settlement tip/activation metadata, with the exact JSON-RPC envelope. The
bounded corrected fixture serves only those constructor reads and grants no
actual chain, mining or Runtime.Run evidence.

The complete normal check passed in 33.079s. The aggregate race attempt reached
its unchanged three-minute limit during encrypted-wallet derivation, after the
create/reopen and all seven interrupted-prefix cases passed. It is preserved as
a timeout, not a pass. Bounded successor groups cover the same race checks;
`provision-attempt7` completed them in 52.816s, 118.203s and 55.153s, each
with the original three-minute bound. All eight provision tests passed under
race. No-CGO refusal (2.699s), cheap-profile rejection before effects (2.830s),
vet (1.303s), build (3.357s) and binary-profile inspection also passed. The
composite normal/race evidence binds identical source files, with 4,595 inputs
unchanged and all process groups joined. The resulting normal provisioner SHA256
is `1073f7138768704c81b7510ab6ade4daf0db8a07684d25a73c91c5d71132a455`.
See `docs/evidence/set74/operator-provision.json` for the exact composite.

## Existing-wallet remote attachment

The explicit remote attachment now borrows the exact established schema14
wallet/account/vault, verifies both original recipient markers before opening
wallet state, attaches the selected private local node once and retains a
separate outbox. Only explicit Start mines or delivers pending work. The
controller reuses concrete signed fixture login and enrolled receipt validation;
it cannot provision a wallet, repair missing vault controls or repin old history.
This is existing-account composition, not the deferred sign-in/onboarding work.

Finite source review corrected three defects before installation: an empty
Merkle branch lost its canonical nonnil representation during copying; a failed
source-audit descriptor close lost its retained owner; and an existing-only
vault open could resume an empty initializer with a missing marker. Successor
source/control tests preserve the predecessor artifacts and old APIs. Original
View/Mining fences, stopped Open/Login, separate owner lifetimes, joined expiry
and cleanup, exact recovery and sticky failed-owner retention remain required.

`remote-attachment-attempt1` passed normal25.722s, race61.726s, no-CGO11.174s,
cheap-refusal9.428s and vet1.105s with4,606 unchanged inputs. The paired controller
passed normal11.553s, race15.035s, no-CGO compilation9.028s and vet0.938s with4,613
unchanged inputs. All process groups joined. The component-bound report is
`docs/evidence/set74/remote-attachment.json`; it does not claim actual remote
wallet/miner composition or native application acceptance.

The subsequent `remote-attachment-actual-attempt1` passed in 13.894 seconds of
runtime after normal builds. It selected the closed Set73 baseline's existing
incrementing256 wallet and a separate TLS OperatorV2 runtime, mined a block
(copied chain height 2 to 3), obtained both an accepted share and fighter receipt,
then stopped and reopened the exact controller/outbox without automatic work.
Closed operator audit matched the original receipts. All 2,967 selected inputs,
the original fixture, and 226 protected copied history/control paths stayed
unchanged; both isolated nodes and all process groups joined without signals.
This proves actual attachment/admission, not native UI, ownership replay or
payout presentation. See `docs/evidence/set74/remote-attachment-actual.json`.

The broader GitHub CLI authorization was canceled when Mike restricted access
to SLUG/taytr-llc and excluded Wandercom/Growmaple. He installed a fine-grained
credential through a hidden local prompt, stored in Keychain for this repository
without changing the global login. Its saved GitHub selection is only `slug`,
with Contents/Workflows write and Actions read. The topic push through
`388546a8` succeeded, activating `.github/workflows/operator-linux.yml`.
This supersedes the earlier authentication-blocked workflow state above.

The first actual GitHub run, `44819586232`, failed workflow validation before
creating a job: `runner.temp` is unavailable in job-level `env`. The successor
moves that expression into step-level environment and the artifact upload input.
Preserve the original failed run; the corrected Linux execution is still pending.

## Remaining integration

- Prepare the concrete operator handoff and deployment selections without
  exposing real private material to the agent or using fixture-default custody.
- Add an explicit remote desktop attachment and OperatorV2 history adapters,
  preserving original local wallet/recipient/recovery source bindings.
- Capacity continuity or an explicit finite-run limit, then remote-host rehearsal.

No real key or seed has been read or created. Existing fixture applications,
production genesis/custody gates and final native/design acceptance remain
separate. Operator host/custody configuration will be made concrete before any
required deployment input is requested.
