# Pool wallet recipient bindings

Set 58 binds each published fixture gym to its actual native-SegWit wallet
recipient. Pool payout destinations must use these wallet keys rather than
assuming the separate gym-signing key is a receive address. This is fixture
engineering; D73 production profile allocation and custody remain separate.

The payment path is `m/84'/0'/0'/0/0`, taken unchanged from the independent
`slugwallet/recovery/testdata/recovery_v1.json` corpus. Its SHA256 is
`d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a`.

| Fixture | Recipient | Script |
| --- | --- | --- |
| zero256 | `slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme` | `001416d630413cea75fd5ad88433cd815ae5ba1ba645` |
| incrementing256 | `slug1qslk39wvggqa0vl8nd6jckaz54dw3vk45c8educ` | `001487ed12b988403af67cf36ea58b7454ab5d165ab4` |

`walletRuntime.EnsurePoolRecipient` is separate from `EnsurePayout`, whose
legacy scope44/index0 contract remains unchanged. The new helper checks the
complete published root/gym identity and recovery profile, the exact public
scope84 cursor, RPC ownership, in-process derivation information and native
P2WPKH script. It never derives or accepts an arbitrary caller-selected payee.

A nil expected recipient permits first allocation only when the native84
external cursor is zero. A nonnil expected recipient is verify-only: an
established destination missing from the wallet fails closed. A retained
recipient is recovered directly without advancing the cursor.

Wallet address allocation commits before notification and HTTP acknowledgment.
Any failure after dispatch poisons allocation on that wallet runtime. The owner
must close and reopen it, reattach its node connection and allow synchronization
before recovering the exact index0 destination. No replacement allocation is
made after an unknown response. Reattachment re-establishes retained watches.

The outer installer writes public, immutable `SLUG-POOL-RECIPIENT.json` intent
before allocation. It binds account authority, account ID, published fixture,
full recovery identity and complete pool source configuration/binding ID.
`SLUG-POOL-RECIPIENT-READY.json` appears only after ownership/path checks, wallet
synchronization and reauthorization of the original token and login view.
Generation and view credentials are checked live, never serialized as authority.

Both markers use bounded canonical JSON and exclusive atomic publication.
Existing files must match exactly; accepting an existing marker repeats the
directory synchronization, including recovery after uncertain publication.
Established callers cannot recreate a missing intent, and READY without intent
is invalid. Reading public marker JSON alone does not authenticate ownership.

Wallet RPC never holds an account/view lease. Short leases protect initial
intent and final publication; callback omission, swallowed failures or repeated
callbacks fail closed. Reset, account switching or token replacement cannot
authorize an in-flight operation under the previous view. Mining and receiving
into the same wallet remain permitted during D72's post-reset hold.

The current desktop route, old wallet markers and old node configuration stay
unchanged. A shared pool parent must use an explicit txindex-enabled owned node,
pin both recipient scripts before creating its immutable settlement store, and
drain borrowed wallet/account resources in the established lifecycle order.
Shared-source fighter isolation and desktop wiring follow this binding layer;
the full joint D74 design review remains last.
