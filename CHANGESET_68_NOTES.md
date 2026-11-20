# Change set 68 — reviewed candidate wallet withdrawals

Status: implementation and qualification complete. Independent references,
wallet/core/desktop suites, actual funded withdrawals, the exact Mac package
and required network smoke pass. Both repositories use the stacked
`slug-reviewed-withdrawals` topic. No seed rollout or product-release approval
is implied; final commit and push bindings are retained with the evidence.

## Result and scope

The Pool Pilot's existing balance panel prepares a payment and opens a native
review of its full destination, exact recipient amount, network fee and total
debit. Signing and delivery require the original freshly authenticated, unheld
account session after that exact review. Cancel leaves an unsigned operation
available; Stop joins work and cannot recall a transmitted payment. A lost
reply is recovered by its original operation/action identity. Explicit retry
sends the original signed bytes, without new selection, change allocation,
signature or fee. The owned node must observe those exact bytes before the
operation becomes submitted; an RPC acknowledgment alone is insufficient.

Only the published zero256 and incrementing256 fixture wallets on the existing
private Phase A profile are enabled. The current native SegWit (BIP 84) wallet
path, schema 9, 24-word recovery contract, genesis, activation at block 62,
pool accounting and checkpoint rules remain intact. This is not production
custody, public funds, real signup, transfer policy or the final product
design review. The paired wallet topic is also `slug-reviewed-withdrawals`;
both repositories are required to build it.

Candidate selection requires confirmed native SegWit (BIP 84) inputs, with
coinbase maturity preserved. It visits at most 4,096 mined-credit rows,
including skipped rows, selects at most 16 inputs, and bounds
previous-transaction decoding to 4 MiB each and 16 MiB total. Selection is
deterministic; final inputs/outputs follow BIP 69. All four existing standard
recipient types are supported. The recipient amount is never reduced by fees.
The fixed candidate fee is 1,000 atoms/kB, with a total fee ceiling of 100,000
atoms including discarded dust. A consumed change address is retained even
when no change output survives. Personal payment fees are not charged to the
pool's 1% settlement cut.

The wallet atomically saves the exact unsigned draft, change cursor and
prepared input reservations. Signing returns scratch only. The app journal is
the sole durable signed authorization, fenced separately from dispatch intent;
wallet retention then permanently excludes those inputs. An uncertain
publication poisons the owner until reopen. One unresolved operation and 128
retained rows bound the candidate namespace. Only known unsigned abandonment
releases a prepared reservation; signed inputs are never reused based on time
or apparent chain absence. Ordinary automatic selection and explicit generic
signing paths also honor candidate reservations.

The parent installs an immutable downgrade guard and paired INIT/READY before
opening withdrawal-capable children. Earlier Pool Pilot builds reject the new
guard before opening the wallets. Reopen audits both directions of wallet/app
correspondence, permitting only the documented intent-to-prepared crash gap.
Missing established components fail closed. Private reconciliation and
delivery handles must belong to the exact retained journal pointer. The saved
change address is watched on the existing wallet websocket before signing; a
bounded watch failure shuts down that owned transport and requires child
reopen.

Logout and reset invalidate the view and durably revoke the old authority
before waiting for the old child to drain. The D72 hold permits current-view
reads and already-authorized local reconciliation but blocks new withdrawal
actions. Human review holds only bounded metadata, with no account lease,
wallet lock or running work slot. Helper/child replacement, expiry, stale or
duplicate confirmation and failed outer authorization cannot revive a review
or start backend work.

## Independent expectations and review

`REVIEWED_WITHDRAWALS_ADOPTION.md` and `reviewed-withdrawal-contracts.json`
index the frozen contracts and additive clarifications. Historical proposal
headings and the original incorrect getrawtransaction boolean shorthand are
preserved; the separately recorded RPC erratum uses the inherited integer 0/1
parameter. No new implementation output was copied into a golden.

