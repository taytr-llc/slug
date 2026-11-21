# Independent selected DAILY wallet wire fixtures

This directory contains an **unexecuted** Python standard-library producer for
the selected `WIRE.md` family. Root alone may execute it after finite source
review and an explicit execution lock. No Go/product module is imported, no
runtime service is called, and no expected output has been generated here.

The copied catalog contains exactly named public source declarations and the
previous independently qualified Set81 selection and Set79 DAILY reference.
It contains no active wallet, credential, runtime store, host tree or private
input. `load_inputs()` reads and checks these copies only; absolute source paths
in the catalog record provenance and are not opened during generation.

`producer.py` exposes `generate() -> dict` and a root-execution CLI:

```text
python3 producer.py --lock ROOT_AUTHORIZED_LOCK --output FRESH_EXTERNAL_JSON
```

The lock must have `execution_authorized: true`, the exact `source_root`, and the
complete list of local source/input files with byte lengths and SHA256 values.
Root stores its authorized lock outside this frozen source directory. The
proposed lock deliberately says false. The CLI refuses unbound/missing
files, writes only an exclusive external destination, and rechecks every bound
input before publication. The output cap is16MiB. Root should retain a finite
process timeout and capture bound and compare exact output bytes from both
selected Python interpreters. Generation has no clock, randomness or network.

The output schema is:

- `selection`: complete prior independent recoverable remote config, its pinned
  ID, one explicitly modeled original View, and exact selected native binding.
  The new ViewID/generation are correlation literals, not a created session.
- `cases`: each row has `name`, `kind` (`request` or `response`), `valid`, `scope`,
  `frame_hex`, `frame_bytes` and `sha256`. Frames include exactly one LF. Response
  rows include their ordered `request` object. A safe-generation snapshot row
  includes a full `selection_override`; other rows use the top-level selection.
- `continuations`: hand-selected first/current cache, sparse global window,
  stale, old-sequence and terminal cases. These return a modeled selection
  outcome only, never transport or account authority.
- `sequence_model`: five hand-derived monotonically increasing request-ID
  examples, including maximum uint64 and refusal of wrap/repetition.
- `bounds`: separate conservative byte arithmetic and a valid wide literal
  containing four4096-digit fraction components in both retained pages. The
  model checks reduced fractions and exact cumulative int64 strings. A
  disjoint16KiB envelope/snapshot allowance is added to two independently
  bounded pages, and the whole result must remain below65536bytes including LF.

Hand-tallied coverage is170 frame cases (55 accepted conditional shapes and115
refusals),11 continuation cases, and5 request-ID sequence cases. The producer
asserts these totals but no output has been generated to verify them yet. The
two peer-found source corrections preserve cached stale windows after a failed
continuation and the original payout validator's all-zero txid *shape* allowance;
neither is new account or payment authority. Source preimages are preserved in
the separate `reference-before-stale` sibling directory.

Cases independently check strict field order, required/null arms, scalar type,
canonical decimal spelling and overflow, full selected binding and View,
same captured header, exact eight-slot progression, retained previous captures
after failures, and exact framing. Existing per-payment numbers, time bounds
and safe numeric Snapshot/View generation are preserved. Empty matching pages
are not EOF. Cached first/current windows may remain readable after a capture
becomes stale; a new remote continuation may not silently recapture.

All financial and account combinations are **conditional codec/state-shape
models**. The wide numeric snapshot is not asserted to be a reachable wallet
state. Rebinding prior public money shapes to the selected gym is not proof of
that gym's balance. The copied source declares hash framing; the producer
independently calculates the settlement configuration ID and otherwise retains
the prior independent selected IDs. It does not validate all inherited crypto,
configuration or authority premises a second time.

Actual owner admission, authentication, original credential leases, View
post-response fencing, helper dispatch, output cancellation, live pagination,
wallet readiness and renderer behavior require separate component/actual tests.
No success of this producer closes those runtime gates.
