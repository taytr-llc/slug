# Change set 44 — candidate event snapshots

Branch `slug-event-snapshot`, stacked from set 43 implementation
`17e0362244c2bd419ed30ee902887b0ece2ad268`. Planning/contract preparation is in
progress; no implementation, package or deployment is claimed yet.

D76 and D77 are accepted and recorded in the spec and `EVENT_LOCK_POLICY.md`.
They freeze the complete noon eligibility/ownership/condition snapshot and
require waiting without seed replacement or automatic cancellation, with a
chain-incident escalation threshold at the next Saturday lock.

This slice prepares bounded, recoverable candidate snapshots using retained
roster/entry history and original source evidence. Exact contract, independent
reference generation, implementation and execution checks follow. The fixture
assignment model cannot establish transferable production ownership. Actual
seed selection/finality and operator incident handling remain later integration
requirements. Existing D65/condition/fight-engine behavior, birth, chain
consensus, wallet derivation and D72 restrictions remain unchanged.

The prior set's package/deployment closeout is now recorded in
`CHANGESET_43_NOTES.md`. Mike's portrait check and thorough joint D74 design
review remain pending before friends/tester/public release.
