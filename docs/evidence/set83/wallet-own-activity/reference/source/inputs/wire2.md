# Selected DAILY wallet activity wire 2

This is an additive local helper family. WIRE1 (`SLUG-REMOTE-DAILY-WALLET-1`) and all its bytes, eight methods and limits remain unchanged. The existing explicit DAILY wallet route selects version 2 only with `--daily-wallet-helper-version=2`, together with `--daily-recoverable-issue --daily-wallet-helper`. No remote HTTP/config, account, wallet, payout or authorization schema changes. Start remains explicit.

## Ordered envelope

Every frame is exact canonical ASCII JSON in the order below, followed by exactly one LF. Required fields cannot be omitted. Duplicate/unknown/reordered keys, noncanonical escaping/numbers, whitespace, null scalars and trailing bytes are refused. IDs and sequence/cursor values named below are canonical unsigned decimal uint64 strings (`0` or a nonzero digit followed by digits), never JSON numbers. Other uint32 and int64 fields retain their existing bounded JSON number encoding. No conversion through a JavaScript Number is allowed for a string counter.

Request order:
`version, application, id, command, config_id, view_id, fixture, sequence, capture_sequence, before_batch, after, limit, txid, output_index, batch_sequence`.

Version is 2, application is `SLUG-REMOTE-DAILY-WALLET-2`. `id`, `sequence`, `capture_sequence`, `before_batch`, `batch_sequence` are decimal strings; `after`, `limit`, `output_index` are uint32 numbers. Config ID is the original complete recoverable remote config ID. ID 1 is the initial snapshot with empty View; every later request ID strictly increases on this original process. Login has the original selected fixture and empty View; all other state-changing/read commands require the current original View. Snapshot retains WIRE1's optional View convention.

Response order:
`version, application, id, command, config_id, view_id, ok, snapshot, job, read, activity_job, activity_read, activity_page, activity_detail, payout_observation, error`.

Correlation is exact; required result arms are null except the single arm selected by a successful command. A refusal has all result arms null and a nonempty existing WIRE1 top-level error. Successful responses have error empty. The existing eight commands (`snapshot`, `login_fixture`, `start`, `stop`, `logout`, `daily_payout_begin`, `daily_payout_read`, `daily_payout_cancel`) retain their original semantics, original payload shapes and 65,536-byte response limit; all five new request fields are zero/empty. The snapshot payload remains version 2, independently of the local envelope version.

New command selection (all have nonempty original View and empty fixture):

| Command | Nonzero/nonempty selectors | Zero/empty selectors | Success arm |
|---|---|---|---|
| `wallet_activity_begin` | sequence | capture_sequence,before_batch,after,limit,txid,output_index,batch_sequence | activity_job |
| `wallet_activity_read` | sequence | same as begin | activity_read |
| `wallet_activity_cancel` | sequence | same as begin | activity_read (after original join) |
| `wallet_activity_page` | capture_sequence; limit 1..8 | sequence,before_batch,txid,output_index,batch_sequence | activity_page |
| `wallet_activity_detail` | capture_sequence; txid 64 lowercase hex | sequence,before_batch,after,limit,output_index,batch_sequence | activity_detail |
| `daily_payout_observe` | capture_sequence; batch_sequence 1..32767; txid 64 lowercase hex | sequence,before_batch,after,limit | payout_observation |

Page after is 0..256 and must not exceed captured Count. Output index is 0..255 for observation. Hash validation retains Set67/70 zero-hash behavior: identifiers are syntactically 64 lowercase hex; actual record authority/membership is independently enforced. New request frames are at most 2,048 bytes including LF. Activity job/read and payout observation replies are at most 16,384 bytes; activity page/detail replies at most 98,304 bytes. These are per-command bounds; they do not raise old command limits. The independent producer must derive finite maxima before qualification.

## Activity payload order and invariants

`activity_job`: `sequence,state,error`. Sequence is a nonzero decimal uint64 string in a separate activity sequence space. State is working/done/failed/cancelled. Error is empty except failed, whose error is exactly busy/capacity/activity_unavailable. Cancellation requested through the original operation yields cancelled with empty error; deadline exhaustion yields failed/activity_unavailable. Admission is next sequence or exact last/current repeat. Unknown reply never allocates a replacement sequence. No job ID hex string or legacy ManifestViewPin is fabricated.

`activity_capture`: `sequence,binding,metadata`. Sequence names the successful activity job, not a later failed refresh. Binding is the complete ordered WIRE1 DailyWalletBinding, including original RemotePoolView. Metadata is the unchanged ordered Set70 WalletActivityCapture (`version,captured_at_ms,wallet_height,wallet_tip,node_height,node_tip,count,has_older`). Metadata version remains 1, count <=256; heights/tips equal, time positive and safely bounded, HasOlder implies count256. This is a dated own-wallet capture, not a remote settlement or chain-finality claim.

