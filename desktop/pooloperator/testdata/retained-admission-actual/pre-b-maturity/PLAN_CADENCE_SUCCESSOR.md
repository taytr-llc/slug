# Set81 pre-execution cadence correction

The approved parent plan remains byte-identical at `../RETAINED_ADMISSION_ACTUAL_PLAN.md`, SHA256 `62ddde39c811001865cd91c9b11152aa7166c4a7f69044d5946afd018ee23a72`. The qualified actual selection17557b28 and its producer/input freeze remain unchanged.

Source authoring found that the production operator CLI publishes status every30seconds (`desktop/pooloperator/command.go`), independently of the selected1000ms maintenance interval. A new ten-second HTTP hold cannot reliably capture that next status. Root approved this correction before helper freeze or execution:

1. Hold the exact second-accepted-pool-receipt fighter request before forwarding. Immediately Stop/join that original CLI and its held handler within the unchanged call/hold bounds. Preserve the original signed pending row. No upstream admission occurs.
2. With no held HTTP request, observe original pool-capacity drain and both readiness conditions through the existing public operator status. Use the inherited90-second read-only readiness bound, within the same900-second parent budget. Stop/join the operator and perform the full closed original source/issuer/outbox audit.
3. CaseA reopens the same operator, observes its fresh full-source readiness before starting the same CLI/outbox, then short-holds/releases the original target request, loses one genuine successful reply, joins, audits and reopens again for exact historical recovery. The additional initial stopped CLI generation is recorded.
4. CaseB keeps the initial target never-forwarded, waits its real remaining expiry with every worker/handler stopped, then reopens the same original owners and retries that exact signed row for authoritative expired completion. No held call spans the real-expiry wait.

No request deadline or ten-second relay hold is increased; no manual submission, clock, signature or replacement target is introduced. Every target request/job/proof/authorization remains identical. The overall900/1080-second and180-block ceilings, actual two-reward requirement and fixed economics remain. Unrelated finite terminal q rows are retained and reported as already selected.

This adds lifecycle evidence rather than depending on lucky alignment between mining and the CLI ticker. A failure to stop/join, reach fresh readiness, remain live for CaseA, or finish the originally selected UTC due window still fails the attempt.
