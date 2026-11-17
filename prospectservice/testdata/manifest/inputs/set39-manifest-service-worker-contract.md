# Set 39 exact engineering appendix

Draft for root and independent review. No set39 runtime or expected-value corpus is authorized by this document alone. This appendix makes the proposal `/Users/taytrrip/slug/set39-manifest-service-worker-proposal.md` (SHA256 `defe604b0933644eed6c68a5df8ac434380be821eea1673164f163f371e76d5a`) precise. It supersedes that proposal's optional client-clock wording, incomplete CA binding and overly broad new-authority-for-policy-change wording. All other scope, D68/D69/D72, candidate-only and old-format compatibility constraints remain.

## Ownership and implementation boundary

New `prospectworker/manifest*.go` and distinct tests belong to the worker owner. New `prospectservice/manifest*.go` and distinct tests belong to the service owner. Old worker/service files, public types, accepted versions and fixtures remain unchanged. Reuse existing value-only private helpers where signatures fit; do not modify them to silently accept new schemas. Independent fixture authoring starts only after contract approval; copied set36/37 expectations remain byte-for-byte original. Root owns opted-in acceptance composition and release review. No wallet/node/registry runtime change, default desktop switch or public listener is required.

The worker is feasible as an isolated implementation: existing Node, Signer, Status, candidate v1, proof v2/auth v2, public error sentinels and value-only helpers can be reused; job, authority, config, scan, entry, receipt, persistence and validation types remain manifest-specific. This avoids a generic old/new union or a large old-engine refactor. Separate new tests must prove old entry points still reject the new versions.

## Exact service names and configuration

Keep all signatures proposed for ManifestClient/ManifestServer, plus `(*ManifestServer).ServeHTTP(http.ResponseWriter,*http.Request)`. `ManifestNode` has exactly Resolve, RecheckContext and RecheckAdmission with the current prospectservice signatures. No old Recheck requirement is added. `SessionAuthorizer` is the existing interface.

Add these functions with canonical owned-value returns:

```go
ParseManifestServerConfig([]byte) (ManifestServerConfig,error)
ValidateManifestServerConfig(ManifestServerConfig) (ManifestServerConfig,error)
ParseManifestClientConfig([]byte) (ManifestClientConfig,error)
ValidateManifestClientConfig(ManifestClientConfig) (ManifestClientConfig,error)
ManifestTransportTrustID(ManifestClientConfig) (string,error)
```

Client and server configs use service Version3. Their full required field order remains as in the proposal; authority_pin is the complete version 5 pin. Parsers accept only exact decoded field names with no unknown/duplicate/null/missing members. Server parse/structural validation permits a nonnil clients array of length 0..64. Static NewManifestServer then requires 1..64, while NewManifestSessionServer requires exactly 0 and a nonnil session authorizer. Nil programmatic Clients normalizes to an owned empty slice; serialized null is refused. Duplicate public keys/token digests fail. Do not infer authentication mode from an empty array. Constructor copies config containers and requires the matching writable schema 5 store before any listener starts.

Client config validation retains the old URL/token/listener grammar but accepts only version 3 and the new pin. Both service Validate functions reject oversized individual inputs before copying/PEM parsing and require their final canonical serialized config to fit 65,536 bytes; a successful result must be accepted by its corresponding parser. The original raw transport bound still applies independently. It parses custom CA PEM strictly as below and returns canonical PEM for the sorted unique DER certificate set. No public parsed URL or private transport is returned. Parsers and validators perform no network, mutation or secret access. Constructors and every operational new worker/outbox path refuse slugtestpow before network/filesystem activity; pure public configuration validation and trust-ID inspection remain available in that build.

No SignAndSubmitManifest helper is required: the worker already owns signer persistence and uses the unchanged cooperative Signer callback.

## Immutable transport trust identity

Add `(*ManifestClient).TransportTrustID() string`. Add required field `TransportTrustID string` with JSON name `transport_trust_id` to ManifestConfig, immediately after `authority_pin` and before capacity fields. Add the identical getter to ManifestAuthority.

Compute a lowercase SHA256 hex ID over exactly:

```
ASCII "SLUG manifest transport trust\0v1\0"
LE32 byte-length of normalized origin UTF-8
normalized origin bytes
one mode byte
LE32 certificate fingerprint count
fingerprints, each raw 32-byte SHA256(DER), sorted unsigned lexicographically
```

