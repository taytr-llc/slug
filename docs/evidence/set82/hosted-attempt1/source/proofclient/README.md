# Normal hosted public proof client

External fixture-only helper. Agent work is source-only; root owns formatting,
builds, execution and qualification. Product packages and the normal hosted
scenario are unchanged. This helper opens no account, vault, wallet or custody
store, signs nothing, generates no blocks, and runs no worker/operator process.

The three commands share these mandatory arguments:

```
--reference ABS_REFERENCE_V1_JSON
--setup ABS_SELECTED_PUBLIC_SETUP_JSON
--remote ABS_PROTECTED_SELECTED_REMOTE_JSON
--expected-setup-id ROOT_CHECKED_HOSTED_SETUP_ID
--expected-remote-id ROOT_CHECKED_HOSTED_REMOTE_ID
```

Use the exact174293-byte reference17557b28921290461fdf47b9ed6f1b800d3d077ee5d325a4ec59277ab3faca6a.
The two IDs must come from the independent `check-selection.py` result. The
helper validates canonical original product setup/remote selection and permits
only the previously admitted host transport/schedule changes relative to that
reference. Full source/settlement/miner/profile/payee crossbindings remain.
No desired financial amount is learned from a new product response.

Each invocation has one60second parent bound. Original replay HTTP operations
use20second child bounds; only their original exact definite busy/rate_limited
refusals retry, with1second pacing. Money uses the original pure-read policy
(definite busy/rate_limited/unavailable), within the same60seconds. Unknown,
authentication, malformed response and other failures never become success or
silently trigger a new issuance. No new Issue is sent with a valid credential.
Root must retain its process timeout/join and complete output checks.

All inputs are canonical nonsymlink single-link bounded files. Remote config,
credentials and local RPC config must be in original current-owner0700 parents,
with0400/0600 file modes. Their identity/content is rechecked during admitted
calls and at completion. No credential value, credential hash, authorization
header, RPC username/password or private configuration is output. The helper
creates no file: root captures one fresh public JSON frame to an exclusive
artifact. Frame bound16MiB. A partial frame, nonzero exit, deadline, signal or
cleanup/write failure is a failed invocation even if a partial report exists.

## 1. Closed original outbox plus exact replay

```
slug-host-proof replay COMMON_ARGUMENTS \
  --attachment-root ABS_ORIGINAL_REMOTE_ATTACHMENT \
  --pool-credential ABS_POOL_DOT_CREDENTIAL \
  --sequence 1
```

Both miner and stopped wallet observer must first stop/join. The helper holds
**the original attachment directory flock** throughout both closed outbox audits
and replay, so a new attachment cannot start between those operations. The
qualified A2 `outbox_audit.go` is copied unchanged: original full recoverable
outbox audit/close precedes bounded StrictAll q/e/h/p/meta/scan/cursor projection.
It exposes only those public mining/delivery records, never sibling wallet data.

The requested sequence must already have an accepted pool receipt. This command
will not reconcile an unknown/pending pool submission, adopt latest work, create
a request, or search a replacement nonce. It validates the retained original
normal-PoW work/nonce, then performs original v3 Lookup and exact same Work/Nonce
Submit. Both returned complete receipts must equal the local retained receipt.
The original row and all q/e/h/p/control state are unchanged afterward. No
mutable observation field is cleared for this full-receipt comparison.

The first credential is the canonical local wrapper emitted by reviewed
`client-flow/credentials.py`: outerVersion1, BaseVersion2, exact
DailyRemotePoolConfigID(base), first gym, pool audience, current positive
generation/expiry, exact IssueProfileID and token64, followed by oneLF. The
helper checks the wrapper; original server/client authenticate actual use.

Output includes all original rows, before/after complete outbox controls and
namespace hashes, selected Work/Nonce/full receipt, bounded ordered attempt
trace/deadlines, and zero new-Issue calls. Running the same command after the
normal same-root host restart, while generation3 serves and the local observer
is again joined, suffices for final full-outbox comparison. Root compares the
entire prior/current rows and controls; no separate read-only command is needed.

## 2. Both gyms' authenticated whole balances/history

Before mining:

```
slug-host-proof money COMMON_ARGUMENTS \
  --pool-credential ABS_POOL_DOT_CREDENTIAL \
  --other-pool-token ABS_POOL_TOKEN_1 \
  --phase initial
```

`pool-token-1` is the second enrolled gym's already published rotated raw64 token,
with **no LF**; no attachment or wallet is opened for that gym. The first wrapper
and second raw token must differ. They construct two original concrete
NewRecoverableDailyClient instances, each pinned to its original gym/payee.

