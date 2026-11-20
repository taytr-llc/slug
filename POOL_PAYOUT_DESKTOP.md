# Set67 desktop, wallet and private-wire contract

**Exact proposal for root review; not yet frozen.** November 19, 2026. This defines an additive pool-only family on branch `slug-payout-history`, after Set66 commit `247edc868a620dd39ac3bfdf678241066fa4f844`. No implementation or expected vectors have been generated from this proposal. The core payout projection/HTTP contract is a separate prerequisite; its `PayoutItem` and captured Balance identities are referenced below, never redefined by the desktop.

## 1. Scope and independent authorities

A current authenticated gym may capture its own retained pool payments, page that immutable capture locally, and explicitly ask its already-owned wallet about one captured payment. A current session held after reset may view. Original revoked sessions/views may not finish disclosure. No mutation of settlement, transactions, addresses, mining controls or recovery authority occurs.

The following remain distinct:

- Captured settlement state and its **settlement report timestamp** are historical pool journal observations. A payment's creation timestamp is not its confirmation time.
- A **wallet observation timestamp** dates one separate bounded wallet read. It is not a full-node proof, current canonicality, sixty-descendant finality, absence of conflicts, an unspent-output claim or current spendability.
- Current pool balance may refresh independently. It never substitutes for the balance captured with a payment history.

The complete original `ManifestViewPin`, exact `poolservice.BindingID(SourceConfig)`, and independently retained Set58 native84 recipient script bind every nonblank desktop result. The full pool Config and transport trust remain private immutable inputs. The browser cannot select a gym, source, recipient, wallet or arbitrary transaction.

## 2. Exact limits and ownership

Constants for this family only:

| Limit | Value |
|---|---:|
| Request frame, including newline | 1,024 bytes |
| Response frame, including newline | 12,288 bytes |
| Binding object canonical encoding | 4,096 bytes |
| Snapshot canonical encoding | 5,120 bytes |
| Job canonical encoding | 6,144 bytes |
| Capture metadata canonical encoding | 512 bytes |
| Page canonical encoding | 10,240 bytes |
| Wallet observation canonical encoding | 7,168 bytes |
| Captured complete HTTP history response | 131,072 bytes, including its transport newline |
| Retained own payment rows | 366 maximum; runtime MaxBatches is366 (MaxRounds256 is separate) |
| Local page size | 1 through8 |
| Retained helper capture allowance | 131,072 bytes |
| Payout operation scratch allowance | 2,097,152 bytes |
| Wallet gettransaction HTTP response body | 524,288 bytes |
| Decoded raw transaction | existing poolsettle.MaxTransactionBytes =100,000 bytes |
| Asynchronous capture deadline | 30 seconds |
| Selected wallet read deadline | 5 seconds |

The core contract must independently establish that its maximum complete response fits131,072 bytes. A512-byte independent row cap multiplied by366 is not that proof. Full captured Balance fractions remain in the helper; their potentially4096-digit numerator/denominator strings do not appear in desktop jobs/pages/observations. No old transport/cache limit changes.

One PoolChild owns at most one payout operation, either capture or wallet read, with no queue. At most one successful capture and its exact done-job metadata are retained. The current active/latest attempt may have separate small job metadata, so two distinct job IDs can be retained: prior successful capture A and replacing attempt B. No unbounded job list is kept. A new attempt evicts a prior failed/cancelled attempt's metadata, but never evicts successful capture A before validated replacement succeeds. Rejected admission preserves all previous state.

Reserve2 MiB scratch before source/RPC work. It covers one incoming capture's bounded raw/decoded/canonical working representations, or one wallet response's bounded raw envelope, selected hex, decoded/canonical transaction and metadata. This is an **accounted allowance**, not a measured hard Go heap limit. The old131,072-byte capture remains a separate reservation while B is prepared; after successful publication, release A before the operation reservation ends. Helpers never retain raw wallet replies/transactions after the observation is built. Local pages create at most one bounded response's owned rows through the existing serialized helper command path; callers own returned DTO copies. Existing stable/history/evidence allowances remain separate and unchanged. Account transition must join the old child before admitting another child's payout owner.

