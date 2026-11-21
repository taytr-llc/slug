# Independent daily reservation bounds

This corpus extends no V1 expected values. The separate frozen DAILY contract
and literal source/round/indexed-ledger frames define conditional whole-history
bounds. The root-qualified Python 3.14.3/3.9.6 outputs are byte-identical:
`2db086115f51b0c7e22d2fd7688ad5b46c56e238fa1346d709c9a19bebb7aa78`
(81,247 bytes). `qualification.json` preserves the original report.

`reference-v1.json` contains 14 recovery cases, six independently exhaustive
ordinary envelopes, 14 invalid-input cases, 34 configurable-byte edges, nine
producer JSON refusals, full grammar and separate request-copy bounds. It also
records conditional source-byte holdback and kernel requirements; those are not
proof of authority, canonicality, actual admission or real funding.

Reproduce to a **fresh absolute output path** from this directory:

```
python3 reference/produce.py --lock reference/producer-lock.approved.json --output /absolute/fresh/reference.json
```

The producer checks every frozen input before and after computation. Its
source-map file is provenance, not an import of product source. No product
encoder, node, wallet, clock or private store is read. The exact executable input
copies and original locations are recorded in `provenance.json`; no self-derived
replacement oracle is accepted by the Go tests. Original V1 oracle stays separate.
