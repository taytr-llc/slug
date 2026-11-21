# Core selected-file ownership and reference consumer successor

Source-only preparation; no Go/gofmt/build/runtime execution. Root alone installs and qualifies.

The registry delta keeps the exact original marker, guard, LF, namespace, signed limits, request/job and source state semantics. Only new selected constructors use private bounded exclusive file helpers. They retain the exact descriptor and sticky error only on Close failure; no repeated Close occurs on that descriptor. Post-DB constructor failure still closes the original DB once and retains any joined cleanup failure. The same helper handles constructor directory-sync descriptors so a later failed Close cannot overwrite the retained failure with a successful DB Close. Legacy file helpers and constructors are unchanged.

Two helper tests distinguish conditional already-closed-descriptor retention from real successful Close. They do not claim to induce a kernel close failure in an actual constructor or to measure leaked resources. Existing constructor/reopen tests continue to prove normal byte selection.

The poolshare test-only delta fixes a consumer inventory mistake exposed by root core-normal-attempt1: bounds.source has two aggregate four-input cases, not only six-input single-request cases. Both forms now use production reservation/headroom helpers against frozen independent values. Aggregate cases verify the one shared canonical receipt reserve. Outside-selected-row/template input cases are explicitly domain assumptions enforced by decoders, not claimed arithmetic helper refusals; overflow is checked against the real occupancy predicate. Corpus bytes and producer remain unchanged.

No files in the main checkout were changed by this stage. Apply only after source-bound runs finish and all recorded preimages match. Root gofmt may change whitespace before qualification and must record the resulting source inventory.
