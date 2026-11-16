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

Birth admission uses a separate schema-3 registry and the fixed D68 rule:
age starts at job issuance, the job lasts exactly five minutes, and there is
no late-submission grace period. The issuer chooses its own clock and random
challenge and saves the job durably before returning it. No command accepts a
caller-supplied time, lifetime or birth date.

```sh
slugregistry init-birth --db /path/to/new-birth-registry --network 1397511495 --genesis HEX
slugregistry issue-job-node --db /path/to/birth-registry --policy policy.json --rpc-config rpc.json --public-key <66-hex-public-key>
slugregistry get-job --db /path/to/birth-registry --id <64-hex-job-id>
/tmp/slugprospect commitment-job --job job.json
slugregistry record-job-node --db /path/to/birth-registry --policy policy.json --rpc-config rpc.json --proof share.json --authorization authorization.json
```

Save the public `issue-job-node` output as `job.json` for the mining client.
`commitment-job` returns `proof_version: 2`, the job ID, context, gym ID and
commitment script. The client includes that commitment before mining, then
automatically signs the version-2 admission payload with its gym key.
`auth-payload` selects the authorization version from the proof version.
Job files are bounded to 8192 bytes. A correctly encoded job file alone is
not proof that an authority issued it: new birth admission requires the exact
job already present in that registry.

New admission checks the stored job, authorization, work, current policy,
node context and strict expiry before committing the immutable birth receipt.
The success output reports `birth_rule: "job-issuance"`,
`context_source: "trusted-node-snapshot"` and `current_node_checked: true`.
Birth uses the issuer's `issued_at_ms`; `accepted_at_ms` records admission
within the window. These are trusted issuer assertions, not an exact
cryptographic measurement of when a hash was found.

An already committed proof returns the original receipt with
`context_source: "historical-receipt"`, `current_node_checked: false` and
`inserted: false`. Historical recovery works during a node outage and after a
policy change; it does not renew age or claim current eligibility. Valid
policy and RPC configuration files are still required inputs. A reorganization
does not erase the original birth or permit a replacement birth date.

Schema-1 and schema-2 evidence cannot be promoted into a birth receipt or
substituted for job-bound work. Existing `get`, `list` and `export` commands can
read schema-3 receipts. Conditional proof verification remains available for
version-2 work, but it does not check issuance or the birth clock;
`verify-auth`, `verify-node` and `verify-auth-node` explicitly report
`birth_verified: false` for these proofs. Birth registration still does not
establish account existence, transfer rights or league eligibility.

Prepare a complete payable block candidate from a fresh node template:

```sh
/tmp/slugprospect prepare-work-node --job job.json --policy policy.json --rpc-config rpc.json --payout-address SLUG_ADDRESS --extra-nonce 0
```

The payout address is an explicit public SLUG mainnet destination supplied by
the local wallet or operator. The command preserves the node's transactions,
fees, header version and witness data, adds the job commitment to its coinbase,
and asks the node to validate the full candidate using GBT proposal mode. It
checks chain identity and the current parent before and after that proposal.
If the additional coinbase data exceeds a block resource limit, it fails and
requires a new template; it does not silently remove transactions or fees.

Success returns `proposal_accepted: true`, `work_verified: false`,
`admitted: false`, and `candidate`. The candidate includes the full `block_hex`,
the matching version-2 proof with a stripped coinbase, the payout address,
coinbase value, transaction count, weight and sigop cost. Its header nonce is
zero; the separate 64-bit coinbase extra nonce defaults to zero. This command
does not scan nonces, verify qualifying work locally, sign an admission, submit
a block, or record a fighter. Proposal acceptance is an observation of the configured node
and may become stale immediately. Local job-time checks before and after
preparation are hints only; they cannot establish authority issuance or extend
the issuer's fixed admission window. No private key input is accepted.
