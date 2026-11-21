# Original A2 read-only diagnostic — source-only

Root alone compiles and executes this separate helper. The current actual
helper/checker and its failed A2 evidence remain unchanged. Every original build
leaf is copied byte-for-byte from helper manifest `087cfdad`, except `main.go`
renames only `main` to `actualMain`; that retained runtime entrypoint is never
called. `diagnostic_main.go` is the only new entrypoint.

Arguments: `--original-root` must equal the original A2 owned path;
`--artifacts` must name a fresh sibling
`artifacts/set80/issue-recovery-a2-readonly-diagnostic-attemptN` directory.
The diagnostic has one 60-second context and a bounded 16-MiB public report.
Root should preserve compile/runtime failures separately and impose its own
bounded process-capture/cleanup watchdog. No source author executed it.

Only three exact hashed public inputs are read initially: `01-selection.json`,
`02-gap.json` and the joined failed `actual-report.json`. The existing full
selected `readOutbox`, `readRequest` and `auditIssue` helpers then open only the
original read-only outbox and issuer/source/round/settlement stores. They retain
all current full audits, complete key/count/reverse census, owned Close behavior
and before/after file identity checks. No node/RPC, operator, wallet/controller,
credential, private fixture, entropy or signer operation is invoked.

The report includes exact closed outbox rows/control and original request result,
the complete issue/financial audit and stored source submissions. Pure normal-PoW
preparation/checks and the existing client receipt predicate are evaluated against
those retained rows without constructing a client or doing HTTP. Predicate false
is reported as diagnostic evidence; it is not accepted as a valid receipt. Full
original audit failures still fail the diagnostic. All input/public-store/outbox
identities are checked again on completion/failure.

This can establish whether retained original rows satisfy the source and client
predicates after shutdown. It cannot recover the omitted live Lookup error or
prove which transport/contention event happened during A2. A2 remains failed;
there is no actual qualification, repair, retry-policy change or payout effect.
