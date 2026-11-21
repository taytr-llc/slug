# Independent live-round reference

`reference.json` is the exact 52,592-byte output produced independently under
Python 3.14 and 3.9; SHA256 is
`e26f0045170f1a25b91663e45d69309a420f310884cdd091fbf92710c37985fc`.
`qualification.json` records both successful runs. All producer inputs, the
frozen source, original approval lock and copy provenance are retained here.
The producer resolves its inputs relative to `reference/produce.py`.

The oracle covers conditional allocation arithmetic, full-Sync atomicity and
live-read trace expectations. It does not authenticate hashes, issue work,
confirm chain state or grant storage/financial authority. The Go modeled page
test consumes its exact fractions and 8/1/0 pages through a private decoder;
concrete owner/source/Sync/Refresh tests use the independently qualified normal
PoW pool fixture separately. Warm-work counts in the oracle are requirements,
not measurements from product execution.