## 3. Scalars, grammar and canonical encoding

All objects below are closed. All listed fields are required, including empty strings, zeros, empty arrays and explicit nullable fields. Reject duplicate, unknown, missing, case-aliased and wrong-type fields, trailing JSON, invalid UTF-8 and disallowed nulls. Integer tokens are base10 without exponent, fraction, leading zero or negative zero. No floats are accepted in this private family. The native wallet's legacy RPC numeric Amount/Details fields are separately ignored, never forwarded.

`id` is1..9,007,199,254,740,991. `version` is exactly1. Correlation view/job IDs and hashes are exactly64 lowercase hex; job IDs are nonzero. `source_binding_id` and block/tip hashes are nonzero. Timestamps are integer Unix milliseconds0..253,402,300,799,999; a positive timestamp is required where specified. Atoms are integer0..poolsettle.MaxMoney (2,100,000,000,000,000); actual item amounts use the core contract's minimum1 SLUG. All arithmetic is checked before comparison/serialization.

Canonical encoding is compact UTF-8 JSON, declared field order, no HTML escaping, no internal insignificant whitespace and no trailing newline in an individual object. Transport adds exactly one newline and its frame limit includes that byte. Incoming JSON need not use canonical field order/whitespace, but must satisfy the closed grammar and bounds. Nested core rows use the independently frozen core field order. A nullable pointer must be JSON null, never omission; arrays must be[] when empty, never null.

### Shared binding

```text
PoolPayoutBinding {
  view: existing ManifestViewPin,
  source_binding_id: hash64,
  recipient_script: "0014" +40 lowercase hex
}
```

The nested View uses the existing exact closed ManifestViewPin grammar and candidate descriptor allowlist. It must equal the original live view, including authority pin/origin/trust, account/authority IDs and generation. Source binding equals the configured pool source, not a value trusted merely because it arrived from HTTP. Recipient script equals the existing Set58 ready binding. The desktop always has a configured recipient; a core unconfigured/empty-recipient response is rejected by this desktop even if that core response is valid for another caller.

### Captured small metadata

```text
PoolPayoutCapture {
  version:1,
  count:uint32,
  paid_atoms:int64,
  reserved_atoms:int64,
  settlement_observed_at_ms:int64,
  captured_at_ms:int64
}
```

Count0..366. paid/reserved are copied from the same validated complete captured Balance, and independently equal the checked sums of its confirmed and prepared/broadcast rows. Their checked sum does not exceed MaxMoney. The complete fractional Balance remains private. settlement_observed_at_ms is copied unchanged from the core response;0 is allowed only for its valid never-observed empty capture. captured_at_ms is the local positive capture-completion time, sampled after successful transport/validation and before final original-view publication. It is not the journal timestamp and must not be synthesized from it. The two clocks are not required to be ordered against each other. All metadata is immutable for that successful JobID.

## 4. Exact private request/response family

```text
PoolPayoutRequest {
  id:uint64,
  version:1,
  command:string,
  view_id:string,
  job_id:string,
  after:uint32,
  limit:uint32,
  txid:string,
  output_index:uint32
}
```

Command matrix (every unmentioned selector is its exact blank/zero value):

| command | Required selectors |
|---|---|
| pool_payout_snapshot | none; view_id/job_id/txid empty |
| pool_payout_begin | view_id, new native-preknown job_id |
| pool_payout_job | view_id, job_id |
| pool_payout_cancel | view_id, job_id |
| pool_payout_page | view_id, successful captured job_id, after0..366, limit1..8 |
| pool_payout_wallet | view_id, successful captured job_id, exact captured txid/output_index |

output_index0 is a real output selector only for pool_payout_wallet; otherwise it must be0. The wallet selector is0..255, matching at most256 canonical recipient payments; operator change is not a selectable payment. A wallet request's outpoint must exist exactly once in the owned capture before any wallet/RPC work. after must not exceed captured count. No supplied row, amount, script, source, address or Balance can enter this family.

