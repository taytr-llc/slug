# Remote native helper protocol v1

External source contract, 2026-11-20. This is section A's first functional slice from `../remote-native-boundary/NATIVE_HISTORY_PLAN.md`: the existing remote controller's snapshot, fixture login, Start, Stop and Logout. It adds no authentication flow, payout/history API, source migration or UI organization. No implementation, Go execution or checkout mutation is included here.

## Original selection and concrete API

The native owner independently selects the existing local root, separate attachment root, complete validated `pilot.RemotePoolConfig`, protected credential paths, and private local RPC selection. Local RPC retains the existing HTTP `127.0.0.1:explicit-port`, no-CA boundary. Credentials and RPC values do not enter this protocol. The helper opens the actual `accountpilot.RemotePoolController`; opening it and taking snapshots never starts the worker. The selected fixture and public local identity come from `Config.ExpectedLocal.Recipient.Config`. No response supplies a replacement config, source, payee, endpoint or authority.

Proposed names, fixed for this slice:

```go
// New wire DTO uses the same real field types with explicit fields. The
// concrete controller adds a mechanical field-for-field WireSnapshot method;
// its existing Snapshot return type, JSON tags and bytes remain unchanged.
type RemotePoolControllerAPI interface {
    WireSnapshot(context.Context) (RemotePoolWireSnapshot, error)
    LoginFixture(context.Context, string) error
    Start(context.Context, string) error
    Stop(context.Context, string) error
    Logout(context.Context, string) error
    Close() error
}
// Pure codecs receive the complete independently selected config, never a
// config inferred from a result. The prior view is an owned public copy.
ValidateRemotePoolRequest(request, expectedConfig) error
ParseRemotePoolRequest(raw, expectedConfig) (request, error)
ValidateRemotePoolWireSnapshot(snapshot, expectedConfig) error
ParseRemotePoolResponse(raw, request, expectedConfig, priorView) (response, error)
ServeRemotePool(ctx, input, output, concreteController, expectedConfig) error
```

These function signatures are descriptive where Go types are not shown; implementation uses concrete request/response structs, `[]byte`, `RemotePoolConfig`, and `*RemotePoolView`. There is one typed result, `*RemotePoolWireSnapshot`, not `any`, a raw result union, or an `OwnedGymControllerAPI` adapter. `RemotePoolWireSnapshot` has the exact current controller fields and real `AccountSnapshot`, `RemotePoolView` and `RemotePoolSnapshot` types. `accountpilot.RemotePoolController.WireSnapshot` calls its actual `Snapshot` and copies those fields; it adds no state, getter, alternate owner or interpretation. Existing controller/owned/legacy DTOs, JSON tags, bytes and codecs remain unchanged. The new explicit null/empty fields avoid feeding an `omitempty` shape into the strict reflected decoder.

## Exact framing and ordered fields

One compact ASCII JSON object followed by exactly one LF per frame. Request JSON is at most **2,048 bytes**, response JSON at most **16,384 bytes**, excluding that LF; the receiver bounds accumulation before decoding and rejects CR, a missing terminal LF, extra/trailing JSON, blank lines and extra unsolicited frames. These are family limits below the existing general pipe bounds. Every field is required, in the listed order, including empty strings and nullable pointers. No unknown, duplicate or reordered fields. Strings must decode to ASCII without control characters. JSON numeric tokens are unsigned decimal integers, with no sign, fraction or exponent; JSON booleans are not integers. Maximum safe integer is `9007199254740991`. No arrays/maps occur in this wire. Canonical marshaling uses no whitespace or alternate escapes; parsing uses the existing strict ordered shape plus these field constraints, not raw-byte equality for harmless JSON whitespace/escapes inside the bounded object.

Request fields:

```text
version, id, command, config_id, view_id, fixture
```

`version=1`; `id` is 1..maximum-safe, strictly increasing on this original pipe, starting with 1. `config_id` is the nonzero lower-case 64-hex `RemotePoolConfigID(expectedConfig)`. Only the five commands below exist. An inactive string is exactly empty. A nonempty `view_id` is nonzero lower-case 64-hex; it is correlation, never a session token.

| Command | view_id | fixture | Successful result |
|---|---|---|---|
| `snapshot` | empty before login; otherwise original retained view | empty | Actual current snapshot; an expired/childless result may retire the original view |
| `login_fixture` | empty | exactly the fixture selected in expectedConfig | Signed-in snapshot with a fresh view, stopped attachment |
| `start` | original retained view | empty | Signed-in snapshot of that exact view after Start returned |
| `stop` | original retained view | empty | Signed-in snapshot of that exact view, stopped or failed attachment, zero displayed hash rate, after joined Stop returned |
| `logout` | original retained view | empty | Signed-out childless snapshot after joined Logout returned |

