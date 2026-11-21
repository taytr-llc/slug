# Recoverable daily issuance 1 — proposed byte/state/API contract

This specializes approved plan `7136b5aa` for a fresh, explicitly selected path.
Status: proposed, source-only; root selection and independent reference freeze
precede implementation. All named types below are new unless explicitly called
an inherited type. No old root, byte format, hash domain or economic rule changes.

## 1. Encoding and selection

Objects use the field order printed below. Every field is required, including
empty strings, zero counters and explicit `null`; no `omitempty`. Canonical JSON
is UTF-8, compact, with the existing Go-compatible JSON string escaping. Integers
are canonical unsigned decimal JSON numbers unless named `*_at_ms`, which are
nonnegative int64 values; HTTP counters inherited from DAILY retain their existing
decimal-string encoding. Duplicate/unknown fields, reordering, alternate numeric
spellings, invalid UTF-8 and unexpected arrays/nulls refuse. Inherited objects
retain their independently frozen canonical order and validation, including GBT
field order and all original ManifestJob2/template rules.

`H(label,bytes) = lowercaseHex(SHA256(ASCII(label) || 00 || ASCII("v1") || 00 ||
uint32LE(len(bytes)) || bytes))`. All lengths are checked before conversion.
`sha256(bytes)` below is ordinary lowercase hex, without a domain. IDs/hashes are
64 lowercase hex; IDs must be nonzero. Public keys are the existing canonical
66-hex compressed secp256k1 keys. A request ID is product-generated random32,
not a challenge, authentication token or permission to choose issuance time.

`IssueProfile1 P = {version:1,base_setup_id,source_config_id,max_requests}`.
`ProfileID=H("SLUG pool issue recovery profile",JSON(P))`.
Both IDs name the unchanged, completely validated Base Setup2 and Config2.
`max_requests` MUST equal Base Source.MaxWorks, in 1..8192; it is not a new
operator-adjustable capacity. The four-field maximum encoded size is 202 bytes.
Lower libraries validate shape/source/count; the original operator composition
must also recompute Base Setup2 ID and compare the actual original ledger/source
and enrollment map. A remote client cannot derive Setup2 ID from a partial config;
it pins that public value, as well as its independent source and money binding.

Wrappers, in order:

| New wrapper | Ordered fields |
|---|---|
| RecoverableDailySetup1 | `version:1,base:<Setup2>,issue_profile:P` |
| RecoverableDailyClientConfig1 | `version:1,base:<DailyClientConfig2>,issue_profile:P` |
| RecoverableRemotePoolConfig1 | `version:1,base:<RemotePoolConfig2>,issue_profile:P` |
| RecoverablePoolConfig1 | `version:1,base:<PoolConfig2>,issue_profile:P,settlement_config_id,recipient_script,transport_trust_id` |
| RecoverableDailyCredential1 | `version:1,base:<DailyCredential2>,issue_profile_id:ProfileID` |

Base objects stay byte-identical. In particular, Base PoolConfig keeps its V2
transport trust; the last wrapper field holds the new trust. The selected trust
is `H("SLUG pool issue recovery transport",ASCII(DailyTransportTrustID(base)) ||
ASCII(ProfileID))`. Config IDs for the setup and remote wrappers use H with labels
`SLUG pool issue recovery setup` and `SLUG pool issue recovery attachment`.
Credential tokens are excluded from every journal and public ID as before.

Explicit `--daily-recoverable-issue` selects this wrapper, fresh namespaces and
`/pool/v3/`; it is mutually exclusive with all older mode selections, including
explicit-false conflicting flags where the existing launcher tests presence.
Native UI/onboarding remains out of scope. Embedded source/worker/entry versions
remain 2, so existing DAILY recipient, CaptureDaily and profile guards still run.
Admission-only HTTP v3 and ownership HTTP v2 are unchanged. New constructors
refuse old markers; old constructors refuse new markers. No format sniffing,
limit increases on reopen, migration, ID-less-hold adoption or Config2 repinning.
Legacy NewDailyServer and NewDailyPool must also reject already-open recoverable
Pool/outbox owners; a borrowed embedded Config2 or ProtocolVersion check alone
does not enforce selection. Original round/ledger Base Config2 consumers of the
same concrete source remain permitted and unchanged.

## 2. Fresh on-disk envelopes

Keep current component filenames and DB paths, but require these different envelopes at
the explicitly selected fresh root. Existing extra-file/symlink/ownership/ACL
rules remain. Registry marker files retain their single final LF; pool and worker
markers retain their current no-LF rule. DB values have no LF.

