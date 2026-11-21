# Set80 actual A/B source lane

Root approved source authoring only against the frozen ISSUE_RECOVERY_ACCEPTANCE_PLAN.
No producer, build or scenario is executed by this lane. Root owns the outer runner,
final source lock, selected runtime budget and both actual executions.

A and B are separate fresh-root invocations. Each starts with one immutable parent
deadline (initial proposal:900seconds; outer1080seconds; height180). A fixes its
real UTC payout due once at initial selection. B waits on the original issued
job's real wall expiry. No run resets a deadline, source, outbox or request ID.

The inherited Path C node/manual-pipe/financial/evidence helpers and Path B real
operator CLI ownership are copied byte-for-byte under inherited/. copied-inputs.json
binds every original path/hash/length. Working helpers are separate derived files.
Only public source, declared fixture catalog metadata and qualified public reference
blobs are copied; there is no agent read of a wallet, credential, seed or runtime DB.

Selected files:

- selection_reference.py plus locked input recipe: independent stdlib-only framing
  of the new Setup/profile/remote/worker/credential wrappers over the qualified
  a48a854d Path C selection, preserving one/two-reward financial values. Root alone
  executes after review. Actual ephemeral ports/TLS/due are separately normalized
  against that fixture before independent framed digest comparison.
- selection.go and fixture_setup.go: exact public inputs; protected synthetic
  selections; original published schema16 fixture wallet created and joined before
  CLI ownership. No runtime is run in process. CLI init/rotate performs original
  setup before the first serving child.
- operator_process.go and cli_process.go: bounded manual output pipes, original
  inherited PGID, one Wait per child, explicit generations and one intentional
  SIGKILL at the acknowledged proposal barrier. Every other stop remains strict.
- proposal_relay.go: forwards unchanged RPC to the original normal node; validates
  the successful proposal result, then holds only its original reply until kill.
  Captured block bytes are checked against the closed audited job/template.
- issue_tls_relay.go: independently selected external TLS origin, distinct actual
  ingress listener, original Host/TLS correlation preserved. It withholds exactly
  one fully validated successful response for the original request and gates later
  forwarding until the original CLI has stopped/joined. No credential in evidence.
- issue_audit.go and outbox_audit.go: full original selected read-only audits first,
  then bounded public namespace/index/control projection and unchanged-tree checks.
- scenario_a.go: pending/issued gap, exact source completion/lost successful reply,
  same outbox re-open/replay, actual original worker PoW, strict Stop, stopped reopen,
  original schema16 scheduler payout and original schema14 wallet receipt.
- scenario_b.go: same independently verified gap in a fresh case, real expiry,
  issued_unused terminal, exact repeated Lookup/Resume, zero work/credit/payment.
- main.go, README.md and source manifest: fail-closed conjunction, public failure
  evidence, all original cleanup results, no deletion of uncertain fixture roots.

Case L remains the separately scoped package-local checkpoint control in the frozen
plan; it is not falsely represented by the external TLS pre-dispatch barrier.
