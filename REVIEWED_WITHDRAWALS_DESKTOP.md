# Set68 candidate withdrawal desktop contract — proposal

November 19, 2026. External engineering proposal for root review; not frozen and not implementation authorization. No repository files, wallet state, chain state, expected vectors or generated goldens are changed by this document. Read with `set68-withdrawal-feasibility.md` and the separately proposed wallet and journal contracts. The existing candidate app layout, fixture recovery profiles and public chain rules remain authoritative.

## 1. Complete functional slice and fixed candidate limits

The existing pool balance panel gains destination and exact amount inputs, Prepare, native Review and send, and the current withdrawal status with explicit recovery/retry/stop actions. It does not gain a new navigation section. Only the two published fixture wallets are eligible. There is no arbitrary PSBT/raw-transaction input, generic send RPC, send-max, replacement/RBF, fee estimator or fee-rate UI.

Root-selected engineering constants: one standard destination of the existing44/49/84/86 forms (P2PKH, P2SH, P2WPKH, P2TR), canonical candidate-chain address of at most90 ASCII bytes and matching decoded script; exact positive recipient atoms; fixed1000 atoms/kB; absolute fee ceiling100000 atoms including any dust remainder; one-confirmation input minimum plus consensus coinbase maturity100; at most16 selected owned native84 inputs, at most4096 visited credit entries,128 retained operations. Change is native84 internal change from the same fixture wallet; no change output still consumes the upstream-authoring change cursor where that is the authoring result. Recipient amount never shrinks to fund fees. These are candidate engineering limits, not production fee or finality policy.

The app journal is the sole authority for committed signed bytes and delivery intent. Wallet signing produces private scratch only and neither publishes nor queues rebroadcast. A prepared wallet reservation remains active until the application commit/abandon state is reconciled. Signed inputs remain permanently excluded from later candidate operations even after observed submission, orphaning, a missing wallet row, lease expiry or later conflict.128 retained operations bound that candidate state; capacity refuses new intent while preserving all existing recovery.

A durable operation is unresolved in `intent`, `prepared`, `signed` or `dispatching`. `submitted` and `abandoned` permit a later new operation; neither means finality. Only an actually unsigned operation may become abandoned/release its unsigned reservation. `submitted` requires observing the exact saved signed transaction through the owned node; a send acknowledgment is separately recorded. An acknowledgment followed by unavailable/missing lookup leaves the operation unresolved, without automatic resend. A later observation never releases signed reservations or changes exact signed bytes.

Mining need not be stopped by this feature: the narrow wallet writer admits fail-fast, uses the existing synchronized wallet and returns busy when another wallet owner prevents safe work. It never toggles Start/Stop, resumes mining or creates blocks. Backup and child shutdown cancel/join the new operation owner before accessing or closing the wallet. The final joint design review remains separate.

## 2. Three identities and the original authorization

- **OperationID:** native-generated nonzero random32-byte lowercase hex, created before the first mutating helper call. It names one durable intent, draft, signed transaction and delivery history across process restart, logout and account generations. Intent fields cannot change under that ID. It never contains a token.
- **ActionID:** a fresh native-generated nonzero random32-byte lowercase hex for one explicit prepare, review/send, retry, abandon or observation task. The helper captures the exact original login token, account ID/generation, original ViewID and immutable child. The credential stays private. An action never substitutes a newly supplied token, even for the same account or after a token refresh.
- **ReviewID:** a fresh native-generated nonzero random32-byte lowercase hex, retained by native before asking the helper for a send/retry review. It binds the exact ActionID, OperationID, current original view, review kind, immutable draft digest and, for retry, exact committed TxID. JavaScript cannot supply a replacement review tuple or raw transaction to the approval method.

The existing `ManifestViewPin` is preserved byte-for-byte, including its real transport-trust derivation. A new `WithdrawalBinding` contains ordered fields `{view,source_binding_id,journal_id,withdrawal_binding_id}`. journal_id is the random established namespace ID from the parent initialization intent. withdrawal_binding_id is the journal contract's deterministic SHA256 binding of that random ID and the exact public Pin. Parent builds that Pin from retained account/fixture wallet identity, recovery profile, candidate chain and shared source; it never derives expected trust from a journal marker or operation reply.

