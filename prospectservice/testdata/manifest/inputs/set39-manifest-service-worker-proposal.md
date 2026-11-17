# Set 39 proposal: candidate manifest service and durable automatic worker

Status: engineering proposal for root review; no runtime or oracle has been created for this set. This combines service v3 and an explicitly separate worker/outbox format into one useful milestone. It does not activate an official fighter generator, migrate existing receipts, change the desktop default, or establish current ownership.

## Recommended deliverable

An explicitly configured local candidate authority issues Job2 over HTTP, an automatic worker builds the committed candidate, mines normal production Argon work, broadcasts block winners immediately, and retains a fully checked schema-5 receipt with its immutable manifest/portrait digests. After process loss, expiry or node outage, the same public outbox can recover the original receipt through the same pinned authority. A normal-PoW executable acceptance run should show this entire path, including an enrolled account's held mining session. No webview/card/roster work is necessary to make the engineering milestone concrete.

Implement service and worker together. Shipping only the wire DTOs would leave the durable mining path unusable; shipping the worker separately would prolong two partially connected formats. This is one bounded extension of established interfaces, not a new account, custody, consensus or generation system. A small explicit CLI path may expose the runnable fixture composition if it can reuse current configuration and lifecycle code without adding key handling; that glue must not become a new generic signer or credential-provisioning feature.

## Inputs and constraints already fixed

The approved set36 contract is `manifest-job-contract.md`, SHA256 `6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66`. Set37 is `manifest-admission-contract.md`, SHA256 `d51178db1decf13ad09986802bb3eb7861827d372a7f35e99c13adc739420f27`. The one supported descriptor is candidate-only; the production catalog remains empty. It binds static DNA/manifest/portrait selection, not condition, XP, fight rules, roster or ownership.

Current source boundaries reviewed: `prospectservice/{types,config,json,client,server}.go`, `prospectworker/{types,config,validation,outbox,worker}.go`, `gymaccount/sessions.go`, and `desktop/pilot/account.go`. The current service has protocols 1/2 and schema 3/4; its worker has config/outbox version 1, Job1 and service2 receipts. The current authenticated service already uses account-before-registry leases, and the worker already keeps separate block/admission outcomes, found-before-send durability, bounded retries and exact-credential replacement outside the journal. Preserve those behaviors.

D68 is fixed: durable issue time is birth; the original five-minute interval has no grace; the final authority clock check follows proof work, static generation/rendering and the node/authorization guard. D69 is fixed: broadcast a durable block win before signing/admission; `submitblock` success is not canonical proof; only the existing exact canonical-winner exception can admit after the original parent advances, within the original interval. D72 is fixed: pending reset and old generations cannot use mining authority; after completion and required same-wallet recovery, a new mining session may mine during the 24-hour hold. Recovery export and other held capabilities remain blocked. No new policy question is required for this slice.

## Compatibility boundary

Keep existing exported DTOs, parsers, constructors, route versions and bytes unchanged. Old service config accepts only 1/2; old worker/outbox constructors accept only 1. They must reject the new types rather than drop fields. New constructors reject old stores and outboxes. No optional manifest fields on old receipts, no union Job that silently accepts either type, no automatic store upgrade, no re-rendered enrichment of schema 1–4 history.

Use explicit new types in the existing packages, with private helpers reused only where that preserves old validation and durable behavior. Do not rewrite the old worker into a broad abstraction merely to remove duplicate code in this set. Shared value-only transport/durability routines are reasonable; schema-specific validation and typed records remain separate. Candidate block DTO/version 1, proof/auth version 2, ProspectID formula and node RPC interfaces remain unchanged.

## Proposed service API and wire contract

Add `prospectservice.Version3 = 3` and these separate DTOs. Field order below is the canonical encoder order; all fields are required.

- `ManifestServerConfig`: `version`, `listen`, `authority_pin`, `clients`, `max_jobs`, `max_records`. `authority_pin` is the complete set37 ManifestAuthorityPin, whose own version is 5. Static credentials retain existing Credential shape; session mode requires an empty clients array and a nonnil authorizer, with no static fallback.
- `ManifestClientConfig`: `version`, `url`, `token`, `ca_certificate`, `authority_pin`. The service version is 3; the nested pin version is 5. No token is copied into a worker journal.
- `ManifestAdmissionResult`: `version`, `context_source`, `current_node_checked`, `authentication`, `birth_rule`, `inserted`, `record`. Record is the required-value schema-5 ManifestRecord. Labels remain `gym-key-control`, `job-issuance`, and either `trusted-node-snapshot` for a new admission or `historical-receipt` for recovery. This DTO describes candidate evidence, never official issuance or current ownership.

Proposed public signatures:

