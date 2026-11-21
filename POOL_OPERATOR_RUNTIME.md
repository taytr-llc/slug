# Standalone pool operator

Set74 adds `desktop/pooloperator` and the headless `desktop/cmd/slugpoold`.
They use the existing paired desktop/wallet workspace; the seed daemon has no
new wallet dependency. Component qualification and isolated normal-PoW TLS
mining, scheduled payment and restart acceptance pass. Remote-host acceptance
remains a separate gate. The companion offline provisioning tool is described
in `POOL_OPERATOR_PROVISIONING.md`.

## Setup and custody

An independently selected canonical Setup binds the complete OperatorV2 profile,
pool source, immutable payees, round/settlement limits, scheduler, enrollment,
schema16 wallet guard, node fee policy, public TLS origin and private upstreams.
The exact byte contract is in `desktop/pooloperator/testdata/SETUP_BYTES.md`.
Setup contains no credentials. The journal cannot supply its own expected setup.

Create writes an exclusive intent and constructs seven fixed children: issuer,
pool, rounds, settlement, scheduler, ownership and miners. READY follows their
construction/audit. It means complete storage construction, not current chain
replay, funded custody or spendability. Partial construction resumes only that
same intent; established missing children fail closed. The original root stays
exclusively locked until clean shutdown. There is no history migration or reset.

The runtime borrows the concrete online authority and dedicated schema16 wallet.
Their custody contexts must survive Runtime.Close. Runtime work is canceled and
joined first; then the caller closes wallet and authority. Failed cleanup retains
the original handle and remaining dependencies. A backend binds once to the
original settlement; replacing a failed runtime requires wallet close/reopen.

The wallet has one fixed native84/index0 funding address and starts locked.
Explicit unlock renewal uses the originally selected protected passphrase file,
outside Sign. Its conservative expiry starts before reading/derivation. A failed
renewal locks custody; inability to establish locked state stops runtime work.
Signing requires the original settlement's immediate exact-request capability.
No generic signer, key getter, imported balance or alternative payee is exposed.

All selected local files use retained descriptors, owner/mode/ACL checks and
original identity checks. Their immediate directory is owner-only; files are
regular, single-link, mode0400/0600. Storage assumes a trusted private local
filesystem, not a hostile same-UID writer. Darwin requires CGO for ACL checks;
unsupported builds refuse. The official checkpoint private key is never an input.

## Service lifetime

Run starts the private pool, admission-only and ownership services before public
TLS ingress. Only the pool issues jobs. Authentication stays in each service;
ingress never trusts forwarded identity or retries uncertain requests.

Bounded public-history maintenance recovers original pending references before
publishing admissions, recording chain evidence and replaying ownership. Round
maintenance refreshes earnings but cannot prepare payments. One persisted UTC
scheduler performs recovery before daily payment preparation. Fees, minimums,
confirmation, maturity and exact-byte retries retain D58/D79/D80/D81 semantics.

Any unexpected service completion cancels siblings. Run joins every handler and
loop before locking the wallet. Close joins local actions and releases dependents
before their sources. Status contains dated observations, never current funding
or ownership authority. Local actions report outcome-unknown if mutation may have
committed before response or cleanup failed; retry the exact original action.

## Command

Build `./cmd/slugpoold` from `slug/desktop` with its committed `go.work`, the paired
wallet checkout, CGO and the recorded `scripts/candidate-go.sh` profile. It does
not launch Wails. A future Linux image needs both repositories and independent
target execution; the seed's existing Docker build is not an operator package.

Commands are `init`, `serve`, `status`, `rotate`, `revoke` and `checkpoint`. Each
requires these explicit local paths:

```
--setup-file --root --wallet-root
--wallet-public-passphrase-file --wallet-unlock-passphrase-file
--admission-seed-file --publication-seed-file --cutoff-seed-file
--rpc-config-file --tls-certificate-file --tls-key-file
```

The wallet must already exist with the matching schema16 guard, prepared by the
operator using `slugpool-provision`. The runtime command does not generate keys
or create a wallet. Init constructs/resumes the exact runtime;
other commands require READY. Stop serve before local management commands so the
original exclusive root is available. Rotate additionally requires `--gym-id`,
`--audience`, `--expected-generation`, `--token-file`, `--expires-at-ms`. Revoke
uses the same identity/generation flags without token or expiry. Checkpoint takes
`--checkpoint-file`, an already-signed public envelope. There is no online
checkpoint signing path, registration or hosted account recovery endpoint.

Serve emits bounded JSON readiness and dated status. SIGINT/SIGTERM stops intake
and joins shutdown. The supervisor must consume output; a blocked output write
is interrupted after five seconds and joined. Credentials and raw internal errors
are not printed. A listening process does not imply a public mining launch.

## Operational limits

The initial operator is a finite controlled run. Setup pins at most 256 jobs,
4,096 submissions, 64 enrolled gyms, 4,096 historical credential issuances and
explicit round/batch/byte limits. Authority history also has fixed quotas.

The runtime derives a smaller submission ceiling that reserves room for the
original rounds and settlement history, including exact fractional claims and
change-only batches. Even the largest currently supported setup admits at most
183 submission rows. Pending rows count, and separate nonces in one job consume
separate rows. A smaller setup can permit fewer rows or recovery only. These
limits can be reached before coinbase maturity; they do not provide continuous
public operation or guarantee payment of below-minimum or otherwise held carry.

New intake starts paused until original public replay and round synchronization
are ready. Reaching a validated limit permanently drains new job/share/fighter
intake for that runtime lifetime. Exact saved submissions and historical receipts
remain recoverable; retained rounds can confirm, custody renewal continues, and
the original daily scheduler still settles eligible earnings or recovers its
retained transaction. Status reports the reservation, count and drain reason.
Corruption, uncertain writes, custody and cleanup failures remain failures; they
cannot masquerade as ordinary capacity. Reopening an already over-bound old
journal permits drain only and cannot claim a retroactive reservation.

Rollover and compaction are not implemented. Do not remove financial journals,
repin a source or discard fractional balances to resume mining.

Set74's `e0e71ac0` closes the ordinary-intake D80 boundary within V1: an unseen
canonical winner from an original job can cross the ordinary ceiling after a
separate whole-history row/block reservation. Actual normal-PoW full-intake,
expiry, restart and unchanged-payout acceptance passed. A physically full or
financially unbacked journal still holds; the unallocated coinbase grants no
authority to spend or discard the miner's reward. Continuous public operation
still requires durable continuation beyond those limits.

Set75 starts that work with streaming round verification and settlement reads;
it does not raise the V1 ceilings. D84 requires visible whole-batch deferral if
all ready funding/payments cannot fit one daily transaction. Keep all balances,
do not select a partial payout, and recover previously signed bytes first. See
`POOL_CAPACITY_CONTINUATION.md` for the remaining acceptance boundaries.

Retain the complete operator root, independently selected public Setup, wallet
and protected custody. Public hostname/TLS, real online authority, enrolled
payees, node access and hosting must be selected before remote deployment. Only
published synthetic fixtures have been used in qualification. D73 production
issuance, D82 login and the final joint D74 design review remain separate.