```text
PoolPayoutResponse {
  id:uint64,
  version:1,
  ok:boolean,
  snapshot:PoolPayoutSnapshot|null,
  job:PoolPayoutJob|null,
  page:PoolPayoutPage|null,
  observation:PoolPayoutObservation|null,
  error:string
}
```

On ok=true exactly the command's arm is nonnull and error="". snapshot→snapshot; begin/job/cancel→job; page→page; wallet→observation. On ok=false all four arms are null and error is one of:

```text
invalid_request, unauthorized, stale_view, busy, capacity,
payout_unavailable, payout_not_found, operation_conflict, outcome_unknown
```

No error envelope carries an old account payload. `outcome_unknown` means only that a capture task's admission/reply cannot yet be resolved; it never implies sending, reserving or confirming a payment. No generic wallet error message or path/credential crosses the wire.

### Snapshot

```text
PoolPayoutSnapshot {
  version:1,
  state:"unavailable"|"idle"|"busy",
  error:string,
  binding:PoolPayoutBinding|null,
  activity:"none"|"capture"|"wallet",
  active_job_id:string,
  last_job_id:string,
  capture_job_id:string
}
```

Unavailable: error="payout_unavailable", binding=null, activity="none", all IDs empty. No other unavailable error is permitted; unauthorized/stale are envelopes. Idle: error="", binding nonnull, activity="none", active ID empty. Busy: error="", binding nonnull, activity capture/wallet, active ID nonempty. During capture active ID is B; during wallet read active ID is its selected retained capture A. last_job_id is the latest retained terminal capture attempt (done/failed/cancelled), or empty. capture_job_id identifies the one successful retained capture, or empty. They may differ after B fails. No successful capture is claimed merely by seeing an ID; its exact known done job must be read and validated.

### Capture job

```text
PoolPayoutJob {
  version:1,
  job_id:hash64,
  state:"running"|"done"|"failed"|"cancelled",
  binding:PoolPayoutBinding,
  error:string,
  capture:PoolPayoutCapture|null
}
```

Running: error="", capture=null. Done: error="", capture nonnull. Failed: capture=null, error one of busy/capacity/payout_unavailable. Cancelled: error="", capture=null. Terminal fields never mutate while retained. Cancel of the exact running capture signals cancellation and returns its current running/terminal job; it does not release the slot or claim cancelled until entered work joins. Cancel of a retained terminal capture returns that unchanged job and cannot cancel another capture or an unrelated wallet read. Ordinary source/transport mismatch is payout_unavailable, with no partial rows. Explicit cancellation produces cancelled after entered work joins; deadline failure produces failed/payout_unavailable. Lookup of an evicted/foreign job is payout_not_found or stale_view, never another job. A duplicate Begin for an exact currently retained JobID/view returns its existing job without another source read; a mismatched retained intent is operation_conflict. Native does not automatically repeat Begin after uncertainty.

### Local page

```text
PoolPayoutPage {
  version:1,
  binding:PoolPayoutBinding,
  job_id:hash64,
  capture:PoolPayoutCapture,
  after:uint32,
  limit:uint32,
  next:uint32,
  has_more:boolean,
  items:[core PayoutItem]
}
```

Only current successful capture A may be paged. Its binding/capture equal the stored done job exactly. `len(items)=min(limit,count-after)`, next=after+len(items), has_more=(next<count). Rows are the exact owned slice in newest-first order; no re-sort/re-observe/requery occurs per page. after=count returns[] with next=count and has_more=false. No rows move if the scheduler appends or changes recorded state. B's successful atomic replacement invalidates all A page/wallet requests; B's failure/cancellation leaves A usable under the same original view.

## 5. Exact dated wallet observation

```text
PoolPayoutObservation {
  version:1,
  binding:PoolPayoutBinding,
  job_id:hash64,
  capture:PoolPayoutCapture,
  payment:core PayoutItem,
  observed_at_ms:int64,
  state:"not_observed"|"unconfirmed"|"confirmed"|"unavailable",
  error:string,
  wallet_synced:boolean,
  wallet_height:uint32,
  wallet_tip:string,
  confirmations:uint32,
  block_hash:string
}
```