```go
NewManifestServer(ManifestServerConfig, prospectnode.Policy, ManifestNode,
    *registry.ManifestStore) (*ManifestServer, error)
NewManifestSessionServer(ManifestServerConfig, prospectnode.Policy, ManifestNode,
    *registry.ManifestStore, SessionAuthorizer) (*ManifestServer, error)
NewManifestClient(ManifestClientConfig, prospectnode.Policy) (*ManifestClient, error)

(*ManifestClient).ProtocolVersion() uint32 // 3
(*ManifestClient).AuthorityOrigin() string
(*ManifestClient).AuthorityPin() registry.ManifestAuthorityPin // owned value
(*ManifestClient).IssueManifestJob(context.Context, string) (prospectjob.ManifestJob, error)
(*ManifestClient).SubmitManifest(context.Context, prospectjob.ManifestJob,
    prospect.Proof, gymauth.Authorization) (ManifestAdmissionResult, error)
(*ManifestClient).Close()
(*ManifestServer).Run(context.Context, func(string)) error
```

`ManifestNode` needs only Resolve, RecheckContext and RecheckAdmission with existing signatures; RecheckAdmission receives only the registry's internally verified result. Reuse the existing SessionAuthorizer contract and lifecycle discipline. Provide separate strict config parsers/validators; do not expand the old accepted versions.

Expose only `POST /v3/jobs` and `POST /v3/admissions`. Requests remain the existing exact shapes: `{public_key}` and `{proof,authorization}`. Clients cannot submit a job, pin, descriptor, birth or Verified object. The job response is a raw typed Job2; the admission response is the version-3 wrapper above. Wrong-version routes, methods, query strings, duplicate/unknown fields and old/new shape substitutions fail. No metadata-discovery, receipt import, unauthenticated enumeration or SVG endpoint is needed.

Transport JSON may vary whitespace, field order and escaped key spelling, as old transport does; decoded fields/values must meet the exact nested schemas. Reject duplicate decoded keys, case aliases, unknown/missing/null fields, malformed numbers, excess depth and trailing data before expensive work. Receipt fields that schema5 stores canonically must arrive with canonical values; formatting tolerance is not a license to normalize an arbitrary receipt into evidence. Each nested Job2 is validated with its new parser and exact expected pin. Keep request 65,536 bytes, response 73,728 bytes, config 65,536 bytes and response-header 8,192 bytes. The 65,536-byte receipt cap fits the bounded wrapper; test the actual maximum valid proof. A response nested-depth cap of 5 covers wrapper→record→Verified→DNA→Attributes. Do not accept arbitrary additional arrays or markup.

Retain literal-loopback-only HTTP, verified HTTPS/CA behavior, no redirects/proxies/compression/retries, fixed timeout bounds, exact Host checks, one nonblocking active authority operation and existing rate/resource limits. Service construction performs no network/DB initialization; compare writable store Info pin and descriptor to explicit config, policy chain/DNA and compiled candidate support before listening. The schema5 authority namespace is different from a gymaccount authority namespace; never compare or substitute those IDs just because both are 32-byte hex.

## Trust and verification at the client

Trust comes from the configured authenticated transport endpoint plus the explicit full authority pin. The pin is not an issuer signature. Do not discover it from the first job/response or silently adopt Inspect output. An operator/composition can use set37 unsigned inspection to recover a lost create acknowledgment, then make an explicit trusted configuration choice before Open/client construction. Origin/CA and registry pin are complementary: a self-consistent pin does not authenticate a hostile endpoint.

On issuance, validate Job2 support, full ID/preimage reconstruction, descriptor DNA linkage, exact authority/descriptor/network/genesis pin, requested public key/gym and current configured policy. A client clock can reject work that is not locally usable; it does not establish birth or extend eligibility.

On submission, own bounded inputs before waits, validate expected Job2/pin plus proof/auth version 2 and gym/key/job linkage, then send only proof/auth. On response check every wrapper and schema5 field, exact expected Job2/context/proof, first authorization validity (an equivalent different valid low-S signature is allowed on duplicate), positive sequence, D68 birth/acceptance bounds, complete Verified equality from a fresh normal `prospect.Verify`, D69 eligibility binding, and the exact descriptor-derived manifest and portrait digests. Derive from verified display-order WorkHash and compare full DNA. Never trust server-supplied WorkHash/DNA/digests as a shortcut, accept a caller projector/Verified value, or register an alternate descriptor for test convenience.

Any malformed, substituted or canceled response after sending is outcome-unknown, with zero returned receipt: the server may have committed. A valid historical response does not refresh node eligibility, birth or descriptor. New and historical responses retain the current exact labels (`inserted == current_node_checked`); a canonical-winner receipt is distinguished inside Eligibility, not by claiming submit success proves canonicality. Client validation does not authenticate the authority's wall clock or its historical node observation beyond the trusted service/storage boundary. No public JSON-to-admitted-record helper is introduced.

## Service commit and account lease ordering

