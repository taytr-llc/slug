# Set 57 — automatic pool mining and durable recovery

Branch: `slug-pool-worker`, stacked from completed Set 56 `e0832882`.

## What changed

Added a separate automatic `PoolWorker` and version-1 `PoolOutbox` with explicit
source, gym and transport binding. The worker consumes the pool's original job
and template, fixes extra nonce at zero, and scans only the uint32 nonce range.
Every found outcome is durable before transport, including a hash completed
during cancellation. Stop joins active work; restart keeps the same job/nonce.

Pool credit, block publication and fighter admission have separate outcomes.
Prospect-only work bypasses pool submission; pool-only work creates no fighter.
Block wins publish first under D69. Lost responses recover through original
receipt lookup. Pending sequence-zero block intents are not earnings. D80 late
pool credit never signs an expired new fighter; previously signed work can
recover a historical admission with its original birth.

The outbox reserves future authorization and receipt growth before hashing.
Capacity pauses new work while retaining recovery. Review found and fixed a
case where a byte-capacity pause could issue another unusable job after expiry:
the running worker now latches that pause until reservation succeeds or the
owner explicitly closes/reopens it. Uncertain writes poison the handle until
explicit reopening and audit. No automatic eviction or evidence migration exists.

Added `NewManifestAdmissionServer`, which shares the pool's concrete manifest
issuer but disables outside job issuance. Its unchanged admission rules still
validate fighter work independently; the pool alone checks its exact template
and payout for credit. There is no implication that a fighter receipt proves a
pool payout. See `POOL_WORKER.md` for lifecycle and storage boundaries.

## Deliberate non-changes

No consensus, proof targets, genesis, job lifetime, accounting/settlement policy,
authority keys, frozen fixtures or wallet derivations changed. Existing solo
workers, journals and desktop routes remain intact. No real private key was
created or read; tests use explicitly published fixtures only.

No pool is deployed publicly and no desktop route is switched in this set.
Shared desktop runtime, recipient binding, balances and the daily scheduler
are the next functional integration. D74's thorough joint design review stays
last, before friends/tester or public product release.

## Verification

Evidence is under `../artifacts/set57/` using the pinned Go 1.27.4 no-CSE profile.

- First compile passed for worker, prospect service and pool service packages.
- Focused worker tests passed, covering pool-only/prospect-only separation,
  cancellation drain, same-nonce recovery at capacity and source rebinding.
- Admission-only tests passed: disabled issuance never reaches authentication,
  body reads, node resolution or storage, while original receipt recovery and
  revocation remain enforced.
- Pure outbox tests passed, including owned copies, atomic cursor/find,
  uncertain synced writes and reopening, capacity recovery, nonce exhaustion,
  identity isolation, corruption and canonical decoding.
- Actual normal-PoW HTTP composition passed against two owned loopback nodes,
  a real fixture account and shared pool/admission issuer. The first committed
  pool response was deliberately lost. Reopening a capacity-one outbox recovered
  exactly one pool credit and one fighter with the same job, nonce and birth:
  one issuance, one share submission, one signing operation, zero new mining
  attempts during recovery. Retained proof verification still rehashes work.
  `actual-attempt2` passed before the capacity fix; `actual-attempt3` passed
  against final source after that fix (4.826 seconds including the wrapper).
- Final focused checks in `validation-attempt2` passed: normal 7.371 seconds,
  race 23.253 seconds, cheap-profile refusal 4.864 seconds and vet 0.491 seconds.
  These include D69 broadcast ordering despite pool failure, D80 expired winner
  suppression, historical signed-fighter recovery and the byte-capacity expiry
  regression. Full-repository gofmt and `git diff --check` are clean.

Preserved failed attempts: `actual-attempt1` stopped in test setup because the
fixture account schema requires both published principals; the correction added
the unused second principal without changing production behavior. The first
pure-outbox and final-check attempts caught a test using the frozen witness
vector's nonzero extra nonce for pool-found work. The correction keeps that
witness template for copy-isolation checks and uses the original independently
frozen extra-nonce-zero coinbase vector for found-work checks. No expectation
was regenerated from Go output or assertion weakened.

## Deployment and remaining work

The changed code is a worker/service integration boundary. Dependency checking
confirms none of `prospectworker`, `prospectservice` or `poolservice` is linked
into the daemon; no deployment configuration changed. No seed rollout is
needed. Master and the live seed remain at Set 50 `592e143c`. The last qualified
desktop bundle remains Set 51 until the shared pool runtime is connected and
packaged. No new product policy decision was needed for Set 57.