The first request is `snapshot`, ID 1, empty view/fixture. Its success must be the newly opened controller's signed-out, childless, error-free snapshot. There is no extra `startup`/`scope` command. The server seals a deep copy of the selected config before this request. It does not learn selections from the first snapshot.

Response fields:

```text
version, id, command, config_id, view_id, ok, result, error
```

The first five fields exactly echo the request. On success `ok=true`, `error=""`, and `result` is the complete typed snapshot. On refusal `ok=false`, `result=null`, and `error` is one of `invalid_request`, `stale_view`, `busy`, `unauthorized`, `forbidden`, `cancelled`, `remote_unavailable`, `outcome_unknown`, `cleanup_incomplete`. No raw errors or private paths. A malformed frame, wrong config, reused/out-of-order ID or unknown command terminates the pipe before dispatch; no best-effort recovery from a corrupt stream.

## Exact snapshot grammar and validation

All objects use the following ordered fields, copied from the real controller/attachment DTOs; the new wire DTO explicitly encodes `view=null`, `remote=null`, `error=""`; the existing controller `omitempty` encoding remains unchanged.

```text
controller:
  version, config_id, state, account, view, remote, error
account:
  state, fixture, id, generation, hold_until_ms, held, can_export, last_error
view:
  version, view_id, account_authority_id, account_id, generation, fixture,
  root_public_key, gym_public_key, gym_id
remote:
  version, config_id, view, source_binding_id, original_source_binding_id,
  state, worker_state, last_error, attempts, hashes_per_second_milli,
  records, pending, wallet_state, wallet_height, spendable_atoms,
  immature_atoms, unconfirmed_atoms, payout_address
```

Validation first validates the complete expected config and computes its ID using the already frozen config recipe. Each version is 1. Controller and remote config IDs equal that ID. Account fixture equals the selected fixture. `account.can_export` is always false: this controller has no recovery API. Controller state equals account state.

- Controller `state` is `signed_out`, `signed_in`, `failed` or `expired`. Transitional signing states are not disclosed as successful snapshots: the concrete controller serializes admissions and returns Busy while an action/expiry owns its gate. Closed owners refuse reads.
- For a childless state, `view` and `remote` are null; account ID is empty, generation/hold are zero, Held is false. Controller `error` equals account `last_error`: empty for signed-out, `remote_unavailable` for failed, empty or `remote_unavailable` for expired. No dormant child or old view is serialized.
- Signed-in requires both view and remote, empty controller/account errors, exact account ID from expected local recipient, and generation 1..maximum-safe. Account generation equals view generation. Hold time is 0..maximum-safe; Held=true requires nonzero hold time. Held is an observation by the original owner, not recomputed from the parser's later wall clock. The same retained view must keep exactly the same identity and generation.
- Both copies of view are identical. All view identity fields other than fresh view ID/generation equal the independently selected original recipient: authority ID, account ID, fixture, root public key, gym public key and gym ID. These values have already passed the original config's fixture/recipient validation. A response alone confers no View or Mining capability.
- Remote `source_binding_id` equals expected `Miners.SourceID`; `original_source_binding_id` equals expected local original source ID. Payout address equals the exact audited original recipient address. Neither source may be substituted for the other.
- Remote `state` is `stopped`, `starting`, `running` or `failed`. `worker_state` is `ready`, `stopped`, `failed`, `paused`, `preparing`, `retrying`, `hashing`, `delivering` or `unavailable`. No assertion that a Start reply must still be running: the real worker may finish/fail before disclosure. A non-running snapshot has zero hash rate.
- Remote `last_error` uses the installed attachment's closed vocabulary: empty, `unavailable`, `outcome_unknown`, `busy`, `capacity`, `rate_limited`, `expired`, `rejected`, `not_found`, `unauthorized`, `forbidden`, `invalid_request`, `too_large`, `stale`, `clock`, `cycle_unavailable`, `signer_unavailable`, `worker_unavailable`, `cleanup_incomplete`.
- Attempts and hash-rate thousandths are 0..maximum-safe; records 0..expected MaxRecords; pending 0..records. Counts are durable delivery observations, not ownership, earnings or finality. `wallet_state` is `unavailable`, `syncing` or `ready`; height is 0..2147483647; each atom field is 0..2100000000000000. Unavailable wallet requires zero height and amounts. Those are local wallet observations, never remote account balances.