The first dedicated withdrawal scope handshake discloses the trusted parent's established namespace through the original retained helper/concrete child. Native first matches its full View and source to the already accepted PoolSnapshot, reconstructs the expected complete Pin independently from the known fixture identity/account/chain/source, and recomputes withdrawal_binding_id using the disclosed journal_id. It then seals both IDs to that exact helper instance and original View. This is an explicit parent namespace-identity handshake; operation/action replies cannot select or replace their own expected pin. A new helper or session performs a new handshake, while preserving a known operation recovery tuple for exact comparison. No native filesystem reopening or wallet access is required. A restored operation belongs to the same pinned account/wallet/source even when its creation generation differs from the current View. New disclosure is fenced by the current original View; stored old authorization is not reused as a current capability.

A mutation action requires `gymaccount.Withdrawal`, including the existing reset hold and five-minute authentication freshness. Add Withdrawal to the parent's mutation-state gate: exact child/session/view, `signed_in && !operation`, and same account generation. `signing_in` and `resetting` cannot authorize it. View-only lookup, exact transaction observation, and stopping an entered task remain available under View during the hold. A new durable unsigned-abandon action also uses Withdrawal in this initial contract; stopping a task is distinct from authorizing that abandonment. Deterministic cleanup of private scratch and owned temporary resources requires no new authorization and never releases a committed signed reservation.

Parent login already has the verified principal's `AuthenticatedAtMS`; retain that immutable value privately with the child, rather than exposing or deriving it from a token. A review's expiry is no later than `min(original_authenticated_at_ms+300000, original_session_expires_at_ms, review_created_at_ms+300000)`. The review uses a half-open interval and cannot extend the existing fresh-authentication window. The existing authoritative WithSession check still runs at every commit/intent boundary. Native wall-clock countdown is advisory only. A review that expires, loses its view or enters the reset hold cannot approve. Use the existing explicit sign-out/sign-in flow to obtain a genuinely fresh login; token rotation is not fresh authentication. The same operation can then be reviewed through a new ActionID/ReviewID. A click must never silently refresh credentials.

## 3. Parent reset, owner lifetime and commit ordering

Current pool reset drains the child before `ApplyReset`; change that ordering before enabling withdrawal. Split detach/revoke/drain explicitly:

1. Atomically mark the parent operation, clear its published ViewID and capture the exact old child/token without waiting for the child. Reject all new Withdrawal callbacks immediately.
2. For verified reset, verify the fixture reset assertion and apply the durable account reset/revocation before any old-child Stop/Close join. On verification or reset-storage failure, leave the view invalid and fail closed; still cancel/join and close the captured old child outside all leases, preserving both reset and cleanup failures. Do not report a completed reset or reopen a child until the reset and prior cleanup have both succeeded. Logout similarly revokes its exact token before joining. Neither provider work nor account work runs under a controller lock.
3. Release all account/controller leases, cancel and join the old child, then close its retained wallet and persist/report cleanup failures. A revoked old action cannot newly commit signed bytes or a delivery intent while the join is blocked.
4. Publish/reopen a new child for the same wallet only after the previous owner has fully closed. A reset acknowledgment that proves durable revocation is not a claim that wallet cleanup has already finished. Existing parent operation states continue to communicate that distinction.

One withdrawal work slot belongs to the current wallet child. It admits fail-fast before wallet/journal work; no queue. A human dialog retains only bounded immutable review metadata, never this work slot, a wallet lock, account lease, DB transaction or goroutine blocked waiting for user input. The durable unresolved-operation rule prevents another preparation while review is open. Read-only lookup remains possible.

Prepare, sign, exact input revalidation, wallet DB operations, node calls and joins all occur outside account leases. Acquire any wallet/journal permit before entering the final original capability fence; never acquire or wait for their locks inside it. Stage/size-check exact journal records and complete preparatory file sync outside the fence. The final fence may publish the already-admitted bounded application journal record using the established atomic/durable primitive; this is not permission to run a wallet DB transaction under the account lock. The durable signed-commit and delivery-intent ordering points must be unambiguous. A possible write, failed final synchronization or revoked final check poisons that operation's acknowledgment as outcome-unknown; no signed bytes are exposed to delivery on an unresolved commit. Reopen/reconcile the exact operation, not a new wallet selection. No asynchronous publish-then-sync gap is treated as success.

