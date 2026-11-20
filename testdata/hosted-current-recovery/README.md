# Set71 independent public references

The contract, 75 public source inputs and producer source were frozen before
the first expected output or dependent implementation. Python 3.14.3 and 3.9.6
produced identical bytes. `reference/reproduction.json` records those hashes;
each `*-producer-lock-v1.json` records the separately approved source hashes.
The original failed preparation proposals and review corrections remain in the
external Set71 preparation evidence; none was executed as an oracle.

The three expected files are stored once at their consumers:

- `provider.json`: exact provider claims/signatures, setup/control markers and
  driver arithmetic; the inherited RFC7515 signature/corpus is reproduced first.
- Paired `slugwallet/wallet/testdata/hosted-current-recovery/public.json`:
  synthetic mixed-row framing, bounded public inventory and conservative sizes.
- Paired `slugwallet/recoveryvault/testdata/hosted-current-recovery/current.json`:
  current public metadata bytes, CAS/audit arithmetic and modeled file stages.

These are public fixture evidence only. Model IDs grant no account, wallet,
retention or filesystem authority. Synthetic encrypted spans do not prove real
ciphertext handling. Concrete owner, capability, database and file lifecycle
tests remain necessary. Dated checkpoints do not imply continuous wallet backup.

With the paired checkouts in the workspace layout used by `desktop/go.work`:

```sh
python3 -B testdata/hosted-current-recovery/reference/reproduce.py --output /tmp/slug-set71-reference-new
```

The output directory must not exist. No Go implementation, live service, wallet
database, private export or production key is used by these producers. Sources,
inputs and expected files are never overwritten by reproduction.
