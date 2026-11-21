# Independent daily binding reference

The producer and inputs were frozen before Go implementation. Root executed the
same producer with Python3.14 and Python3.9; both generated the adjacent
`../daily-v1.json`, SHA256
`9b8e52f41690db7ca1fee37c34f99dfcd0a7266b81a7793859871bd9b006ae30`.
`qualification.json` records that execution; `source-ready.json` pins each
relative producer/input file. Published synthetic wallet identities are inherited
unchanged. This producer derives configuration and guard bytes, not signatures.

From this directory, using a new output path:

```sh
python3 model.py --inputs inputs --out /tmp/slug-daily-wallet-reference.json
```

The generator refuses an existing output. Compare its bytes with
`../daily-v1.json`; never update expected values from Go output. The inherited
transaction/signature corpus remains in `../reference-v1.json`. This evidence
does not qualify a live Store capability, real keys or a funded payout.