A successful signed commit authorizes retention of exact signed bytes, not delivery. A separately durable delivery intent under the same original action's Withdrawal fence is required before the actual context-aware network attempt. If reset wins before that intent, no new app delivery begins. If a durable intent won first, cancellation can stop/join entered work but cannot promise recall of bytes already handed to the network. Any later **new** app delivery attempt requires a new, freshly reviewed unheld Withdrawal action on those same saved bytes.

### Upgrade guard and paired initialization

Before creating any withdrawal journal or wallet namespace, parent setup must exclusively persist and synchronize `shared/SLUG-POOL-WITHDRAWAL-V1.json`. The closed ordered object is `{application,version,source_binding_id,source,account_authority_id}`, where application is `slug-pool-withdrawal-upgrade`, version1, source is the complete original poolshare.Config, source_binding_id is its validated BindingID and account_authority_id is the retained account store authority. Store canonical json.Marshal bytes plus one LF,0600, with the existing owned-file/symlink checks and directory durability. Marker hashes below use SHA256 of canonical bytes without that LF. An existing exact marker is revalidated and synchronized; mismatched or uncertain setup fails closed. Set67's strict shared-root inventory rejects this previously unknown file before opening the wallets. The new code accepts only this exact new name/grammar, not arbitrary unknown root entries. Existing source, recipient, stable, child and career markers are unchanged.

The new approved directory is `shared/withdrawals`, containing only the two exact account-ID directories. Each account directory contains `INIT.json`, `READY.json` when established, and `journal/`; no arbitrary child names. INIT is the immutable ordered `{application,version,upgrade_guard_sha256,journal_id,pin,withdrawal_binding_id}` with application `slug-pool-withdrawal-init`, version1. Its random journal_id is generated once outside account leases and synchronized before journal creation. Pin is the exact journal ordered Pin; withdrawal_binding_id uses the frozen journal domain/length/canonical payload. READY is the immutable ordered `{application,version,intent_sha256,journal_id,withdrawal_binding_id}` with application `slug-pool-withdrawal-ready`, version1 and exact INIT hash. The same canonical/LF/owner-only rules apply.

Parent provisioning validates the guard and immutable INIT, creates/audits the exact empty app journal first, then creates or verifies the matching empty wallet namespace under the retained actual wallet, and synchronizes READY only after both are verified. This setup performs no signing, spending, allocation of a payment change address or mining. Setup for both known fixtures precedes publishing withdrawal-capable children; future children borrow the established scope and open/verify only.

Crash rules are explicit: no INIT means no journal/wallet initialization may have begun; use a new random ID only for genuinely absent initialization. With an exact INIT and no READY, resume only the original ID and initializer's bounded partial-creation protocol. A complete app journal with an absent wallet namespace may finish the first empty namespace creation. A matching existing wallet namespace is opened/verified rather than recreated. A wallet namespace with a missing/mismatched established app journal is always loss/corruption, not permission to generate a new journal. After READY, missing guard, INIT, journal marker/state or wallet namespace fails closed. Never reconstruct the guard or INIT from a surviving child component. These guards prevent supported binary downgrade/accidental namespace replacement; they are not a claim to withstand owner-controlled deletion/rollback of the complete filesystem.

## 4. Exact public review and native confirmation

`WithdrawalIntent` is `{destination,recipient_script,amount_atoms,fee_rate_atoms_per_kb,fee_limit_atoms}`. Destination/script are a canonical pair, fee rate is exactly1000 and fee limit exactly100000. The helper independently validates the candidate chain and standard script. UI/native amount parsing uses exact base10 integer atoms, not float64: no signs, exponent, thousands separators or more than8 fractional digits; amount is1..MaxMoney and final authoring rejects dust/unfundable requests. The private wire carries atoms, not amount text.

`WithdrawalDraftView` is the compact public projection of the owned wallet draft:

```text
{version:1,draft_digest:hash64,unsigned_txid:hash64,
 destination:string,recipient_script:hex,amount_atoms:int64,
 fee_atoms:int64,total_debit_atoms:int64,input_count:uint32,
 input_atoms:int64,recipient_output_index:uint32,change_atoms:int64,change_output_index:int32,
 prepared_at_ms:int64,observed_tip:hash64,observed_height:uint32}
```

