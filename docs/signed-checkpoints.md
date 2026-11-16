# Signed league checkpoints

SLUG checkpoints declare the official league record. Nodes verify the
operator's signature and report conflicts; they continue ordinary proof-of-work
validation and chain selection. The key never vetoes a competing chain.

## Operator authority

Mike supplied this Ed25519 public key on November 15, 2026. Set 10 compiles
it into `chaincfg.MainNetParams.CheckpointPublicKey` for the disposable
Phase A SLUG network:

```text
811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3
```

`getcheckpointinfo` reports `authorityconfigured: true` and this public key
in builds containing set 10. Other networks have no enrolled authority.
Set 11 archives Mike's first signed record at
[`checkpoints/phase-a/height-1.json`](../checkpoints/phase-a/height-1.json).
It was verified and imported locally, reporting `finalized` at tip 61 with
60 descendants. The seed serves that chain but still runs the older client;
no deployment or automatic record loading is implied.

The offline utility is documented in [cmd/slugcheckpoint](../cmd/slugcheckpoint/README.md).
Build it from the repository root:

```sh
go build -o slugcheckpoint ./cmd/slugcheckpoint
```

Keep the private file on the Mac and back it up offline. Share only the
printed 64-character public hex string or a signed public JSON document.
The agent must never read the private seed. A future key change requires
explicit enrollment and distribution in a client update.

## Publish a checkpoint

From a fully validated node, select a positive height at most
`current tip height - 60`. Verify its hash and chain before signing.
Height 1 first becomes eligible at tip 61. The first Phase A record was
signed after this threshold was reached and independently verified through
a fresh node connected to the seed. Do not sign an immature block and
describe it as finalized.

Sign offline using the explicit public height/hash and your private file,
following the tool's README. Distribute only the resulting JSON document.
The utility verifies its signature but cannot prove chain membership or
burial without access to a node; that remains the operator's check.

A node built with the enrolled public key loads documents explicitly:

```sh
slugd --signedcheckpoint=/absolute/path/checkpoint.json
slugctl getcheckpointinfo
```

For the bundled Phase A record, use its absolute checkout path. The Docker
image installs it at `/usr/share/slug/checkpoints/phase-a/height-1.json`;
deployment still requires adding that explicit startup flag. See the
[checkpoint directory](../checkpoints/README.md) for verification commands.

Repeat `--signedcheckpoint` for several heights. Restart to reload. Files
must fit within 4096 bytes and contain exactly the six fields `version`,
`network`, `genesis`, `height`, `hash`, and `signature`. Unknown, repeated,
missing, null, malformed, wrong-chain, or invalidly signed records fail
startup. Duplicate heights are rejected, including identical copies.

Never translate these records into `--addcheckpoint`: that older option
changes validation behavior. `--nocheckpoints` controls legacy checkpoints
only and does not suppress league reporting.

## Read the report

`getcheckpointinfo` is available to read-only RPC users and through
`rpcclient.GetCheckpointInfo`. It returns one coherent best-chain snapshot,
the compiled authority, the required 60 descendants, and every authenticated
signed record with its current status:

| Status | Current local observation |
| --- | --- |
| `pending` | The node has not reached the signed height. |
| `provisional` | The hash matches, with fewer than 60 descendants. |
| `finalized` | The hash matches, with at least 60 descendants. |
| `conflict` | The local hash at that height differs from the signed official record. |

The top-level `conflict` flag is true if any record conflicts. Consumers
must check it across the complete record set, rather than treating a later
matching record as erasing an earlier conflict. Matching records expose
their descendant count; pending/conflicting records report zero there.

A signature authenticates the official declaration; status describes the
current local chain. A reorg may change status without revoking the signed
history. Gyms, the marketplace, and league services decide how to act on
that conflict; the node does not switch or reject branches because of it.

## Format and key changes

Ed25519 signs the ASCII domain `SLUG signed checkpoint` plus a NUL byte,
then version uint32, network magic uint32, genesis hash bytes, height uint32,
and block hash bytes. Integers are little-endian and hashes use internal
wire byte order. The supported version is 1; heights are positive int32.
JSON hashes use conventional reversed display hex, and the signature is
128 hex characters. Signing JSON text itself is not the protocol.

There is one compiled authority per network. Losing the private key stops
new signing until a client update enrolls a replacement public key. Historical
documents remain independently verifiable against the old public key;
multi-key trust and rotation policy are not implemented in this set.
