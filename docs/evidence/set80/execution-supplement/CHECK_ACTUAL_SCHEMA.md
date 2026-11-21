# Set80 public actual checker

`check-actual.py` exports `check(owned: pathlib.Path, case: str, probe_pid: int)`.
It returns `{absolute_public_path: {"bytes": int, "sha256": lowercase_hex}}` only
after every required assertion succeeds. It raises on missing, malformed,
changed, wrong-case or inconsistent evidence. Root's outer runner must compare
that exact inventory again during finalization before reporting PASS.

The checker reads an explicit flat allowlist of public phase JSON and original
operator/CLI stdout and stderr. It never opens the private fixture, original or
copied wallet, credentials, node data, or LevelDB files. Public file inventories
inside the closed audit are checked as evidence, not followed as filesystem paths.
Every artifact read is bounded, no-follow, duplicate-key checked and rechecked at
return. Source/profile/binary/copy preservation, group-empty verification and the
900-second scenario / 1080-second outer process budget remain the outer runner's
separate obligations.

Expected wrappers and economics are fixed by the independently qualified
173,644-byte `0f0890e529b64c10ed000673aab7861b9bd526057cc3c2bbbe05b868028f3d10`
reference. The checker recomputes the selected base setup, source, issue profile,
setup wrapper and remote wrapper IDs. Only the original transport fields and
original seven-minute UTC due may differ from the reference. Schema14 local
account/recipient, schema16 guard, source, settlement, limits and economic rules
remain byte-identical. No expected result is learned from actual output.

Both cases require one exact intentional original operator SIGKILL, all other
operator/CLI generations joined with their selected clean exits, two clean nodes,
and matching child PID/parent/PGID and public logs. The closed source/issuer gap
must have one pending/issued request, one original job/template, no retained work,
receipt, birth, credit or batch. Its local q remains active/null; the independently
computed canonical null-q digest must match. Original job and expiry never renew.

Case A additionally requires the real successful reply lost before client delivery,
identical same-ID replay and persisted response, actual consumption of that original
job, one or two complete canonical rewards, original-vault authorizations and exact
admission census, stopped original-wallet/outbox reopen, real maturity before due,
and one scheduled payment. The reader parses the signed native transaction,
recomputes its txid and checks every funding input, recipient/change output, fee,
closed ledger body, credit/claim, captured payout page and original wallet delta.
Complete q/e/h/p/control namespace hashes must remain identical after stopped reopen.
Original completed work must replay unchanged after actual expiry.

Case B requires actual wall expiry, the permanent original issued-unused result,
unchanged job/template and zero work/share/birth/credit/payment. Its stopped worker
retains the original null request: local terminal consumption belongs to the
separately identified focused worker control and is not claimed by this actual.

The source-bound Go helper performs real normal-PoW, signature verification,
canonical node membership and complete original-owner storage audits. This Python
reader cross-checks their public bytes/counts/relationships and independent
transaction arithmetic; it does not replace those authorities or claim a new
cryptographic proof. The money-read schema has no separate call-start timestamp;
the unchanged helper supplies the fixed 120-second child deadline, and this reader
checks retained definite-read refusals and full final authenticated results without
inventing a phase-write latency limit.

Authored before first Set80 actual execution. AST parsing is source inspection;
this checker was not imported or executed by its author.
