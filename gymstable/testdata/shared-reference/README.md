# Independent shared stable reference

`reference.py` uses Python's JSON, SHA256 and little-endian integer encoding.
It never imports or invokes Go. Inputs are the already frozen Set41 public pins,
whose complete corpus SHA256 is checked before deriving new values.

Both new pins deliberately select the first fixture's source authority for a
modeled shared configuration. This asserts no new admission, changes no signed
job and does not splice the old separate issuers' records into a shared store.
Dynamic tests obtain genuine multi-gym records from one newly created issuer.

The new Pin2 domain, marker bytes and sparse cursor models are engineering
contracts. Assignment sequences stay original source sequences. Model owner
labels are synthetic inputs for paging arithmetic, not authoritative receipts.

Regenerate with `python3 reference.py /absolute/path/to/corpus.json` and compare
bytes with `shared_v1.json`. Old pins, IDs, receipts and corpora stay unchanged.