| Location | Ordered envelope |
|---|---|
| registry `SLUG-REGISTRY.json` | `application:"slug-manifest-issue-recovery",version:1,base:<manifestMarker5>,issue_profile:P` |
| registry `meta` | `application:"slug-manifest-issue-recovery",version:1,base:<manifestMetadata5>,issue_profile:P,requests` |
| pool `SLUG-POOL.json` | `application:"slug-pool-issue-recovery",version:1,base:<pool marker2>,issue_profile:P` |
| pool `meta` | `application:"slug-pool-issue-recovery",version:1,base:<pool metadata2>,issue_profile:P,requests,request_bytes,request_reserved` |
| outbox `SLUG-POOL-WORKER.json` | `application:"slug-pool-worker-issue-recovery",version:1,config:<RecoverablePoolConfig1>` |
| outbox `meta` | `application:"slug-pool-worker-issue-recovery",version:1,config:<RecoverablePoolConfig1>,last_sequence,records,bytes,pending,reserved,requests,active_request_id` |

Registry's existing ownership guard2, signed profile and publication codecs are
unchanged; the selected inventory still has the original three files/directories.
New registry marker/metadata caps are twice the old respective 4096-byte caps;
new source marker/metadata caps are twice the old 8192-byte cap. Outbox wrapper
caps are twice its existing marker/metadata cap, with inherited base caps still
enforced before decoding. This permits only bounded wrapper overhead, not larger
templates, jobs, configuration arrays or journals. Wrapper depth is at most two
levels above the corresponding inherited decoder; all inherited array caps stay.
Setup/remote/client/credential wrappers likewise have a cap equal to their old
whole-file cap plus 1024 bytes, and retain the old inner cap/terminator rule.

Original outer mode selection is also durable. New operator files are
`SLUG-POOL-OPERATOR-ISSUE1.json` (Setup wrapper + LF) and
`SLUG-POOL-OPERATOR-READY-ISSUE1.json` containing ordered
`{version:1,runtime_id:<base>,setup_id:<wrapper ID>,issue_profile_id}` + LF.
Provisioning uses corresponding `SLUG-POOL-PROVISION-{INTENT,READY}-ISSUE1.json`:
intent is `{version:1,base:<ProvisionIntent2>,issue_recovery_version:1}` + LF;
ready is `{version:1,base:<ProvisionReady2>,setup_id:<wrapper ID>,issue_profile_id}`
+ LF. The intent fixes selection before entropy/Setup exists; it must not invent
a ProfileID early. The protected backup is a selected wrapper
`{version:1,base:<ProvisionBackup2>,setup_id:<wrapper ID>,issue_profile_id}` at
`SLUG-OPERATOR-POOL-WALLET-BACKUP-ISSUE1.json`, with the same custody rules and no
public-secret output.
Existing inner policy/namespace/wallet-guard IDs stay unchanged.

Remote attachment filenames stay `SLUG-REMOTE-POOL-{INTENT,READY}.json`; new
INTENT order is `version:1,application:"slug-remote-pool-issue-recovery",config_id,
config:<RecoverableRemotePoolConfig1>`. READY order is `version:1,application,
config_id,intent_sha256`, with the same new application. Both have one LF; SHA256
includes INTENT's LF. ConfigID is the new wrapper ID. Credential wrapper Base2
ConfigID must match the old Base RemotePoolConfig2 ID and its outer profile must
match the selected wrapper; the enclosing original attachment retains/checks the
new ConfigID on every call. Old marker/credential parsers cannot accept these
wrappers. Ready caps remain 1024; other new marker caps are old cap+1024.

Do not flatten a new wrapper into old serialized metadata. All full-audit APIs
validate the complete selected namespace; unrelated tampering must still fail.
Open audits legitimate intermediate states only. It performs no repair, network
call, issuance or terminalization. Read-only opens cannot resume. The old direct
`IssueManifestJob`, staged Prepare/Commit, and no-request pool Issue methods must
refuse a selected request-only owner.

## 3. Identity, rows and reverse references

`Intent1 I = {version:1,profile_id,request_id,gym_id,public_key,template_sha256}`.
Gym derives from the original public key/chain. Template SHA is over the exact
canonical original Template JSON, not an RPC formatting variant.
`IntentID=H("SLUG pool issue recovery intent",JSON(I))`.
The profile commits complete Setup2/money selection transitively; transport also
checks the direct settlement ID and recipient. No extra payee authority lives in
poolshare. First source admission derives I under the original live session and
configured node. A caller-supplied digest or decoded row never grants authority.

