# Root checker fields

`actual-report.json` is emitted on success/failure: version1, caseA/B, passed,
completed, start_ms/due_ms, initial_height/tip, setup_id/remote_id/issue_profile_id,
request_id/intent_id/job_id/issued_at_ms/expires_at_ms (aftergap), failure_phase and
bounded failure if refused, seconds, clean_join/cleanup_errors, operator_generations,
cli_generations, node_cleanup, intentional_crash_count, tls_relay, sends and
send_attempts/send_error. All operators include generation/command/PID/parentPID/
PGID/joined/intentional_crash/unexpected_exit/signal/count/exit/cleanup/io flags and
canonical frames. All CLI generations preserve the inherited canonical status,
stdout/stderr joins, exact same config_id and singleSIGTERM/zeroexit assertions.
The expected operator serve generation counts are A3, B2; both also retain init
and three one-shot rotate process records. CLI generation counts are A3, B1.

Both: 01-selection.json exact wrappers/start/due;02-gap.json barrier{block,sha256,
at_ms},audit,operator;03-local-gap.json comparable selected outbox control.
`audit` includes full financial{source/job/share/birth/round/credit/header/body/
report,rowhashes,filesbefore/after} and exact source_request/issuer_request/job/
template/templateSHA, complete requests/issuer_rows/jobs census, source/issuer
request and inverse counts, selected exact Work (nullable), readonly flag.

A:04-response-loss.json TLS events plus exact withheld response/hash;05-completed-
source.json{audit,local};06-replayed-work.json full same response;07-worker-closed
.json{rows,request,control,cli};08-stopped-reopen.json original controller snapshot;
09-mature.json{snapshot,due_ms,observed_at_ms,height,economics};10-confirmed-payment
.json{payment,confirmation_block,confirmed_at_ms};11-payment-wallet.json
{payment,balances,pages,wallet_before,wallet_after,confirmation_block};outbox-after
.json exact unchanged selected state;historical-replay.json{lookup,resume,
observed_at_ms};12-final-audit.json full selected audit;13-complete.json (copied
fields into final report): completed,case,request/job,accepted_rewards/fighters,
pending_fighters,original_signatures,stopped_reopen,paid_atoms,fee_atoms,send_count,
attempts,final_audit_unchanged,final_height. Final report also miner_atoms,
scheduled_payment and original_wallet_received. money-read-evidence.json retains
bounded pure-read refusals and successful final completion, inherited PathC.

B:04-expired-terminal.json{expired_observed_at_ms,original_expires_at_ms,terminal,
replays[lookup,resume]};12-final-audit.json{audit,outbox,local_terminal_consumption};
13-complete.json (copied into finalreport) completed,case,request/job,expiry_ms,
resumed_at_ms,state issued_unused,works/submissions/accepted/births/credits/batches/
send_count all0,final_height2,read_only_unchanged.

Original source may retain other fully audited pending requests only in A's
normal Stop race, never as a replacement for the recovered ID. No original
request count grows during gap completion/reply loss. Canonical q result and
original job/template/issued/expiry are exact across reply replay; final complete
namespace audit corroborates all other request states and every inverse.
