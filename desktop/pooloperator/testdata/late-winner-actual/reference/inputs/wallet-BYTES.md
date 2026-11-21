# Pool wallet16 bytes — proposed freeze

This appendix fixes new operator-only bytes before an independent producer. It does not modify any existing settlement record, backend interface, hosted/recovery guard or production profile table. Root approved derivation identifier `slug-phase-a-operator-pool-native84-v1`, standard BIP32 `m/84'/0'/0'/0/0`, for current Phase A only.

All JSON below is compact UTF-8, exact field order as listed, no insignificant whitespace, duplicate/unknown/missing fields, escaping aliases or trailing bytes. Integer fields use ordinary nonnegative decimal integer tokens. Hex is lowercase, fixed-length, no prefix. Decoders bound bytes before decoding and require exact canonical roundtrip. A hash `H(label,raw)` is SHA256(ASCII(label) || NUL || ASCII `v1` || NUL || LE32(len(raw)) || raw); result lowercase64hex. This is not the old poolservice digest recipe.

`PoolSettlementGuard`, cap4096 bytes, ordered fields:

1. `version` =1
2. `manager_schema` =16
3. `wallet_owner_id`: nonzero64hex, independently selected before wallet creation
4. `operator_profile_id`: nonzero64hex, independently selected full operator profile identity
5. `source_binding_id`: nonzero64hex from unchanged `poolservice.BindingID` over full original source
6. `settlement_config_id`: nonzero64hex from `ConfigBindingID` below
7. `derivation_id`: literal `slug-phase-a-operator-pool-native84-v1`
8. `network`:1397511495
9. `genesis`: literal `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`
10. `root_public_key`: canonical SLUG Base58Check extended public master key, at most120 ASCII bytes, decoded78-byte payload; version534c5547, depth0, parent fingerprint0, child0, compressed secp256k1 point. This is not a serialized private key.
11. `funding_public_key`: canonical compressed secp256k1 point,66hex
12. `funding_script`:44hex, exactly `0014`+HASH160(decoded funding_public_key).

`PoolSettlementGuardDigest` is H(`SLUG operator pool wallet guard`, canonical guard). Root/path consistency is checked against the concrete encrypted manager at create/open; shape validation alone proves no key ownership. There is no mutable ready/locked/bound value, cursor claim or financial ledger in this guard.

`poolsettle.ConfigBindingID` first uses the existing closed settlement config validation, then canonical `json.Marshal(Config)`, cap65536 bytes, existing order `version,change_script,destinations,max_rounds,max_batches`; each destination stays `gym_id,script` in strictly increasing GymID order. Digest is H(`SLUG operator pool settlement config`, bytes). Existing Config meaning/grammar is unchanged. `poolservice.BindingID` remains SHA256(`SLUG pool service source`+NUL+`v1`+NUL+canonical source), without a new length prefix. Original pool source payout address must encode the same funding script as settlement ChangeScript and the guard.

`wallet.PoolWalletBinding` is an owned in-process configuration, not a new persisted wrapper protocol: version1, WalletOwnerID, OperatorProfileID, full Source, full Settlement and Guard. Validation requires exact identities and both independently recomputed config hashes. Return copies own all slices. No binding digest is substituted for retaining full original expected configuration/owner pointers.

Manager schema16 stores canonical guard at `waddrmgr` → `main` → byte key `poolsettlementguardv1`. Existing `mgrver` is LE32(16), `slug-derivation-mode` LE32(1). Recovery-version/public/private, candidate fixture/progress, hosted allocation and carrier guard/progress namespaces must be absent. The new creation branch selects only scope84/coin0, account0 plus ordinary empty imported-account bookkeeping. It installs exactly external index0 with external next1/internal next0 and no skipped child; all other addresses/imports/accounts/scopes refuse. Existing encrypted root/account/key fields retain their btcwallet codecs. A single DB transaction publishes those fields and schema16; no old-wallet stage or recovery payload is persisted. Audit validates the exact public root, fixed account/path/key/address row and known empty bookkeeping before private signing. An expected guard comes from retained operator setup, never disk adoption.

The immediate settlement signing capability is in-process only. Its private request digest may use H(`SLUG operator pool signing request`, canonical ordered struct `version,config,source,inputs,payments,fee_atoms,change,unsigned`). The capability itself is never serialized or returned as DTO authority. The adapter checks the original Store/backend pointer, owned config/source/coins/unsigned and active lifetime during the synchronous Sign call. It expires when Sign returns, before the rest of RunDaily proceeds. No durable reservation or second debit ledger is added.

Independent inputs: the three already-published Set24 recovery fixtures supply only synthetic HD seed/root/path material (zero256, incrementing256, official-leading-zero-account). Their stored BIP39-derived seeds are test inputs to BIP32, not a new operator BIP39 claim. Existing independently published pool payout source/config provides source grammar and gym IDs; change/payout is explicitly replaced by the derived fixed funding key, and payees use published synthetic internal index0 keys so they differ from funding. Literal owner/profile IDs are test-only nonzero64hex. Modeled previous outpoints make transaction vectors structural/signature evidence only, never chain/credit attribution. No real/private file is read.
