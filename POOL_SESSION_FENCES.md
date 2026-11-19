# Pool session and commit boundaries

Set 60 on `slug-pool-session-fences`, stacked from Set 59 `48092142`, removes
normal proof work and node/source waits from the pool's account leases. A slow
operation for one gym can now leave the account authority available for another
gym or a verified reset. The original session still protects each actual local
commit and result disclosure. This is an engineering integration contract, not
a change to reset policy, mining eligibility, accounting or release approval.

## Original session authority

`gymaccount.Store.WithSession` and `prospectservice.SessionAuthorizer` keep their
existing contract: the authorizer serializes reset/revocation against its entire
callback. Its temporary `recheck` is synchronous and invalid after that callback.
Do not make that callback run unlocked or retain its `recheck` for later work.

`internal/mininglease.Capture` briefly authorizes and retains the operation
context, the exact original bearer token and the enrolled public key. It does
not fetch a replacement token or persist credentials. The concrete account
authorizer continues to enforce the token's account, generation, expiry and
capability; the helper additionally requires the same key at every fence.

- `Check` obtains a fresh short lease for that original credential. It authorizes
  disclosure at that boundary, not a future mutation.
- `Do` reenters the same credential and holds the authorizer's lease through its
  callback and final recheck. Durable mutations belong inside this callback.
- Missing, repeated or malformed authorizer callbacks, changed keys, swallowed
  callback failures and failed final checks fail closed. A failure after the
  protected callback may have run is conservatively outcome-unknown.

Neither method can promise that a socket reader receives a result before a
subsequent reset. Publication linearizes at the final original-session check;
network writes happen after bounded encoding and after operation permits are
released. No result is republished under a successor token or account view.

## Pool operation boundaries

`poolshare.Pool` keeps its existing fail-fast operation permit for the complete
call. The permit protects pool counters, retained work and duplicate identities
while the account lease is released. The existing 30-second operation context,
normal-PoW requirement, journal/configuration formats and capacity limits remain.

| Work outside the account lease | Work inside a short original-session fence |
| --- | --- |
| Template resolution, proposal validation, current-parent and canonical-block observations, block RPC transmission | Pool clock highwater writes and final local work/share/delivery batches |
| Normal nonce checking and historical share verification | Final timestamp/expiry checks synchronized with their receipt batch |
| Issuer metadata/job reads and exact candidate preparation | Staged registry issuance commit, using its fail-fast registry lock |
| Owned result preparation | Original-session result disclosure checks |

Issuance resolves an owned template, prepares a registry issuance handle and
checks node context outside the lease. It commits the job inside `Do`, then
performs proposal/context checks and source preparation outside the lease before
the pool work batch. The issued job's original timestamp remains its birth
boundary. A later failure can leave a real issuer job without acknowledged pool
work; it does not justify replacing or silently recycling that issuance.

Submission keeps the original job and nonce. Source reads precede the final
fence: `saveSubmission` receives the exact issued job instead of acquiring an
issuer lock while holding the account. `saveWork` likewise receives previously
verified preparation. Local encoding, index checks, final clock sampling and
synchronous writes remain inside the lease. A clock-only observation does not
turn a known low-work or expiry refusal into accepted work.

A winning block's exact durable intent still precedes its transmission. Node
transmission and canonical checks run outside the account lease. Reset during
those calls can prevent a later delivery/credit update or disclosure; retained
intent remains available for exact recovery. A transmission or prior durable
work/share mutation makes a subsequent failure outcome-unknown. Uncertain local
writes still poison the pool handle and require reopen. D69 block-first behavior,
D80's canonical late-winner exception and the separate fighter birth rules are
unchanged. See [POOL_WORKER.md](POOL_WORKER.md) and
[POOL_ACCOUNTING_POLICY.md](POOL_ACCOUNTING_POLICY.md).

## Staged registry mutation

The registry adds trusted, opaque preparation handles rather than accepting
caller-supplied verification results or receipt JSON:

| Preparation outside the account lease | Commit inside the account lease |
| --- | --- |
| `PrepareManifestIssue(context, publicKey)` | `CommitManifestIssue(prepared, check)` |
| `PrepareManifestAdmission(proof, authorization)` | `CommitManifestAdmission(prepared, eligibility, check)` |

Preparation may wait for the registry mutex and perform proof/signature work,
appearance binding or entropy acquisition. It neither samples the registry
clock nor writes registry state. Handles bind the concrete open store, complete
authority pin and descriptor. Admission proof access returns owned Merkle
siblings. A handle is not serializable authority and cannot cross store reopen.

