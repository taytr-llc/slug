# Set79 explicit public DAILY contract — proposed for review

2026-11-20. Source-only proposal; no new oracle output or product implementation
is authorized by this document. Root must select this coherent byte/API scope
before an independent producer is frozen. Set78 direct-owner acceptance is
separate predecessor evidence, not acceptance of this new public composition. Companion:
`minimum-public-daily-plan.md` and `transport-review.md`.

## Result and compatibility

One fresh headless DAILY operator, a production enrolled-credential owner,
TLS daily client/service, and daily durable worker/outbox compose with the
existing Source2/Round2/indexed ledger2/schema16 backend/daily scheduler. The
miner can obtain work, recover receipts and read its exact dated money/history.
No native route, account login, host deployment or continuous-capacity claim.

Every V1 public constructor, parser, domain, file grammar and route stays strict
and byte-identical. New constructors select mode before effects; neither markers
nor server responses infer it. No existing root/outbox is adopted, repinned,
reset or silently enlarged. Shared private implementation may select one immutable
mode; old APIs refuse daily handles and new APIs refuse V1 handles.

## 1. Policy2, Setup2 and pure bindings

Use the current `ProvisionPolicy` and `Setup` ordered field inventories, changing
only the explicitly selected mode and their component values. Canonical JSON is
compact ASCII, exact field order, no duplicate/unknown/missing fields, no null,
no alternate numeric spelling or trailing bytes. Existing parsers remain V1.

Policy2 field order is `version,scope,request_id,network,genesis,descriptor_id,
checkpoint_public_key,mining,limits,enrollments,schedule,fee,transport,
maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms`. `version=2`;
`scope="phase-a-finite-operator"` remains finite. All nested field order and
non-limit constraints match `provision_config.go`. Daily limits select existing
ceilings: works1..8192, submissions1..16384, pool bytes4096..8GiB,
rounds1..16384, batches1..32767, round bytes4096..8GiB; preserve
MaxRounds>=MaxWorks. Exactly1..64 sorted unique enrollments, derived GymIDs,
public keys and native recipient scripts; no fallback/test enrollment. Keep
actual MainNet genesis, descriptor/DNA, explicit current public checkpoint key,
existing fee choices, UTC minute/retry, maintenance and unlock bounds.

Setup2 field order remains `version,runtime_id,authority,ownership_store_id,pool,
round_limits,settlement,schedule,miners,wallet_guard,node,ingress,
maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms`. Require version2
for Setup, source, round limits, settlement, schedule, miner config and ingress.
Public OperatorProfile2 and its ManifestAuthorityPin5 remain their current
schemas/limits; node RPC/fee Config remains its current version1, selected through
`NewDaily`. Schema16 Guard.Version1/derivation stays exact, with daily source and
settlement ID values; the concrete wallet binding is Version2. Public material
and certificate projections retain Version1 and their unchanged derivation/parser.
Daily fee/source/economic configuration is immutable after construction.

Proposed additive APIs in `desktop/pooloperator`: `ValidateDailyProvisionPolicy`,
`ParseDailyProvisionPolicy`, `DailyProvisionPolicyID`, `ComposeDailyProvisionSetup`,
`ValidateDailySetup`, `MarshalDailySetup`, `ParseDailySetup`, `DailySetupID`, and
`CreateDaily`/`OpenDaily` using the corresponding current argument shapes.
`ParseDailySetup` accepts canonical bytes only; its CLI wrapper may remove exactly
one final LF as the existing command does. `ParseDailyProvisionPolicy` preserves
the existing optional-one-final-LF convention. Caps remain Policy64KiB/Setup256KiB.

Define `H2(label, body)=lowerhex(SHA256(ASCII(label)||00||ASCII("v2")||00||
LE32(len(body))||body))`. New policy ID uses label `SLUG pool provision policy`;
setup ID uses `SLUG pool operator setup`. The seven namespace roles remain
`authority,publication,ownership,wallet,runtime,scheduler,credentials`; each uses
`H2("SLUG pool provision identity", decodehex(request_id)||ASCII(role))`.
No new key generation rule. Source ID is existing `poolshare.DailyConfigID`;
settlement ID is existing `poolsettle.DailyConfigBindingID`; do not rederive
those with H2 or reuse V1 service BindingID.

Add pure `poolschedule.DailySettlementConfigBinding(config,source)(string,error)`
by exposing/reusing the already-selected private daily encoding, not copying a
second implementation. It validates both daily components and hashes the exact
existing `{"config":Config2,"source":Source2}` with the existing v2 framed domain.
It grants no owner authority. `DailySettlementBinding(ctx,*Store)` and the daily
scheduler constructors must still verify the original writable Store and compare
that same digest. Setup preflight uses the pure helper; construction repeats the
concrete checks. This seam is root-approved; V1 binding is unchanged.

