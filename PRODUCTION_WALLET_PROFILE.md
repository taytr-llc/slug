# Production recovery profile — decision to freeze before issuance

Prepared November 16, 2026. **Design approved by Mike on November 16.**
His explicit response was: "Approve the production profile design." This is D73;
implementation follows set29 on its own stacked set30 branch. Final descriptor
bytes, coin allocation and permanent genesis remain unresolved issuance gates.
This replaces only the derivation-decision portion of the earlier
set-28 planning proposal.
No repository source, wallet, real key, provider resource or registry entry was
created. No PR was opened and no maintainer was contacted.

Recommend a new `slug-production-bip39-v1` profile, with a dedicated SLUG coin
type, four explicit payment-script accounts, native SegWit as the default, and a
gym key bound to the permanent chain. Existing fixtures retain their exact
profile. The final descriptor cannot be activated until the coin allocation and
production genesis are fixed; approving this design does not approve either a
genesis or real issuance.

**Approved decision:** Adopt this separate production recovery profile,
with native-SegWit receive/change by default and all four script scopes retained,
accepting that recovery requires SLUG-aware software plus the manifest rather
than assuming an arbitrary Bitcoin wallet can recover it?

Mike approved the profile design; real issuance stays closed until its final
bytes and recovery tests are frozen. The tradeoff is a
clear permanent SLUG recovery contract versus committing to compatibility that
generic wallets do not currently demonstrate. Reusing the fixture profile is
not recommended. Native SegWit is an engineering recommendation using a script
already exercised end to end; this is not a claim of external SLUG-wallet support.

## What is already settled

D70's 24 English BIP39 words, empty additional BIP39 passphrase, and recovery
manifest remain unchanged. Login/encryption passwords do not become derivation
inputs. D72's same-wallet verified reset and 24-hour security hold remain
unchanged. This decision asks nothing about MFA, reset assurance, a provider,
hosting, KMS, or external provisioning. Permanent genesis remains a separate
launch decision.

## Exact proposed payment paths and bytes

Use standard BIP32 secp256k1 derivation, including fixed-width private-key
serialization. Account `0` only initially; external branch `0`, change branch `1`.
The purpose/coin/account components are hardened and branch/index are not.
These are the hierarchy conventions specified by
[BIP32](https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki) and
[BIP44](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki).

Candidate coin number: **1397511495**, hex `0x534c5547`.
Its hardened child number is **3544995143**, hex `0xd34c5547`; BIP32 serializes
that child index as bytes **`d3 4c 55 47`**, big-endian. Its numerical resemblance
to Phase-A network magic is a naming choice, not a protocol requirement tying
the production network magic to this value.

| Purpose | Exact candidate receive path | Exact candidate change path | Script on both branches |
| --- | --- | --- | --- |
| 44 | `m/44'/1397511495'/0'/0/i` | `m/44'/1397511495'/0'/1/i` | P2PKH |
| 49 | `m/49'/1397511495'/0'/0/i` | `m/49'/1397511495'/0'/1/i` | P2SH-P2WPKH |
| 84 | `m/84'/1397511495'/0'/0/i` | `m/84'/1397511495'/0'/1/i` | P2WPKH; default for new production receive/change |
| 86 | `m/86'/1397511495'/0'/0/i` | `m/86'/1397511495'/0'/1/i` | Single-key P2TR with the BIP86 TapTweak |