- Wallet reference: 24 positive unsigned/signed transactions, 13 invalid
  drafts, 5 invalid signed transactions. SHA256
  `d7c7b1e4c94e02f734a162b5974749853c91828ea1a0a4c4446c379bddc841c3`.
- Journal reference: 2 identities with 11 modeled states each. SHA256
  `ee4ba4ecc89289b76e2ce805e4d8f88ced79c6b69542d71ee74a7a37b1c1bd29`.
- Desktop wire reference: 142 positives, 80 negatives, 9 raw cases. SHA256
  `60fe12e20e0c842a43cfc8f41892a8fcf4a7c991592363b17f87a8180098af9c`.

Each corpus reproduced byte-for-byte with Python 3.14 and 3.9. Portable
checked-in launchers independently reproduce the files without replacing them.
Synthetic UTXOs, account metadata and node states are labeled and do not claim
actual new chain events. The known-address wire model is finite. Separate
reviews covered the producers, journal durability/original fences, wallet
reservations, paired setup, node parsing, desktop actions and native
confirmation.

Review corrections include converse wallet inventory, exact live journal token
provenance, strict present-null node fields, inherited RPC parameter types,
known-account setup inventories, original source equality, positive draft
fees, failed outer admission before backend entry and fresh input inspection
before send-review publication. The independently produced transaction bytes
remained unchanged through these fixes.

## Qualification record

Evidence is retained under workspace `artifacts/set68/`. Completed gates:

- Core independent references,
  durability/failure/recovery/capacity/provenance, normal/race/cheap-profile
  refusal, vet and Linux/Windows compilation: `core-final-attempt1`.
- Wallet focused normal/race, final signer/lifetime/inventory delta
  normal/race, full normal tests of all six modules, six builds/vet and cheap
  refusal: `wallet-attempt3`, `wallet-race-attempt1`, `wallet-delta-attempt1`,
  `wallet-delta-race-attempt1`, `wallet-full-attempt1`,
  `wallet-build-vet-attempt1`. See the paired wallet `CHANGESET_68_NOTES.md`.
- Paired setup normal/race: `setup-attempt1`, `setup-race-attempt1`.
- Owned-node RPC protocol, input inspection, exact bytes, malformed/nullable
  replies, cancellation and no automatic retry normal/race: `node-attempt1`.
- All ten chain module builds: `chain-module-builds-attempt1`, 2,217 unchanged
  compiled inputs. Current normal node/control build: `smoke-attempt1`; its
  1,770 inputs exactly match Set 67. Runtime smoke passed: isolated two-block
  mining, 10-SLUG subsidy, report-only checkpoint conflict, read-only seed sync
  at height 61, 60 checkpoint descendants, activation at 62, chain verification,
  stale-template refusal and clean restart. No public-chain mining occurred.
- Focused desktop normal/race/vet across the controller, pilot, bridge and
  native package: `desktop-final-attempt4`, with unchanged source. The exact
  reset-during-scratch-sign regression also passed 25 race repetitions in
  `desktop-race-repeat-attempt2`. All 354 frontend tests pass in
  `desktop-frontend-final-attempt2`; the successor changes trailing whitespace
  only from the preceding qualified frontend. These are focused gates, not the
  complete desktop suite or a visible native review session.
- Actual fixture withdrawals: `actual-withdrawal-attempt1`, first execution
  passed in 16.22 seconds (22.976 seconds including the test command). Both
  whole drafts and signed bytes matched the independently frozen native SegWit
  (BIP 84) transaction cases; only the application binding ID changed.
- Portable reference reproduction: `reference-portable-attempt1`; desktop wire
  first/reproduction runs retain their own source-bound reports.

- Complete six-package desktop suite: `desktop-full-attempt1`, 196.375 seconds,
  unchanged inputs. Native linker warnings about the SDK deployment target
  remain visible; this Mac run does not qualify older macOS versions.