`total_debit_atoms=amount_atoms+fee_atoms`; `input_atoms=total_debit_atoms+change_atoms`; every addition is checked within MaxMoney. input_count1..16; recipient_output_index is0 or1 and the exact lowest matching recipient role in final BIP69 order. Change output index is -1 only when no change output, otherwise0 or1 with positive change. The complete selected outpoints, previous scripts/amounts, derivation paths, consumed change cursor, exact unsigned bytes and owned previous-output metadata stay in the helper/core journal. There is no durable PSBT field; any signing packet is private scratch reconstructed from that immutable data. The digest binds those omitted fields via the frozen wallet/journal contract. A native84-only final signed TxID must match the checked unsigned transaction identity; signed bytes still receive their own exact hash and validation. No browser DTO is an opaque wallet signing handle.

`WithdrawalReview` is:

```text
{version:1,binding:WithdrawalBinding,operation_id:hash64,
 action_id:hash64,review_id:hash64,kind:"send"|"retry",
 draft:WithdrawalDraftView,txid:string,
 reviewed_at_ms:int64,expires_at_ms:int64}
```

For send, the operation is prepared and txid is empty before signed commitment. For retry, txid is the exact previously committed transaction and the draft is unchanged. The helper caches one exact original-authority review tuple; it does not hold a temporary WithSession recheck across the dialog. Any later signing/delivery action obtains a new synchronous check through the captured original session closure.

Native takes and validates this exact review from the original helper/view before presenting a native confirmation dialog. It displays the **full destination**, **exact received amount**, **exact network fee**, and **exact total debit**, each with8 decimal SLUG places, plus candidate-network wording. Change may be explained as returned to the same wallet; it is not part of the recipient amount or an additional charge. Default/cancel button is Cancel; affirmative button is Send exact payment or Retry same payment. There is no truncation of destination, floating-point formatting, editable approval field, default acceptance or JavaScript `approved:true` shortcut.

The native bridge retains the helper instance, original full binding, OperationID/ActionID/ReviewID, kind, digest, TxID, complete public draft and expiry through the dialog and until the approval handoff is resolved. After Yes, require that exact helper process instance and the helper's captured concrete old child instance are still the original owners, in addition to comparing the tuple and rechecking original authorization. Equal serialized view values from a replacement helper/child never revive a pending review; approve accepts IDs/digest only from retained native state. No also-latest operation, current destination textbox or latest helper snapshot can replace it. After No/closed dialog, retire the review action without signing; the durable unsigned operation stays prepared until explicit abandonment. UI wording is “Review cancelled,” not “payment cancelled.”

## 5. Finite private wire and native entry points

All objects are closed: all named fields required, nullable arms explicitly null, no duplicate/unknown keys or trailing JSON, UTF-8 valid, integer tokens only for integer fields, no negative zero. Correlation id is1..9007199254740991. Hash IDs are lowercase64hex and nonzero for OperationID/ActionID/ReviewID/journal/source. Times are0..253402300799999, positive when an event exists. Candidate heights are0..2147483647. Existing ManifestViewPin validators and exact source/profile checks remain in force.

Proposed request shape (all fields required):

```text
{id,version:1,command,view_id,operation_id,action_id,review_id,
 review_kind,draft_digest,txid,intent:WithdrawalIntent|null}
```

Selectors not named below are empty strings/null. Each command returns only the named arm.

| command | Required active selectors / capability | response |
| --- | --- | --- |
| withdrawal_snapshot | none; current View if signed in | snapshot |
| withdrawal_operation | view_id,operation_id / View | operation |
| withdrawal_action | view_id,operation_id,action_id / original View | action |
| withdrawal_prepare | view_id,operation_id,new action_id,intent / Withdrawal | action |
| withdrawal_review | view_id,operation_id,new action_id,new review_id,review_kind,draft_digest; retry also exacttxid / Withdrawal | review |
| withdrawal_approve | exact view_id,operation_id,action_id,review_id,draft_digest; retry also exacttxid / original captured Withdrawal | action |
| withdrawal_stop | view_id,operation_id,action_id / original View | action |
| withdrawal_abandon | view_id,operation_id,new action_id,draft_digest if prepared / Withdrawal | action |
| withdrawal_observe | view_id,operation_id,new action_id,exact committedtxid / View | action |

