# Provision v1 exact bytes and composition

This freezes new provisioning metadata only. Existing Setup v1, wallet schema16,
operator V2 profile, settlement and player recovery-v1 remain unchanged. All
expected bytes must come from the separate independent producer before Go uses
them. Production CSPRNG/key generation is performed only by the operator later.

## Common encoding

Canonical compact ASCII JSON, exact order below, no unknown/missing/duplicate
fields, alternate string escaping, whitespace, null, boolean or floating values.
Numbers are unsigned decimal integer tokens (no sign, leading zero except0,
exponent or fraction), within the stated ranges. Strings are canonical ASCII.
All hex is lowercase without prefix. Hash/ID values are64hex; nonzero where an
identity or key is required. Object maximum16 members, array maximum64 members,
JSON depth maximum8; byte caps below apply before decoding. Input Policy may have
zero or one final LF. Every persisted JSON file requires exactly one final LF;
that LF is not part of canonical nested JSON or any domain digest.

H(label,raw)=SHA256(ASCII(label)||NUL||ASCII(v1)||NUL||LE32(len(raw))||raw).
PolicyID=H(`SLUG pool provision policy`,canonical Policy).
Namespace(role)=H(`SLUG pool provision identity`,decoded request_id32||ASCII(role)).
These two labels are the only new public hash domains; no secret fingerprint or
hash of the recovery phrase is published.

## Policy — canonical core at most65536 bytes

Top-level order: `version,scope,request_id,network,genesis,descriptor_id,
checkpoint_public_key,mining,limits,enrollments,schedule,fee,transport,
maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms`.

- version1; scope `phase-a-finite-operator`; request_id nonzero64hex.
- network1397511495; genesis
  `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`.
- descriptor_id64hex resolved by the existing candidate descriptor resolver;
  its DNAVersion must match mining. checkpoint_public_key is exactly
  `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`.
- mining order `dna_version,prospect_target,pool_target`: dna_version1;
  both targets nonzero64hex; preserve the existing target semantics.
- limits order `max_works,max_submissions,pool_max_bytes,max_rounds,max_batches,
  round_max_bytes`: ranges1..256,1..4096,4096..1073741824,1..256,1..366,
  4096..1073741824 respectively; max_rounds>=max_works.
- enrollments1..64; entry order `public_key,payee_script`. public_key is a
  canonical compressed secp256k1 point66hex. payee_script is44hex `0014` plus20
  bytes. Derive GymID from existing chain-bound gymauth recipe; entries strictly
  increasing by that derived ID, unique keys/IDs/payee scripts. Once funding is
  known, no payee script may equal the pool funding/change script.
- schedule order `daily_utc_minute,retry_ms`:0..1439 and1000..3600000.
- fee order `mode,confirmation_target,atoms_per_kvb,max_atoms_per_kvb,
  max_fee_atoms`: exact existing poolsettlenode.FeePolicy. mode `node` requires
  target1..25 and rate0; mode `fixed` requires target0 and
  rate1..max_atoms_per_kvb. Max rate1..1000000000; max fee1..2100000000000000.
- transport order `listen,origin,pool_url,admission_url,ownership_url`: existing
  ingress grammar, Listen<=128 with nonzero explicit port, remaining URLs<=512;
  canonical HTTPS public origin; three distinct HTTP literal-loopback upstream
  origins with explicit ports. No userinfo/path/query/fragment or implicit URL.
- maintenance_interval_ms1000..60000; unlock_lifetime_ms60000..86400000;
  unlock_renewal_ms1000..3600000 and <=unlock_lifetime_ms/2.

Use the existing full nested validation after composition; the list above is
not permission to weaken any existing validator. No field is populated from a
fixture example or learned from a remote response.

## Pure public inputs and exact Setup mapping

`ProvisionPublicMaterial`, canonical core<=2048, order:
`version,created_at_ms,root_public_key,funding_public_key,admission_key,
publication_key,cutoff_key`. version1; time1..253402300799999. Root is the
canonical SLUG master xpub<=120 with zero depth/child/fingerprint. Funding is a
canonical compressed point66hex. Each online key is64hex accepted by the existing
operatorauthority.ValidateKeys together with the explicit checkpoint key. These
public values alone prove neither private derivation nor custody.

`ProvisionCertificate`, canonical core<=1024, order:
`version,certificate_sha256,certificate_spki`. version1, both hashes nonzero64hex.
The command constructs this projection from its original selected public PEM:
SHA256(exact file bytes), SHA256(leaf.RawSubjectPublicKeyInfo). Pure composition
may validate shape, never assert that a caller-created projection is a validated
certificate. Actual original-file, hostname, validity, non-CA server-leaf checks
remain at the concrete command boundary. Independent composition vectors may
supply explicit synthetic file SHA and independently encoded public SPKI/hash;
that models composition, not X.509 parsing, key possession or TLS acceptance.

Seven namespaces map exactly:

