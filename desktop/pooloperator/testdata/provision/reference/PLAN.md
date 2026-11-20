# Independent provisioner public reference plan

Source-only, frozen for root review before generation. The producer uses no Go, wallet/database creation, node/network, random generation, real key file, provider or production custody input. Outputs are independent expected bytes, not authority or financial evidence. Any entropy/phrase represented here is exclusively an already-published synthetic recovery fixture.

Inputs are copied byte-for-byte from the previously reproduced public wallet16, authority-v2, operator Setup and recovery corpora. The archived independent BIP39/BIP32 and Ed25519 Python sources corroborate the public master/fixed native84 key and the three online role keys. First32 capsule bytes are existing BIP39 entropy with an empty additional passphrase, never a raw BIP32 shortcut; only published fixture entropy is used. No private checkpoint value is present. The policy's checkpoint public key is the exact literal in frozen BYTES, 811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3.

Frozen CONTRACT.md and BYTES.md are copied under inputs/policy. The seven namespace roles use exactly H("SLUG pool provision identity", decoded request ID || fixed role); PolicyID uses the separate frozen policy domain. Existing public pin, source, settlement, wallet guard, miner, schedule and Setup byte/digest recipes remain unchanged. The producer recomposes those public structures from explicit policy and public material, with no copied fixture namespaces or Go-derived expected bytes.

Six positives:

- Each of the three independent published wallet fixtures with its own explicit request ID, two ordered pre-enrolled public gyms and distinct native84 payees.
- Same material with a different request ID: all namespace IDs and resulting Setup change.
- Same request/material with explicit node fee mode: namespaces remain, PolicyID and Setup change.
- Same policy/material keys with a later creation time: Setup remains, retained intent/READY/operator-wallet backup time bytes change.

Each positive contains canonical Policy (and one-LF file), exact PublicMaterial and Certificate projections, all seven namespace IDs, Setup/core/file bytes, original profile/source/settlement/miner/schedule/guard/Setup IDs, intent and READY bytes, and the distinct operator-wallet backup bytes. Backup phrases are the already-published synthetic 24-word fixtures and are explicitly not deployment material or player-recovery-v1.

The 61 structured refusals are 39 Policy cases, seven PublicMaterial cases, three Certificate-projection cases, one payee/change collision and 11 original-backup correspondence cases. They cover malformed identity/chain/descriptor/checkpoint choice, nested caps, key/target/ordering/script errors, unsupported transport/fees/timing, fixture online signers, and original metadata/phrase/guard mismatch. Eighteen raw Policy file cases include two accepted encodings (canonical and one LF) plus 16 refusals for aliases, duplicate/missing/unknown fields, numeric alternatives, extra whitespace/LF/data, truncation and cap overflow. These finite controls do not claim exhaustive filesystem/crash validation; concrete product tests retain those responsibilities.

A deterministic public certificate fixture is independently DER-encoded and signed with the inherited Ed25519 source from two explicitly published ASCII seed recipes. It contains a CA and distinct non-CA server leaf, serverAuth, DNS pool.example.test and IP127.0.0.1, validity2000–2100. The server-chain PEM bytes and leaf SPKI are hashed into the Certificate projection. Internal Ed25519 signature verification checks construction; actual X.509 parsing and original-file/time/hostname/TLS custody remain separate concrete command tests. No synthetic TLS private key is emitted.

Producer execution requires --execute plus the exact SHA of a root-approved source lock (approved_by=root), and a fresh absolute output path. Every input/model/plan is checked before and after model evaluation. Output is exclusive, synced, compact ASCII JSON with one LF, at most2MiB. Python3.14 and3.9 should run separately against the same lock and produce byte-identical files. Failures retain the original source/lock/output artifacts before any successor. The current source passed AST syntax parsing only; the producer has not executed.
