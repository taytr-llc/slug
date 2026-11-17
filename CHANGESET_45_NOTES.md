# Change set 45 — candidate event seed evidence

Branch `slug-event-seed`, stacked from set 44 implementation
`a00ada0d2d3c5e903a4160e598905c667c10ee32`. Engineering contract work is in
progress. No Set 45 implementation, acceptance or deployment is claimed yet.

Connect a complete immutable event snapshot to authenticated candidate
publication and retained seed evidence. The intended slice records the first
qualifying anchor, a fixed future target, explicit finality evidence, conflicts
and following-Saturday escalation without substituting a seed or cancelling.
It does not execute fights, award results or replace production ownership.

D76/D77 remain controlling. The exact publication, scan, checkpoint and
capability contracts require review and an independent reference freeze before
implementation comparisons. A block inclusion proof alone does not establish
firstness, a transaction digest is not publication authority, and a future
block hash is not proof of unbiased randomness or universal prior ignorance.

The candidate design uses only published fixture material and isolated owned
nodes. General wallet spending, real operator keys, public-chain publication
and public-chain mining remain outside this slice. One fixed publication chain
must survive account switching and restart; another fixture's chain cannot be
used as a fallback. Preserve D72 restrictions and explicit mining controls.

Set 44's exact release is deployed at master
`1fc307e07eec1be7f2466e0aabedb505f8844b0d`; its full 900.044-second live
observation and clean restart passed using frozen binaries independently of
this new topic branch. See `CHANGESET_44_NOTES.md` for the completed closeout.
Mike's complete joint D74 review remains required once major flows are ready,
before friends/tester or public release. The portrait-render check remains
pending at his request.
