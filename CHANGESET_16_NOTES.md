# Change set 16: trusted current-parent prospect context

Branch: `slug-trusted-prospect-context`, based on set-15 release `6443d163`.
Mike explicitly authorized deploying and verifying set 15 while building set
16, followed by deploying and verifying set 16 after its gates pass.

Set 15 deployed as `5a67df18-7bda-47a1-ac58-4db0a41a0891` at `6443d163`.
Its fresh public observer verified the existing 61-block chain and report-only
checkpoint, remained stable across 31 samples over 900.045 seconds, then
restarted and retained the same tip and finalized checkpoint with clean exit.

## What changed

- Added `prospectnode`, which combines an explicit operator prospect policy
  with current-parent assertions read from a configured trusted node. It
  brackets a mining template with chain identity/tip checks and validates
  height, median-time bounds, canonical compact difficulty and target.
- Added `slugprospect context-node`, `slugprospect verify-node` and
  `slugregistry record-node`. Online verification resolves its own context
  and rechecks the node after production-PoW verification. Claimants do not
  supply chain context on these command paths.
- Added `registry.RecordChecked`, which runs a mandatory external check after
  evidence verification and before a new write or duplicate acknowledgment.
  A failed check reserves no ID and writes no receipt. The existing atomic
  batch and uncertain-write handling remain intact.
- Used a bounded standard-library HTTP client with verified TLS or literal
  loopback HTTP, separate credential configuration, matching RPC IDs, strict
  critical fields, deadlines, redacted errors and no redirects/proxy/retries.
- Documented the exact trust, policy and observation boundaries in
  `TRUSTED_PROSPECT_CONTEXT.md` and updated CLI documentation.

## Deliberately unchanged and remaining work

The node remains trusted; this is not independent proof of its honesty or
global currentness. The adapter supports only the existing normal SLUG
network and genesis, rejecting cheap PoW and inherited networks with different
timestamp/difficulty behavior. The node's existing mining-template connection
and synchronization checks, including the height-0 exception, remain in force.

The final node observation and database commit are not one distributed
transaction. A reorg can occur after the check. Receipt schema and stored
`supplied-context` verification labels are unchanged; the invocation's source
label is not persisted or signed provenance. No post-commit network check
misreports a committed receipt as rejected due to a changed tip.

There is no default or automatically selected prospect target. Tests use an
explicit all-ones engineering policy, never installed on the live seed.
D30 calibration, birth/freshness rules, account/transfer authority, D44
immediate ownership, reorg-status history and signed publication remain ahead.
No new signing step is introduced for players.

No node consensus, genesis, production PoW parameters, node RPC methods,
checkpoint rules, seed startup, Dockerfile, network exposure or dependencies
changed. No wallet or operator signing key was accessed. Deploying the CLI
does not create a production registry, configure a policy or start a service.
Set-15 platform/durability limitations and older CI/address-vector debt remain.

## Verification

- Combined production-PoW race suite passed:
  `go test -race ./prospectnode ./registry ./cmd/slugprospect ./cmd/slugregistry
  ./prospect ./fighter ./cmd/slugfighter -count=1 -timeout=120s`. Package times
  were 3.613s, 11.160s, 3.908s, 7.036s, 6.483s, 1.292s and 1.301s.
- Tagged refusal suites passed for prospectnode, registry, prospect CLI,
  registry CLI and prospect (0.529s, 0.841s, 1.088s, 1.591s and 1.114s).
  Cheap-profile online calls send no node requests and admit no evidence.
- Covered strict policy/credential parsing, endpoint restrictions, both-end
  chain identity, malformed/missing/null/duplicate RPC fields, inconsistent
  parent/height/MTP/target, canonical compact boundaries, moving time bounds,
  post-hash reorgs, before-write and duplicate guards, deadlines, response
  limits, denied redirects/retries, TLS trust and hostname mismatch, secret
  redaction, output failures and unchanged offline behavior.
- All ten normal module builds passed. Linux/amd64 builds of both updated
  CLIs passed. Vet, repository-wide formatting and diff checks passed.
- Required normal node smoke passed: two isolated blocks and a 10 SLUG first
  subsidy, report-only conflict, normal seed-chain adoption to height 61,
  finalized/no-conflict checkpoint with 60 descendants, `verifychain 1 61`,
  and clean restart/exit.
- Extended that real-node smoke with context resolution, online proof
  verification, guarded admission, persisted replay and wrong-parent refusal.
  Python constructed a fresh header/coinbase independently; the standalone
  x/crypto Argon2id oracle and independent DNA derivation matched the complete
  returned verified record. The explicit test policy was all-ones. The online
  commands did not publish that proof or submit its header as a block; the
  public seed tip stayed unchanged.
  Evidence: `/private/tmp/slug-set16-smoke.json`; data/logs:
  `/private/tmp/slug-set16-smoke-8l26ad_j`.
- Independent implementation review found no remaining actionable issue
  against the specified observation contract. The node's genesis exception,
  post-check reorg possibility, transport trust and non-persisted provenance
  limits are documented, not treated as solved ownership or birth policy.

The complete blockchain/integration execution evidence remains in set 9;
this change does not claim that the outstanding CI maintenance work is done.
