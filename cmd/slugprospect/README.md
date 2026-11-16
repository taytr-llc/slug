# slugprospect

`slugprospect` prepares public prospect commitments and verifies one work proof
against supplied context or a configured trusted node. Build from the repository
root in workspace mode:

```sh
go build -o /tmp/slugprospect ./cmd/slugprospect
```

The offline commands require a separately supplied trusted context. There is no
default chain policy or prospect target. The node or league adapter must
establish the context's chain, parent, target, difficulty and time bounds;
the offline commands do not fetch or independently authenticate them.

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
complete block. The separate local registry persists duplicate IDs and first
evidence; freshness, birth anchors and ownership remain additional work.
Neither offline command contacts a network or accesses private keys.

The online commands use an explicit policy and a separately configured trusted
node. They support the normal SLUG mainnet identity only and refuse
`slugtestpow`. They accept no `--context` flag:

```sh
/tmp/slugprospect context-node --policy policy.json --rpc-config rpc.json
/tmp/slugprospect verify-node --policy policy.json --rpc-config rpc.json --proof share.json
```

The policy JSON requires `version`, `network`, `genesis`, `dna_version` and
`prospect_target`. The RPC configuration requires `version`, `url`, `username`,
`password` and `ca_certificate`; the certificate is PEM text, or an empty string
to use system roots. Keep credentials in that file, never in command arguments
or URL user information. HTTPS verifies the server certificate; plaintext HTTP
is restricted to literal loopback addresses. Protect the credential file with
local file permissions. Policy and RPC files are bounded to 4096 and 65536 bytes,
respectively, with strict schemas. There is no default prospect target.

`context-node` prints the resolved public context. It is a snapshot that may
become stale; passing it to an offline command does not perform a new node check.
`verify-node` resolves context itself, verifies the proof, and checks the node
again before returning `{ "context_source": "trusted-node-snapshot", "context":
..., "verified": ... }`. Each online command has a 30-second deadline; an Argon
calculation already in progress cannot be interrupted. No successful result is
returned after a failed recheck.

The source label describes observations during this call. The inner verified
record remains `"supplied-context"`; neither snapshot establishes lasting chain
canonicality, authenticated discovery time, fighter birth, or account control.
For guarded local storage, use `slugregistry record-node --db PATH --policy
policy.json --rpc-config rpc.json --proof share.json`. That command checks the
node after proof verification and before writing or acknowledging a duplicate.
Use `slugprospect help` or a command's `-h` option for usage.

Gym-key authorization uses a canonical compressed secp256k1 public key and a
strict DER, low-S ECDSA signature. These commands accept public inputs only:

```sh
/tmp/slugprospect gym-id --context trusted.json --public-key <66-hex-public-key>
/tmp/slugprospect auth-payload --context trusted.json --proof share.json --public-key <66-hex-public-key>
/tmp/slugprospect verify-auth --context trusted.json --proof share.json --authorization authorization.json
/tmp/slugprospect verify-auth-node --policy policy.json --rpc-config rpc.json --proof share.json --authorization authorization.json
```

`gym-id` derives the chain-bound gym identifier. `auth-payload` returns public
fields `version`, `action: "prospect-admission"`, `gym_id`, `public_key` and
`digest` for a wallet client to sign. Preparing this payload does not verify
qualifying work. The CLI has no signing, key-generation or private-key input
command. The eventual wallet client signs automatically; this is not a player
signing step. Export and recovery of the wallet-controlled gym key remain
client responsibilities.

The authorization file contains exactly `version`, `public_key` and
`signature` and is limited to 4096 bytes. `verify-auth` checks authorization
before production proof hashing; `verify-auth-node` obtains node context first,
checks authorization before proof hashing, and performs the same final node
recheck as `verify-node`. Success adds `authentication: "gym-key-control"` to
the output. The inner verified result remains `"supplied-context"`. Gym-key
control does not establish account existence, registered ownership, fighter
birth, transfer rights or team-gym authority.

Authenticated evidence requires a new, explicitly initialized registry:

```sh
slugregistry init-auth --db /path/to/new-auth-registry --network 1397511495 --genesis HEX
slugregistry record-auth --db /path/to/auth-registry --context trusted.json --proof share.json --authorization authorization.json
slugregistry record-auth-node --db /path/to/auth-registry --policy policy.json --rpc-config rpc.json --proof share.json --authorization authorization.json
```

These commands persist authorization with the first evidence in schema 2.
Legacy evidence stores remain schema 1; neither format is automatically
upgraded or substituted for the other. Unauthenticated recording commands
cannot write authenticated stores, and authenticated recording commands cannot
write legacy stores. Existing `get`, `list` and `export` commands read either
format and re-verify its evidence. A replay preserves the original receipt.