Keys use literal ASCII prefixes followed by **raw32** decoded IDs:

| Owner/key | Canonical value |
|---|---|
| source `issue/r/` + requestID | `SourceRequest1={version:1,intent:I,intent_id,state,job_id,resolved_at_ms,reason}` |
| source `issue/t/` + requestID | Exact original Template JSON; present for every state except `work` |
| source `issue/j/` + jobID | raw32 requestID; present exactly for source `work` or `issued_unused` |
| issuer `issue/r/` + requestID | `IssuerRequest1={version:1,profile_id,request_id,intent_id,gym_id,public_key,state,job_id,sealed_at_ms,reason}` |
| issuer `issue/j/` + jobID | raw32 requestID; present exactly for issuer `issued` |
| outbox `q` + requestID | `OutboxRequest1={version:1,request:<IssueRequest3>,result:<IssueResponse3 or null>}` |
| selected outbox `scan` | `{version:1,request_id}`; linked to one retained `work` response |
| selected outbox `cursor` | Existing 8-byte unsigned big-endian nonce cursor, 0..2^32 |

`IssueRequest3={request_id}`. The profile/root scopes each journal, so request ID
is globally unique within that selected source; reuse by another gym/key refuses
without disclosing the existing result. Reusing it with a different intent is a
conflict. Exact retries cannot pick another template. Source/issuer request row
caps are 4096 bytes each; Template and ManifestJob caps remain 16MiB and 8192.

Every new reverse row is mandatory and bijective. Registry `issued` joins to the
exact old `job:<hexID>` body; every job on this new owner has one such request.
Source `work` joins to the exact old `w<hexID>` body; every source work has one
request. Template hash and owned fields must agree. `issued_unused` retains its
template and original issuer job. `not_issued` retains its template and an issuer
seal with no job. Unknown keys, dangling/multiple references, incompatible state
pairs and excess counters are corruption. Inverse outbox job linkage is rebuilt
during full audit: every e/ row's exact Work must match one retained completed
request, and two request results cannot claim the same job. No extra local
durable index is needed. Existing e/h/p row bytes, order and nonce dedup stay exact.

## 4. Legal durable states and result bytes

Source state is `pending`, `work`, `issued_unused` or `not_issued`. Issuer state
is `issued` or `sealed`; absence is not a stored row. Only these cross-owner pairs
are legal:

| Source | Issuer | Meaning |
|---|---|---|
| absent | absent | New request may be admitted |
| pending | absent | Owned source intent; no issuer effect yet |
| pending | issued | Issuer/source commit gap; recover exact original job |
| pending | sealed | Seal committed; finish exact source terminal |
| work | issued | Exact work committed and eligible for historical lookup |
| issued_unused | issued | Original job expired before any work publication |
| not_issued | sealed | Original request closed without a job |

Pending source fields: job ID/reason empty, resolved time zero. Work fields: exact
job ID, empty reason, original work commit time with `IssuedAt <= time < ExpiresAt`.
Unused: exact job ID, reason `expired_before_work`, time >= original ExpiresAt.
Not-issued: job ID empty, reason `parent_changed` or `intake_drained`. Its time is
at least the issuer seal time. Issuer issued has exact job ID, zero seal time,
empty reason; sealed has no job, positive seal time and that same terminal reason.
All stored times obey original clock/high-water checks and the unchanged public
maximum 253402300799999. No new caller clock or time source is accepted.

`IssueResult1={version:1,state,intent_id,job_id,resolved_at_ms,reason,work}`.
`IssueResponse3={version:3,binding_id,settlement_config_id,recipient_script,
issue_profile_id,request_id,gym_id,public_key,result:<IssueResult1>}`.
For state `work`, Work is the exact inherited MiningWork object; otherwise it is
explicit null. Terminal states carry the audited exact job ID/digest above,
not claimant work. Their authenticated terminal result proves protocol closure;
it is not a public proof of nonpublication or miner eligibility. The source keeps
full original job/template evidence even though the terminal wire frame is small.
For completed work the client recomputes IntentID from its request/selection,
key/gym and exact template, in addition to all current work validators.