Under the one-operation permit, authenticate a static credential or an enrolled mining session and bind its gym key to the request. Session mode holds the existing account lease through the registry operation; the only lock order is account→registry. Resolve a fresh context for issuance; perform current-context and authorization rechecks at the final issuance guard. Authority/descriptor/time/challenge come from the store.

Admission calls only RecordManifestJobChecked. Its callback checks new-record capacity and current policy, calls RecheckAdmission with internally verified work, builds the existing rule-2 Eligibility and checks the same session/ctx immediately before returning. The registry then performs its final D68 clock/commit path. Historical duplicates bypass that callback, so perform a live same-session recheck before disclosing the original result; do not turn bypass of node/expiry checks into bypass of account authorization. Retain the existing serialized authorization linearization: reset/logout cannot interleave through the held account gate. No new cross-store transaction or generic callbacks that release secrets are required.

Outage, reset, canceled request, corrupt store and write uncertainty must remain distinguishable through sanitized stable codes. Uncertain writes poison the service/store path and return outcome_unknown; close/reopen and replay identical evidence. Never silently reconstruct missing authority state or retry a mutation inside the transport.

A pending reset cannot issue/admit or disclose through an old mining token, even with a previously valid gym signature. A new post-recovery mining token for the same enrolled gym may recover retained work under the existing rules. Hold does not authorize seed access: existing vault signing uses the gym envelope only; this set must not unwrap recovery material to drive mining. The journal is public evidence and holds no sessions, provider assertions, private keys or recovery bytes. Account lifecycle code remains responsible for canceling its owned worker on logout; denial at the authority is the commit boundary, not a claim that already prepared public block hashing/broadcast can be revoked retroactively.

## Proposed typed worker and outbox

Add a separate `ManifestConfig` with exact fields `version`, `policy`, `public_key`, `payout_address`, `authority_origin`, `authority_pin`, `max_records`, `max_bytes`; version is 2. Add `ManifestAuthority` with ProtocolVersion/AuthorityOrigin/AuthorityPin and the two typed client methods above. Add `NewManifest(config, authority, node Node, signer Signer, outbox *ManifestOutbox) (*ManifestWorker,error)`. Reuse Run/Status semantics and existing Signer type: proof2 already commits Job2's opaque ID, so wallet APIs do not need arbitrary descriptor/key/path signing.

`CreateManifestOutbox(path, config)` and `OpenManifestOutbox(path, config)` return the separate handle. Keep `SLUG-WORKER.json` and the existing application identity with explicit version 2, at a new directory. Marker and DB metadata pin the same canonical ManifestConfig; jobs and receipts pin its full authority tuple. Old OpenOutbox rejects this version and new Open rejects version1, even if the chain/gym happen to match. No migration, generic tagged union or optional admission enrichment.

ManifestEntry version2 has the same ordered state fields as Entry1, but Job is ManifestJob and Receipt is *ManifestAdmissionResult. ManifestScan version2 likewise has ManifestJob; Template and Candidate formats remain unchanged. Retain the separate scan cursor, sequence/hash/pending indexes and atomic transitions. A capacity increase can be allowed exactly as current Open permits; no identity/origin/pin/payout/policy change is an increase. Reconnection with a fresh credential uses a new client adapter with the same origin/pin/key, never edits durable evidence. Authority port changes still require deliberate namespace planning, not automatic origin rewrite.

Preserve current bounds: 12 MiB per entry, 32 MiB per scan and public list page, 128 KiB metadata, 1..100 entries per cheap outbox list, MaxRecords 1..1,000,000 and MaxBytes 64 MiB..1 TiB. Exact canonical persisted encodings reject duplicate/unknown/null/version substitutions, mismatched pins, impossible state/index/count relationships and overflows. This is distinct from registry's at-most-eight full proof verifications per trusted page. No full history rehash or SVG persistence during Open/List.

Before hashing, reserve room for a worst-case public found entry plus future authorization, maximum manifest service response and bounded state/error growth. Store the immutable scan durably before nonce attempts. A found record and scan advancement commit together with Sync before broadcast, signer, service request or acknowledgment. Cancellation during a noninterruptible hash still drains and stores found evidence. Unknown writes poison; no retry or deletion disguises uncertainty. Retain accepted and terminal evidence; capacity pauses mining instead of evicting it.

Opening validates complete jobs, candidate payout/commitment, receipt schema/pin, cheap expected fields/static binding and all indexes. It does not prove disk WorkHash or recreate transport provenance. Label cached receipts as retained conditional authority evidence, not newly admitted or freshly node-verified. Pending work is rehashed before any action. The explicit trusted client path fully validates returned evidence; the worker must also validate its authority adapter's result against the exact local work and all receipt/binding fields before changing state to accepted. Preserve the current independent worker proof verification rather than introduce a public verified-result bypass. Sequential redundant validation is an acceptable initial cost; optimize only with a separately justified private exact-evidence rule. Do not overlap rehash/mining arenas in one worker.