binding/job/capture/payment must equal the retained original selection. observed_at_ms is a positive local completion/attempt time, independently dated from captured_at_ms and settlement_observed_at_ms. wallet_height is0..2,147,483,647.

- not_observed: only the exact typed missing-transaction wallet response with stable synchronized pre/post tip. error="", wallet_synced=true, valid wallet_tip/height, confirmations0, block_hash="". This means absent from that wallet's saved transaction view, not absent from the chain or proof the pool failed.
- unconfirmed: exact raw transaction/output matches; wallet read is stable/synchronized; error="", wallet_synced=true, valid tip/height, confirmations0, block_hash="".
- confirmed: same exact match and stable/synchronized read; error="", wallet_synced=true, valid tip/height, confirmations1..wallet_height+1, nonzero block_hash. This is a wallet-reported confirmation count, not independently verified inclusion.
- unavailable: authorized original selection is still valid, but wallet/RPC/transaction validation/sync/tip consistency failed. error="wallet_unavailable", wallet_synced=false, wallet_height0, wallet_tip="", confirmations0, block_hash="". This is not an empty-wallet result. No partially trusted wallet fields are emitted. The page and previous dated observation may remain visible, with this later failed attempt clearly separate.

Busy, cancellation, closed/revoked/replaced view, invalid selector or capacity refusal are error envelopes, not an unavailable observation. After a successful response, later changes to wallet/chain do not silently edit this observation; explicit Check in wallet performs a new read.

### Method-specific wallet boundary

Use the already-owned walletRuntime and its private authenticated literal-loopback gettransaction RPC. No new RPC allowlist entry, live wallet handle, receive address, wallet rescan, chain query, rebroadcast, signing or UTXO/spendability read is added. Acquire walletRuntime.mu via TryLock; busy refuses immediately instead of waiting beyond a five-second deadline. Never hold account/View/cache locks while acquiring it. Keep it for the compound wallet read to prevent concurrent DB closure, then release it before final View publication.

Read ChainSynced and Manager.SyncedTo (height and hash) before and after exactly one RPC. Both must report synchronized, with identical nonzero tip hash and nonnegative height. ChainSynced is the wallet's own reported flag and may remain true during an unrecognized backend disconnect; this is not assurance of network freshness. The UI and DTO meaning are wallet-reported synchronized only. No hidden resampling loop. The five-second context applies to RPC and links to child cancellation. Same-height different-hash change is unavailable. Missing/incomplete sync is unavailable. Background sync may continue; this guard is a bounded consistency check, not an atomic independent-chain proof.

The new narrow HTTP decoder bounds the entire body to524,288 bytes before parsing. Require HTTP200, no redirects/compression, valid JSON-RPC1.0 envelope, exact request correlation, and no duplicate fields. The envelope's required fields are jsonrpc/result/error/id; error success is null. A missing transaction is recognized only as result=null and error exactly `{code:-5,message:"No information for transaction"}` from this exact method (`legacyrpc.ErrNoTransactionInfo`). Any other RPC/HTTP/JSON error is unavailable. Current generic walletHTTPClient.call remains behavior-compatible for its existing callers.

On success require this exact legacy gettransaction shape, preserving the field names in btcjson.GetTransactionResult and GetTransactionDetailsResult (wallet RPC key order is not canonicalized):

