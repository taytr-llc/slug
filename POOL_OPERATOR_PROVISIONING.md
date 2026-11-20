# Offline operator provisioning

`desktop/cmd/slugpool-provision` prepares the finite Phase A pool operator on
the operator's computer. It creates a dedicated encrypted settlement wallet,
three online signing roles and their matching public Setup. It never creates,
reads or accepts the offline checkpoint signing key. This tool is separate from
player onboarding and production-chain issuance.

Build from `desktop` with the paired wallet checkout and the candidate compiler
profile. Darwin requires CGO for protected-file ACL checks. The public command
accepts only absolute file paths:

```
slugpool-provision create \
  --policy-file /absolute/private-inputs/policy.json \
  --root /absolute/new-operator-provision \
  --tls-certificate-file /absolute/private-inputs/server-chain.pem \
  --wallet-public-passphrase-file /absolute/private-inputs/public-passphrase \
  --wallet-unlock-passphrase-file /absolute/private-inputs/unlock-passphrase
```

The operator runs `create` themselves. Its only entropy source is the operating
system. There is no seed argument, environment override or fixture default.
Passphrases are exact file bytes, separate from the wallet's recovery words.
Selected files must be regular, single-link, owned by the current user and mode
0400 or 0600, inside private 0700 directories with no extended ACL. Keep selected
inputs outside the new output root. Do not paste private contents into chat.

The policy explicitly selects the existing Phase A chain, descriptor, official
checkpoint public key, enrolled gym public keys and fixed payout scripts,
bounded storage, daily UTC schedule, fee limits and TLS/private-service endpoints.
It contains no private material. The canonical byte contract and independently
derived examples are in `desktop/pooloperator/testdata/provision`. Examples use
published test identities and are not deployment configuration. The certificate
must have a valid non-CA server leaf for the selected public hostname. Its private
TLS key is not needed by the offline provisioner.

## Outputs and backups

The fresh private root contains these distinct outputs:

- `public/setup.json`: public runtime identities and configuration.
- `wallet/slugwallet.db`: the encrypted schema16 fixed native84 settlement wallet.
- `online/admission.seed`, `online/publication.seed`, `online/cutoff.seed`: the
  three online signing roles, required by the operator process.
- `offline/material-v1.bin`: the original wallet entropy and all three online
  role seeds. Keep this offline and back it up privately.
- `offline/SLUG-OPERATOR-POOL-WALLET-BACKUP-V1.json`: the 24 English recovery
  words, with no additional BIP39 passphrase, plus the exact chain, fixed path
  and wallet guard. Keep this offline and back it up privately.
- Intent and READY markers that bind this exact provisioning operation.

Only public Setup and public result fields are safe to share. Do not deploy the
`offline` directory or the entire provision root wholesale. Online operation
needs its separately protected wallet, online role files and runtime inputs;
checkpoint signing remains offline. The command never copies passphrase files
into its output root and never prints words or signing seeds.

The wallet backup is an operator-specific format. It is not a player recovery
file and does not reconstruct pool shares, earnings, payments, pending signed
transactions or the scheduler cursor. Preserve the full service journals and
their original public Setup alongside the wallet under a separate operational
backup procedure. Rebuilding keys alone cannot safely resume settlement.

## Interrupted setup

Use `resume` with the same five selected paths after an interrupted create. It
audits the original intent and material and finishes only missing pre-READY
outputs. It never overwrites, resets or substitutes a wallet or signing role.
An existing wallet directory is opened with the original expected guard; an
empty or torn wallet directory stops as `incomplete_wallet` for inspection.
Never remove that directory to force a retry.

`verify` requires READY and every original output. Both established `resume`
and `verify` check the original files and wallet, including unlock, lock and
clean close. Neither command starts a listener, contacts a node, mines or makes
a payment. Missing established files fail closed. A failed or lost response
after a write reports an unknown outcome; keep the files and resume the exact
operation. READY proves completed offline construction, not deployment health,
funding, current finality or permission to launch the product.
