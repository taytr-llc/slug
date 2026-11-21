# Hosted proofclient attempt 5 — exact auth refusal correction

This new source tree preserves the original eight production leaves, changing only `money.go:noAuth`; `go.sum` is an unchanged build-support copy. Original source/attempt4 evidence are untouched.

Source premise: `poolingress/protocol.go:requestSelectedRoute` requires one syntactically valid bearer for pool routes. Missing auth is rejected by `poolingress/server.go:ServeHTTP` before forwarding, with HTTP400, exact `pool ingress unavailable\n`, `Content-Type: text/plain; charset=utf-8`, `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`. `net/http.Error` provides that plaintext frame. A well-formed wrong bearer reaches selected poolservice, which returns HTTP401 with the exact canonical Version3 unauthorized/outcome_unknown:false JSON envelope. Ingress retains its no-store/nosniff headers and forwards the service application/json body.

The six existing probes remain: missing/wrong bearer for issue, balance and payouts. Events record bounded status, safe selected headers and body size/hash; the expected missing-auth literal or typed canonical service response is added only after its corresponding checks. No arbitrary response/body/error is accepted. Original deadlines, no-retry auth behavior, original selected bindings and all replay/money/payment assertions are unchanged. Refusals do not themselves prove allocation counts: root still correlates original status/closed census.

Only root may format, build or execute this unqualified source. The separate proof-control5 changes its A root and selects the newly built native-attempt5 binary, which must be exactly bound in attempt5 execution-selection. No runtime result is asserted.
