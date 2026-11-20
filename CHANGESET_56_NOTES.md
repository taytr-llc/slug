# Change set 56 — authenticated pool transport

Branch: `slug-pool-service`, stacked from completed Set 55 `f5fec1f8`.

## What changed

The new `poolservice` connects the concrete issued-work/share journal and its
matched settlement journal. A bounded loopback HTTP service exposes only work
issuance/recovery, nonce submission/lookup and the current gym's historical
balance. Gym identity comes from the original session; difficulty, pool payout
and complete source identity are pinned operator configuration.

`poolshare.IssueMiningWork` shares the original single durable issuance path.
The transport exports the retained template once, and the client reconstructs
the exact extra-nonce-zero candidate with the existing builder. Replies are
checked against locally recomputed normal PoW, target flags and job identity.
Ambiguous submission outcomes recover the same job/nonce. Pool acceptance and
block delivery remain distinct from fighter admission and balance finality.

The desktop `PoolSessionAuthority` refreshes mining credentials between calls
while preserving the original account, generation, view and pool trust. It
checks the credential actually used after HTTP and discards responses after
account changes. Network calls never hold account/view leases. Close cancels
and drains the call. This supplies the credential bridge for the dedicated
pool worker and balance view without changing current desktop snapshots.

`POOL_SERVICE.md` records the protocol, limits, failure semantics and next
integration boundary. Independent Python source/transport identity vectors
freeze the new hashing domains without using implementation output.

## Deliberately unchanged

No consensus, genesis, checkpoint, public seed deployment, production custody
or app organization changes. The current automatic manifest worker builds a
gym-owned coinbase and changes extra nonce, so it is not silently switched to
pool work. Existing wallet payout markers remain intact. A later worker must
preserve separate prospect-only admission, D69 block-first delivery and D80's
expired-fighter exclusion. The full joint D74 design review remains last.

## Validation

Evidence is in `../artifacts/set56/`, using the pinned Go 1.27.4 no-CSE profile.

- `source-compile-attempt1` preserves a test-helper pointer increment compile
  error. The corrected dereference passes in `source-compile-attempt2`; no
  runtime expectation changed.
- `server-attempt1`: compile and all server/security tests pass. Coverage
  includes canonical request/Host/Origin/body limits, excluded operator routes,
  original-token auth, gym isolation, post-commit revocation recovery,
  fail-fast admission, matching sources and cancellation/drain.
- `identity-reference-attempt1`: independent Python regeneration is byte-identical
  and the Go contract test passes. The frozen identity fixture SHA-256 is
  `361a08179c56d024836c45646de215024096d3a234fab93fe9566dbcd3c37a1c`.
- `actual-attempt1`: the focused original-work export check passes (1.364s).
  A two-node normal-PoW HTTP composition passes (63.131s), with real fixture
  account enrollment/mining tokens. A dropped winning-share response recovers
  through lookup; duplicate submission retains accepted sequence 2. Sixty
  descendants show immature funding over HTTP; ninety-nine descendants permit
  next-block coinbase maturity and one payout of 495,000,000 atoms per gym.
  Payout transaction:
  `7ec8d3a9ac30862bf36fc79116a19637628a42d5d3fa4407489e8e75ce62f99a`.
  Reset revokes account A's old work/balance token, leaves B unaffected, and
  allows the recovered held account to view and mine as D72 permits. Both
  owned nodes shut down cleanly; logs and source/profile bindings are retained.
- `desktop-attempt1` preserves a two-minute mock-server cleanup timeout: its
  unread request body prevented net/http's disconnect watcher from starting.
  The mock now consumes the bounded body like the real service, with the same
  cancellation assertion. `desktop-attempt2` passes normal checks (4.979s),
  race (10.647s), cheap-profile refusal (9.111s) and pilot-package vet (0.669s).
  It covers token refresh, unchanged trust, cross-account refusal, response
  suppression after view changes, uncertainty and cancellation/drain.
- `validation-attempt1`: final service/share/settlement normal tests pass
  (19.120s), as do race checks (86.589s), cheap-profile refusal (4.462s) and vet
  (0.334s). The seed dependency check confirms none of the changed packages is
  linked into the daemon. Formatting and diff checks are clean. Independent
  review found no additional actionable service or desktop-adapter defect.
  No seed rollout or app rebuild is needed for this not-yet-selected runtime
  path; master/live seed remain Set 50 and the last qualified desktop bundle
  remains Set 51.

## Remaining work

Implement the durable pool worker/outbox and a shared parent-owned desktop
runtime, preserving the separate prospect admission path. Add explicit native
SegWit recipient bindings and the balance presentation in the existing app
layout. Daily scheduling, public hosting and reviewed production signing/custody
remain separate from the fixture transport proof.