Mode 1 is literal-loopback HTTP with no CA; mode 2 is HTTPS using system roots; mode 3 is HTTPS with custom CA certificates. Modes 1/2 have count 0. Mode 3 has at least one certificate. Fingerprints are unique; duplicates do not change the ID. No separator, final newline, token, account session, registry pin, or policy is added to this preimage. Those other public identities are pinned separately; bearer tokens may rotate without changing it. The domain contains actual NUL bytes, not backslash text.

The normalized origin is exactly the existing client URL normalization applied to the new config: parse and validate with net/url, remove the optional single root slash by setting Path to empty, supply explicit :80 only for HTTP with an absent port, then use URL.String(). Preserve accepted host/IP spelling and DNS case; HTTPS absent port and explicit :443 remain different trust namespaces. Do not collapse any additional URL equivalence or add a second worker-specific URL canonicalizer. For example, http://127.0.0.1 and http://127.0.0.1/ both normalize to http://127.0.0.1:80; https://example.invalid and https://example.invalid:443 remain distinct. Custom CA input consists solely of ASCII space, tab, CR or LF surrounding/between PEM blocks of type CERTIFICATE, no PEM headers, no non-certificate blocks and no garbage before/after/between blocks. Every DER value must parse as x509.Certificate. Sort/deduplicate by the raw SHA256 fingerprints; use the corresponding complete DER values in the trust pool and canonical PEM. A duplicate fingerprint with different DER is rejected. Existing 65,536-byte input-config bounds apply before PEM allocation/parse. PEM layout, certificate order and duplicates do not change the trust ID.

Mode 2 pins the explicit system-trust policy choice, not an immutable snapshot of platform root certificates. OS root maintenance remains an external trust boundary. This is CA validation, not a new leaf-certificate/SPKI pinning feature. Registry AuthorityID is not a TLS identity and cannot replace this binding.

NewManifest compares protocol 3, normalized origin, exact authority pin and trust ID to the outbox config. Typed authority adapters are trusted dependencies, not arbitrary callers who can lie about their getters. They must be immutable for a worker lifetime. A fresh-token adapter must build each client with the same owned transport settings and verify its trust ID/pin before use; it cannot report one ID while silently constructing another trust pool. Recheck the declared interface tuple before IssueManifestJob and SubmitManifest so accidental adapter mutation fails visibly before a new authority call. Client tokens are never persisted.

## Exact transport parsing and failure semantics

The existing `ErrorResponse` shape is reused with version 3 and the existing APIError/ClientError types and stable code→HTTP status mapping. No raw node, filesystem, credential, response body, provider or certificate text is exposed in errors. `outcome_unknown` is the only wire code with outcome_unknown=true. Unknown codes, mismatched statuses or wrong versions fail as untrusted transport.

Bounds apply to actual raw JSON including whitespace: request 65,536; response 73,728; config 65,536; nested Job2 8,192; nested Context 4,096; Proof 49,152; Authorization 4,096; ManifestRecord 65,536. Record proof has exact 80-byte header, 1..16,384 stripped coinbase bytes and at most 20 siblings, as existing rules specify. Birth/Eligibility/StaticBinding each has exactly its known fields; all nested Verified/DNA/Attributes fields are required. The only arrays in an admission response/request are proof MerkleBranch arrays with <=20 canonical hash strings. Empty branch is `[]`, never null. Config clients is the only config array. No descriptor, SVG, manifest payload, free-form JSON object or future component selector is accepted in a response.

Apply an object/array container depth cap of 5 to an admission envelope (root container depth 1); lower nested parser limits still apply. Strict schema checks precede expensive proof work. Canonical decoded values are required for a returned schema 5 record, including lowercase hex and empty-array shape. Whitespace, key order and escaped spelling that decodes to the exact field are allowed on transport; therefore a whole-response byte-reencode equality test must not be the transport parser. Stored outbox bytes have a different rule below.

IssueManifestJob has no client wall-clock gate. It verifies syntax, support, pin, identity and current configured policy, then returns the job. The worker alone applies its explicit local usability window. SubmitManifest has no current-time or fresh-node check for historical receipts. For inserted=true require current configured policy; for inserted=false require exact expected historical Job2/context and all other proof/binding checks, without requiring equality to a changed current prospect target. Both require the same pinned chain, authority, descriptor and full supported job.

A failure before request dispatch returns a definite sanitized validation/unavailable error with no output. After request dispatch, transport failure, bad/missing/oversized/unverifiable success response or cancellation returns zero output and outcome_unknown; this includes issuance because an unobserved job may have committed. A job retry is a new issuance, not reconstruction or exactly-once replay of the lost job. Well-formed non-200 error envelopes retain their specific status/code/unknown bit. The transport itself never retries.