- Required result fields: amount (finite legacy float token), confirmations (int64 integer), blockhash (string), blockindex (int64 integer), blocktime (int64 integer), txid (string), walletconflicts (array), time (int64 integer), timereceived (int64 integer), details (array), hex (string). Optional result field: fee (finite legacy float token).
- Each Details object requires account (string), amount (finite legacy float token), category (string), and vout (uint32 integer). Its only optional fields are address (string), involveswatchonly (boolean), and fee (finite legacy float token). Optional absent fields are permitted; explicit null is not a substitute for their values. No other field or duplicate name is accepted.
- Details length is0..258 (up to257 credits plus one send entry). WalletConflicts length is0..256, containing only lowercase64hex transaction hashes. Arrays are never null. Each detail account/address/category string is at most256 UTF-8 bytes. The overall524,288-byte response cap still applies. Ignored legacy numeric Amount/Fee values may use decimal/exponent syntax but must parse to a finite float64; they are never forwarded or used as an amount oracle. Other required numeric fields must be integer tokens in their declared range. No unconstrained map/array allocations are permitted; stream or selectively decode the bounded shape and retain only the selected identity fields plus bounded raw hex.

This leaves all existing generic wallet callers unchanged. Amount/Fee/Details values are not used to calculate the payout. Require lowercase even hex of1..100,000 decoded bytes, canonical transaction parsing with1..256 inputs and1..257 outputs, recomputed displayed TxID equal both RPC txid and selected txid, output_index in range, and exact integer atoms/script equal the retained payment and Set58 recipient. Malformed/trailing/noncanonical transaction bytes fail. Check every compact-count/length against its limit and remaining bounded bytes before allocating; do not call a decoder that can allocate from an unchecked input/output/witness count. Witness marker/flag and all varints must be canonical; reject a superfluous witness marker/flag when every input has zero witness items. An individual zero-length witness item is a serialized item and is not the same as an absent stack. Witness item counts/lengths must be bounded by remaining bytes before walking, and walks remain bounded by the100,000-byte total without attacker-selected arrays. Compute TxID from the canonical non-witness serialization. Validate nonnegative output values and checked total within MaxMoney. This performs no script/signature or independent chain validation. Missing/negative/overflow confirmations fail; zero confirmations with a nonempty block hash fail; positive confirmations require a nonzero block hash and bound above. WalletConflicts is not a proof of conflict status; the current server does not save conflicts. No conflict-free field or automatic accounting rollback exists.

### Exact private adapter seam (root-owned implementation)

```go
type walletPayoutSelection struct {
    TxID string
    OutputIndex uint32
    Atoms int64
    RecipientScript string
}
type walletPayoutRead struct {
    State string // only not_observed, unconfirmed or confirmed
    ObservedAtMS int64
    WalletHeight uint32
    WalletTip string
    Confirmations uint32
    BlockHash string
}
func (r *walletRuntime) ObservePoolPayout(ctx context.Context,
    selected walletPayoutSelection) (walletPayoutRead, error)
```

Successful read implies stable synced=true and all method/transaction checks above. No View, source/capture/job, raw transaction, full RPC result or wallet credential crosses this adapter seam. It validates the expected selection before RPC. Nil/invalid selection or wallet/RPC/transaction inconsistency yields private sentinel `errWalletPayoutUnavailable`; failure to acquire wallet mutex yields `errWalletPayoutBusy`; cancellation/deadline returns the context error. Every error returns a zero read. Successful ObservedAtMS uses the real local clock after the second sync sample. The wallet mutex is released before return. The child maps busy to its busy envelope, cancellation/deadline to a no-payload payout_unavailable envelope, and ordinary unavailable to a dated unavailable observation only while its original request/View/selection remain valid. The child creates that failed-attempt timestamp; it never invents successful wallet fields. Root owns the adapter and its finite RPC/parser tests; desktop integration owns the final original-view fence and DTO assembly.

## 6. Child/parent task lifecycle and native recovery

Suggested additive Go surface:

```go
PoolPayoutSnapshot(context.Context) (PoolPayoutSnapshot,error)
BeginPoolPayoutHistory(ctx context.Context, viewID, jobID string) (PoolPayoutJob,error)
PoolPayoutJob(ctx context.Context, viewID, jobID string) (PoolPayoutJob,error)
PoolPayoutPage(ctx context.Context, viewID, jobID string, after, limit uint32) (PoolPayoutPage,error)
CancelPoolPayoutHistory(ctx context.Context, viewID, jobID string) (PoolPayoutJob,error)
ReadPoolPayoutTransaction(ctx context.Context, viewID, jobID, txid string, outputIndex uint32) (PoolPayoutObservation,error)
```

