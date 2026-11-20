# SLUG mainnet address fixture derivation

This bundle preserves the frozen independent source and inputs used to translate
inherited Bitcoin mainnet test encodings. Only Base58Check version bytes
(0x00→0x53, 0x05→0x47), or the Bech32 HRP (bc→slug), and their checksums change.
All decoded hash160 and witness-program payloads remain identical. The original
before/address tests and input params snapshot are retained verbatim. Production
code was not used to derive the expected strings.

`source-ready.json` froze the producer and all inputs before execution.
`qualification.json` records identical output under Python3.14.3 and3.9.6.
It is Python reference evidence only, not a claim of Go test qualification.
The output includes11 positive translations and2 checksum mutations. The Go
tests additionally retain12 Bitcoin-mainnet refusal cases, including uppercase
P2A. Existing network-independent BIP173/BIP350 primitive vectors are unchanged.

To reproduce with a standard-library Python3 interpreter from this directory:

```sh
python3 reference.py /tmp/slug-mainnet-vectors-reproduced.json
cmp reference-py314.json /tmp/slug-mainnet-vectors-reproduced.json
```

The producer checks all frozen source/input hashes and known Base58Check,
Bech32 and Bech32m primitive controls before writing the output. It validates
each original checksum and verifies payload equality after translation. Do not
replace expectations from Go output; preserve prior source and failed runs if
this independent recipe ever needs correction.
