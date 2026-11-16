# slugprospect

`slugprospect` prepares public prospect commitments and verifies one work proof
offline. Build from the repository root in workspace mode:

```sh
go build -o /tmp/slugprospect ./cmd/slugprospect
```

Both commands require a separately supplied trusted context. There is no
default chain policy or prospect target. The node or league adapter must
establish the context's chain, parent, target, difficulty and time bounds;
this CLI does not fetch or independently authenticate them.

Prepare a commitment script for an existing public gym identifier:

```sh
/tmp/slugprospect commitment --context trusted.json --gym-id <64-hex-gym-id>
```

The indented JSON contains `version: 1` and `script_hex`, the public output
script to include with value zero in a coinbase template. This command neither
signs nor mines. It does not create an account or a private key.

Verify a proof against the supplied context:

```sh
/tmp/slugprospect verify --context trusted.json --proof share.json
```

On success, standard output contains the verified binding and DNA as indented
JSON, with `verification: "supplied-context"`. Errors go to standard error
with a nonzero exit status and no verified record. Verification uses production
Argon2id with a 256 MiB memory arena; `slugtestpow` builds refuse verification.

Inputs must be regular files. Context files are bounded to 4096 bytes and
proofs to 49152 bytes, including whitespace. Unknown, duplicate, missing or
null JSON fields and trailing values are rejected. The complete schema and
commitment rules are in [PROSPECT_PROOFS.md](../../PROSPECT_PROOFS.md).

A successful result verifies work and the committed gym identifier only
against the supplied context. It does not authenticate account control,
register a fighter, grant ownership, establish discovery time, or validate a
complete block. A future registry must handle duplicate prospect IDs,
freshness, birth anchors and ownership. Neither command contacts a network
or accesses private keys. Use `slugprospect help` or either command's `-h`
option for usage.