Initial mode sends bounded unauthenticated and all-zero-invalid-token requests
to `/pool/v3/issue`, `/pool/v3/balance`, and `/pool/v3/payouts`. Each must return
exact401/Version3/unauthorized/outcome_unknownfalse; arbitrary failures are not
accepted. The issue body contains one fixed public request ID, but there is no
authenticated issuance call. Public output records route/status/typed refusal
and body hash/length only. **Remote allocation counters are not inferred from
HTTP**: root must corroborate initial/final original operator status or closed
census, and retain the original source ordering that authenticates before any
issuance effects. These negative probes do not stand in for a positive wallet
or source authority check.

Initial positive reads require both gyms' complete zero balances, highBatch0,
beforeBatch1, Done=true, NextBefore0 and no payout items. Revision may legitimately
be nonzero in an empty ledger; it is not forced to zero.

After root observes actual confirmation:

```
slug-host-proof money COMMON_ARGUMENTS \
  --pool-credential ABS_POOL_DOT_CREDENTIAL \
  --other-pool-token ABS_POOL_TOKEN_1 \
  --phase confirmed --txid EXACT_ORIGINAL_TXID \
  --after-ms ORIGINAL_CONFIRMATION_OBSERVATION_MS
```

Both complete balance and captured page must be fresh after the supplied
confirmation observation, source/config/payee bound, and report highBatch1,
beforeBatch2, Done=true, NextBefore0. The first gym has exactly one confirmed
sequence1 item for the original txid/output0/1,980,000,000atoms; the second has
no matching items and paid0. Both have reserved0 and exact zero confirmed and
available fractions. Because this selected fixture has high0/1 and the required
Done flag, one bounded page is its **entire** history; an empty Items list is
never treated as EOF on its own. The concrete client validates wireVersion3/
IssueProfile before returning normalized existing DAILY DTOs (Version2), which
is why output records `wire_protocol:3` separately. Integer money strings and
captured cursor strings retain the original DTO encoding.

## 3. Independent canonical reward/payment check

```
slug-host-proof chain COMMON_ARGUMENTS \
  --node-rpc ABS_PROTECTED_ORIGINAL_LOCAL_RPC_JSON \
  --audit-report ABS_ROOT_CAPTURED_CONFIRMED_HOST_AUDIT_JSON \
  --audit-sha256 ROOT_RECORDED_AUDIT_SHA256 \
  --txid EXACT_ORIGINAL_TXID
```

The audit input is a successful confirmed report from `slug-host-audit`, captured
while the original remote operator was stopped/joined. Its exact SHA is supplied
independently by root; this helper does not adopt arbitrary JSON as authority.
No remote store is read here. ParseRPCConfig retains the original endpoint
rules, and this command additionally requires a literal loopback local node.
HTTPS uses its selected CA; plaintext is allowed only by the original loopback
configuration rule. Methods are hardcoded read-only: getblockcount/getblockhash/
getblock. There is no generate, sendrawtransaction or wallet RPC path.

The local node must report the original production genesis and height165. The
helper independently decodes complete canonical raw blocks63/64/165, recomputes
their block hashes, validates exact original accepted reward membership and
coinbase shape, and locates exactly one1,000,000,000-atom operator reward output
per reward block matching each audited credit. It decodes the stored signed
payment, validates its txid, exactly2 unique original credit outpoints, ordered
stored inputs, nonempty witnesses and exactly the configured miner1,980,000,000
and change19,999,791 outputs with209 fee. It finds exactly that complete raw
transaction once in canonical block165 and rechecks all three canonical hashes
afterward. The original full ledger audit and canonical node acceptance provide
signature validation; no second signing backend or witness policy is invented.

Output includes bounded public raw reward/confirmation blocks, exact credits,
raw payment/hash, selected txid, economics and read-only RPC response hash/length
trace. It proves canonical membership observed through the original local node;
it is not a wallet delta or a count of sendrawtransaction invocations. The
stopped original wallet observer supplies the independent ready-height165 and
spendable delta1,980,000,000; the two stopped operator audits establish unchanged
signed batch/history across restart. Do not infer one RPC-send call from one
txid; duplicate-payment protection is the exact batch/raw, whole history and
original wallet delta evidence.

## Build and provenance

Eight production inputs are fixed in `production-checkpoint.json`9b3870be;
root's `../proofclient-formatted-manifest.json` records any formatting successor
and preserved preimages. The entire A2 outbox leaf is copied byte-for-byte.
`bindings.go` contains the four original A2 client/worker config function bodies
without semantic changes (only an isolated imports/module wrapper).

Root adds module `github.com/taytr-llc/slug/desktop/hostedproof` to the same exact
paired17-module workspace plus reviewed helpers, copies inherited desktop/go.sum,
and retains explicit versioned local replacements and normal candidate-go
profile. Build target `./hostedproof`, binary `slug-host-proof`. Root must record
full selected source/dependency/profile/binary hashes and joined result. No
product go.mod, helper runtime, endpoint, key, limit or deadline is changed by
these instructions. Normal/cheap/refusal execution status lives in root's
separate reports, never in this source-only document.