One separate pool-only interface avoids widening existing worker/solo API requirements. The child original View/publication lease is mandatory. Preflight captures immutable original view/source/recipient and reserves the payout slot under a short fence. Source/RPC reads, bulk parsing/validation and waiting/joining run outside all account leases. Final cache assignment and each DTO disclosure are fenced by the same original view/parent epoch; no freshly minted replacement credential can finish an old operation. The source authority may mint a token only within that original account/generation/view, and rechecks the actual token used.

Asynchronous capture starts a child-lifetime30-second context after synchronous admission, not one canceled when the helper dispatch response ends. It performs exactly one complete authenticated Payouts read. Local pages perform no source proof/HTTP work. Parent/child Stop, backup and Close first prevent new admission, cancel and join the payout slot outside locks/leases, then close/reuse wallet/source resources. Existing refresh/stable-lane drains alone do not join this independent slot. Backup cannot begin retained-wallet work until it joins. Stop cancels the task and leaves already-authorized retained history, but does not start/resume mining. Logout/reset/expiry/Close blank the old view and release its capture after join. Parent source/nodes/scheduler keep their prior ownership.

Historical capture may coexist with mining. It shares the existing bounded PoolSessionAuthority transport permit and server source permit, so contention fails busy rather than creating a hidden queue, another authority or automatically stopping mining. Wallet observations use the same payout slot and cannot race a replacement capture. Existing wallet activity may produce a fail-fast busy response.

Native maintains current capture-action identity before any preflight/dispatch: original helper, full view/source/recipient, native-generated random64hex JobID. A lost Begin reply resolves only that exact ID. A generic snapshot may separately expose a previously known capture ID for continued dated display; it must not treat that old ID as the successful new action. If the exact new ID is not found, native marks definite no-start and clears its new active/last-action correlation; retained old capture stays explicitly old. Definite pre-dispatch errors also replace the current action state so an earlier same-view success cannot masquerade as this refresh. No blind Begin retry. Job/page/wallet correlations bind the exact known capture and reject substitution. At most current action plus retained successful capture expectations are kept; no unbounded native map.

Native public methods use existing bridge serialization and no file dialog:

```text
PoolPayoutSnapshot()
BeginPoolPayoutHistory(viewID)       // native supplies preknown JobID
PoolPayoutJob(viewID,jobID)
PoolPayoutPage(viewID,jobID,after,limit)
CancelPoolPayoutHistory(viewID,jobID)
ReadPoolPayoutTransaction(viewID,jobID,txid,outputIndex)
```

## 7. Existing-panel presentation and finite acceptance

The existing pool balance panel gains Payment history, Refresh/cancel capture, previous/next pages, and Check in wallet. Exact eight-decimal atom formatting remains. Clearly label pool-recorded state and its report timestamp separately from wallet state/observation time. Keep a prior dated page on retryable failure and clear it on original-view/source invalidation. Empty means a verified count0 capture; unavailable is never shown as empty. No navigation reorganization, new branding, export/import, generic explorer, send control or D74 review.

Independent fixtures must cover each exact closed arm, all selectors/error matrices, maximum sizing,0/1/8/9/16/256/366 page counts, old capture plus new failed/unknown action, exact transaction output/value/script and typed missing versus arbitrary RPC error. Lifecycle tests must prove held current View access, original reset/logout denial during blocked source/RPC, no account lease over waits, joined backup/Stop/Close, one owner/refusal preserving old capture, stable same-height tip rejection and no implicit mining/settlement/wallet mutations. Actual qualification uses both fixture gyms' existing normal-PoW batch and the exact owned wallet transaction, with reopen and no-double-payment behavior; modeled wallet states are labeled modeled.

Root must review this exact grammar and bounds together with the core contract, then preserve/hash the frozen copies before independent output production. No new user policy approval is required.