On the server, record whether the protected operation callback has started. If an outer WithSession implementation returns an error after that callback may have mutated the registry, return outcome_unknown regardless of the outer error label, with no result. Only a failure known to precede the operation can be represented as definite unauthorized. Also preserve unknown outcome after commit-then-error writes, encoding failure or cancellation after successful mutation/recovery. Do not replace an uncertain commit with a definite denial. Historical disclosure still requires a live same-session check; new admission authorization linearizes in its final held guard.

ManifestClient.Close only closes idle HTTP connections, matching existing client semantics. It does not claim to cancel or join active requests. Call owners cancel/join their operations before closing/replacing a client. ManifestServer.Run is single-use and drains actual registered handlers before return, including noninterruptible Argon/storage work; it closes neither borrowed registry nor node. This distinction must appear in API comments/tests.

## Exact worker public API

Add `prospectworker.Version2 = 2` without changing Version1. ManifestConfig required encoder order is:

```
version, policy, public_key, payout_address, authority_origin,
authority_pin, transport_trust_id, max_records, max_bytes
```

Expose:

```go
ParseManifestConfig([]byte) (ManifestConfig,error)
ValidateManifestConfig(ManifestConfig) (ManifestConfig,error)
CreateManifestOutbox(string,ManifestConfig) (*ManifestOutbox,error)
OpenManifestOutbox(string,ManifestConfig) (*ManifestOutbox,error)
(*ManifestOutbox).List(uint64,int) ([]ManifestEntry,error)
(*ManifestOutbox).Close() error
NewManifest(ManifestConfig,ManifestAuthority,Node,Signer,*ManifestOutbox) (*ManifestWorker,error)
(*ManifestWorker).Run(context.Context) error
(*ManifestWorker).Status() Status
```

ManifestAuthority methods are ProtocolVersion, AuthorityOrigin, AuthorityPin, TransportTrustID, IssueManifestJob(ctx,key) and SubmitManifest(ctx,job,proof,auth), with the exact service return types. No public raw hash, verifier, generator, signer key or private fault hook is configurable. Public Signer/Node/Status and error sentinels are reused unchanged. Context nil is not supported by Run or callbacks, matching current Go APIs.

Config parsing and final canonical serialized config are bounded to 128 KiB; successful ValidateManifestConfig output must round-trip through ParseManifestConfig. Reject oversized individual strings/containers before copying or serialization. Parsing uses exact required names, no unknown/duplicate/null/missing fields or trailing data. Canonicalization uses existing policy/public-key/payout validation and the service origin normalization; pin/trust ID themselves must already be canonical. Pin version 5, supported exact candidate descriptor and policy chain/DNA match are required. Validate the trust ID as 64 lowercase hex characters; the worker cannot derive CA material absent from its config, so it compares the client's computed getter. Programmatic configuration is copied by value; no implicit defaults for pin/trust/version/capacity. Validation is pure; actual constructors/Open/Create/Run refuse cheap PoW before touching durable state.

Keep capacity bounds MaxRecords 1..1,000,000 and MaxBytes 64 MiB..1 TiB. Policy, key, payout, origin, pin and trust ID are immutable identity. Capacity may only increase across Open; it must be >= both original marker and current DB values. A new Worker requires exact current capacities as well as identity to match its open outbox. No secret credential appears in this config.

## Exact public journal types and storage

ManifestEntry required fields and order:

```
version, sequence, work_hash, job, candidate, block_win, prospect_win,
[authorization], [receipt], block_state, admission_state, [last_error],
next_attempt_ms, retry_count
```

Square brackets denote fields omitted when absent/empty (`omitempty`), never serialized null. Authorization is *gymauth.Authorization and Receipt is *prospectservice.ManifestAdmissionResult. All other fields retain the existing Entry scalar types; Job is prospectjob.ManifestJob. Version must be 2; Sequence positive; WorkHash canonical lowercase 32-byte display hex. Candidate is still version 1 with proof v2 and Job2 ID. No caller record is inserted through a public journal mutation API.

ManifestScan order is `version,job,template,candidate,extra_nonce,next_nonce`, with existing scalar/template/candidate types and ManifestJob. Its persisted `scan` value always encodes next_nonce 0; separate `cursor` is exactly 8-byte big-endian actual next nonce in 0..2^32. A returned internal scan combines them. Ordinary cursor progress never rewrites the full block/template.