Review kind is send or retry only on review. Approve obtains kind from the retained exact review, not another caller selector. A duplicate review request with the same original ActionID/ReviewID and byte-identical tuple retrieves that unchanged review and never extends its expiry; different fields conflict. After an uncertain review acknowledgment, native first looks up its exact ActionID and may retrieve that exact already-registered review, never substitute the latest one. An approval replay on the exact already-entered ActionID returns its existing action without another sign or delivery intent. Native does not automatically replay approval after an uncertain handoff. The native affirmative callback is one-shot and checks the exact still-live review before dispatch; a late/double callback or a callback after Stop/No/expiry cannot approve. Stop cancels only the named entered action or retires its exact idle review; it never cancels a different operation. It is not durable unsigned abandonment. Observe resolves only the saved transaction and cannot be an arbitrary transaction proxy. It never sends bytes. Prepare with a known OperationID returns the exact already-saved intent/operation only if every immutable intent field matches; it must not allocate change or select inputs again. A conflicting intent is operation_conflict.

Response shape:

```text
{id,version:1,ok,snapshot:null|WithdrawalSnapshot,
 operation:null|WithdrawalOperation,review:null|WithdrawalReview,
 action:null|WithdrawalAction,error}
```

Exactly one arm exists on ok; error is empty. Failure has no payload and error is one of invalid_request,unauthorized,stale_view,held,reauthentication_required,busy,capacity,withdrawal_unavailable,withdrawal_not_found,operation_conflict,draft_stale,insufficient_funds,fee_limit,outcome_unknown. Do not leak wallet/RPC/provider error strings, file paths or credentials. An unknown mutation acknowledgment always names the retained native OperationID/ActionID; error alone does not imply a payment succeeded or even that network handoff happened.

Native methods are proposed as:

```text
WithdrawalSnapshot()
BeginWithdrawal(viewID,destination,amountText)
WithdrawalOperation(viewID,operationID)
WithdrawalAction(viewID,operationID,actionID)
ReviewWithdrawal(viewID,operationID)  // native generates IDs; opens exact dialog
RetryWithdrawal(viewID,operationID)   // fresh native review of saved signed bytes
StopWithdrawal(viewID,operationID,actionID)
AbandonWithdrawal(viewID,operationID) // new explicit action, unsigned only
ObserveWithdrawal(viewID,operationID) // exact stored tx, no arbitrary TxID from JS
```

Native owns fee constants and all mutation/review IDs. The private helper protocol may contain the more complete tuple, but Wails does not expose a browser-controlled approval method. These are an additive candidate-pool family; legacy solo routes and payout/evidence/recovery frames are unchanged.

## 6. Operation, action and recovery state

`WithdrawalOperation` is the public immutable-intent projection plus monotonic journal stage:

```text
{version:1,operation_id:hash64,journal_id:hash64,withdrawal_binding_id:hash64,
 sequence:uint32,revision:uint32,created_at_ms:int64,updated_at_ms:int64,
 prepared_at_ms:int64,stage:string,intent:WithdrawalIntent,intent_digest:hash64,
 draft:WithdrawalDraftView|null,txid:string,signed_sha256:string,
 dispatch:WithdrawalDispatch|null,observation:WithdrawalObservation|null}
```

Exact stage names align with the journal proposal: intent/prepared/signed/dispatching/submitted/abandoned. A record at intent has no public draft/signed identity. Prepared has an immutable draft, no committed signed identity. Signed and later have an immutable draft, TxID and signed-byte SHA. Abandoned cannot contain an authorized signed commitment. sequence is1..128, revision starts1 and is monotonic uint32. Overflow refuses a mutation and preserves reads; no lower artificial retry cap is added. created_at_ms is from the immutable intent, updated_at_ms from the journal's staged publication sample, and prepared_at_ms is0 before the first app draft publication and immutable afterward. It is not wallet allocation time; the same value supplies DraftView.prepared_at_ms. The compact Intent maps core recipient_atoms to amount_atoms and max_fee_atoms to fee_limit_atoms; all other common fields are identical and all fixed purpose/account/min-confirmation constants remain checked. intent_digest is the exact core canonical-intent digest, never a digest of this smaller projection.