- Final normal Mac bundle: `package-attempt2`, version 0.5.7/build 68,
  9.374 seconds, 6,523 bound paired-repository files unchanged during build.
  The bundle is ad-hoc signed; this is not notarized distribution.
- Exact packaged helper: `package-helper-attempt1`, 122.445 seconds, both
  name versions, both fixtures and initial/reopened sessions. Each route
  independently verifies the withdrawal identity, retains the exact unfunded
  intent, explicitly abandons it without signing or sending, and preserves
  that operation on reopen. Existing recovery, balances, payout history,
  league/career, old-view refusal and shutdown checks also pass. No mining
  or payment is requested by this zero-work bundle check; the funded test
  above supplies actual-payment evidence.
- Final gofmt and diff checks: `static-closeout`. After the full suite, gofmt
  inserted only spaces after two commas in the codec; original bytes and the
  exact replacement are preserved in `format-closeout`. Package attempt 2
  uses the formatted source. An untracked Python import cache generated during
  helper qualification was preserved outside the repository and removed before
  commit; it is not an app input and was absent from both package bindings.

Final source, report, package and paired commit hashes are recorded under
`final-binding`. The required joint D74 review and permanent production
chain/profile choices remain release gates. Next is bounded, resumable
restoration for the existing backup format. Live exports already include
current issued/change cursors; the separate hosted vault's immutable snapshot
and future production integration remain distinct work.

## Actual payment evidence

The test used only isolated copies of the retained Set 61 chain and public
source journals, fresh published-fixture wallets, and a new fixture account
store. Both wallets restored their original native SegWit (BIP 84) payout,
enrolled through actual account challenges, and used concrete journal, wallet
and owned-node resources under original account-session fences. Each sent
exactly 1 SLUG to the other fixture.

For zero256, the transport deliberately discarded the first send response only
after the real node completed it. The unknown result was saved; the old
resource and wallet closed, its session was revoked, and a fresh login
explicitly retried the exact saved bytes without another preparation or
signature. Two transmissions produced one transaction/debit. Incrementing256
transmitted once. The inherited wallet RPC learned both direct sends and their
change while live. One real normal-PoW block advanced the owned chain from 109
to 110, and exact canonical observations resolved both operations. Both nodes,
wallets and journals then reopened with one retained operation per wallet and
permanent signed-input exclusions intact.

Each transaction paid the independent expected fee of 141 atoms. The
reciprocal payments cancelled in the final balances: zero256 retained
2,969,999,859 atoms and incrementing256 retained 4,949,999,859 atoms, each
original balance minus one fee. Eight public legacy transaction records, three
operation records, exact transmitted bytes and the full result are retained in
that attempt. This proves the concrete account/journal/wallet/node
composition; it does not claim a visible macOS approval dialog, a production
wallet or completion of D74.

The original source trees remained byte-identical; copied public markers and
logical accounting records remained unchanged. Compiled inputs, normal node
and build profile stayed exact. The owned process group was empty after
ordinary cleanup, with no leftover processes or forced termination.

## Preserved attempts

Initial wallet fixture construction used inherited `NewTxRecord(raw)`, whose
key hashes witness serialization. The reader correctly rejected its mismatch;
the fixture now uses the existing notification-path constructor with the same
original raw bytes. Compile-only import/field mistakes and sandbox loopback
failures are retained. The original GetRawTransaction verbose-parameter
mistake was found by source review before actual execution and has a separate
erratum.

An early desktop race test hid a failed preparation by dereferencing its
absent draft. The later bounded diagnostic identified a test fixture that
rejected concurrent leases as nesting, unlike the real serializing authority.
The failed 25-repeat run is retained as `desktop-race-repeat-attempt1`; the
corrected serializing fixture passed 25 race repetitions in attempt 2 and the
final focused normal/race/vet run. No production relaxation or expected-vector
replacement was made for this test-fixture correction.
