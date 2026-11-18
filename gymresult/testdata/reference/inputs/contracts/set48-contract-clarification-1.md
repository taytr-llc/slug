# Set48 clarification 1 — failed Snapshot error subset

This addendum supplements contract freeze
`11c8322b53d79ddcfefea05f37433eb5807fb3a52541b0e467df314d7ce27e15`.
The original core and helper documents remain byte-identical.

The helper contract's `ResultSnapshot.State == failed` permits exactly these
fatal errors: `result_unavailable`, `result_corrupt`, `result_clock`, and
`result_source_changed`. An internal closed dependency maps to unavailable;
a poisoned/corrupt dependency maps to corrupt. The validated authority fields
and job-ID constraints remain those of the frozen helper contract.

Ordinary busy, funding, scan, cancellation and uncertain-action outcomes belong
to the selected ResultJob/error response. They do not turn the entire Snapshot
into a fatal failure. Setup-incomplete and unavailable Snapshot states retain
their separately specified unions and exact error values.

This makes the inherited fatal-snapshot pattern explicit before reference
generation reaches these cases. It changes no JSON shape, byte cap, capability,
chain rule or player policy and authorizes no runtime code before the reviewed
reference commit. Bind this addendum in the reference input index and review.