Read-only Lookup may return `absent` (empty IDs/reason, zero time, null work) or
`pending` (exact IntentID, other fields empty/zero/null); Resume success returns
only `work`, `issued_unused` or `not_issued`. Neither absent nor pending can clear
the local intent. Terminal responses always echo exact request/profile/source/
money binding, including after credential rotation. No server error clears the
new local intent, including definite Busy or rate limiting.
Every response also checks GymID and public key against the original selected
client, including terminal/absent/pending replies that have no embedded Work.
Sharing a payee script never makes a different authenticated gym acceptable.

## 5. Commit sequence, seals and uncertainty

1. Product worker synchronizes one new OutboxRequest with null result and active
   ID, after complete capacity reservation. Its ID survives every retry/reopen.
2. Source checks the request first, then for a new ID validates original live
   session, membership/intake and all source/issuer headroom. It owns/validates one
   template and synchronizes source intent/template/reservation before issuance.
3. Registry resolves exact request **before entropy/time**. New commit stores
   request + old job body + reverse row + original metadata atomically. Same-ID
   replay returns exact old job; it does not sample a new birth/challenge. Final
   original-session check and existing TryLock discipline remain mandatory.
4. Source validates original job/template with existing preparation, Proposal,
   current-context and expiry rules. One sync stores old work + work reverse +
   final request and converts reserved bytes. It may delete `issue/t/` only in
   that same batch, because identical template bytes now reside in the work row.
   Work is never disclosed before this commit.
5. Worker validates full result outside its mutex; under the original attempt
   fence, one sync stores the exact response, clears active ID, and, for work,
   writes scan reference + zero cursor. Retained result owns the Work permanently;
   clearing an expired scan removes only scan/cursor, never request evidence.

After issuer commit, an expired original job with **no source work/index** can
become `issued_unused` under the original session/clock checks. It remains charged
as a job and request. A completed work row always wins; inconsistency is corrupt.
Do not turn an unknown write into this terminal state before close/reopen/audit.

Before any job, source may obtain a positive current-parent mismatch by resolving
and validating a fresh template under the same policy and rechecking its context;
discard that new template. Generic node errors never prove mismatch. Alternatively
the original daily controller's audited permanent `draining` state permits
`intake_drained`; paused/not-ready/Busy does not. Under the original pool gate,
atomically seal the issuer request first, with original-session/clock checks. An
already issued mapping returns that original job instead; it cannot be sealed.
Then finalize source `not_issued`. Any delayed commit observes the seal. A crash
between these commits leaves the legal pending/sealed pair above.

An original token/epoch revoked during an attempt cannot be replaced. After
joined Stop/reopen and legitimate new authentication for the same exact gym,
the new owner may resolve the same durable request. Completed history does not
renew session authorization. Unknown storage outcomes poison the original owner;
reopen/audit, never an in-place blind retry or substitute journal.

## 6. Counts and byte reservations

All arithmetic is checked before allocation/addition/subtraction. Recompute
counts/bytes/indexes from full journals before classifying a limit; malformed
history must not become a convenient capacity result. No counter resets or
history deletion. Source RequestCount counts every source request, including
terminal requests. Registry RequestCount counts issued plus sealed mappings:
`Works <= Jobs <= RegistryRequests <= SourceRequests <= P.max_requests`.
Pending/absent issuer rows explain the only allowed count difference.
During full source census, count issuer rows actually matched to source requests:
matched total must equal the independently audited RegistryRequests and matched
issued must equal Jobs. The original registry's full selected request/job/reverse
audit supplies those counts under its gate; parsed metadata alone is insufficient.
Thus an unrelated valid issuer seal cannot replace a missing mapping for another
source intent while preserving only the numeric inequality.

Let T be exact canonical template length, J=8192, Q=4096, and K=40 (each new
`issue/x/` key). An original saved work is at most `W(T)=T+J+20` bytes, still
within the old work cap. Source request extra bytes count **keys plus values** of
all issue/ rows, while Base.Bytes retains its old work/submission payload meaning.
For each pending request, reserve
`max(0, (K+Q + K+32 + W(T)) - (K+len(requestRow) + K+T))`.
This covers maximal completed work/request/reverse growth; unused and sealed
forms are smaller. Terminal requests reserve zero. Source effective occupancy is
`Base.Bytes + request_bytes + request_reserved`. Every selected write/headroom
check uses it, plus the unchanged ordinary/canonical receipt holdback. No pending
intent may consume bytes reserved for accepted evidence or a late canonical row.