Scopes 49 and 84 use their specified nested/native witness scripts; scope 86
uses the tweaked output key, not the untweaked derived public key.
[BIP49](https://github.com/bitcoin/bips/blob/master/bip-0049.mediawiki),
[BIP84](https://github.com/bitcoin/bips/blob/master/bip-0084.mediawiki),
[BIP86](https://github.com/bitcoin/bips/blob/master/bip-0086.mediawiki).

Recommend retaining the existing SLUG encoding values in the new descriptor:

| Encoding | Candidate bytes/value |
| --- | --- |
| P2PKH / P2SH Base58Check version | `53` / `47` |
| WIF version | `d3` |
| Witness HRP | ASCII `slug` (`73 6c 75 67`); witness-v0 Bech32 / witness-v1 Bech32m |
| Extended public / private version | `53 4c 55 47` / `73 6c 75 67` |

The witness checksum split follows
[BIP350](https://github.com/bitcoin/bips/blob/master/bip-0350.mediawiki).
These are SLUG encodings read from
[chain parameters](chaincfg/params.go), not Bitcoin
`ypub/zpub` encodings or a claim of SLIP-0132 registration. The reviewed
[SLIP-0132 registry](https://github.com/satoshilabs/slips/blob/master/slip-0132.md)
contains neither SLUG nor these extended-key values. The manifest must therefore
explicitly identify script, full path, profile and chain; an extended-key prefix
alone is insufficient. Encoding reuse also does not distinguish Phase A from
production: restore must check the exact genesis/profile.

## Coin registry status and limitation

Checked the primary SLIP-0044 file at commit
[`570ed55b7fde158f1116be34fc2faa35dada5912`](https://github.com/satoshilabs/slips/blob/570ed55b7fde158f1116be34fc2faa35dada5912/slip-0044.md),
the latest file change returned by GitHub on this review date. Exact file SHA256:
`54f602c02277494e65dec2812f01fa94bf124ea209d43bc8a203fed37a5770e1`.
Neither `1397511495` nor `SLUG` appears. The candidate fits the unhardened
coin-number range before the hardening bit is added. Coin `0` is Bitcoin; coin
`1` is the shared testnet entry. Absence is **not reservation or allocation**.
The registry requires an implementing wallet; acceptance and timing are not
guaranteed. [SLIP-0044](https://github.com/satoshilabs/slips/blob/570ed55b7fde158f1116be34fc2faa35dada5912/slip-0044.md).

Fresh collision checking and resolving the numeric allocation are engineering
release prerequisites. Do not silently fall back to coin `0` or change a
production path after issuance. Any future registration submission must follow
the standing no-PR rule and explicit authorization; this task authorizes none.

## Exact proposed gym scheme

Use a separate scheme identifier, for example
`slug-gym-slip13-slip10-secp256k1-v1`, derived from the same approved BIP39 seed.
The identity is the immutable ASCII URI
`slug://<g>/gym/v1`, where `g` is the **final production genesis's lowercase
64-character RPC-display hex**, with identity index `0`. There is no username,
port, query, fragment, trailing slash, provider hostname or login identifier.
The URI is an exact identity string; derivation performs no DNS or network lookup.

The frozen path algorithm is:

```text
h = SHA256(00 00 00 00 || ASCII("slug://" + g + "/gym/v1"))
w[j] = littleEndianUint32(h[4*j : 4*j+4]), j = 0..3
actual child numbers = [0x8000000d, w[0]|0x80000000,
                       w[1]|0x80000000, w[2]|0x80000000, w[3]|0x80000000]
display path = m/13'/(w[0]&0x7fffffff)'/(w[1]&0x7fffffff)'/
                    (w[2]&0x7fffffff)'/(w[3]&0x7fffffff)'
```

This follows SLIP-0013's identity/path construction. **SLIP-0013 specifies
SLIP-0010 derivation**, not an unconditional substitution of a BIP32 library.
Use SLIP-0010 **secp256k1**, including its defined invalid-master/invalid-child
retry behavior. Ordinary results coincide with BIP32, but rare invalid-child
handling differs: SLIP-0010 retries at the same index whereas BIP32 proceeds to
another index. Freeze/test this distinction. SLUG still signs its existing
typed enrollment/admission domains; it does not adopt SLIP-0013's generic
Bitcoin-message challenge protocol.
[SLIP-0013](https://github.com/satoshilabs/slips/blob/55de03357a0eb1dc3df31ce5f548ec8cc553fef2/slip-0013.md),
[SLIP-0010](https://github.com/satoshilabs/slips/blob/master/slip-0010.md).

The final numeric gym path cannot honestly be supplied before production genesis
exists. As a reproducible **algorithm example only**, inserting current Phase-A
genesis gives hash
`348c121bca6c3a36328ebf41846c51fc0ed6deb16bbff6cb7ffa4ffbbe65d913`
and path `m/13'/454200372'/909798602'/1103072818'/2085710980'`.
Child serialization bytes are `8000000d 9b128c34 b63a6cca c1bf8e32 fc516c84`.
This is neither the existing fixture gym path nor an approved production path.
The calculation also reproduced SLIP-0013's published worked-example hash and
child numbers using only Python `hashlib`/`struct`.

## Preservation and engineering freeze

Existing `slug-phase-a-fixture-bip39-v1` retains coin `0`, gym
`m/1397511495'/1'/0'`, scope-49 nested receive **and native change**, its 4096
cursor cap, and all existing restoration/RPC behavior. Do not “correct” these
bytes in place. The proposed uniform nested scope-49 and native default belong
only to the new profile. Current authoritative fixture definitions are
[recovery/types.go](https://github.com/taytr-llc/slugwallet/blob/slug-production-profile/recovery/types.go).

Mike's product decision is adopting this permanent recovery contract and its
default address behavior. Selecting the exact registry number, profile ID/schema
encoding, endianness, test oracles, bounded restore implementation and failure
handling are engineering responsibilities presented here for review—not numbers
Mike needs to invent. Payment/gym paths must never become per-request options.

Before real issuance, engineers must freeze one complete canonical descriptor
and digest: final coin allocation, genesis/network identity, encodings, payment
schemas, exact gym algorithm/path, and recovery inventory. Use bounded resumable
scanning and durable cursor high-water marks across the supported nonhardened
index range; the fixture lifetime cap is not a production choice. A plain BIP44
20-address gap search does not guarantee recovery beyond a deliberately unused
gap, so restore must honor the exported inventory.
[BIP44 discovery](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki#account-discovery).

Independently test all four payment scopes, gym derivation (including synthetic
invalid-child cases), exact exported metadata, and restore with the old database
and hosted services unavailable. A placeholder genesis or unapproved profile
must fail before entropy issuance. These are implementation gates, not new
provider/MFA/product approvals. Changing an issued profile requires a new
version; it cannot be relabelled as the same recovery contract.
