# slugcheckpoint

An offline Ed25519 signing tool for SLUG checkpoints. It never contacts a
node or a network service. Build it from the repository root in workspace
mode:

```sh
go build -o /tmp/slugcheckpoint ./cmd/slugcheckpoint
```

## Operator key

Run key generation yourself, from a private directory outside the source
checkout. The output file contains a 32-byte raw Ed25519 seed, not a wallet
seed or a mnemonic. Keep it offline and back it up privately.

```sh
/tmp/slugcheckpoint keygen --out checkpoint.seed
```

This prints only the 64-character hexadecimal **public key**. That is the
value to share for compilation into chaincfg. The private seed is never
printed or accepted on the command line. File creation uses mode 0600 and
refuses to overwrite any existing path, including a symlink. On Windows,
use an operator-only directory and file ACL; Unix permission bits are not
a substitute for Windows ACLs.

## Sign and verify

Obtain the chosen height and block hash from a fully validated node and
apply the agreed checkpoint publication policy before signing. Replace
`HEIGHT`, `BLOCK_HASH`, and `PUBLIC_KEY_HEX` below with public values:

```sh
/tmp/slugcheckpoint sign --key checkpoint.seed --network mainnet \
  --height HEIGHT --hash BLOCK_HASH --out checkpoint.json
/tmp/slugcheckpoint verify --pubkey PUBLIC_KEY_HEX --network mainnet \
  --file checkpoint.json
```

`mainnet` means the disposable SLUG Phase A network defined by
`chaincfg.MainNetParams`. The other supported choices are `regtest` and
`simnet`. The selected network supplies both network magic and genesis;
an optional `sign --genesis HASH` checks the expected genesis and cannot
override it. The block hash must have exactly 64 hexadecimal characters,
and height must be a positive int32.

The signature covers chaincfg's domain-separated binary checkpoint
encoding, not the JSON formatting. The public JSON file can be distributed;
private seed material is not included. Signing refuses to overwrite an
existing checkpoint file. Verification requires an explicit trusted public
key, checks the selected network and genesis, and bounds the input file.

A valid signature authenticates the operator's statement. This offline
tool does not prove that the block exists, belongs to the current best
chain, or has the required burial depth. Node checkpoint-state reporting
handles those checks separately.