`WithdrawalDispatch` has ordered fields `{attempt,authorization,intent_at_ms,result,result_at_ms,error}`. attempt is positive uint32; overflow refuses a new intent. authorization is the exact ordered `{action_id,review_id,view_id,generation,authorized_at_ms}` from that persisted attempt. result is pending/acknowledged/unknown/rejected; pending has result_at_ms0/error empty. Only the latest attempt is retained, not a claim of complete delivery history. Stored old-view authorization is audit metadata and never current permission. An acknowledgment is not inclusion.

`WithdrawalObservation` has exact ordered fields `{version,txid,observed_at_ms,state,node_tip,node_height,confirmations,block_hash,error}`. Version1 and TxID equals the exact committed transaction. Unknown is a dated failed attempt with node_tip/block_hash empty, height/confirmations0, error node_unavailable. Other states have emptyerror and a nonzero stable verified owned-node tip. Not_observed requires the narrowly recognized missing result under that stable tip and zero confirmations/empty block_hash. Mempool requires exact saved signed raw bytes, zero confirmations/empty block_hash. Confirmed requires those same bytes and a canonical reported block under the stable tip, with positive confirmations exactly tip_height-block_height+1. Body/case/serialization changes refuse even if TxID matches. This is dated evidence, not irreversible finality or unspentness. It never modifies the signed transaction or its reservations.

`WithdrawalAction` is:

```text
{version:1,binding:WithdrawalBinding,operation_id:hash64,
 action_id:hash64,kind:"prepare"|"send"|"retry"|"abandon"|"observe",
 state:"running"|"review"|"done"|"failed"|"stopped"|"outcome_unknown",
 review_id:string,error:string,operation:WithdrawalOperation|null}
```

Review is a live metadata-only action awaiting the native choice. Approved send/retry moves that exact action to running; a fresh action is required after expiry/revocation/reopen. Terminal action fields do not mutate while retained. Done means the named task completed, not “payment final”: display its operation stage. Stopped means work has joined; if signed/delivery state exists, UI explicitly says payment may already have been submitted and remains recoverable. A timeout or rejected send cannot become abandoned or release signed inputs. Failed/outcome_unknown retains the original operation identity and latest safely read journal stage; if authorization is revoked, disclose no payload to that dead view.

`WithdrawalSnapshot` is `{version,state,error,binding,active_action_id,last_action_id,unresolved_operation_id,latest_operation_id,can_prepare,can_review}`. Unavailable means error withdrawal_unavailable, binding null, all IDs empty and both booleans false. Idle/busy snapshots have original current binding and emptyerror. IDs are hints for explicit current-view lookup, not proof a native action completed. In a new session, discovering the account's unresolved ID is explicitly “Recovered withdrawal”; it cannot finish an old native action or inherit its authority. Capability booleans are advisory and never replace final checks.

Native reserves the exact action identity before preflight and marks whether dispatch was reserved. A definite local refusal may retire only an exact never-dispatched intent. A local busy result for an earlier reserved/uncertain action must not discard it. An uncertain response is reconciled by exact ActionID first; if action metadata was lost by helper restart, query the known OperationID and compare the exact saved intent/digest/TxID. That is recovery/read, not automatic replay. No generic latest success can satisfy a new request, even if recipient and amount match. A new session uses a new ActionID/ReviewID and its own original token to explicitly act on the recovered same operation. Durable operation lookup never silently binds an old action to that new token.

## 7. Bounds, deadlines and cleanup

All byte limits below include a final LF at the outer frame; nested limits are canonical JSON bytes. These are admission/accounting bounds, not measured hard Go heap caps.

