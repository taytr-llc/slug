# Offline operator provisioning — contract v1

Frozen source contract for independent reference preparation; implementation waits
for that corpus. No key material has been read or generated; no build,
network, wallet creation or deployment has run. This tool is for the operator to
run personally on a private device. Agent tests use published synthetic material.
It completes local preparation for the existing finite Phase-A operator, not
production-genesis authorization, official descriptor issuance, public deployment,
checkpoint signing, hosted login or a new payout policy.

## Tool and authority boundary

Add headless `desktop/cmd/slugpool-provision`, with a thin signal wrapper over
`pooloperator.RunProvisionCommand(ctx,args,stdout,stderr *os.File) error`. Reuse
private selected-file/descriptor, ACL, exclusive-root and sticky cleanup leaves
from pooloperator; do not weaken them to the older checkpoint tool's checks.
Commands are `create`, `resume`, `verify`. All require the independently selected
`--policy-file`, `--root`, `--tls-certificate-file`,
`--wallet-public-passphrase-file`, `--wallet-unlock-passphrase-file`. Arguments
contain paths only. The two passphrases are exact file bytes, nonempty and at most
1024 bytes. No secret argument, environment override, clipboard, prompt echo or
secret stdout is supported. No checkpoint seed, RPC credentials, TLS private key,
miner private key or token is an input to this tool.

`create` selects a fresh nonexistent canonical absolute root; `resume` and
`verify` require its original root and exact public policy/certificate inputs.
No command contacts a node, signs a publication/transaction, creates runtime
journals, enrolls credentials, starts services or mines. `verify` is read-only
apart from the existing wallet's ordinary open/lock lifecycle; it is not a
physical read-only database audit. Its original owner must close before return.

Follow `cmd/slugcheckpoint`'s operator-run convention: OS CSPRNG, exclusive
protected output, public-only result, no overwriting and no private seed in argv.
This is a separate tool; do not add online roles to the checkpoint tool or import
its private key. The existing compiled checkpoint public key is an explicit
public policy input and must match the selected current Phase-A profile.

## Exact public policy input

`ProvisionPolicy` is version1 canonical compact ordered UTF-8 JSON, canonical core
at most65536 bytes; an input file may add exactly one terminal LF (65537 total).
`BYTES.md` fixes the complete Policy/marker/backup grammar and composition map. Reject duplicates,
unknown/missing fields, alternate encodings and all implicit fixture defaults.
The field order is:

1. `version`=1, `scope`=`phase-a-finite-operator`, `request_id` (nonzero64hex).
2. `network`, `genesis`, `descriptor_id`, `checkpoint_public_key`.
3. `mining`: `dna_version`, `prospect_target`, `pool_target`.
4. `limits`: `max_works`, `max_submissions`, `pool_max_bytes`, `max_rounds`,
   `max_batches`, `round_max_bytes`.
5. `enrollments`: 1..64 ordered entries `public_key,payee_script`; compressed
   secp256k1 public keys and native P2WPKH destination scripts only. Derive each
   chain-bound GymID with `gymauth.GymID`; require strictly increasing derived
   GymID order and uniqueness. No supplied balance or user-selected GymID.
6. `schedule`: `daily_utc_minute,retry_ms`.
7. `fee`: the existing ordered `poolsettlenode.FeePolicy`, unchanged.
8. `transport`: `listen,origin,pool_url,admission_url,ownership_url`.
9. `maintenance_interval_ms,unlock_lifetime_ms,unlock_renewal_ms`.

Use the existing validators and exact caps for every nested field. Explicit
network/genesis must equal the currently supported Phase-A chain; descriptor
must resolve through the existing candidate resolver. The public checkpoint key
must equal the current compiled trusted checkpoint key. There is no network or
profile selector that silently authorizes another genesis. Fixed D58/D79/D80/D81
rules, authority quotas, ownership rule/scan origin and derivation identifier are
existing compiled constants, not new configurable economics.

The selected certificate file is public PEM, bounded65536 bytes, held and checked
like other selected inputs. Parse a valid non-CA server leaf, validate the chosen
Origin hostname/IP and current validity, and derive SHA256 of its SPKI. Retain
its exact file-byte digest in the provisioning intent; do not fetch or renew it.
TLS trust-chain/deployment/private-key correspondence remain runtime/host checks.
Do not generate a fixture certificate or choose an Origin for the operator.

