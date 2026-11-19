# Automatic pool worker

Set 57 adds `prospectworker.PoolWorker` and a separately initialized
`PoolOutbox` version 1. Existing workers and outbox versions remain readable
and unchanged. No production key, account, descriptor or genesis is activated.

`PoolConfig` pins the complete pool source, public gym key, pool origin/trust,
and a separate admission origin/trust using that same manifest authority pin.
Neither configuration nor journal contains a bearer token or private key.
Runtime adapters own credentials and must preserve their declared identity.
`NewPoolWithHooks` retains the existing synchronous account-cycle and verified
receipt-observer boundaries. Only an explicit caller of `Run` starts mining.

The source supplies both the issued job and exact block template. The worker
reconstructs the pool payout with extra nonce zero and changes only the uint32
nonce. Exhaustion requests another job. It never replaces the payout with the
gym wallet address or edits the pool template. Normal Argon2id is mandatory;
cheap-profile builds refuse creation, opening and execution.

The immutable scan reaches disk before hashing. Before each attempt the
outbox reserves space for the largest possible found record and its remaining
authorization/receipt growth. A qualifying nonce and advancement synchronize
in one batch. Cancellation joins one already-started hash, saves its result,
then stops without starting transport or another attempt. An interrupted miss
may be hashed again; an interrupted found submission retains its exact identity.

Pool, prospect and block targets are independent. A pool-only result creates
no fighter record. A prospect-only result takes the admission path without a
pool submission. Block wins broadcast before pool receipt recovery, signing or
admission, even if they miss either other target. Local canonical observation
does not itself award pool earnings or fighter birth.

Pool delivery first looks up the original job and nonce. Only an explicit
missing result or retained pending block intent permits submitting that same
pair again. Uncertain responses preserve evidence. Accepted receipts retain
their first binding; a sequence-zero block receipt remains pending and is never
reported as earnings. A pool failure does not suppress fighter admission.

Fighter authorization synchronizes before admission. Unsigned expired work
is marked expired without invoking the signer. Already signed pending work may
recover a historical receipt; the unchanged authority rejects any new expired
admission under D68/D80. The pool's late canonical-winner exception confers no
fighter exception. Verified fighter receipts synchronize before observers run.

`NewManifestAdmissionServer` exposes only the existing v3 admission/recovery
route for a pool-owned issuer. Its job route is disabled before authentication,
body reading or issuance. This permits both services to share the same concrete
manifest store while keeping issuance exclusively inside the pool work journal.
Owners drain both servers and worker calls before closing borrowed resources.

The outbox has its own `SLUG-POOL-WORKER.json` marker and LevelDB directory.
Opening audits canonical bounded public records, reciprocal indexes and byte
accounting. Stored digests remain claims until pending delivery rehashes exact
work. Capacity pauses new mining; it cannot prevent finishing retained records.
There is no eviction. Explicit capacity increases are allowed on reopen within
the supported bounds; source and transport identity never change. An uncertain
write poisons the handle until explicit close/reopen and audit.

The shared desktop runtime, native-SegWit recipient enrollment, visible pool
balances and daily settlement scheduler follow this worker. The current app
route is not silently switched. D74's joint design review remains the last
release gate once the functional flows are ready.
