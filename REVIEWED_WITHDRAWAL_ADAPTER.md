# Set68 owned-node and retained-wallet adapter proposal

Root review draft, November19. This is the concrete bridge between the frozen
wallet seam and the reviewed app journal. It does not introduce another wallet,
generic RPC route or source of account authority.

The adapter borrows only PoolChild's existing wallet, literal127.0.0.1 RPC config,
full-node client and source lifetime. No browser-supplied endpoint/address path,
raw transaction or previous-output list reaches this constructor. Retain original
wallet identity/profile/source and the journal's independently established random
namespace/deterministic binding. It owns a fail-fast work permit; wallet mutex,
DB and all network work remain outside account leases. Close cancels and joins
before unloading the wallet; neither a timeout nor reset abandons a goroutine.

The new wallet handle is created/opened only through the paired durable setup
intent/READY, after the shared-root downgrade guard is synced. Reconcile exact
journal rows against wallet records before enabling any new selection. A missing
established journal, wallet namespace, operation or retained reservation is an
error. Temporary prepared reservations are never cleared just because a response
or transaction is missing. Retain and Abandon accept only the concrete journal's
owned reconciliation handle under its operation permit. Reset-held lookup may
complete already-authorized local reconciliation; it cannot create new signing
or delivery authority.

Preparation holds wallet lifetime while calling the bounded unsigned seam. It
returns an owned exact draft; no newly computed output substitutes an existing
operation. Before publishing an actionable review and before signing, validate
its exact selected inputs against the owned node. Require stable verified
network/genesis/height/hash around the read, matching synchronized wallet tip,
canonical original input block hashes, exact native84 scripts/atoms/coinbase flag,
minimum confirmation1 and consensus maturity100. Inspect all selected outpoints
in one bounded gettxspendingprevout call in addition to gettxout: this node's
gettxout alone does not hide canonical outputs spent by mempool transactions.
A mempool/tip race can still make later delivery fail; no atomic network lock or
permanent spendability claim is made. Stale/refused inputs do not trigger new
coin selection or alter the reviewed transaction.

Signing uses the fixture wallet's existing short unlock behavior, preserving
its prior locked state. The wallet signs only its saved ID/digest and returns
bounded scratch. Independently validate exact unsigned equivalence, prevouts,
outputs/fees and every witness before staging the signed app record. Original
Withdrawal authority fences that journal publication; wallet retention follows
outside the account lease. No wallet PublishTransaction call, unmined-record
insertion or notification/rebroadcast queue is used before delivery intent.

The node adapter has only internally called inspect, submit-exact and observe-
exact methods. Requests use context, no proxy, redirect, transparent retry or
compression. Each RPC is at most5seconds; the complete delivery/observation is
at most15seconds within the30-second task. Cap response framing before parsing,
reject duplicate keys, nonminimal transaction lengths and oversized arrays. A
raw transaction is at most4096 bytes,16 inputs and2 outputs, checked before wire
allocation. Never include RPC credentials/backend strings in public errors.

Submit receives only the owned journal delivery handle for a newly persisted
original-action intent. It transmits the exact saved bytes once and verifies an
exact matching txid acknowledgment. Acknowledge, refusal and uncertainty are
separate observations; none creates another payment or permits changed fees.
No automatic resend occurs on missing state, startup, a timer or a new login.

Observe receives the same retained signed bytes/txid, never arbitrary browser
selectors. A fixed txindex-enabled owned node can return the exact raw bytes in
mempool or a block. Compare bytes, not just txid (native witness bytes do not
change txid). Confirmed observations also check the reported block's canonical
height/hash and exact confirmation count at the same stable tip. Exact typed
-5 missing response is not_observed only when identity and before/after tip
checks pass; disabled index, RPC failure and malformed data are unavailable.
A successful exact mempool/canonical observation ends the unresolved operation;
it does not release signed-input reservations or imply finality. Later loss or
conflict is a new dated observation; saved signed bytes remain immutable.

Normal wallet spend notifications should learn an authorized direct send,
including its owned change. Reopen/rescan is the recovery path for missed wallet
notifications, not a second payment. Actual acceptance must verify this on both
published fixture wallets and exact owned-node inclusion, including lost send
acknowledgment followed by reopen/retry of the same bytes and one debit.

Exact RPC framing: each HTTP response is at most65536 bytes, JSON depth at most8,
object keys at most64 per object, arrays at most32 entries, decoded strings at
most8192 bytes and numbers at most64 bytes. Duplicate decoded keys and trailing
values fail before typed allocation. The outer RPC envelope is closed, requires
matching integer id, result and error, and permits only optional jsonrpc1.0/2.0.
Node result objects retain documented upstream extension fields inside these
bounds; required fields are checked separately. gettxspendingprevout returns
exactly one row per original selected outpoint in the requested order. Confirmed
transaction observation fetches raw getrawtransaction(false) first (exact bytes),
then its bounded verbose record for location; it never decodes untrusted wire
counts. gettxout proves the selected script/value/coinbase/confirmations and the
stored original block is checked by height. Every inspect/observe encloses these
reads in verified network/genesis/activation/stable-tip snapshots. Known RPC
errors are reduced to local codes. Only -5 with the node's exact own-tx missing
message means not_observed; only transaction rejection codes -22/-25/-26 mean
rejected delivery; all other transport/response outcomes remain unknown.

The wallet setup addendum supplies absent-only Open semantics and audited Empty.
Before READY, an existing wallet namespace requires an already complete exact
empty journal. No component is repaired from a surviving counterpart. During
ordinary child open the complete journal is audited and all retained operations
are reconciled under the concrete owner before new selection. Intent-only rows
may have no wallet record or one exact prepared record after a lost preparation
reply; that record is recovered by the controller without another allocation.

The adapter uses core ParseDraft/DraftDigest/ValidateSigned for independent schema
and signed-byte validation and preserves typed wallet not-found for lookup. It
never interprets acknowledgment as finality. These caps and semantics are frozen
before implementation; locked wallet/journal Python vectors supply byte expectations.
