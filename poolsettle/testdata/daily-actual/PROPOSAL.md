# Bounded actual daily source-to-wallet proof

Source-only proposal, before harness implementation or execution. This qualifies
fresh explicitly selected daily financial owners against isolated normal-PoW
nodes. It is not a production service, TLS/remote worker, public deployment,
64-wallet-custody or scheduler qualification. Existing V1 runtime, poolminer and
service bindings remain strict and are not reused with a relabeled Config2.

## Structure and original owners

Use one opt-in external `poolsettle_test` actual test. Its paired
`daily_actual_export_test.go` belongs to package poolsettle and exposes only a
narrow test-clock setter for an idle original daily Store. This is the Go
export_test pattern: no shipping Go helper, build-tagged production backdoor or
public clock API. The external test can import the real wallet module using an
explicit GOWORK containing the existing local chain/wallet modules. The export
file imports no wallet package, so it creates no test import cycle.

Retain these concrete owners in order: fixture online operatorauthority owner;
original OperatorProfile registry issuer; prospectnode.Client; explicit
poolshare.CreateDaily/OpenDaily; poolround.CreateDaily/OpenDaily; fresh
wallet.CreateDailyPoolSettlementWallet/OpenDailyPoolSettlementWallet;
poolsettlenode.NewDaily; original daily wallet backend; poolsettle daily Store;
one BindDailySettlement before any signing. The backend borrows custody/node;
close ledger work and backend before node/wallet, then rounds/source/issuer and
authority. Retain any failed owner and fail qualification; never replace it.

Current poolminer.ValidateConfig binds V1-only source/config digests. Root has
therefore selected a test-only immutable64-entry SessionAuthorizer for this
proof: published deterministic token→public-key entries, original bounded
context and closed/joined callback lifetime, no caller-selected key or mutable
enrollment. It substitutes only fixture authentication, never source rows,
proofs, balances, original Store capability, wallet signing or node evidence.

## Inputs frozen before expected output generation

`reference/recipe.json` and its pinned public inputs define the complete
composition. Start from the independently derived late-winner Setup authority,
zero256 operator wallet root/funding key and fixed fee policy. Keep the complete
OperatorProfile and its independently expected role keys unchanged; no private
checkpoint role is loaded. Recompute Source2, Config2 and guard IDs using the
already approved independent domains, never Go output.

Fixture account i=0..63 uses explicitly published secp256k1 scalar i+1. An
unchanged independent Python arithmetic helper derives the compressed public
key, gym ID under the fixed network/genesis identity domain, and native P2WPKH
payee script. Sort immutable settlement destinations by gym ID; issuance cycles
scalar order, which is a separate recorded order. All64 scripts must be unique
and distinct from the retained operator funding script. No miner wallet is
created or claimed. Expected allocation/output amounts derive from integer
counts and fixed99%/1% policy, independently of product accounting.

Selected immutable limits: Source2 works8192/submissions16384/bytes8GiB;
Round2 rounds16384/bytes8GiB; Config2 rounds16384/batches32767. Bind the exact
64-gym config, wallet Binding2/guard, operator profile and node config in the
fixture output. These are fresh files, not migration of an old root.

## Exact two-generation sequence

1. Start the two inherited normal localwalletpilot nodes, private loopback RPC,
   txindex and only each other as peers. Generate one uncredited bootstrap block
   at height1 to the existing separate bootstrap script. No consensus target,
   proof hash, activation height, genesis or clock is overridden.
2. Issue one original Source2 job for gym0. Prepare that exact job/template,
   search at most64 normal CheckNonce candidates without submitting ordinary
   results, submit its exact winning block to the real node, verify exact
   canonical block bytes/coinbase, then submit that original job+nonce to the
   concrete Pool. Require one retained work/accepted block row at height2 and
   issuer Jobs1, no fighter admissions. This deterministic row count does not
   depend on a lucky first nonce or automatic worker behavior.
3. Mine uncredited maturity blocks to height102. Sync/observe the original round
   and import it normally. At settlement date A, RunDaily signs A through the
   original active Store capability and real schema16 wallet. Require one
   input, one gym output990000000 atoms, fixed fee141, change9999859. Include A
   canonically in uncredited block103; recover/confirm A. Save its complete
   immutable economic body and original day/creation metadata.
