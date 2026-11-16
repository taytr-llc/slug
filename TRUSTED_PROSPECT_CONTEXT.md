# Trusted prospect context, version 1

Change set 16 adds `prospectnode`, an explicit node/policy adapter for the
set-14 verifier and set-15 evidence registry. It reads an operator-selected
node and a separately configured prospect policy. Claimants supply the proof,
not the target or chain assertions used by the online verification commands.

This version supports only the existing normal SLUG mainnet profile and
genesis. It rejects `slugtestpow` and inherited Bitcoin/test networks. The
restriction avoids treating timestamp-dependent testnet difficulty as a
single expected-bits interval. A permanent production genesis or another
network requires a deliberate adapter update.

## Explicit policy and node configuration

The policy is strict, bounded JSON, with all fields required once:

```text
version: 1
network: 1397511495
genesis: 7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd
dna_version: 1
prospect_target: exactly 64 hexadecimal characters encoding a nonzero target
```

The target is an unsigned big-endian integer. It is independent of the block
target and pool variable difficulty. There is no default target, automatic
calibration, policy signature, or policy download. The operator must supply
the accepted policy; a proof cannot select its own policy file. The policy
ID and coinbase commitment remain exactly the set-14 byte contract. Changing
the target changes the committed policy and does not rewrite old receipts.
Calibration to D30's two-to-three-day discovery cadence remains required
before real gym registration.

The separate RPC configuration has required fields `version` (1), `url`,
`username`, `password`, and `ca_certificate`. The last field is an explicit
PEM trust bundle, or an empty string to use system trust roots. A supplied
bundle replaces the system roots for this client. Keep this credential file
private, normally mode 0600.
The commands accept its path, not credentials in command-line flags or URL
userinfo. They do not read wallet files or the league operator's signing key.

HTTPS verifies the peer certificate and hostname. Plain HTTP is permitted
only for literal loopback IP addresses, such as `http://127.0.0.1:29332`;
hostname-based HTTP endpoints are rejected. Redirects, environment proxies,
and automatic retries are disabled. Errors do not include RPC credentials,
URLs, response bodies, or server-provided error text. Policy input is bounded
to 4 KiB, RPC configuration to 64 KiB, and each response to 16 MiB. Calls and
complete operations have deadlines. The response bound can reject unusually
large templates; the adapter never silently falls back to caller assertions.
An Argon2id calculation already in progress cannot be interrupted, but a
failed or expired final node check cannot return a successful verification.

## Resolving one current-parent context

The adapter uses existing node RPCs; no consensus rule or RPC endpoint is
added. It checks network and genesis, reads a chain snapshot including the
tip and median time, requests `getblocktemplate` with `coinbasevalue`, then
checks the tip and chain identity again. It requires:

- The configured network and genesis at both identity checks.
- A consistent tip hash and height across the snapshot and final tip read.
- The template's previous hash and next height matching that tip.
- A positive canonical compact block target within the network limit, with
  the template's target matching its bits.
- A valid timestamp interval, the template time inside it, and minimum time
  equal to the parent median time plus one.

The resulting `prospect.Context` combines those node-derived values with the
explicit DNA and prospect-target policy. The node's peer/currentness checks
for mining templates remain in force, including the node's existing height-0
exception to its synchronization check. A disconnected node, a synchronization
failure reported by the node, inconsistent snapshot, malformed reply, wrong
network, or timeout fails the operation. No stale-template cache or
historical-parent fallback is used.

Trust is in the configured node and policy source. TLS protects transport;
it does not prove that a node is honest or globally current. A node-local tip
is an observation, not a permanent canonicality assertion. Header timestamps
and local receipt sequences are not authenticated mining or birth times.

## Verification and recording

`verify-node` resolves the context, performs normal production-PoW
verification, and resolves the node context again before returning success.
The parent, height, bits, and policy must still match, and the proof timestamp
must remain valid. An advancing upper time bound is allowed; a changed tip
requires a new attempt. No automatic retry reinterprets the proof.

`record-node` uses `registry.RecordChecked`: after proof verification, and
after checking existing evidence on a replay, a mandatory callback performs
the same node recheck immediately before the atomic write or duplicate
acknowledgment. A failed check writes no new receipt, consumes no sequence,
and returns no record. It does not erase a receipt that already existed.

Node state can change after that final observation and before or after the
database write. There is no transaction spanning the node and registry, no
consensus lock, and no lasting canonicality guarantee. The command performs
no network check after committing that could disguise a successful write as
a stale-tip rejection. Existing uncertain-write and output-failure rules
still apply: reopen and retry by stable ID rather than resetting storage.

The receipt schema stays version 1. Stored `verified.verification` remains
`supplied-context`, because the stored evidence is conditional on its context.
The online command's `context_source: trusted-node-snapshot` describes checks
performed for that invocation; it is not persisted node provenance or a
signature. Offline commands remain available with their explicit conditional
semantics. Failed or stale online checks never fall back to the offline path.

## Commands

```sh
slugprospect context-node --policy policy.json --rpc-config rpc.json
slugprospect verify-node --policy policy.json --rpc-config rpc.json --proof share.json
slugregistry record-node --db /path/to/registry --policy policy.json --rpc-config rpc.json --proof share.json
```

`context-node` emits the plain context format for inspecting the current
snapshot or preparing work; saving it does not keep it current. `verify-node`
emits the context and verified result with the source label. `record-node`
emits the original receipt and insertion flag with that label. Online commands
do not accept `--context`. Neither command mines, signs, broadcasts a block,
or creates a wallet. Registry initialization remains explicit.

Deployment packages these commands but does not create a league registry,
install a target policy, expose RPC, or start an admission service. Account
control, immediate ownership under D44, birth/freshness rules, reorg status
history, target calibration, and signed publication remain subsequent work.