| role | destination |
| --- | --- |
| runtime | Setup.RuntimeID; Schedule.RuntimeID; Miners.RuntimeID |
| authority | Setup.Authority.Source.AuthorityID = Pool.AuthorityPin.AuthorityID |
| publication | Setup.Authority.PublicationID |
| ownership | Setup.OwnershipStoreID |
| wallet | Setup.WalletGuard.WalletOwnerID |
| scheduler | Setup.Schedule.SchedulerID |
| credentials | Setup.Miners.CredentialID |

Source is ManifestAuthorityPin version5 with selected chain/descriptor and the
above authority ID. Authority is PublicManifestPin version2, selected three online
keys plus explicit checkpoint key, existing PublicOwnershipRuleID, scan origin
height0/genesis, and exact default limits in existing order:
1000 admissions,16384 publications,4096 carriers,1024 checkpoints,256 cutoffs,
65536 headers,268435456 logical bytes. Validate via NewOperatorProfile, not the
fixture v1 constructor.

Funding script is `0014`+HASH160(compressed funding point); address is existing
SLUG native P2WPKH encoding. Pool version1 gets Source, mining Policy version1
(same chain/DNA/prospect target), derived payout address, selected pool target
and three pool limits. Settlement version1 gets that change script, sorted
`gym_id,script` destinations derived from enrollment, and max_rounds/max_batches.
RoundLimits version1 gets max_rounds/round_max_bytes. Node version1 gets selected
chain, funding key/script and FeePolicy. Ingress version1 gets selected transport
and derived certificate_spki. Copy maintenance and unlock timings unchanged.

WalletGuard is existing version1/schema16: above wallet ID, full OperatorProfile
ID, existing poolservice.BindingID(Pool), existing poolsettle.ConfigBindingID,
derivation_id `slug-phase-a-operator-pool-native84-v1`, selected chain and exact
root/funding/script. Schedule version1 gets its namespace/RuntimeID, selected
UTC/retry values and existing H(`SLUG pool settlement binding`,canonical ordered
`config,source` containing Settlement,Pool). Miners version1 gets its namespace,
RuntimeID, profile ID, pool source ID, settlement config ID, then sorted existing
`public_key,gym_id,payee_script` enrollment rows. Finish with ValidateSetup and
MarshalSetup. Existing SetupID and PoolSettlementGuardDigest remain unchanged.

## Capsule, protected backup and markers

`offline/material-v1.bin` is exactly128 raw bytes, no header/LF: bytes0..31 are
BIP39 entropy,32..63 admission seed,64..95 publication seed,96..127 cutoff seed.
The first32 bytes use existing recovery.Mnemonic (24 canonical English words)
and recovery.SeedFromEntropy (PBKDF2-HMAC-SHA512,2048 rounds, salt `mnemonic`,
64-byte seed, empty additional BIP39 passphrase). Feed that64-byte seed into
standard BIP32 with current SLUG network versions; derive exactly
`m/84'/0'/0'/0/0`. No skipped child, raw32-byte BIP32 shortcut or extra passphrase.
Online role seeds are independent32-byte values, not derived from wallet entropy.
All-zero entropy remains a valid published test vector; production has no fixture
selector or entropy override. All four fields are secret, never JSON/log output.

`offline/SLUG-OPERATOR-POOL-WALLET-BACKUP-V1.json` (total<=8192 including LF),
exact ordered members:
`version,kind,request_id,policy_id,setup_id,created_at_ms,mnemonic_profile,mnemonic,
derivation_id,funding_path,network,genesis,wallet_guard,wallet_guard_id`.
Version1; kind `slug-operator-pool-wallet-backup`; mnemonic_profile
`bip39-english-256-empty-passphrase`; mnemonic<=256 ASCII bytes, exactly the24
lowercase English words with single spaces and valid existing checksum;
derivation_id `slug-phase-a-operator-pool-native84-v1`; funding_path
`m/84'/0'/0'/0/0`. Other values equal the original intent, composed Setup, existing
full canonical PoolSettlementGuard and its digest. Network/genesis duplicate the
exact guard values; created_at_ms is the original wallet birthday. Decode the
phrase, rederive xpub/funding, and compare the complete expected guard and IDs.
No new digest covers this secret file. No wallet unlock password, online seed,
checkpoint key material, player account inventory or mutable service state is
included. This distinct grammar is not recovery.Export/player recovery-v1; a
valid phrase never authorizes creating blank financial journals or replacing an
existing runtime. Independently retain public Setup and all service histories.

`SLUG-POOL-PROVISION-INTENT-V1.json` (total<=131072 including LF), order:
`version,policy,policy_id,certificate_sha256,created_at_ms`. version1, full canonical
Policy, exact PolicyID/certificate digest, time1..253402300799999. The first
successful time is retained forever; no retimestamp on retry.

`public/setup.json` is existing canonical Setup plus exactly one LF, total<=262145.
`SLUG-POOL-PROVISION-READY-V1.json` (total<=1024 including LF), order:
`version,request_id,policy_id,setup_id,wallet_guard_id,created_at_ms`. All values
match original intent/composition and are recomputed on every original-root open.
No marker claims current chain, funds, backup availability, or deployed service.