Issuer progress also needs reserved space: a mere preflight before a concurrent
publication is insufficient. At fresh construction and **every** selected
registry/publication commit, add this holdback to the existing public reservations:

`(P.max_requests - RegistryRequests) * (K+Q + K+32 + 68+J)`
`+ (8192-len(manifestMetaWrapper)) + (8192-len(publicMeta))`.

Here 68 is the unchanged `job:` + hex64 key; the metadata terms reserve only their
bounded future growth. Actual namespace bytes, this holdback and the existing
unsigned-admission reserve must fit the unchanged signed LogicalBytes ceiling.
Never replace the old reserve with this one. Consuming a request slot releases
one worst-case slot while atomically charging its exact rows; sealed requests
never become jobs later. This can refuse/drain earlier, not enlarge signed limits.
Initial profiles that do not fit refuse before new roots/issuer effects.

Outbox RequestCount counts all q rows; Records/LastSequence retain e-row meaning.
Require `Records+Requests <= Base.MaxRecords`, `Requests<=P.max_requests` and exact
config equality. Bytes count all q/e/scan/cursor payloads; Reserved includes the
unchanged pending-entry reserve and active request's future growth. Before a new
request require `Records+Requests+2<=MaxRecords` (request plus first found row),
and bytes for worst bounded IssueResponse/request row, scan reference/cursor and
one old `poolMaxEntrySize` outcome. Precisely, with a 64-hex request ID, let
`Bq=len(JSON(OutboxRequest with result:null))-4+MaxWorkResponseSize`,
`Bs=len(JSON(scan reference))+8`, and `E=poolMaxEntrySize` (40MiB). The active
intent's extra byte reserve is `Bq-currentQPayloadBytes+Bs+E`; these are conservative
codec bounds, not a claim that all component maxima are simultaneously attainable.
On validated work, convert to actual retained bytes plus the existing precise
`poolReservationEntryBytes` for that original work. A live scan with no retained
e-row for its job reserves that first outcome's bytes and one record slot; after
the first found row its existing pending-row reserve takes over. Clearing the scan
or receiving a terminal releases only that unused first-outcome reservation.
Recompute this from the exact scan/request/e linkage on reopen. Update
fits/reserve/found/Run-full consistently; a request must not crowd out the first
synced result after hashing. Exact duplicate work/result recovery consumes no
second slot. Further mining still checks the full existing capacity each nonce.

## 7. Public API, transport and original owners

Proposed signatures (names/types frozen by this proposal, bodies not authorized):

```text
registry.IssueProfileID(IssueProfile) (string,error)
registry.CreateRecoverableOperatorManifestAuthority(ctx,path,descriptor,OperatorProfile,IssueProfile,*Owner) (*ManifestStore,error)
registry.OpenRecoverableOperatorManifestAuthority(ctx,path,OperatorProfile,IssueProfile,*Owner) (*ManifestStore,error)
registry.OpenRecoverableOperatorManifestAuthorityAudit(ctx,path,OperatorProfile,IssueProfile) (*ManifestStore,error)
(*ManifestStore).AuditIssueBinding(ctx,expectedPin,expectedProfile) (ManifestAuthorityInfo,uint64,error)
(*ManifestStore).TryIssueRequest(ctx,requestID) (IssuerRequest,error)
(*ManifestStore).PrepareIssueRequest(ctx,Intent,prospect.Context) (*PreparedIssueRequest,error)
(*ManifestStore).CommitIssueRequest(*PreparedIssueRequest,check func()error) (IssuerRequest,ManifestJob,error)
(*ManifestStore).SealIssueRequest(ctx,Intent,reason,check func()error) (IssuerRequest,ManifestJob,error)
poolshare.CreateRecoverableDaily(ctx,path,Config2,IssueProfile,*ManifestStore,Node,SessionAuthorizer) (*Pool,error)
poolshare.OpenRecoverableDaily(ctx,path,Config2,IssueProfile,*ManifestStore,Node,SessionAuthorizer) (*Pool,error)
poolshare.OpenRecoverableDailyAudit(ctx,path,Config2,IssueProfile,*ManifestStore) (*Pool,error)
(*Pool).RecoverableDailyBinding(ctx) (Config,IssueProfile,error)
(*Pool).ResumeIssue(ctx,token,requestID) (IssueResult,error)
(*Pool).LookupIssue(ctx,token,requestID) (IssueResult,error)
poolservice.NewRecoverableDailyClient(RecoverableDailyClientConfig) (*Client,error)
poolservice.NewRecoverableDailyServer(ctx,ServerConfig,*Pool,*Store,SessionAuthorizer,IssueProfile) (*Server,error)
(*Client).ResumeIssue(ctx,requestID) (IssueResponse,error)
(*Client).LookupIssue(ctx,requestID) (IssueResponse,error)
```