The composer requires exact enrollment=destination membership/order/payees,
RoundLimits.MaxRounds=Settlement.MaxRounds and the existing wallet/profile/node
funding agreement. Derive `DailyEnvelope` from the selected full limits/gym set;
never accept a caller-supplied quota. An insufficient reservation refuses before
creation; a returned zero ordinary maximum is explicitly non-enabled/draining,
not promised useful capacity.

Fresh operator markers select `SLUG-POOL-OPERATOR-V2.json` and
`SLUG-POOL-OPERATOR-READY-V2.json`; provision INTENT/READY use their existing names
with `-V2`. Ordered bodies retain their current fields with Version2 and daily
IDs. The protected backup becomes `SLUG-OPERATOR-POOL-WALLET-BACKUP-V2.json`,
Version2, same kind/field order/BIP39 English256/no-extra-pass/native84 convention,
and exact new public guard/IDs. This is a wallet-key backup, never ledger restore.
All private capsule, seed/passphrase/TLS file handling and incomplete-wallet stops
remain unchanged; no secret output/argv, entropy override, online checkpoint seed,
production-genesis grant, missing-file recreation or journal reset. Prefix resume
and READY verification select the original retained files only.

Root selects an explicit `--daily` command selector for the existing headless
operator and provisioner. No selector retains V1. An explicitly supplied false daily mode
must not authorize V2 parsing; mismatched public versions refuse before effects.
This mode selection does not change the canonical byte rules.

## 2. Credentials, client trust and selected routes

`poolminer.CreateDaily/OpenDaily/ValidateDailyConfig/DailyConfigID` preserve current
argument shapes. Config/State version2; same marker filename and ordered Config
and State fields, no schema conversion. Keep64 enrollments/128 current audience
credentials/4096 retained generations, <=30-day issuance lifetime, monotone clock,
exact replay, durable revocation/rotation and original gate/active recheck/Close.
The current enrollment, credential and token-reuse hash recipes use their exact
existing bodies/LE32 framing with domain v2. Do not store token plaintext.
ConfigIDs pin original profile, DailySourceID, DailySettlementConfigID and exact
public-key/GymID/payee relation. Concrete runtime supplies this original owner's
separate pool/admission Sessions; no fixture adapter qualifies production.

Root-selected distinct `poolservice.DailyClientConfig` order:
`version,url,token,ca_certificate,public_key,source,settlement_config_id,recipient_script`.
Version2/Source2; required canonical nonzero64-hex settlement ID and native44-hex
recipient script. Reuse original manifest TLS origin/CA validation, compressed gym
key and bearer bounds. RecipientScript cannot be empty for an enrolled daily
client. The daily transport trust is:

`SHA256("SLUG pool transport trust\x00v2\x00" || manifestTrustID ||
DailySourceID || compressedPublicKey || SettlementConfigID || RecipientScript)`

Here `\x00` denotes a zero byte and all IDs/key/script are their fixed-width
canonical ASCII encodings; no separators, extra LF or length prefix. Token is
excluded, as in the existing manifest trust. Rotation changes credentials, never
this selected trust or outbox identity. Public ownership has its separate profile
pin and gains no authority from this digest.

