# Change set 12: authorized Phase A closeout

Branch `slug-phase-a-closeout`, stacked on set 11 (`af67093f`). Mike
explicitly authorized the master merge and Railway seed deployment on
November 15, 2026. `PHASE_A_CLOSEOUT.md` records the preflight evidence,
exact rollout settings, acceptance checks and paired image/config rollback.

Updated the standing rules for master's transition from pristine upstream
to the live deployment branch. The historical upstream commit and frozen
`slug-rebrand` rollback branch remain available. Added startup logs for
loaded public report-only checkpoints: authority, tip, record height/hash,
status and descendants. This permits verification from Railway logs while
keeping RPC private; it does not influence validation or chain selection.

No private key, production-genesis change, disk migration, external message,
PR, new public port or volume replacement is part of the rollout. Existing
CI/release and security limitations remain explicit. Post-deployment results
must be recorded from observed Railway state, not inferred from this commit.

Preflight verification passed: all ten normal module builds; root tests
(1.501 s), offline signing tool (0.583 s), btcjson (0.316 s), rpcclient
(4.248 s), gofmt and diff checks. The production smoke again mined two
10-SLUG blocks, reported a real checkpoint conflict without blocking the
chain, synchronized the seed's height-61 chain, reported finalized, and
retained that report after restart. It explicitly checked the new startup
log's authority and finalized/60-descendant summary. Existing complete
blockchain/integration results are recorded in set 9.
