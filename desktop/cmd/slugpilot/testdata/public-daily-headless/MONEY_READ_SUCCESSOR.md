# Pure money-read successor

Path A attempt1 remains a failed attempt. It reached the real scheduled payment,
validated its exact raw inputs/payee/change/fee and mined that transaction, then
stopped on its first DailyBalance `unavailable`. Original owners and both nodes
joined; the original fixture histories were preserved. This successor does not
relabel that attempt as an authenticated-money or completed acceptance pass.

The read path MinerBalance → verifyDaily → dailySource returns original
WalkSettlement errors. `poolservice.operationFailure(..., false)` recognizes
poolshare/poolsettle Busy but not poolround Busy, so that error maps to
`unavailable`. Client transport/validation failures can map to the same public
code. The attempt did not capture the underlying error; contention is a
source-supported possibility, not a proven diagnosis.

Only DailyBalance and PayoutsPage retry definite `unavailable`, alongside their
existing definite busy/rate_limited reads. One120-second child context bounds all
calls and polling under the original900-second scenario; no mutation or Lookup
retry changes. Other errors, outcome_unknown, cancellation, persistent failure,
invalid bindings or incorrect complete money evidence still fail. The32 initial
refusal samples, total count and last refusal are retained even on failure.

`07-confirmed-payment.json` is written after exact payment validation and minedTx
verification but before the read wait. `money-read-evidence.json` records the
bounded read outcome. On success the outer checker binds both artifacts to the
unchanged final payment/audit. Original1/2 reward economics,900/1080/180 bounds,
source/owner identities and all pending/issuehold assertions remain unchanged.
No product, Path B, private-input, Go or runtime work was performed.
