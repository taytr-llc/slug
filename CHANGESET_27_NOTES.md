# Change set 27: account enrollment and verified-reset recovery

`slug-account-enrollment` adds the D72 account boundary and a native macOS
Account Pilot using two published wallet fixtures. Verified reset preserves the
same wallet and gym, revokes existing sessions and starts a durable 24-hour
hold. The hold blocks withdrawals, fighter transfers, recovery phrase/file
export, recovery-authority changes, marketplace purchases and listings. Viewing
and explicit mining into the existing wallet remain available. Distinct resets
restart the hold; replay of the same verified event preserves its first result.

## Implementation

`gymaccount` owns a separately initialized schema-1 store with immutable
principal/account/root/gym binding, bounded login and child mining sessions,
domain-separated enrollment/recovery challenges, synchronous audit writes,
reset generations, clock rollback protection and capacity limits. Protected
operations hold an authorization lease through their commit and recheck expiry.
Reset, logout and rotation revoke derived credentials. Uncertain writes require
reopen; no schema migration, pruning or replacement-wallet fallback occurs.

The paired wallet fork signs only pinned enrollment/recovery challenges through
its retained wallet key lease. Independent Python vectors cover both public
fixtures. Rekey, reopen, retained export and fresh restore preserve the same
wallet and gym. Root public keys and hardened gym paths are bound by the explicit
fixture allowlist; a gym signature alone is not proof of root-key derivation.

The session-aware prospect service authenticates before bounded request bodies,
revalidates under the account lease through admission and historical receipt
return, and retains per-gym rate limits across credential rotation. Its active
operation permit precedes authentication so slow authorization cannot queue
unbounded requests. Existing static-credential mode remains explicit and separate.

The Go desktop helper prepares an account without mining, then enrolls or
recovers the original gym. Start remains explicit. Native login, logout and
reset controls share the recovery-dialog gate; the webview receives only public
account/hold state. Sensitive projection is cleared during failure and account
changes. A separate LocalAccountPilot namespace preserves prior pilot data.
Native recovery export and file-only backup verification use the retained wallet.

## Mining continuation regression

Native verification found a node bookkeeping defect: accepting a full block
without a preceding header left the best-header view behind. The private pilot's
readiness guard then correctly refused further templates, and its peer could
not obtain the missing header. A regression reproduced one canonical block and
one receipt followed by no continuation on the released set-26 node.

After successful full-block connection, the node now advances the header view
using the canonical index node only when cumulative work is greater. Ahead
headers, equal-work forks and rejected blocks do not get displaced. Existing
header-first node identity is retained. `getblockchaininfo.headers` now reports
the actual header height rather than duplicating full-block height. No PoW,
chain-selection rule, checkpoint policy or private readiness guard was weakened.

## Validation

Completed account gates include independent challenge vectors; race tests for
store, sessions, reset serialization, crash after durable reset acknowledgment,
uncertain writes, exact expiry and all hold capabilities; cross-gym admission
and revocation tests; wallet signing/rekey/reopen/export/fresh restore; and both
fixture identities in actual normal-PoW desktop flows.

Recorded local results include wallet signer race 146.756 seconds, account-store
race 3.599 seconds, prospect-service race 13.503 seconds, full desktop pilot race
332.051 seconds, and root RPC race 12.816 seconds. All ten chain and six wallet
modules build. Native bridge/projection/dialog tests and all eleven frontend
behavior tests pass. The initial account integration passed in 31.88 seconds
against the frozen set-26 node, with eight listeners closed after cleanup.
Actual native macOS actions verified account login, export, same-wallet reset,
held export refusal, sign-out and isolated second-account login.

Final continuation, updated node suite, exact-source remote CI, packaged artifact
and deployment evidence are recorded in the workspace release report after
completion; this note does not claim those pending gates have already passed.

## Deliberate boundaries and remaining work

This is a private mock-provider fixture app, not public registration or hosted
wallet deployment. Service recovery authority is custodial; exporting a backup
does not revoke it or constrain an independent exported copy. There is no
production derivation allocation, real key issuance, deployed identity provider,
KMS integration, marketplace execution or fighter transfer in this set. Their
future authority calls must enforce the capabilities already defined here.

The app is ad-hoc signed, not notarized; Windows runtime is unverified. The
normal-PoW account gate reuses set-25/set-26 funded maturity/spending evidence,
and does not claim a new funded spend. Exact hold expiry is tested with a private
test clock, while real-time integration exercises the active hold. Railway
continues to run only the normal node against its existing volume and history;
no account service, wallet, mining or pilot readiness flag is deployed there.