Both commits use `TryLock`. `registry.ErrBusy` means no registry change occurred;
release the account lease before retrying. This prevents a final account fence
from waiting behind a stable reader that holds the registry mutex while hashing.
Do not acquire an account lease from a registry-held eligibility callback: that
would reverse the account-to-registry order and can deadlock.

Commit checks the live store, immutable job and duplicate state again without
rehashing. It samples issuance/acceptance time and enforces the original expiry
at the write boundary. The supplied `check` may only recheck the already-held
account lease; it must not call a node, wait on another service or reenter the
registry. A copied issuance handle cannot issue twice after a successful commit.
A changed admission duplicate returns `ErrConflict` for preparation of the same
proof again outside the lease. Historical recovery verifies the original record
during preparation and confirms its exact bytes/index during commit; it keeps
the original birth, acceptance and eligibility without a new node decision.

These methods supplement `IssueManifestJob` and `RecordManifestJobChecked`.
Existing methods and historical storage formats retain their established
semantics. The registry remains a trusted local component, not a public account
authority; callers still supply trusted context and node eligibility.

## Explicit admission-only service

`prospectservice.NewFencedManifestAdmissionServer` selects the staged flow. It
requires a real session authorizer, rejects static credential configuration and
typed-nil authorizers, and retains the existing protocol-3 admission/recovery
route and strict request bounds. The jobs route is disabled before body reads;
the pool alone creates jobs in its dedicated issuer.

The service captures the original credential, prepares and verifies the proof
outside its lease, and obtains trusted node eligibility outside it for a new
receipt. Its final `Do` commits the prepared record under the original session.
Historical recovery skips new-birth eligibility but still requires final live
authorization. Registry busy/conflict is retryable `busy`; authentication fails
closed; a failure after a possible commit is `outcome_unknown` with no receipt.
Encoding/cancellation after success also remains unknown. No wire version,
receipt schema or error-envelope shape changes.

One parent must own **one admission writer** for this issuer. Its serialized
`MaxRecords` check depends on that ownership; the companion pool writes jobs,
not admission records. Do not attach a legacy or second admission writer to the
same shared runtime. `NewManifestServer`, `NewManifestSessionServer` and
`NewManifestAdmissionServer` keep their prior behavior for their existing
isolated uses; the new runtime must choose the fenced constructor explicitly.

## Balance reads and owner lifetime

`poolsettle.MinerBalance` retains the settlement operation permit, captures the
original session, then verifies the concrete round/share source without holding
the account lease. It copies only that gym's balance and performs a final
original-session check before disclosure. The stored observation time is
preserved: a read is neither a fresh chain observation nor a settlement run.
No other miner, funding input, destination table or signed transaction is exposed.

Service authentication must stay outside calls to pool or settlement methods,
which take their own brief leases. A commit fence must contain no normal proof,
node/wallet/network call, blocking source read or helper drain. Account authority
and borrowed issuer/journals must remain alive until their owning operations and
listeners drain. Reset revokes session authority; it does not erase prepared
transactions, share intents or already committed evidence.

Pool HTTP preserves conservative mutating failure classification, including the
new internal `poolshare.ErrOutcomeUnknown`. Nonmutating failures disclose no
partial result. Existing clients continue to recover the same job/nonce or
signed admission proof. Validation and preserved failed attempts are recorded
in [CHANGESET_60_NOTES.md](CHANGESET_60_NOTES.md).

## Next runnable integration

The next milestone is an explicit shared fixture desktop runtime: one parent
node pair/issuer/pool/round/settlement owner, two actually enrolled fixture
wallets with Set58 recipient bindings, per-gym PoolWorkers/outboxes and Pin2
stables, authenticated visible balances and a bounded daily settlement scheduler.
Use a new storage/route namespace; preserve the existing solo app and its data.
Only explicit Start begins mining. Account switching/reset drains the current
gym child without replacing the shared source; Quit joins children, scheduler,
listeners and stores before stopping nodes and closing accounts.

Acceptance must demonstrate the runnable app, separate fighter admission, sparse
per-gym inventory, account isolation, retained pending work and an actual
maturity-funded fixture-wallet payout with retry recovery. Shared event/career
attachment remains explicit rather than repinning old histories. This work
does not activate production custody, permanent genesis, friends' wallets or
the final joint D74 design review. The detailed engineering proposal is the
workspace [shared runtime plan](../shared-pool-runtime-plan.md); this document
records the implemented Set60 boundaries where that earlier proposal differs.
