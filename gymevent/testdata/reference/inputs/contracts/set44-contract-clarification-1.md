# Set 44 contract clarification 1 — terminal history cursor

Accepted engineering clarification, November 17, 2026, before independent
reference freeze or any comparison with Go runtime results.

The approved core contract defines HistoryPage's fields but does not specify
the terminal value of `next_event_id`. The exact rule is:

- When `has_more` is true, `events` is nonempty and `next_event_id` equals the
  last returned EventStatus.Event.EventID.
- When `has_more` is false, `next_event_id` is the empty string, including an
  empty terminal page. It does not repeat the incoming cursor.
- `after_event_id` still echoes the requested exclusive cursor, which must be
  empty or identify an actually retained event. This rule grants no permission
  to adopt arbitrary cursors or to omit/duplicate retained rows.

This closes an omitted engineering detail; it changes no user decision,
historical input, hash domain, field order, old entry/stable API, or source
journal. The independently authored reference and companion helper consumer
already use this terminal-empty interpretation. The new event runtime must
implement that rule. No Go output is used as a reference expectation.

The original frozen contracts and their reviews remain intact:

- Core: 7e0c2842cb6fac2c67896c8e646ffe5778f1a0d03d8ec0e386a074e289537379
- Helper: fedecce1bb5f7a4c975390c915dd8671850790d284899194e23b1e7329cc82f4
- Approval: 13ee5d8f202289bc3c216ab4b5524e487bc11f81f9ea5d87d27e0562da504133

The final independent reference must bind this addendum alongside those bytes.