Marker `SLUG-WORKER.json` is canonical compact JSON with no LF, fields application/version/config, application=`slug-prospect-worker`, version 2. Metadata order is application/version/config/last_sequence/records/bytes/pending/reserved, with existing uint64 counters. `meta`, `scan`, `cursor`, `e`+BE64 sequence, `h`+64hex WorkHash, `p`+BE64 NextAttemptMS+BE64 sequence retain existing index shapes. Hash/pending values are exact entry keys. All counts/index links must match actual records and contiguous positive sequences. Audit actual iterator entries, not an untrusted LastSequence loop. Checked arithmetic rejects overflow/underflow.

Limits: marker/metadata 128 KiB; entry 12 MiB; scan 32 MiB; GBT Block JSON 16 MiB; container nesting<=8 before typed decode; list limit 1..100 and the full encoded JSON array including brackets/commas<=32 MiB. A byte-limited page may return fewer than the requested count; continue after the last returned sequence. Empty terminal pages are nonnil empty arrays. Corruption within the examined page returns nil/error, never a trusted prefix; no examination beyond page budget. Lists/Open perform no Argon or node checks and return retained conditional evidence, not newly authenticated portable receipts.

Before allocating typed containers, apply byte/depth caps. Decode exact typed fields and require byte equality with canonical reencoding for every persisted JSON row; unknown aliases/duplicates/null/trailing/format substitutions fail. Required empty proof branches, GBT transactions and dependency arrays persist as `[]`; own live typed input and normalize nil empty required slices to empty slices before persistence. Optional empty GBT metadata arrays are omitted by their existing tags. Do not change the GBT component schema: keep ValidateTemplate/BuildManifest's transaction/dependency/count/script/weight/size checks, including <=16,666 transactions and dependency count<=transaction count. A typed adapter's giant containers are rejected before copying/serialization where their known caps allow it.

Open requires an existing plain directory, regular marker and existing DB. Validate marker/pin/config before LevelDB can create lock/log files; then audit canonical metadata/all reciprocal links/candidate/receipt binding. A missing/mixed/incomplete directory never recreates itself. Create takes a new path only and syncs marker, DB and relevant directories. Lost create acknowledgment is recoverable by explicit Open with the caller's original config; no random unknown outbox identity or public discovery API is needed. Initial failures retain their incomplete path. An uncertain Sync write poisons the handle; close/reopen audits actual committed state. Repeated Close returns the first close error; Close while Run owns the box returns ErrInUse without closing it.

## Immutable data, state transitions and receipt validation

Entry immutable identity consists of version/sequence/work_hash/job/candidate/block_win/prospect_win. Authorization may transition absent→one valid canonical signature only; once saved its bytes are immutable. Receipt may transition absent→one completely checked result only; first receipt bytes remain immutable thereafter, including inserted metadata, accepted time, first authority signature and static binding. The local authorization need not equal a historical record's first signature, but both must be valid for identical evidence/key. Caller/callback/result slices are copied before waits or retention.

Block states remain `not-applicable`, `pending`, `submitted`, `canonical`, `rejected`; admission states remain `not-applicable`, `needs-signature`, `pending`, `accepted`, `expired`, `rejected`. Non-applicable is fixed by independent target flags. New records use pending for block wins and needs-signature for prospect wins. Runtime transitions are pending→submitted; submitted→submitted/canonical; needs-signature→pending after authorization Sync; pending→accepted/expired/rejected. Existing terminal states do not regress. Block `rejected` is recognized as a terminal format value but is not inferred from an RPC rejection: the current conservative runtime retains uncertainty and performs canonical reconciliation. Admission accepted requires a receipt; pending/accepted require authorization; no receipt is allowed in other states.

Only a definite service `expired` makes admission expired; definite `invalid_request` or `too_large` makes it rejected. Unauthorized, forbidden, rejected, unknown_job, unavailable, busy, rate_limited, capacity and all uncertain transport outcomes stay retryable with exact evidence. No local clock expiry discards pending work. Keep the existing LastError allowlist and retry count<=32; delays are min(30s,2^min(retryCount,5)s) after increment, with checked timestamp addition. Local scheduling time is not an authority clock or a durable birth rule; backward movement within Run pauses new scanning as before.

Before every recovered action, validate the complete block/payout/coinbase commitment/merkle/witness against Job2 and rehash the exact header under normal PoW. A private cache may reuse only an exact header actually hashed by this worker; never disk summaries or a caller's Verified value. Determine block/prospect flags independently from their targets and compare retained values.