4. Issue720 more original jobs, cycling the64 fixture accounts. Each contributes
   exactly one real canonical winning block/accepted row, heights104..823. No
   ordinary rows, admission signatures or fabricated accepted receipts. Final
   source/issuer counts are721; round members are each their single original
   accepted winner. Advance only uncredited blocks to height923 so all720 new
   coinbases are mature and60-descendant confirmed.
5. Select settlement date B=A+1 UTC day via the test-only setter while no
   operation is admitted. Work expiry, proof, credential, node and process
   clocks remain actual. RunDaily builds the complete ready candidate:
   720 coinbases plus original A change =721 inputs,64 gym outputs and change.
   Independent conservative shape is31588 stripped/78591 witness/204943 weight/
   51236 virtual bytes; fixed rate1000 atoms/kVByte gives fee51236. Exact B miner
   principal712800000000 atoms, B change7209948623. Cumulative principal paid
   after A+B is713790000000 atoms. No subset selection or consolidation.
6. At B's first broadcast, the inherited bounded relay forwards exact bytes to
   the real node and loses only the response. Require pending original B and
   retained reservation. Close/join every source/round/ledger/backend/custody
   owner, retaining the same fixture paths. Reopen each exactly once using
   explicit expected daily configs/guard. Cold Source2 rehashes its721 retained
   receipts once; later round/ledger reads use only the qualified retained-work
   memo, which conveys no canonicality or financial authority. Restore the same
   test settlement date before recovery; audit must not fabricate current time
   or bypass the stored high-water check.
7. Recover B through the original reconstructed Store/backend binding. The
   actual node's existing exact B transaction resolves the lost response; no
   replacement signing/batch/raw/txid/fee/input/output/day or source claim is
   permitted. Include exact B in uncredited block924 and recover again. Require
   exactly two batches, one B payment per gym, no extra sends after the original
   forwarded B, and full unchanged A economic body. An explicit fresh node
   Inspect on B's single retained change must verify all721 distinct origins,
   proving the old256-origin boundary is crossed in one change coin too.

The expected final claims are operator-only for all721 original rewards. The
first reward begins B with9999859 operator atoms; every later reward begins with
10000000. The existing deterministic input/claim order charges B's51236 fee to
that operator frontier, never miners. Actual txids/block hashes remain runtime
facts, checked against canonical block bytes and the independently expected
ordered economic rules; the reference never invents those facts.

Date A/B scheduling is a controlled bookkeeping seam; no real24-hour scheduler
claim is made. The same-day occupied-slot and immediate repeated recovery
controls require no additional block or signature. D80 real five-minute expiry
is not repeated here; its separate actual proof remains preserved.

## Reuse and finite bounds

Reuse source leaves from `desktop/pooloperator/testdata/late-winner-actual`:
normal node start/stop, private loopback RPC, actual block/template reads,
bounded winning-nonce search, exact canonical/coinbase checks, post-forward
lost-response relay, source/profile/log/process ownership checks. Generalize
only known case counts and owner composition; do not import the old Runtime1,
TLS clients, CLI child, economic assumptions or whole-json ledger audit.

Before code, freeze a new input manifest and root-approved execution bounds.
Propose30-minute overall actual context,35-minute go-test timeout and40-minute
independent outer group guard; these are new scale-test bounds, not increased
product timeouts. Retain each source/node operation's30-second bound, four node
RPC workers, each original nonce search at most64 and4minutes inside the overall
context, and5-minute cold source reopen. At most924 blocks; exactly721 pool
winning submissions. No retry creates a replacement job or new signing request.
Retry only exact definite Busy outside retained leases within the original
operation context; unknown outcomes stop and use explicit original recovery.

Use the existing source-bound candidate wrapper, offline cache/GOMAXPROCS2,
capped logs, joined process groups and unconditional source/profile/artifact
checks. Bind both checkout trees before compilation and after cleanup. All
generated synthetic private fixture inputs stay under t.TempDir/owned scratch
and are never printed or committed. Public phase reports record counters,
canonical hashes, exact economics, elapsed cold/warm/Inspect/sign durations and
cleanup only. This case does not need further source cold audits after final
closure: compare the original owner's verified final report and closed bounded
metadata, with explicit evidence scope.

Implementation remains blocked on root's daily ledger APIs and independent
fixture output. Optional daily scheduler composition may follow its separately
qualified explicit API; it does not delay this concrete financial proof.
