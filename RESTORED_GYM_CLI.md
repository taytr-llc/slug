# Independent gym recovery command

`slugrestore` is the separate Set73 consumer of a saved SLUG recovery file and
public fighter history. It restores the same wallet and gym into a new local
directory without a hosted login or the original app's private journals.
This candidate supports the two published Phase A fixtures. Production wallet
issuance and sign-in/onboarding remain separate work.

The owned macOS build includes it at
`SLUG Candidate Pilot.app/Contents/Resources/bin/slugrestore`. Building that
bundle does not itself establish end-to-end acceptance; see CHANGESET_73_NOTES.md
for the current qualification status.

## Inputs and continuity

Every invocation except `request-id` requires these selected paths:

| Flag | Purpose |
| --- | --- |
| `--directory` | Absolute, private state directory. `init` requires a fresh empty directory or a new path. Subsequent commands reopen the same directory. |
| `--public-config` | Independently obtained complete ownership pin and public evidence endpoint, version1. The endpoint cannot supply its own trusted pin. |
| `--node-config` | Private node RPC configuration file. This candidate accepts the existing literal-loopback HTTP profile only. |
| `--recovery-file` | Original SLUG recovery file for initialization and unfinished restoration. Once READY, ordinary commands do not need it. |

Keep input files outside the new state tree. Symlinks and hardlink aliases are
refused; the original recovery bytes and pinned source/node identity must remain
unchanged. The command never accepts phrase, private-key or
password literals as arguments. Keep the recovery file private and backed up;
the public source configuration is not a replacement for it.

`init` records the exact original file, source and locally chosen identities
before scanning. `resume` continues the same interrupted restoration. Readiness
requires explicit schema12-to15 activation and an audited wallet/journal pair.
An existing incomplete or damaged directory is never replaced automatically.

```sh
slugrestore init --directory /absolute/path/restored-gym \
  --public-config /absolute/path/public-config.json \
  --node-config /absolute/path/private-node-config.json \
  --recovery-file /absolute/path/SLUG-recovery.json
```

Use the same flags with `resume` after interruption. A relocated exact copy of
the original recovery file is acceptable. Different recovery bytes are not a
continuation, even if they belong to the same gym. After READY, retain the same
directory and public/node configuration for the following commands.

## Public reads and recovery discovery

`status` reports restoration state; `fighters` refreshes public evidence and
lists currently effective ownership. Transfers stay pending until the signed
checkpoint and sixty descendant blocks establish ownership under D83.
Unavailable history is an error, not an empty collection.

`discover` examines canonical commits belonging to this gym. It matches funding
and return paths only within the original file's exported native84 bounds.
Wallet lookahead or later allocations cannot enlarge those recovery bounds.

| Discovery status | Meaning |
| --- | --- |
| `matched` | The original file covers the needed paths and the exact escrow is currently unspent. A separate review is still required to sign. |
| `outside_original_file` | The file does not cover the commit's required funding or return path. |
| `currently_absent` | The exact escrow is absent at this observation, including a possible mempool spend. This is not permanent proof of a confirmed spend. |

Discovery does not create an action or sign a transaction. `actions` lists
retained requests; `status --request-id ID` inspects one exact request.

## Separate signing and delivery

For a new send, run `transfer --prospect-id ID --recipient-gym KEY`. For an
existing canonical commit, run `recover --commit-id TXID --kind recover_reveal`
or `recover --commit-id TXID --kind recover_refund`. Supply the common path flags
above. An optional `--request-id ID` lets you preselect a public correlation ID;
`request-id` generates one without opening a wallet.

The command emits the request ID before creating durable action state, then
shows the exact fighter, parties, return path and fees. Type the displayed
`approve REVIEW_ID` only after reviewing it. Approval signs the retained action;
it does not broadcast. A refund returns the escrow and does not transfer the
fighter.

Broadcast separately with `deliver --request-id ID --leg commit` or `--leg spend`,
each with its own current review and approval. A new transfer delivers its
commit first; its spend follows an observed commit. Commit-only recovery delivers
only the recovered spend, never the old commit again. Use `observe` with the
same request and leg to resolve an unfinished delivery. Once delivery is complete,
`observe` returns its unchanged saved receipt, including its original observation
time and mempool or canonical state; it does not refresh transaction inclusion.
Current fighter ownership comes from public replay. Delivery completion,
transaction inclusion, and fighter ownership finality are separate facts.

After interruption, inspect the saved request and use `resume-action` for that
same request where appropriate. `abandon` applies only to an eligible unsigned
action. Opening or inspecting a directory never automatically broadcasts,
replaces a transaction, mines, or installs a replacement hosted wallet. A retry
cannot change the retained transaction. `outcome_unknown` requires inspection
and exact reconciliation; creating a new request is not a safe substitute.

Each invocation has a ten-minute bound and cancels on interrupt. Public JSON
lines are bounded, while errors use closed codes rather than raw credentials or
source messages. Cancellation preserves durable progress and joins its owned
work before exit. The command is an engineering recovery path; the final
onboarding and recovery screens remain part of the later joint D74 review.