## Dispatch, view and helper lifetime

The server is the sole sequential caller of the retained concrete controller. Before dispatch it obtains and validates an actual snapshot. A missing/mismatched required original view refuses before mutation. Start/Stop/Logout still pass that same view into the concrete methods; a precheck never replaces their own final fences. Login is allowed only with no retained signed-in view and only the selected existing fixture. It invokes the existing signed fixture login/enrolled-only path, not account creation.

After a successful action, the server obtains and validates another actual snapshot with the command rules above. It does not fabricate a stopped/account result. Failed or mismatched post-action disclosure returns `outcome_unknown`, even if the action itself returned nil. A controller mutation error is conservatively `outcome_unknown` unless the server itself established a pre-dispatch refusal. This avoids mistaking a downstream joined error for proof that Start/Stop/Login had no effect. Read-only/pre-dispatch errors can use the narrower closed codes; mixed/cleanup errors take precedence over Busy. There is no automatic mutation retry.

An original view may be acquired only from a successful login response. Later signed-in results require byte-equivalent retained view metadata. A snapshot that observes the original view expired/signed-out may clear it; it cannot replace it with a different signed-in view. Logout echoes the retired original view in the envelope while returning null child/view in its result. Empty-view snapshot after bootstrap is allowed only while no view is retained. Lost/uncertain login never enables view adoption by a later empty-view snapshot.

`RemotePoolBridge` owns one concrete launched helper/pipes/reader/process wait, one copied expected config and monotonically increasing request ID. `RemotePoolContext` privately binds that original bridge and full original view; its public fields are not authority. A context from another bridge is refused even if all DTO fields match. Helper correlation is the original process/pipe owner, not an attacker-supplied helper ID. Native code cannot replace a bridge or selected root by receiving a matching JSON identifier.

One request is in flight. Per-exchange maximum is the existing two minutes, further bounded by the caller. Failure before any write is an ordinary cancellation/refusal. Partial write, timeout/cancellation after possible dispatch, malformed/cross-request response, unsolicited frame or helper death invalidates the bridge, closes pipes and joins its original work/process; an affected mutation is unknown. An `outcome_unknown` or cleanup-incomplete reply also retires that helper lifetime. No replacement login, new request ID or alternate helper is used to resolve it automatically. Closing input/EOF, cancellation and native Quit cancel and join the controller; sticky failed cleanup stays attached to the original owner. A read result never resurrects a closed helper.

The root-owned launch integration adds a mutually exclusive explicit remote mode and protected local input selection, without changing old mode defaults. It must not put token/RPC secret values into argv, stdout or frontend data. Darwin's remote-capable helper is explicitly qualified with CGO=1 for protected-file ACL checks; existing CGO=0 routes retain their existing behavior. Native GUI and packaging acceptance remain separate from codec/pipe qualification.

## Independent reference and finite acceptance

The reference source will copy and hash-lock the independently generated remote-config corpus `pilot/testdata/remote-pool/reference-v1.json` (SHA256 `5f67fcaaba0c78b97b0f116a4c598eb0ec62ee1436bbcf280edcc747856e5177`) and its approved input recipe/provenance. It will use both original fixture identities and exact selected public config/recipient bytes as inherited inputs; it will not recompute X.509, wallet derivation or prior profile validity. New view IDs use fixed SHA256 ASCII domains and generation/observational values are explicit literal recipe inputs. A pure Python model will independently construct ordered new request/snapshot/response bytes and validate their new cross-bindings before Go fixtures exist. This proves conditional wire composition, not a live session, worker, wallet balance or source authority.

Finite corpus families: all five commands and initial bootstrap; both fixtures; signed-out/signed-in stopped/running/failed/expired snapshots, unavailable/ready wallet and held observation; exact refusal envelopes; wrong config/source/local authority/account/gym/generation/view/payee and command/result correlations; required/null/duplicate/order/unknown-field/numeric/raw limits; modeled original-helper/view transitions. Source, literal case plan and input hashes freeze before generation, with portable Python reproduction and failures retained. No expected bytes are derived from Go marshaling or actual runtime output.

After implementation, tests use these exact public fixtures plus concrete controller/pipe controls for original-view dispatch, post-action unknown, canceled admission versus retained lifetime, no Start on open/reopen and joined Close. A bounded actual CGO=1 helper+bridge must exercise the already qualified synthetic schema14/TLS operator route, then verify stopped outbox reopen and clean joins. Controller/pipe success is not native UI acceptance. Payout reads, OperatorV2 history, recovery/transfer additions and D74 stay outside this contract.
