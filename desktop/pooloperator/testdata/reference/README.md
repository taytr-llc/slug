# Independent setup reference

Run `python3 model.py inputs` in this directory. The output must match the sibling `setup-v1.json` exactly. The producer performs JSON/framing/SHA256 composition independently of Go; it consumes the frozen public authority, wallet and recipient fixtures copied into `inputs/`. Three positive setups, 21 cross-field refusals and five raw-codec refusals reproduce identically under Python 3.14.3 and 3.9.6.

`source-lock.json` preserves the original external source names (its `reference/` prefix denotes this directory); `generation.json` records the two-interpreter check. No private or live configuration is contained here.