A successful SubmitManifest from even a custom typed authority adapter is not enough to mark accepted. Own the result, perform all schema/pin/job/proof/auth/birth/eligibility/static checks and one fresh prospect.Verify, and compare complete Verified. Use only fixed descriptor resolution and deterministic generation/rendering; no public projector or arbitrary-record trusted verifier. The ordinary client has independently verified the same response too; preserving this sequential duplicate verification is intentional for this first implementation. A malformed success from a custom adapter fails the run with no persisted receipt; healthy pending evidence remains for diagnosis/recovery. The normal client converts malformed transport success to outcome_unknown.

Cheap outbox validation derives expected public fields/static digests from the claimed WorkHash and checks internal consistency; it explicitly does not turn coherent local rewrites into authenticated authority history. A full receipt body reconstructed by an attacker can still be internally consistent. No import or provenance-signature claim is added.

## Exact work ordering, reservations and lifecycle

Keep renewal lead 30 s, node-context refresh 5 s, each external call's 30 s deadline, prepare backoff 1 s doubling to 30 s, one due reconciliation per scan cycle and immediate fresh-block delivery before older signer work. Worker-only usability is issued<=localNow<expires-30000ms before and after preparation. Preparation owns adapter template containers, validates Job2/pin/policy, calls BuildManifest and Proposal, then syncs scan before any hash. ExtraNonce uint64 overflow discards only unfinished scan and requests a new job; nonce cursor exhausted at 2^32 increments extranonce/rebuilds, never wraps a uint32 attempt silently.

Reserve before each hash for max-width new sequence/hash/counters plus full candidate and future response/auth/state expansion, as current logic does. Concretely, the pending future reserve is 512 bytes, plus gymauth.MaxAuthorizationSize (4,096 bytes) and service MaxResponseSize (73,728 bytes) while admission remains nonterminal. Terminal entries reserve 0; unresolved block-only work still reserves 512. Include JSON field/wrapper growth in that 512 allowance and prove it with maximum encodings; count actual existing entry/scan/cursor bytes plus reserved future bytes against MaxBytes. LevelDB/index/compaction overhead remains additional, explicitly not included. No new hash if the corresponding found record could not be retained.

Found entry+hash/pending index+sequence/counters+next cursor commit in one Sync batch before any send/sign. Block wins broadcast first even if prospect misses; persist submitted outcome, then independently query CanonicalBlock. `canonical` means observed canonical at reconciliation, not an immutable promise against future reorg. Canonical confirmation clears the unfinished old-parent scan, not retained evidence. Signer output Sync precedes admission send. Authority success checked fully then receipt/state Sync precedes worker acknowledgment. A write error returns no success and poisons. A lost response or outbox acknowledgment retains enough public evidence to replay; no fresh job/descriptor/time is substituted.

Run is single-use and owns a single sequential hash/action loop. Cancel cooperatively; join the noninterruptible hash and necessary found/state writes, retain a successful callback result before returning even if cancellation arrived meanwhile, and never launch detached verification work. Do not call a later transport after cancellation. Status is concurrently readable and uses existing labels/counters. Worker has no Close method: owner cancels and joins Run, then closes borrowed clients/outbox/node/store in dependency order. User queues and external adapter cooperation remain caller obligations, not a hidden unlimited internal queue.

## Policy evolution without historical rebinding

The registry pin includes chain, authority and descriptor, not prospect target. A changed prospect target can issue future jobs under the same authority when chain/DNA/descriptor still match. Its new worker needs a separate outbox with its new immutable Policy; do not mutate an old outbox's policy. Historical SubmitManifest uses exact old expected Job2 and original receipt under the same pin, even if the running service's current target differs. New/inserted admission still requires current service policy, and inserted client results require current client policy. Descriptor/chain/authority/trust-origin changes require a separate explicitly configured namespace; no automatic fallback, replacement or record enrichment.

The schema 2 account authority and schema 5 manifest authority namespaces remain separate. No trust ID/pin commits a session token, no lease can be reentered by a vault callback, and no held mining operation obtains full recovery entropy. Existing scoped signing and account final guards remain the sole custody/capability boundaries.

## Final acceptance before publication

In addition to proposal acceptance: independently test trust-ID PEM order/duplicate/whitespace equivalence and token exclusion, malformed PEM/DER/modes, and immutable adapter trust across token refresh. Cover pure configuration grammar separately from constructor mode gates. Test issuance lost response explicitly as unknown durable job possibility; no exactly-once claim. Exercise outer lease failure after operation entry/commit, client Close versus server/worker drain, exact encoded list-byte budget, nil-slice normalization versus stored-null refusal, capacity growth and terminal-state immutability.

No remaining product-policy choice is introduced by this appendix. Root/design approval of these engineering contracts is required before implementation/oracles; official assignment, production activation and D74 remain separate.
