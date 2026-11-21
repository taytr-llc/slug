# Hosted attempt 1 public evidence — proposed archive

Source-only draft, November 21, 2026. This directory does not install an archive,
change repository documentation, restart an owner, or qualify a runtime result.
`archive-plan.proposed.json` names each proposed public copy with its exact bytes
and SHA256. Root reviews and installs it after the active initialization fix is
qualified. Its new destination root is `docs/evidence/set82/hosted-attempt1/`.
Do not overwrite an existing destination or reinterpret a preserved failure.

## Outcome and limits

The timed hosted attempt failed at the original standalone operator `init`.
The command returned exit 1 and `outcome_unknown`. The failed report records
only `SLUG-POOL-OPERATOR-ISSUE1.json` under the operator root; READY and every
component directory were absent. Miners, credential rotations, pool serving,
new share submissions and the payout test had not started. The original
operator process was absent afterward. The separate normal node and bridge
infrastructure were retained for authorized work; this is **not** a claim that
every service in the hosted environment stopped or joined.

The preserved selection starts at `1795280647345` ms, selects due time
`1795281060000` ms, and retains scenario/outer deadlines `1795281547345` /
`1795281727345` ms. Neither the failure index nor a later attempt may renew this
attempt's clock or replace its public setup. Its end observation is
`1795280915467` ms. The init command's separately recorded elapsed time is
1.5891809463500977 seconds; the interval from selection to failure observation
is not an operator-runtime measurement.

Root confirmed a deterministic source mismatch: the original operator path
uses `NewOperatorOwnedChainSource`, whose selected constructor at product
`138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c` still required a literal loopback
host. The selected private DNS HTTPS endpoint was therefore refused before
`constructStores` could create `miners`. `init` marks itself mutating before
the constructor and maps any later error to `outcome_unknown`; that public
code alone does not identify an ambiguous database write. The existing package
README recommended verified private-hostname HTTPS without accounting for this
ownership-client guard. This is the current test blocker to fix and qualify,
**not** an item to defer under EF-15 or EF-16. The forthcoming fix and its tests
need their own successor evidence; this draft makes no fix-pass claim.

## Separately observed bootstrap milestones

| Evidence | What it establishes | What it does not establish |
| --- | --- | --- |
| Package verification attempt 2 | Recorded native Linux package-job consistency and normal profile/source identities at product `138553b1` | Hosted volume, private RPC access or payouts |
| Operator build logs 1–6 / node logs 1–2 | Preserved unsuccessful uploads and later image/deployment build records | Successful application initialization or clean provider shutdown |
| Initial/audit image metadata | Original operator binary `7a8f7e783d7798a5200775d27e41bb077a196928e4e5a4c3f9baa510d65a443d`; idle bootstrap UID/GID 10001; mount 0:0/0755 and selected child roots 10001:10001/0700 | A live original operator, intact wallet balances or a final scratch PID1 join |
| Lifecycle overlay install metadata | Root-owned 0555 fixture witness with exact reviewed script hash | Execution of a serve generation or a clean shutdown result |
| `normal-bootstrap62.json` and node state captures | Genuine normal generation of 60 blocks from the original height-2 prefix to height 62, before timed issuance | Any hosted miner reward, payout, or external/public-chain participation |
| Preparation exit 0, public handoff and selection check | Public endpoint/TLS/schedule selection and unchanged independently derived economics; preparation reports original wallet creation/closure | Live TLS handshake, wallet/payment acceptance or successful init; checker explicitly says these remain unqualified |
| Failed init and public child metadata | Exit 1, intent-only frontier and no READY | A completed runtime or a deferred harmless edge |

The selected operator image for this attempt was
`sha256:92fb93be2d23379561180cfedf9fe4a988fed8f52033ffc61d48872d6c12d693`,
deployment `76a33e0e-4111-4f18-adec-ba583c052a19`. The selected node deployment
was `99516d38-4dda-4ae2-9de9-40af04e68039`. These are the original recorded
selections, not permission to target a different deployment or delete a volume.

## Public-only boundary and installation rules

Only the explicit plan entries are eligible. The plan is not a recursive copy
recipe. It includes the public failure/selection/report artifacts, bounded
metadata/status outputs, public certificates and setup, reviewed helper source
and source manifests, context manifests, and identified build logs. Historical
Go files are mapped to `.go.txt` to preserve exact bytes without becoming build
inputs. Existing referenced manifests are documentary records, not a claim
that all paths they mention are archive members or currently qualify.

Never copy `owned/`, runtime database files, custody/wallet files, the transport
directory, RPC credential configurations, private certificates/keys, seeds,
tokens, passphrases, process environments or the image/context tarballs. Do not
follow paths found inside a manifest to expand the selection. Binary identities
are retained in original execution metadata; executable binaries are not
copied. The broad `fixture-copied.json` catalogue is unnecessary to explain this
failure and is deliberately outside this minimal plan.

Before installation, root must recheck every named source against the frozen
hash/size and reject symlinks, nonregular files, path aliases, existing targets,
or an unexpected destination. Copy raw bytes, retain the source-to-target map,
then verify the complete installed file set and hashes. No install has occurred
in this task. Later provider/runtime results belong in a separate named
successor; do not append them into these original failure files.

## Upload failure chronology

Operator build attempt 4 preserved one fatal TLS `BadRecordMac` upload error.
Root reported reconciling the associated deployment to FAILED before attempt 5.
The separately saved `provider-failure-reconciliation.json` is a **later
reconfirmation** of deployment `9f55cd72-d0d5-4410-ab18-6fc91bbc18d1` as FAILED
with zero deployment log entries. It does not timestamp or independently prove
the earlier pre-retry observation. Preserve both this qualification and the
unsuccessful upload. Later successful builds do not turn attempt 4 into a pass.

`FOLLOW_UPS.proposed.md` supplies only EF-15 and EF-16 for root's burn-list
integration. `STATUS_UPDATE.proposed.md` is the concise repository-note text.
