# Remote attachment v1 bytes — first slice

Frozen proposed first-slice grammar following root's approved contract/clarifications, 2026-11-20. Public validation proves shape and cross-binding only, never possession of the referenced local wallet, enrollment acceptance, node currentness or remote service availability. Constructors must obtain those original owners later.

All public encodings are compact ordered ASCII JSON, integer numbers only, no duplicate/unknown/missing members, no null members, no whitespace outside strings and no trailing newline except the two marker files specified below. Nested existing structs retain their existing declared JSON order and unchanged validators. Config maximum is 262144 bytes. Each ID is exactly 64 lowercase hex digits, nonzero.

`RemotePoolConfig` fields, in order:

1. `version` = 1
2. `connection_id` — independently selected local installation identity
3. `operator_pin` — complete `registry.PublicManifestPin`, explicit version 2
4. `source` — complete `poolshare.Config`
5. `settlement` — complete `poolsettle.Config`
6. `miners` — complete `poolminer.Config`
7. `origin` — canonical public HTTPS origin, no path/query/fragment/userinfo; both clients use this same origin and append their existing closed routes. Private ingress upstream URLs are not miner inputs.
8. `ca_certificate` — explicitly selected nonempty canonical PEM certificate bundle under `prospectservice.ValidateManifestClientConfig`; no system-root fallback in this first controlled attachment. This is CA validation, not a client SPKI pin.
9. `expected_local` — the binding below
10. `max_records` — 1..4096, unchanged worker bound
11. `max_bytes` — 67108864..1073741824, unchanged worker bound

`RemotePoolLocalBinding` fields, in order: `version`=1, `manager_schema`=14, `wallet_owner_id`, `allocation_binding_id`, `original_source_binding_id`, `recipient` (the full original `PoolRecipientBinding`). The original source binding equals the recipient's original source binding and its recomputed `poolservice.BindingID`. The existing recipient validator checks the fixed fixture root/gym/native84 association. Owner/allocation IDs here are expected identities, not evidence that such a wallet exists.

Construct `registry.NewOperatorProfile(operator_pin)` and validate miners against `{profile,source,settlement}` using `poolminer.ValidateConfig`. Require the selected local gym to occur exactly once, with identical public key and payee script, and require its settlement destination to match. The local original `recipient.config.source.policy` must equal the remote `source.policy`. No equality between old and remote source IDs is required. Do not modify any nested local field to make an operator pairing valid. Recompute the profile/source/settlement IDs already carried in miners. Do not use untrusted stored values to select expected configuration.

Existing `TransportTrustID` recipes derive pool and admission identities using the same origin/CA, respective source/gym inputs and a placeholder syntactically valid token solely during pure config validation. The real token is not in any public identity. Derived `prospectworker.PoolConfig` retains its existing order/grammar and the exact selected limits; it is not itself a full attachment identity.

Let `J` be canonical config bytes, `LE32` little-endian byte length, and `H` SHA256. `config_id = H(ASCII("SLUG remote pool attachment") || 00 || ASCII("v1") || 00 || LE32(len(J)) || J)`.

`SLUG-REMOTE-POOL-INTENT.json` bytes are compact ordered `{version:1,application:"slug-remote-pool-worker",config_id,config}` plus exactly one LF. `SLUG-REMOTE-POOL-READY.json` is compact ordered `{version:1,application:"slug-remote-pool-worker",config_id,intent_sha256}` plus exactly one LF; `intent_sha256` hashes the entire intent file including its LF. READY contains no time, cursor, secret, fresh ownership assertion or alternate source. Parse requires exact expected config and exact canonical bytes. These codec helpers do not create/open an outbox or establish READY by themselves.

Private credential file bytes (each <=2048) are compact ordered `{version:1,config_id,gym_id,audience,generation,expires_at_ms,token}` plus exactly one LF. `audience` is exactly `pool` or `admission`, selected by which original file owner is being opened. `generation` is 1..4294967295; `expires_at_ms` is a positive integer <=253402300799999; `token` is nonzero64lowerhex. Each file must match the independently expected public config ID/gym and its own audience; the two raw tokens must differ. Generation/expiry are local installation declarations, not authenticated server facts. Current time is used only for refusal of expired credentials; local reopening does not renew them.

Credential owner API stays unexported inside `pilot`: `openRemotePoolCredentials(life, config, selection)`, `beginPool(ctx)`, `beginAdmission(ctx)`, `Close()`. Selection contains only the two original file paths. The returned private leases implement only fixed pool operations or admission submission (no independent admission job issuance), retain the original owner and audience, and have `Close` to join admitted calls. No raw token/HTTP client getter, arbitrary URL/domain/request callback or exported signer is added. Later original local View/Mining fences wrap these purpose-bound operations. HTTP mutations whose post-call owner/file/lifetime check fails return the existing outcome-unknown category.

One admitted lease per audience; a short nonblocking gate returns Busy rather than replacing it. Owner Close cancels all admitted leases and waits for their in-flight calls/lease close before clearing retained token bytes and closing the exact protected descriptors once; cleanup errors are sticky and retain the owner. Lease Close rejects new calls, cancels, joins its active operation and closes only its own HTTP transport. Registration and owner closing share the same mutex, so replacement owners cannot revive old leases. No whole-lifetime lock crosses network I/O.

Private files retain the already reviewed descriptor path walk, regular/single-link/current UID/0400-or0600 file plus0700 parent/no ACL, exact path+descriptor identity checks, bounded reads and sticky cleanup from `pooloperator/private_file_*` and `private_acl_*`. Darwin requires cgo's actual descriptor ACL API; no-cgo and unsupported systems fail closed. New remote-capable helper qualification must explicitly cover CGO1 later. Existing helper/node defaults are unchanged by this external stage.

Independent fixtures compose existing public operator Setup with the original recipient scripts: replace modeled remote destination/enrollment scripts with those literal original scripts and recompute IDs. The untouched old Setup pairing is a refusal, not a rewritten predecessor. Original recipient objects remain byte-identical. The public CA input is an existing published test certificate, not a real credential or proof of an actual endpoint. Synthetic owner IDs and credential tokens are recipe literals. No keys are loaded or generated.