`activity_read`: `job,capture`, where capture is required null or a complete activity_capture. Done requires nonnull capture with capture.sequence == job.sequence. Working/failed/cancelled may retain null or the previous successful capture with strictly smaller sequence. It must never relabel the old capture with the newer job ID. There is no implicit latest job adoption. A successful new capture invalidates selectors for the previous capture; a failed refresh preserves it.

`activity_page`: `capture,after,limit,next,has_more,items`. Items are unchanged ordered Set70 WalletActivityItem. After/Limit exactly equal the request. End=min(After+Limit,Count); Next=End; HasMore=(End<Count); len(Items)=End-After, nonnull array. Items are unique and strictly ordered with WalletActivityItemLess, valid under the enclosed metadata. The native owner returns exactly its retained captured positions; a parser can verify shape/order/length, but cannot invent authority for unseen positions.

`activity_detail`: `capture,item,outputs`. Item is unchanged Set70 WalletActivityItem with exact requested TxID. Outputs are the unchanged ordered Set70 WalletActivityOutput, nonnull; count equals Item.OutputCount, indices consecutive from zero, ownership/change/amount totals reproduce Item. Native detail reparses the original retained raw record and requires equality with the cached summary. Request TxID membership in the original cache is a native-owner check, not inferred from a self-consistent public response.

All Set70 payload amounts retain safe JSON integers <= MaxMoney (2,100,000,000,000,000 atoms). NetAtoms may be negative down to -MaxMoney. FeeAtoms is required null or nonnegative safe integer. No cumulative DAILY paid/reserved string becomes numeric. Existing transaction/raw/input/output/capture memory limits, address grammar, ordering and confirmation semantics remain unchanged.

## Selected payout observation payload

`payout_observation` order:
`capture_sequence,binding,revision,high_batch,captured_at_ms,settlement_observed_at_ms,stale,batch_sequence,payment,observed_at_ms,state,error,wallet_synced,wallet_height,wallet_tip,confirmations,block_hash`.

capture_sequence, revision, high_batch and batch_sequence are canonical decimal uint64 strings. Binding is original DailyWalletBinding. Captured/settlement times and Stale reproduce the exact retained DAILY capture/header. Payment is unchanged ordered poolsettle.PayoutItem (`day,created_at_ms,txid,output_index,atoms,recorded_state`), including its original amount bounds. Its TxID/OutputIndex match the request, batch sequence selects the original DailyPayoutItem wrapper. These are separate remote observations and local wallet observations.

Native selection searches only retained First and Current pages. It deduplicates identical copies of the same batch/txid/output tuple (First==Current is ordinary), and refuses conflicting copies or absent selections. Caller never supplies amount/script. The adapter derives both from this exact retained capture/payment and performs the original bounded five-second wallet output observation. Publication/disclosure recheck the same capture pointer and original View. A stale remote capture may supply a dated selected row; it does not become current.

The local fields retain Set67 semantics: state unavailable/not_observed/unconfirmed/confirmed. Unavailable has error `wallet_unavailable`, zero wallet fields, wallet_synced false, positive observation time. Not_observed has empty error, wallet_synced true, valid nonzero wallet tip and nonnegative wallet height observation (height 0 is allowed), zero confirmations/empty block hash. Unconfirmed has the same with exact output found. Confirmed additionally has positive confirmations <= wallet_height+1 and nonzero lowercase64 block hash. No wallet observation changes remote payment state or proves spend authority/finality. Existing decoder input<=256 is preserved; this slice does not claim arbitrary large operator-batch observation.

## Authority, lifetime and contextual controls

Every new method uses the original attachment/controller View and expected local recipient/wallet/allocation binding. Own-wallet capture uses the original retained HostedAllocator2, with its existing pre/post View checks; no schema9 fallback, new wallet, new address, credential getter or signing right. No account lease spans wallet DB/RPC or a join. One activity task and one bounded cache are owned by the original attachment. Its max30-second task lifetime is distinct from caller Begin/read timeouts, ends on original life, and registers/rolls back under the original View fence. Stop cancels/joins and may retain dated data; Logout/Close join before wallet teardown and clear it.

Pure codec cases must distinguish syntax/shape from current View, exact captured position, retained tx membership and operation identity. Contextual negatives run through the real dispatch/owner seam, never classify a stateless request parser as knowing the current View/cache. Reference frames are conditional DTO controls, not account/ledger authority. Live native, helper/bridge, DOM and original paid-wallet acceptance remain separate gates. UI adds own activity/selected-output controls inside the existing wallet panel, with dated/read-only labels and no navigation, login or marketplace reorganization.
