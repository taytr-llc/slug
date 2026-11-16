# Prospect evidence registry, version 1

Change set 15 adds the local `registry` package and offline `slugregistry`
command. It durably records work already defined by `PROSPECT_PROOFS.md`.
Every result remains conditional on the supplied context. A receipt is not
registered ownership, proof of gym control, canonical-chain acceptance, a
birth date, or permission to enter the league.

## Identity and immutable evidence

A registry belongs to one explicitly configured network and genesis. It
accepts a raw context and proof, runs the production verifier, and records:

```text
version: 1
sequence: positive, consecutive local receipt number
context: the first supplied context
proof: the first supplied proof
verified: the verifier's complete supplied-context result
```

The stable prospect ID is the set-14 header-and-chain identifier. All
admissions are serialized. The first valid submission reserves that ID and
receives the next sequence number. A subsequent valid submission returning
the same verified result returns the original record with `inserted: false`.
Hex case, proof representation, or equivalent context time bounds cannot
replace the first evidence or allocate another receipt. A different verified
result under an existing ID is an error. Invalid submissions reserve nothing.

Records have no update, delete, prune, or reset operation. IDs remain reserved
when the chain later changes; this tool neither follows the chain nor declares
records orphaned. Future status or revocation assertions must preserve original
evidence and IDs. A receipt number orders local storage, not mining events.
No clock or age is stored. Establishing a trustworthy immutable birth anchor
and freshness rules remains required before real prospect registration; late
submission must never make an old fighter young again.

## Storage and failure behavior

Use an explicit dedicated directory, separate from the node's block database.
The implementation reuses the repository's LevelDB dependency. Creation
requires a new directory; normal opens reject missing or unrecognized stores.
An application marker identifies the format before LevelDB is opened, and
database metadata binds the schema and chain identity. Unknown schemas and
corruption fail closed. There is no automatic repair or migration.

The root directory contains `SLUG-REGISTRY.json` and the LevelDB `db/`
subdirectory. Metadata tracks the format, chain, and last sequence. Record
keys use `record:<lowercase prospect ID>`; sequence keys use `sequence:`
followed by an unsigned eight-byte big-endian sequence, with the ID as their
value. Stored JSON is canonical to this format, rather than arbitrary source
JSON text. First-input hex spelling is preserved; an empty branch is stored
as `[]`. Opening checks the records and both index directions for missing
entries, gaps, mismatches, and unrecognized keys.

One synchronous atomic batch writes each record, its sequence index, and the
next sequence. The LevelDB process lock prevents two writers from opening the
same directory; an in-process mutex covers duplicate detection and the write.
Acknowledgment follows the synchronous write. The guarantee depends on the
filesystem and hardware honoring synchronization. This is a logically
append-only API, not an append-only physical medium.

Initialization requires successful file and directory synchronization. A
platform or filesystem that rejects either causes initialization to fail
with the incomplete directory retained. Runtime verification for this change
set is on macOS; Windows runtime behavior has not been validated.

A write error can mean that the commit outcome is uncertain. The handle then
refuses further operations until closed and reopened. Reopening reconciles
the durable metadata and indexes; retry the original proof by ID. Do not infer
that an error means no data was committed, and do not delete the directory to
retry initialization.

Open checks structure and index consistency without rehashing the entire
history. Reading a record re-verifies its stored proof and compares the full
verified result before presenting it. Listing and exporting do the same for
each emitted record. The cheap `slugtestpow` profile cannot verify or admit
production evidence. Records are bounded to 64 KiB and list pages to 100
records. A consistent export streams one JSON record per line in sequence
order. Output can be partial on an I/O or verification error; only an exit
status of zero denotes a complete export.

The database is local trusted storage. An administrator can delete or roll
back the whole directory, or rewrite mutually consistent assertions. The
marker, indexes, and checks do not provide signatures, remote replication,
external rollback detection, authenticated policy, or protection against a
malicious database owner. Stop and close the registry before copying its
complete directory for backup. NDJSON is portable evidence, not a database
backup; there is no import or restore command in this version.

## Offline commands

Build with `go build -o /tmp/slugregistry ./cmd/slugregistry`. All commands
require an explicit database path. Initialization also requires explicit
network and genesis; there is no default chain or target.

```sh
slugregistry init --db /path/to/new-registry --network 1397511495 --genesis HEX
slugregistry record --db /path/to/registry --context trusted.json --proof share.json
slugregistry get --db /path/to/registry --id HEX
slugregistry list --db /path/to/registry --after 0 --limit 20
slugregistry export --db /path/to/registry
```

`record` returns `{ "inserted": true|false, "record": ... }`. `get`, `list`,
and `export` open read-only. `list` returns records after the supplied sequence,
up to the requested limit; the last sequence is the next page's cursor.
Its defaults are `--after 0` and `--limit 100`.
`export` writes NDJSON to standard output. Shell redirection is the caller's
responsibility; avoid overwriting an existing archive inadvertently.

The command performs no network access, mining, signing, wallet operations,
or publication. There is no public admission server. A service integrating
the package still needs bounded queues, resource limits, account authorization,
and an authenticated node/policy source. Normal node startup does not open a
registry, and deployment of the binary does not create a league database.

Set 16 adds `record-node` and `RecordChecked` for a current-node observation
immediately before admission or replay acknowledgment. A failed pre-commit
check writes nothing. The node can still change afterward; this does not
make the receipt permanently canonical or persist authenticated provenance.
The existing offline commands and receipt schema retain their original
conditional meaning. See `TRUSTED_PROSPECT_CONTEXT.md`.

## Verification basis and remaining work

Tests reuse set 14's independently derived synthetic production-Argon2id
fixtures. They are not actual pool attestations or an approved production
prospect-target calibration. Registry tests exercise storage and replay
semantics without defining new cryptographic golden outputs.

Account and transfer authority, trustworthy birth and freshness rules,
canonicality/status handling, target calibration, mining-template integration,
publication signatures and checkpoint anchors remain separate work. League
signatures continue to report the official record without changing normal
chain selection.
