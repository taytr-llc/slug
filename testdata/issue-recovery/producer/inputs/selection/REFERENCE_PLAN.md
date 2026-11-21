# Set80 independent reference plan

Source-only proposal accompanying ISSUE_RECOVERY_CONTRACT.md. No producer has
been authored or executed and no expected outputs have been generated for Set80.
Root reviews the exact contract and input freeze first.

## Inputs and independence

Use the committed, independently derived Set79 public daily corpus at
`slug/testdata/public-daily/reference-v1.json`, SHA256
`a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3`.
Preserve it and its source/qualification bundle byte-for-byte. Select:

* Composition `one-enrollment`, `two-enrollments`, `64-enrollments`: unchanged
  complete Setup2, Policy2, source/settlement IDs, profile, keys and recipient map.
  Derive the new descriptor/wrappers only; no new custody/key material.
* All three immutable transport work families and their original template-wire
  order recipe, plus existing entry/scan cases. Keep original jobs, templates,
  nonce proofs and old expected bytes unchanged as regression anchors.
* Remote `zero256-default` and `zero256-maximum`, original local binding and daily
  credential inputs for new wrapper/cross-mode cases. These remain conditional
  public fixture selections, not a provider/login or real wallet assertion.

Some inherited transport cases are deliberately conditional and do not describe
the full selected composition. Do not combine one with an unrelated Setup2 and
call it an authenticated owner. Lower codec/trust cases may explicitly assume an
independently selected SetupID. Full cross-owner traces must instead use a
matching composition and independently derive its ManifestJob2 from published
context/key/challenge/time inputs, using the frozen manifest-job byte contract
and independent prior model. Such new jobs are a new family; they do not rewrite
old jobs/proof hashes. An unmined Issue candidate needs no new PoW oracle. Actual
normal-node issuance/authentication is separate runtime evidence.

Also pin these source declarations/documents as grammar evidence, never import
them as executable code or use their outputs as expected values:

* `registry/{manifest_types,manifest_jobs,manifest_staged,manifest_store,public_store,public_audit,operator_store}.go`
* `poolshare/{types,config,store,pool,daily,daily_operator_admission,daily_operator_audit}.go`
* `poolservice/{types,daily_types,daily_config,daily_client,daily_server}.go`
* `prospectworker/{pool_types,pool_daily,pool_daily_issue,pool_outbox,pool_worker}.go`
* `prospectjob/{manifest_job,manifest_json}.go`, `prospectwork/types.go`,
  the committed independent job/template model inputs and work-wire-order recipe.
* `desktop/pilot/remote_pool_config.go`, the actual daily marker/credential
  declarations, `desktop/pooloperator/setup.go`, and the relevant current AGENTS,
  POOL_PUBLIC_DAILY, POOL_DAILY_ADMISSION and approved Set80 plan.

At producer admission, materialize a public allowlisted input catalog with each
exact path, byte length and SHA256, plus all producer leaves/dependencies and
contract. No runtime fixture/credential directory, private file, module download,
Go import or live source is a producer dependency. Any selected source declaration
changed by root before freeze requires an explicit catalog successor.

## Derivations and required cases

The independent Python producer will emit ordered canonical JSON/preimage hex,
hash IDs, classified validity and modeled final row/count/byte state. It will
derive every literal frame length rather than copy production constants alone.
Required families:

1. Profile/setup/client/remote/credential/marker/meta wrapper equality; count
   1/8/8192 boundaries; full Config2 equality and wrong Setup/source/count/profile;
   every old/new decoder boundary. Retain old formats as exact regression bytes.
2. Request, intent, source/issuer rows, reverse keys, result/HTTP envelope,
   outbox retained response and scan-reference/cursor. Include exact key lengths,
   canonical GBT order, all required null/empty/zero fields, unknown/duplicate/
   reordered/oversized objects and caps-before-HTTP-trim.
3. All seven legal source/issuer pairs and every forbidden edge; initial local
   intent before dispatch, source-only intent, issuer-only commit gap, completed
   work before reply, reply before local sync and local uncertain commit/reopen.
   Same-ID duplicate/delayed reply must keep the same job/template/time/counters.
4. Expired never-published job → issued-unused, completed expired work → exact
   work replay; no-job positive parent mismatch or permanent intake drain → seal
   then source terminal. Transient node/Busy/cancel/error, absent lookup and a
   pending lookup do not clear intent. A seal never later creates a job. A
   completed-work/terminal contradiction, wrong gym or inverse reference refuses.
5. Source effective-byte equations at exact and one-byte-below headroom, including
   existing canonical receipt holdback. Registry remaining-slot + metadata-growth
   + original unpublished-admission reserves at exact/below signed LogicalBytes;
   concurrent publication cannot consume reserved request completion bytes.
   Seals/unused requests consume request counts; no hidden extra allowance.
6. Outbox requests+entries shared count, required first-found slot, maximal active
   result/scan/outcome reservation, conversion once, preserved completed result
   after scan removal, repeated replay with zero new count, and full capacity
   retaining existing pending delivery. Mutate each count/index/body independently.
7. Same original caller/lease/owner state traces, Stop/cancel/revocation/deadline,
   lost response and poison/reopen. These are requirements for later real tests,
   not measurements of mutexes, durable writes, process joins or authority.

The output must label conditional codec/state projections and inherited PoW
anchors separately. Financial amounts, daily due decisions and D84 are unchanged;
no new payout arithmetic or policy oracle is needed. Existing signed batch/body
identity is an invariance expectation, not a newly derived transaction.

## Execution and implementation gates

After root selects the contract, author a small stdlib-only producer, preserve its
unexecuted source lock, and peer-review it. Proposed CLI: `python generate.py
--lock <approved.json> --output <fresh.json>`. The lock requires explicit root
execution authorization and a complete relative-path inventory. Output creation
is exclusive; no network, product import, random sampling or expected-output
regeneration from Go. Retain a finite time/log/output bound in the root runner.

Root alone runs two existing Python versions and verifies identical complete
output and unchanged bound inputs on success/failure. Only after that qualified
reference may production/test implementation consume expected values. Real
original-owner fault tests and the TLS/headless fresh-process rehearsal then
prove synchronization, original job recovery, session fences and clean joins.
Neither reference generation nor a direct-owner unit test is public-host readiness.