## Mining, retry and recovery behavior

The preparation path validates pinned Job2, obtains one consistent GBT template, calls BuildManifest and Proposal, checks local usability before/after preparation, then persists scan. Exact node version/transactions/witness and payout rules are unchanged. Unsupported descriptor, wrong authority/chain/key or Job1 fails before building/hash/broadcast.

Retain nonce/extranonce exhaustion behavior, early job renewal and parent refresh. Renewal is a new job for future scans only; previously found work never changes job/challenge/authority/descriptor/birth. One due recovery attempt per scan cycle prevents an unresolved old block from starving fresh work. A new block winner jumps ahead of old pending signers after its durable found write.

Track block and prospect delivery independently. Block-only winners are broadcast and canonically reconciled even when they miss the prospect target. A block/prospect winner broadcasts before signing; submission acknowledgment stays separate from CanonicalBlock. Admission then uses the original Job2 and unchanged D69 exception. Stale nonwinners, side branches or arbitrary RPC failure cannot get that exception. Persist the signer result before service submission; preserve the first accepted receipt and original authorization evidence after equivalent retry signatures.

On transport loss, retain exact proof/auth and retry through the same origin/pin. Do not use local expiry to discard pending proof: a prior authority commit may be recoverable after expiry. Only a definite authority refusal can terminate new admission; a malformed response remains outcome-unknown. Historical duplicate recovery does not require a current node or current policy, but does require a live authorized same-gym session and healthy pinned authority. Client/outbox checks cannot change the original receipt's static binding or accepted time. A valid new policy or descriptor requires a deliberately separate new authority/outbox namespace, not a rewrite of pending history.

## Acceptance and ownership plan

Implement in parallel after exact review approval: one owner for new service/config/client files; one for typed worker/outbox and compatibility; one for independent transport/receipt mutations and real crash tests; root owns the final isolated composition/CLI glue and release review. Keep old public APIs and golden bytes in regression scope throughout. Independent expectations can reuse the frozen36/37 corpus byte-for-byte, wrapping it with independently specified v3 envelopes; four extra behavioral nonce cases are not independent goldens. No new production descriptor or private test hasher registration.

Required tests include:

1. Strict v3 config/routes/requests/responses; every pin/job/descriptor/birth/eligibility/static digest substitution; largest valid proof and exact size/depth boundaries; wrong-version and unsupported-catalog refusal before filesystem/network/hash where applicable. Cheap build refuses all new authority/client/worker paths.
2. Existing v1/v2 service, old schema1–4 registry and worker/outbox1 fixtures remain unchanged. Cross-format opens fail; missing or mixed new marker/DB pin cannot fall back. Session mode never falls back to fixture bearer credentials.
3. Real normal-PoW worker→v3→schema5 acceptance using two published wallets and two isolated nodes. Assert exact complete static bindings from independent oracles, both D69 sources, witnessed candidate, block-only winner, stale/side-chain refusal, authority expiry during guard/generation, and same receipt/birth after restart/outage/lost ACK.
4. Process-kill/fault boundaries at scan, found-before-broadcast, authorization-before-submit, authority commit-before-response and outbox receipt-before-ACK; commit-then-error poisoning; replay with no duplicated sequence or changed descriptor; capacity reservation including the larger receipt and no silent eviction.
5. Reset/logout/session-expiry races at the final lease; old generation cannot create or disclose a receipt; pending reset blocks; new same-wallet mining session during hold succeeds through scoped gym signing, while export/rewrap remains denied without recovery unwrap. No cross-account outbox/signer/client fallback.
6. Honest resource/lifecycle behavior: one authority operation at a time, bounded queued work, sequential proof checks, Close waits actual hashing/storage and owned transports, source-frozen normal/race/tagged checks and no fixture processes/listeners left behind.

The acceptance artifact is a reproducible opted-in candidate pipeline and retained receipt, not a new friends/tester release. No default desktop mode switch is included. If a later UI displays it, it must use verified/retained provenance deliberately and render the pinned static tuple; current condition and roster authority remain separate.

## Decisions versus engineering work

Root can approve these protocol/type/format choices as engineering. They neither choose an official bundle nor alter fighter policy. No need to re-ask the user about D68 expiry, D69 broadcast order, D72 hold, hosted recovery or 24-word backups. No operator resources, real provider, real key or production deployment needs provisioning for the fixture acceptance.

Later gates remain genuinely separate: official generator/authority activation and production target calibration; final production chain/profile/provider operational choices; authenticated portable receipt publication; ownership/transfer/roster authority; and the D74 joint design review before friends/tester/public product release. A locally pinned HTTPS service and a valid schema5 JSON receipt do not by themselves settle any of those gates.
