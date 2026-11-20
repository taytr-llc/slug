# Change set 53 — durable pool acceptance

Branch `slug-pool-acceptance` stacks on Set 52 `5e0111b4`. The approved D80
canonical late-winner exception and acceptance contract were recorded at
`7ce4eea2` before implementation. App organization remains deferred to D74.

The new `poolshare` coordinator retains exact issued work and share submissions
in a separate synchronized journal. It uses the concrete manifest issuer,
original authenticated session, Set 52 normal-PoW checker and existing trusted
node adapter. Callers submit only a saved job ID and nonce. The operator's pool
payout and target remain pinned; the journal stores no session credentials.

Timely qualifying pool shares receive an immutable acceptance sequence before
block broadcast. All winning block intents persist before transmission. Late
winners and block wins missing the pool target receive acceptance only after
the exact payout-bound block is observed canonical. Ordinary expired shares are
rejected; no pool path issues a fighter admission or birth. Historical recovery
preserves acceptance across expiry and restart without allocating a second share.
Unknown writes poison the handle until reopen, and retained block status changes
remain possible when new-work capacity is exhausted.

Normal tests (including `prospectwork` regression), race tests, cheap-profile
refusal and vet passed under the recorded Go 1.27.4 no-CSE candidate profile.
Evidence: `../artifacts/set53/validation-attempt2/report.json`. Tests use frozen
independent work vectors and a real RPC client against controlled local node
responses, including wrong parent/winner/header and final-observation reorgs.
This is not a live-chain or deployment claim. Independent source review passed
after lifecycle, payout-validation and capacity findings were fixed.

Preserve attempt 1: it found an empty-slice proof-copy mismatch and was then
stopped by the sandbox's local-listener denial. The copy was corrected without
changing expectations; attempt 2 used permission for the loopback test server.
The acceptance-clock review also added a regression ensuring a slow separate
clock write cannot authorize a stale pre-expiry timestamp.

No existing caller, desktop route or seed configuration changes. There is no
seed rollout or desktop rebuild. Public transport/miner integration, round
allocation, orphan policy, rounding, transaction fees and settlement remain
ahead; no account balance or payout is created by this set.