Pure APIs: `ParseProvisionPolicy`, `ValidateProvisionPolicy`, `ProvisionPolicyID`
and `ComposeProvisionSetup(policy, publicMaterial, certificate) (Setup,error)`.
These accept/return public values only and confer no custody or store authority.
The private command derives publicMaterial from its original retained material;
a caller-created public DTO cannot invoke wallet creation or mint an owner.

## Derived values versus operator choices

The operator chooses public chain/descriptor/checkpoint identity, actual gym
public keys and payees, host/origin/listener choices, finite capacities, target
and fee configuration, and operational timing. Their values must be explicit;
existing approved economics are not reopened. `request_id` is a unique public
correlation value selected before effects, not a private key or policy choice.

Define H(label,raw)=SHA256(ASCII label || NUL || ASCII v1 || NUL ||
LE32(len(raw)) || raw), lowercase64hex. PolicyID is H(`SLUG pool provision policy`,
canonical policy). For each fixed ASCII role `runtime`, `authority`,
`publication`, `ownership`, `wallet`, `scheduler`, `credentials`, derive its
identity as H(`SLUG pool provision identity`, decoded request_id32 || role).
These are independently selected namespace IDs before wallet/store creation;
no existing marker/server chooses them. Reusing a request ID for another
provisioning operation is refused within the original root, not globally tracked.

The tool derives online Ed25519 public keys, master extended public key, fixed
`m/84'/0'/0'/0/0` funding public key/script/address and all existing profile,
source, settlement, wallet-guard, miner and schedule binding IDs. It assembles the
unchanged Setup v1 through `MarshalSetup`/`ValidateSetup`. Do not call the v1
fixture-pin constructor; construct the explicit v2 public pin and validate it.
No copied fixture key, payout address, namespace or Setup is deployable default.
Policy IDs are new provisioning metadata; existing financial/Setup wire bytes
and schemas are unchanged. The first successful intent time supplies wallet
birthday and remains unchanged on resume.

## Private material and output inventory

All directories are owner-only0700; regular files are single-link0400/0600 with
no symlink ancestors or extended ACL. Hold original descriptors/root lock; fail
closed on replacement. Storage assumes the existing trusted private local
filesystem, not protection against a hostile same-UID writer.

Create one `offline/material-v1.bin`, exactly128 raw bytes: independent32-byte
BIP39 entropy followed by independent32-byte Ed25519 seeds in admission,
publication, cutoff order. Derive the wallet seed with existing
`recovery.SeedFromEntropy` and its24 canonical English words with
`recovery.Mnemonic`. The additional BIP39 passphrase is always empty and unrelated
to the selected wallet encryption/unlock passphrases. Feed the resulting64-byte
seed to standard BIP32, then use the fixed native84 path. Obtain capsule bytes
in one bounded OS-random operation; validate the master/fixed path and online
role keys before publishing material. No alternative child search,
secret-derived deployment ID, checkpoint seed or production entropy override. Invalid material fails;
never silently change an already published material file. Clear owned buffers
when their original operations join; do not promise complete historical Go-memory
erasure. The packed material remains offline on the operator's device and is a
private backup, not part of the host deployment allowlist.

Only these fixed outputs are permitted:

- `SLUG-POOL-PROVISION-INTENT-V1.json`: ordered `version,policy,policy_id,
  certificate_sha256,created_at_ms`, canonical+LF, max131072 bytes.
- `offline/material-v1.bin`: the protected128-byte capsule above.
- `offline/SLUG-OPERATOR-POOL-WALLET-BACKUP-V1.json`: protected24-word operator
  wallet backup plus exact public derivation/chain/guard metadata; canonical+LF,
  max8192 bytes, grammar in `BYTES.md`. This is not player recovery-v1.
- `online/admission.seed`, `online/publication.seed`, `online/cutoff.seed`:
  exact32-byte copies of the corresponding retained seed, no LF.
