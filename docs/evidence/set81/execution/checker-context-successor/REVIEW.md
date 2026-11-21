# Narrow pre-execution checker successor

The original checker 505e74d4 and manifest 5fab8e8a are retained byte-for-byte in `predecessor/`. The original A lock f278 has not executed; root will supersede it explicitly.

Only a successful original lookup may now carry a deferred relay context observation of empty or `canceled`. The client attempt must still succeed with no context error, the original fixed deadline must hold, and the exact authenticated response/status/hash/length and complete downstream write must match. The relay failure remains empty. Refusal attempts still require an empty relay context. No unknown, canceled-client, partial-write, timeout or malformed-response acceptance is added.

This accounts for the client consuming and closing a complete reply before the relay records its deferred request context. No helper, product, source fixture, budget or economic predicate changes. Verification was source read and AST parsing only; no checker import, Go or actual run.