| object/resource | bound |
| --- | ---: |
| request frame | 2048bytes |
| response frame | 16384bytes |
| WithdrawalBinding | 4096bytes |
| WithdrawalIntent | 512bytes |
| WithdrawalDraftView | 2048bytes |
| WithdrawalOperation | 8192bytes |
| WithdrawalReview | 8192bytes |
| WithdrawalAction | 14336bytes |
| WithdrawalSnapshot | 5120bytes |
| WithdrawalObservation | 2048bytes |
| one helper/native public metadata reservation each | 65536bytes |
| one aggregate helper withdrawal work reservation | 33554432bytes |
| concurrent work owners per wallet | 1; no queue |
| retained durable wallet/application operations | 128; no pruning to bypass capacity |

The metadata allowance covers one selected operation, one review, and at most two small action records (current + last). Do not duplicate full private transactions in those DTOs. Keep one current native action/review tuple and its known durable operation; replacement cannot erase unresolved correlation. Private wallet/core caps are: draft16384bytes, unsigned raw1024bytes, signed raw4096bytes, canonical wallet/application operation32768bytes, and complete application state8388608bytes/128 records. One canonical state carries clock_high_water_ms; there are no per-operation files, mutable side head or independent index to trim. This reservation spans the desktop adapter and wallet work: at most4096 visited credit entries with only bounded22-byte native84 scripts copied, at most4MiB of decoded prior-transaction bytes per transaction and16MiB aggregate decoded prior-transaction bytes, plus the bounded selected16-input packet/signature/serialization and journal staging components. It is not a measured total Go heap cap and must not conceal an unbounded full-wallet allocation; the32MiB aggregate work reservation is admitted before source traversal/serialization and released only after signing/network/file cleanup joins. Existing payout, stable, history and evidence budgets are unchanged; busy refusal is permitted where their wallet operations contend.

An entered prepare/send/retry/abandon task has a30-second context linked to child lifetime. Each node send/lookup call is at most5seconds; the combined delivery/observation portion is at most15seconds inside that same30-second action. Explicit View-only observation is at most15seconds with each call at most5seconds. Human review has only the bounded five-minute freshness expiry and no entered-work goroutine. The review/awaiting-confirmation state does not start or consume the30-second work deadline; approval starts a fresh bounded work context only after the original review is accepted. Use a window-attached native question sheet with explicit button callbacks (the existing Wails v3 dialog API), retain no application locks around Show/callbacks, and invalidate the captured review on owner change even if a late native callback still fires. Local wallet calls that lack context must be limited by16 inputs/4096 visits and joined; a timeout is never implemented by abandoning a goroutine that still holds the wallet. The normal30-second private transport deadline covers admission/lookup, not waiting for a user dialog. Native dialog handling must release UI/operation mutexes while waiting, allow account reset/close to revoke the action, and discard a late Yes. Hold no account/controller lock while waiting for the dialog or an owner drain.

Stop, backup, logout, reset and Quit join active work before releasing its scratch permit or wallet/source handles. They do not automatically retry delivery or convert signed uncertainty into unsigned cancellation. Restore/export and withdrawal authoring/signing are mutually exclusive. New child installation cannot bypass an old owner's work reservation or an unresolved durable operation.

## 8. Required finite acceptance before implementation is called complete

Freeze contracts and public source inputs before generating any expected transaction/digest/wire bytes. Independent producer outputs must cover exact reviewed recipient/change/fee (including dust-to-fee), native84 input set, final signed bytes/TxID and canonical private messages. The wire producer must derive inherited transport trust correctly; synthetic IDs cannot bypass existing View validation.

Required meaningful cases: lost prepare reply recovers exact OperationID/draft; no second change allocation; native confirmation changes/expired/No/dead-view cannot sign or send; reset durably advances during blocked prepare/sign/network and wins before the respective fenced commit; signed/delivery commit uncertainty poisons and recovers exact bytes; local gate busy retires only a never-dispatched action; concurrent same-ID/reserved dispatch remains recoverable; later sessions can View the same operation during a hold but cannot retry until fresh/unheld; stop after delivery never claims recall; permanent signed exclusions survive restart and later missing/conflict observations; capacity refuses the129th operation before wallet mutation; no arbitrary PSBT/transaction proxy; actual normal-PoW owned-chain withdrawal spends a real settled native84 payout and proves exact recipient output and one debit after lost-ack retry/reopen.

No user policy question is introduced here. Root must verify the combined frozen wallet/journal/desktop bytes and their sizing proof before freezing this document. Implementation, golden generation and source edits wait for that freeze.
