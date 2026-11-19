# Set 60 — keep resets responsive during pool work

Branch: `slug-pool-session-fences`, stacked from Set 59 `48092142`.

## Problem and behavior

The original session API deliberately holds the global account gate until its
callback returns. Pool issuance/submission, manifest admission and balance
verification ran slow node requests and proof hashing inside that callback.
One stalled miner could therefore delay another gym's account access and a
verified reset for the duration of the request.

Pool operations now capture the original token/key briefly, perform slow work
outside the account lease, and reacquire that same credential for each bounded
local write or final disclosure. The pool operation permit still serializes its
own journal. A reset that finishes during node/proof work prevents subsequent
commits under the old credential. A reset arriving during a final admitted
synchronous write waits for that write, then revokes the session. No replacement
token or generation is substituted inside the operation.

Registry preparation fully verifies exact proof/job/auth/static binding into an
opaque handle tied to the current open store. Final issuance/admission uses an
immediate TryLock, checks unchanged source evidence and expiry, and synchronizes
only local state while the original account lease is held. A long registry
reader therefore returns busy at commit instead of blocking the account gate.
Issue entropy is prepared before the final lease; birth remains the commit-time
issuance sample. Historical recovery retains original receipts and birth.

`NewFencedManifestAdmissionServer` explicitly selects this behavior for the
shared pool's admission-only v3 service. Existing full/solo services retain their
original API and locking contract. The new service still refuses `/v3/jobs`;
the pool alone issues work. Balance source verification also runs outside its
account lease, with a final original-session check before disclosure.

No wire or storage schema changes. Uncertain outcomes after job/receipt writes
or block transmission retain the original recovery identity; clock-only
rejections preserve their definite expiry/low-work behavior. D68/D69/D80,
confirmed earnings, fee/maturity rules and reset policy are unchanged.
See `POOL_SESSION_FENCES.md` for lock order and integration requirements.

## Verification

Evidence is under `../artifacts/set60` with the pinned Go 1.27.4 no-CSE profile.
Existing independent normal-PoW corpora remain unchanged; relational tests use
published fixture accounts/keys and genuine issued proof inputs.

- Registry staged normal and cheap-profile tests passed on their first attempt:
  frozen canonical jobs/receipts, final birth sample, copied/foreign handles,
  immediate busy refusal, historical recovery, final expiry, source/duplicate
  changes, session failure and lost-write acknowledgment recovery.
- Fenced service normal/refusal tests passed: disabled issuance, revoked or
  replaced session after node work, cancellation, postcommit uncertainty and
  exact historical recovery, busy registry contention and profile refusal.
- Complete poolshare normal tests passed in 5.478 seconds. Six stalled node
  boundaries allow other-account/reset probes; the actual final database batch
  excludes reset until completion. Postcommit authority failure recovers once
  after reopen. Existing expiry, canonical-late-winner, RPC membership,
  capacity and poisoning behavior also passed. Preserve pool-attempt1's sandbox
  loopback-listener failure; the authorized network retry is pool-attempt2.
- Actual account-store tests passed. While Alice's template/proposal or fighter
  admission node check was stalled, Bob's view and Alice's verified reset
  completed. Releasing the stalled request produced no work disclosure or new
  fighter under the old credential. A separate balance test blocks the real
  registry while proving its source verification holds no account lease and
  discloses nothing after revocation.
- Independent reviews found no actionable defects in staged registry handling,
  pool commit/uncertainty boundaries, original-token fences or balance reads.
- The actual normal worker test now selects the fenced admission constructor.
  Its fresh run passed in 5.573 seconds: lost-response/restart recovered the
  original nonce0 with one issue, submit and signature, one accepted share and
  one fighter. Recovery performed zero new mining attempts.
- Actual two-wallet settlement passed in 82.342 seconds. Both native84 wallets
  received 495,000,000 atoms through transaction
  `439c563dc2f4a8171f9dc3644c27930d290d29ce702873509240e6053a35d1e9`.
  The round confirmed at 60 descendants and deferred while immature. At 99
  descendants its coinbase was spendable in the next block; the payment was
  confirmed in that block. Same-day retry retained one transaction/broadcast
  and both exact wallet recipients. Owned nodes drained cleanly. These are
  isolated public-fixture tests, not real-wallet custody or public-chain mining.
- Final validation passed with unchanged source and build provenance: 196
  normal top-level test passes, 38 race passes and 15 cheap/profile-refusal
  passes, plus vet and a clean diff check. The two actual-node opt-in checks
  above ran separately. See `validation-attempt1/result.json` and its logs.
- The fresh-node HANDOVER smoke passed using newly built normal-profile
  binaries: two blocks with zero peers, 10-SLUG reward, report-only checkpoint
  conflict, read-only public-seed synchronization to height 61 in 12.95 seconds,
  60 checkpoint descendants, unchanged height-62 activation, chain verification,
  stale-template refusal and a clean restart/shutdown. No public-chain mining.
  See `smoke-attempt1/result.json` and `build-provenance.json`.

## Integration boundary

The next deliverable is a runnable shared fixture pool app: parent-owned nodes,
issuer, journals/services and daily settlement; original-view child wallets and
miners; Pin2 inventories and authenticated balances. Use the new fenced
admission constructor for that shared issuer. Existing app routes, storage and
the qualified Set51 bundle remain intact until the new route is qualified.

These packages are off-chain and do not change the seed's compiled inputs.
Master/live seed stay Set50 `592e143c`; no rollout is necessary. No production
keys, permanent genesis, new gameplay decision or D74 release approval is
implied. The full joint design review stays last.