- `public/setup.json`: existing canonical Setup v1 plusLF, max262145 bytes.
- `wallet/wallet.db`: produced only through `CreatePoolSettlementWallet`.
- `SLUG-POOL-PROVISION-READY-V1.json`: ordered `version,request_id,policy_id,
  setup_id,wallet_guard_id,created_at_ms`, canonical+LF, max1024 bytes.

No wallet encryption/unlock passphrase is copied to this tree. The BIP39 phrase
is written only into the protected offline wallet backup, never public output. No miner token is generated or installed;
existing local rotate/revoke commands remain responsible for credentials after
runtime init. TLS key and private RPC files remain separate operator selections.
Only exact wallet/online/public outputs are eligible for a later explicit host
transfer; never copy either offline capsule or wallet backup by a recursive
deployment recipe. The wallet backup contains neither online authority seeds
nor financial journals; it proves key/guard correspondence only. It is not an
operator-service restore format, and this tool adds no player backup import path.

## Partial creation and exact recovery

Persist and sync the exact public intent before randomness or subordinate
creation. Publish each fixed file exclusively, sync its bytes and containing
directory, and retain uncertain writes/failed closers. Existing complete files
must compare byte-for-byte to independently recomputed expected values; they
are never overwritten. There is no reset, cleanup-delete or force flag.

Resume rereads the caller's original policy and certificate, compares the entire
intent, then retains its original creation time. An absent capsule can be created
only when no wallet-backup/online/setup/wallet/READY output exists. If it exists, derive from
those exact bytes; never generate replacement keys. Missing wallet-backup, online or public
outputs before READY may be written with the exact recomputed bytes. A partial,
wrong-sized or mismatched file is a held incomplete outcome, not permission to
truncate it or draw fresh entropy.

Compute and retain the full expected Setup/guard before wallet creation. If the
wallet directory is absent, invoke the existing schema16 factory exactly once.
If present, only `OpenPoolSettlementWallet` with that exact expected binding may
succeed. Do not delete/recreate an empty, malformed or partially initialized DB.
The current factory cannot resume a crash after directory creation but before a
valid wallet commit: report `incomplete_wallet`, preserve all material/paths and
stop. A separately reviewed repair is required; this first tool does not claim
recovery from every torn creation boundary. A complete wallet with lost response
can be reopened and audited, with no new wallet or identity.

Before READY, verify that the backup phrase decodes to the capsule entropy and
rederives its exact root/funding key and complete public guard; verify all other
public derivations and exact online seed copies, open
and audit the wallet with the expected guard, verify the selected unlock
passphrase using a short bounded unlock then Lock, and close the original wallet.
No signing/network occurs. Publish READY only after successful joined cleanup.
An existing READY makes every declared output mandatory; missing files refuse
rather than being reconstructed. `resume` then performs the same exact verification
and returns the retained result without writes. Wallet financial history and
operator journals are never reconstructed by this provisioner. A backup of keys
alone cannot replace settlement liabilities or service history.

## Output, failures and acceptance

Return one bounded public JSON result only after cleanup, with version, command,
request_id, policy_id, setup_id, wallet_guard_id, public role keys and funding
address, and a dated status `provisioned` or `verified`. This proves local creation,
not funding, backups, public service readiness or authority adoption by miners.
Use closed error codes; never print arbitrary input text, raw errors or secrets.
Any potentially committed write followed by failure/output loss returns outcome
unknown and requires exact resume. Preserve typed original cleanup owners; no
retry of a failed raw Close. Reuse the command's bounded interruptible output and
cancellation/join discipline, including SIGPIPE handling.

Before implementation consumes expected bytes, freeze a compact independent
public Policy/ID/Setup/backup composition corpus using published fixture material
only. Secret-bearing corpus examples are exclusively published test entropy;
no production entropy file, CLI flag or environment override is introduced.
Meaningful focused tests: exact derived public outputs versus that corpus; no
checkpoint-input route; malformed/fixture-default/mismatched chain, certificate,
payee and policy refusal before effects; exclusive output and every finite crash
boundary; complete lost-READY recovery without new entropy; partial material/DB
hold and established missing-output refusal; wrong passphrase; original context,
file replacement, cleanup retention and blocked-output uncertainty. A full
synthetic provision→normal slugpoold init/open composition must prove the generated
schema16/public Setup is consumable. Production real material is generated only
by the operator later; no user secret is requested in chat.