Intent/Profile/Result scalar types belong in `registry` (no import of poolshare);
the work-bearing public IssueResult belongs in poolshare. Registry low-level calls
have the same trusted-coordinator boundary as existing staged issuance: only the
original source coordinator derives Intent and context; parsing JSON is not a
public issuance entrypoint. Prepared handles are opaque, store/profile/intent/
context bound and process-local; check only rechecks the already-held original
lease. Seal is no-network/no-entropy and TryLock; clock plus seal commit is atomic.
Returned Job is zero for a sealed result. No caller may inject a claimed job,
challenge, birth, cached verification or replacement signer.

New server construction takes the same concrete Pool, Store and session owners
as NewDailyServer plus the explicitly expected profile; ServerConfig.Version is3.
Its original operator caller must validate the complete Base Setup2/profile
relationship (poolservice does not import the desktop Setup type). The server
compares full original Pool and DailySchedulerBinding source, derives the exact
ledger config/recipient map and checks the original source profile with
RecoverableDailyBinding before listening. Source construction and full census
use AuditIssueBinding on the same original issuer. These are bounded live checks
with complete selected audits and expected-metadata comparison, never metadata
adoption or a parsed profile standing in for original owner identity. DailyBinding
remains unchanged for the same original ledger's Base Config2 check.
New ingress wraps its unchanged
Config2 and selects only the pool/v3 catalog; other routes remain exact. New
worker construction keeps the concrete selected client/credential lease and the
same Node, Signer, admission owner and original outbox; no generic arbitrary
callback or exported credential/source getter is added. Operator/pilot constructors
perform these checks before publishing their exact child/view; Start stays explicit.

POST `/pool/v3/issue` = Resume; POST `/pool/v3/issue-lookup` = Lookup. Both require
IssueRequest3, original bearer/session, request size <=1024 and the current issue/
read rate bucket respectively. All other existing pool routes keep their request
and payload schemas but add required `issue_profile_id` immediately after the
recipient field and set outer Version3. Payout page adds it immediately after
settlement_config_id (there is no common recipient position before its cursors).
Work/result response cap stays `16MiB+64KiB`, ordinary/error <=65536, payout page
<=131072, cap checked before trimming only outer SP/TAB/CR/LF. Response LF remains.
Error codes/status/unknown flags stay current DAILY; no error means safe intent
clear. Unknown after any durable effect retains the exact ID. A source/issuer
conflict remains a fatal refusal, not an automatic new-request instruction.

Each Resume/Lookup preserves current 30-second per-operation bounds and original
parent cancellation. No lifecycle/outbox mutex or account lease spans network
I/O; the original bounded source gate still serializes node calls. Network/full
result validation occurs outside the outbox mutex. Its RAM attempt binds original
worker, outbox, parent context, complete wrapper and durable request ID; all are
rechecked before sync/publication. Busy/rate/unknown recovery may pace later
bounded attempts on that same ID, never renew one attempt's deadline. Stop joins
calls/hash work before outbox/credential/Mining release; Close rechecks original
closing/poison state and retains failed owners. Lookup/Open are read-only. Old
unidentified Version2 holds remain untouched.
Only an explicitly started worker with a live original Run/session may schedule
later resumes. Cancellation/Stop/Open/Snapshot does not start a background retry.

## 8. Acceptance and selection boundary

The companion REFERENCE_PLAN lists exact inputs and independent derivations.
Required controls cover every legal/crash state, illegal reverse references,
cross-mode refusal, positive terminal evidence, concurrent/delayed same-ID calls,
full byte/count boundaries, original-session cancellation/revocation, poisoned
writes/reopen, and old completed work/receipts/payout bytes unchanged. A normal
TLS/headless fresh-process proof must use the actual original issuer/source
commit gap, not a fake HTTP response cache. Full financial import/signing audit,
D80 canonical-only late credit and D84 whole-payment deferral remain unchanged.

No new user/economic decision is identified. The derived registry holdback and
local retained-request accounting are explicit engineering selections for root
review; they reduce finite available capacity and do not solve continuous use.
Root must approve this exact contract before producer or Go authoring.