Add `NewDailyClient(DailyClientConfig)`, `ValidateDailyClientConfig`,
`DailyTransportTrustID`; `NewDailyServer(ctx,ServerConfig,*Pool,*Store,sessions)`
requires ServerConfig.Version2, fully audits original daily owners and exact full
Source2 equality, and retains their pointers. `PrepareDailyWork` and
`SubmissionMatchesDaily` use original extraNonce0/template/gym/source checks.
Only the daily catalog serves `/pool/v2/{issue,work,submit,lookup,balance,payouts}`.
Issue/balance requests stay `{}`, work `{job_id}`, submit/lookup `{job_id,nonce}`.
Every successful daily response binds the selected settlement and recipient,
including work before mining and receipts before acceptance. Work response order
is `version,binding_id,settlement_config_id,recipient_script,work`; submission
response order is `version,binding_id,settlement_config_id,recipient_script,
submission`. Version2/BindingID=DailySourceID, SettlementConfigID and native
RecipientScript must equal the immutable DailyClientConfig. Server values come
from its original audited ledger config and the authenticated exact enrolled gym;
client local getters or a later Balance call are not a substitute for checking
these fields. Missing/wrong fields fail before any work/receipt disclosure. An
invalid success on Issue/Submit remains outcome-unknown; a read fails unavailable.
No error response is promoted into a successful binding handshake. Error envelope
order remains `version,error` with version2 and the existing closed APIError
fields/codes; daily `stale_capture` maps to HTTP409 with OutcomeUnknown=false.
All inherited status mappings stay unchanged. Balance and error envelopes retain
their existing65536-byte cap; payout-page responses retain131072. Embedded ManifestJob.Version2 (carried by admission transport v3),
MiningWork and Submission1 bytes and all duplicate/acceptance semantics are unchanged. Preserve existing work/response
caps; independently include the new fixed-width envelope fields in their margins.
Daily HTTP requests/responses/errors allow outer SP/TAB/CR/LF only (including the
encoder's final LF), with the original total byte cap checked before trimming and
the exact canonical nested body checked afterward. Do not accept other Unicode
whitespace. V1 HTTP and configuration/protected-file framing stay unchanged.

New ingress `NewDaily/ValidateDailyConfig` selects config2/catalog2 only; preserve
exact Host/origin/SPKI/CA, TLS1.3, upstream loopback selection, request/body/deadline
limits, unchanged bearer, no redirects/retries and joined shutdown. Admission v3
and anonymous ownership v2 remain unchanged; admission MaxJobs already supports
8192, so no protocol or cap widening. Never send bearer to ownership.

Underlying operations keep original session captures/final checks. Service must
not hold one account lease around another concrete account-lease-taking call.
Malformed requests fail without effects; authenticated nonmember requests do not
poison/drain the operator. Preserve definite errors versus outcome-unknown writes,
including conservative Issue classification. Unknown Submit/lookup recovery uses
the original job+nonce. Uncertain Issue has no recoverable request identity in the
current `{}` request: the durable hold below prevents automatic replacement
issuance across loops, Stop/Start and process reopen. No idempotent Issue protocol
or latest-job adoption is introduced.

## 3. Daily balances and captured payout pages

Daily balance wire order is `version,binding_id,settlement_config_id,
recipient_script,balance,observed_at_ms`. Its balance order remains
`gym_id,confirmed,available,reserved_atoms,paid_atoms,reason`, but reserved/paid
are **decimal strings**: canonical `0` or nonzero digits with no leading zero,
checked <=MaxInt64. Fractions retain canonical reduced numerator/denominator
strings and original <=4096-character component bounds; no V1 MaxMoney lifetime
ceiling is imposed. Available<=Confirmed-Reserved, amounts nonnegative, and
Confirmed0 requires empty reason. ObservedAtMS is an integer in0..253402300799999, the inherited daily ledger/report
clock bound; timestamp0 means exact empty balances/report. The full int64 bound
applies to cumulative paid/reserved, not to timestamps.
Use the existing closed reason set, including distinct `capacity` and
`transaction-capacity`; unknown reasons refuse. Individual payment amounts still
obey their existing monetary bounds. Every daily balance/page response must
match the client's selected SourceID, SettlementConfigID, recipient and GymID.
The internal Go Authority.Balance projection can retain its existing signature;
wire decoding must validate all selected fields before conversion. Never marshal
that numeric internal projection to a daily JSON/UI boundary.

Proposed `poolsettle.MinerDailyPayoutPage(ctx,sessions,token,request)` and client
`PayoutsPage(ctx,request)` are separate from V1 complete-history APIs. Request
order: `revision,high_batch,before_batch`, all canonical uint64 decimal strings.
`"0","0","0"` requests a new capture. Every other request requires revision>0,
high>0 and 1<=before<=high+1, with checked addition. The server checks high against its selected daily
MaxBatches; a client with only its pinned ConfigID checks the existing global
32767 maximum and exact capture/request consistency. A hash does not reveal
the selected numeric cap. A fresh capture reads original audited header Revision/Batch count;
revision is never0. Effective first before=high+1 (including1 for empty history).
An empty ledger may have a nonzero revision from a prior report operation: its
valid fresh response has high=0, before=1, done=true and next=0. That response is
not a mixed-zero continuation request.

Page response order: `version,binding_id,settlement_config_id,revision,high_batch,
before_batch,next_before,done,recipient_script,balance,observed_at_ms,items`.
The four capture/cursor fields are decimal strings; done is JSON bool, items is
non-null. Each item order is `batch_sequence,payment`; sequence is a decimal
string, payment is the existing ordered PayoutItem with numeric per-transaction
atoms/index/timestamp and unchanged recorded-state grammar. A prepared/broadcast
item can only have batch_sequence==high_batch; being first on an arbitrary page
is insufficient. Item order is strictly descending sequence, with no duplicate
sequence/txid or fabricated output for the selected gym. Returned item Day and
CreatedAtMS are strictly descending as in original payout history; retain each
item's exact UTC day/time and CreatedAtMS<=ObservedAtMS checks. Gaps from filtering
slots do not relax the original single-day or time ordering.

Under the original Store operation gate, capture the original session, perform
full daily/source/namespace/index audit, then inspect at most eight descending
batch **slots** from before-1 to max(1,before-8). Filter the selected gym after
that full audit; emit0..8 payments with exact original output indexes. Do not scan
until eight matches. Preserve initial/final original session checks; no slow work
under a held account lease. A malformed cursor refuses; after full integrity
audit, a changed Revision/HighBatch returns dedicated `ErrStaleCapture`, wire
`stale_capture` with HTTP409/OutcomeUnknown=false and no partial page. New evidence on
unconsumed source tail is not a captured report update or payment-read freshness.

NextBefore is the lowest visited slot; if sequence1 was visited (or no slots),
done=true and next_before="0". Empty matching items otherwise is not EOF. A
read-only arbitrary valid cursor does not certify earlier pages were consumed.
No per-gym persistent index, cross-request snapshot, new revision write or node
observation. A complete non-overlapping unchanged-capture traversal reconciles
paid/reserved totals; a page alone need only have sums <=the full totals. The
balance is the complete dated report projection, not a page sum. Recorded delivery
state is not fresh chain finality. Retain payout-page Request1024/response131072/item256/
balance17408 bounds; prove exact worst ordered frames before freezing a producer.
Eight slots bound returned work/data, not the original full-audit cost.

## 4. Worker, original local signer and runtime composition

Add explicit daily worker/outbox constructors using current public PoolConfig
field order with Version2; its pinned complete TransportTrustID now commits the
money binding above. No new money getter is required on Authority. Daily config
validation uses lower source/gym/origin validation directly, not a dummy legacy
ClientConfig. Constructors require protocol2 and exact source/gym/trust/admission3
agreement. Application/marker/key grammar stays that of the pool outbox;
marker/meta/scan/entry mode becomes2. Marker/scan/entry payload field order remains
unchanged. Daily metadata alone appends the required JSON bool `issue_pending`
after its existing ordered fields: `application,version,config,last_sequence,
records,bytes,pending,reserved,issue_pending`. The V1 metadata codec stays exact,
without that field. Embedded fighter receipt and pool Submission retain their
original modes. Daily reopen requires exact
complete Config, including MaxRecords/MaxBytes; V1's separately permitted limit
increase remains unchanged. No eviction or implicit daily limit increase.

Daily Issue uses the original outbox mutex, synchronous commit and exclusive
running owner. New metadata starts issue_pending=false. Before any Issue call,
validate original owner/context/authority and empty scan, then synchronize
issue_pending=true; no network call follows an uncertain premark write. Mint a
private, nonserializable same-owner/same-live-attempt token only after that sync.
It is not an issuance ID or remote authority and cannot survive Run/reopen. Drop
the outbox mutex before network calls or response validation; reacquire it only
for the bounded completion check/commit, rechecking the same private active
attempt and original owner/context. A successful, fully bound and structurally
validated response is saved as the exact
owned scan+cursor with issue_pending=false in one synchronous LevelDB batch.
Persist even already expired/near-renewal valid work before applying the existing
scan-expiry retirement; do not drop a known response and immediately re-Issue.
If scan storage/validation fails, leave the hold. A failed commit poisons the
original owner as today; never attempt a compensating clear or replacement.

Only the original active attempt may instead synchronize false after an exact
trusted API response that already means definite no effect: `busy` or
`rate_limited`, OutcomeUnknown=false, with no mixed/wrapped uncertainty. Preserve
existing server/client classification; do not relabel a real unknown to exercise
this path. A successfully persisted clear permits the existing bounded backoff;
it is not automatic replay of a possibly committed Issue. Cancellation, timeout,
malformed/wrong-money response, any unknown or other error, and crash before
completion leave true. Original context cancellation cannot clear the hold merely
because the caller stopped waiting. Do not synthesize a JobID.

Full daily outbox audit requires true to coexist with no stored scan/cursor;
retained e/h/p entries and their exact pending/reserved counters remain valid.
The bool is mandatory even when false; it changes no record/byte/pending count,
is included in canonical bounded metadata, and creates no additional DB key.
On reopen or live uncertainty, the worker remains paused with LastError
`issue_unknown`: it can run existing pending-entry reconciliation by exact
job/nonce and original admission identity, but cannot Issue or start a new scan.
The daily attachment deliberately projects that hold to the existing public
`paused`/`outcome_unknown` status, not the V1 sanitizer's generic unavailable;
no new HTTP error or legacy/native status enum is added.
Stop/logout/cancellation still join normally and preserve the flag. There is no
reset/clear/latest-job/alternate-outbox fallback in this slice. A process crash
after premark but before dispatch can therefore hold a request that never left
the process. Resolving that uncertainty is an explicit remaining availability
and broader-readiness gate, not a new manual permission or an idempotency claim.

Root selected the minimal explicit daily attachment around the existing audited
local vault/session/recipient owner. `NewDailyPool` alone is not a production
signer. Add `OpenDailyRemotePoolAttachment` and daily config/credential selectors
beside `desktop/pilot/remote_pool_{config,credentials,credential_clients,attachment,
worker,local}.go`; retain the corresponding current argument shapes and original
OwnedWalletSession/view. The headless controller/launcher must actually select
this adapter; a fixture callback cannot qualify its composition. Preserve current
V1 factories and native/helper routes; do not auto-switch their protocol or JSON.
No new login, wallet creation or auth-flow policy is introduced.

Remote daily config retains the ordered `RemotePoolConfig` fields with Version2,
Source2/Settlement2/Miners2. ExpectedLocal remains Version1/schema14 and binds the
original unchanged wallet allocation/recipient/source markers; never replace its
OriginalSourceBindingID with the remote DailySourceID. Require the same audited
local GymID/public key/native recipient exactly once in the remote enrollment and
destination list. Derive DailyClientConfig's SettlementConfigID from the selected
full settlement and RecipientScript from that original recipient. Attachment ID
uses the existing `SLUG remote pool attachment` framed domain with v2; INTENT/READY
keep their current application/field order/names and exact-final-LF convention,
with marker Version2 and SHA256 of the exact intent bytes. Fresh attachment roots
only; no V1 rewrite. Daily protected local credential record order remains
`version,config_id,gym_id,audience,generation,expires_at_ms,token`, now required
Version2 with the selected daily remote ConfigID, same exact final LF and2048-byte
cap. Both `pool` and `admission` audiences select these daily credential records;
this does not change the embedded admission-v3 record/job/proof versions. Never
read those files in a GUI or expose token/RPC values through argv or status.
Current public config size/path/file/ACL limits are unchanged.

Keep signAdmission private and purpose-bound: require exactly one matching
previously audited daily outbox fighter candidate, original policy/proof/gym,
original Mining lease and vault epoch before signing; recheck original lease/view
before disclosure. Retain the existing-only recipient audit at both fences;
missing READY cannot allocate/repair a recipient. Remote pool/admission bearer
leases are distinct from local Mining authority. Logout/reset/Stop cancels and
joins calls/hashing; failed cleanup retains the original attachment/session.
No generic signer/private-key getter or account capability crosses transport.

Preserve pending-first delivery, durable nonce reservation, block-first broadcast,
separate fighter admission, parent/expiry checks and joined hashing/Close. Worker
stops hashing expired scans; the D80 acceptance scenario uses a previously mined,
explicitly delayed original winner. Do not claim an automatic expired-job worker.

Daily runtime constructs original credential/issuer/source/round/ledger/scheduler/
public owners, selects daily node and schema16 backend, and binds backend once.
It compares complete independent configurations, then attaches Set78's daily
budget using exact sorted destination gyms before any listener can enable intake.
No caller quota or V1 reservation is used. Full Sync stays atomic; explicit live
List/Refresh may optimize maintenance but not financial audit. Capacity stops
ordinary intake while preserving late-winner checks, existing round refresh,
custody and exact pending-payment recovery. Mixed errors remain fatal.

Original scheduler recovery runs first, then real-UTC due dispatch, original
30-second budget/cursor/one-slot semantics. D84 whole-candidate deferral stays
visible; no subset/split/fee change/advance. Constructors and command owners keep
protected inputs and failed original children reachable until joined cleanup;
never replace a failed helper/backend/Store. Public endpoints grant no signing,
operator action, scheduler trigger, raw DB, wallet or token capability.

Finite Source2, ordinary<=4095, round4096 windows, worker4096 records, financial
and signed authority ceilings remain. This can serve a bounded selected pilot,
not unlimited operation. No new product-policy question is needed. Confirmed
funding conflict retains its existing hold; resolution stays a separate policy.
