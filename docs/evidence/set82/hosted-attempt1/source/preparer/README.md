# Synthetic hosted fixture preparer

Source-only, uncompiled external adapter. It reuses the original published
Set81/A2 configuration and funding fixture; it is not a provisioner for real
custody. Root owns build, execution and independent output admission. No product
files, original fixtures, historical A2 outputs or node processes are changed.

Build this module at `slug/desktop/hostedfixture` in the explicitly composed
paired workspace (chain `138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c`, wallet
`dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`), including this module in GOWORK.
Use the original `scripts/candidate-go.sh`, normal PoW, Linux CGO0 profile.
Source files are `main.go`, `files.go`, `selection.go`, `tls.go`, `go.mod`.
No local replacement paths or remote fallback are authored into this module.

The command is:

```text
slug-host-fixture prepare \
  --root /data/hosted-test \
  --selection-file /data/hosted-selection/selection.json \
  --reference-dir /usr/share/slug/fixture \
  [--node-rpc-file /data/hosted-selection/node-rpc.json]
```

The administrator creates only the two new parent directories, then drops to
UID/GID10001. The root must exist, be empty, canonical, owned by the effective
non-root user and0700. Selected host/TLS/RPC files must be that user's regular
protected files. Failure retains every partial output and cannot be resumed by
rerunning this fresh-only adapter. It never deletes or rewrites an old root.

Three immutable public files must appear under `--reference-dir`: selected
`reference-v1.json` (17557b28…ca6a,174293B), `path-c-reference.json`
(a48a854d…7386,46498B), `base-reference.json` (a5c65673…25e3,3176583B).
Their complete SHA256 and exact sizes are embedded in `selection.go`; all are
checked before outputs. The original setup/profile/remote identities and
two-reward arithmetic are checked before the transport-only recomposition.

The host selection is a strict flat JSON object, with all ten fields present:

```json
{
  "version": 1,
  "origin": "https://turntable.proxy.rlwy.net:26354",
  "listen": "0.0.0.0:8443",
  "pool_url": "http://127.0.0.1:18441",
  "admission_url": "http://127.0.0.1:18442",
  "ownership_url": "http://127.0.0.1:18443",
  "due_utc": "REPLACE WITH A FIXED FUTURE SAME-DAY YYYY-MM-DDTHH:MM:00Z",
  "tls_ca_path": "",
  "tls_certificate_path": "",
  "tls_key_path": ""
}
```

The due value has no default and is never moved. Select it only after the image,
volumes, node bootstrap and routing are ready. All three empty TLS paths request
a fresh disposable Ed25519 CA and hostname-valid leaf, ending one hour after due.
Alternatively supply all three explicit protected PEM paths. Both paths verify
the leaf/key, selected hostname, chain, server-auth usage and remaining lifetime
through due+30minutes. Only this test TLS identity uses fresh entropy; the wallet
and online authority keys derive exclusively from the published recipes.

The optional node RPC file is independently parsed using the original strict
RPC grammar, requires HTTPS with explicit CA, and is copied byte-for-byte to
`private/node-rpc.json`. No RPC call is made. If omitted, supply its separately
selected protected path directly to `slugpoold`; it is not part of the public
selection and its credentials are never printed.

Outputs under the fresh root:

- `private/selected-setup.json`, `remote-config.json`, `tls-cert.pem`,
  `tls-key.pem`, `ca.pem`, `public.pass`, `unlock.pass`, the three online `.seed`
  files, `pool-token-0`, `pool-token-1`, `admission-token`, and optional
  `node-rpc.json`. All0600; directories0700.
- `custody/wallet16`: original `CreateDailyPoolSettlementWallet`, public
  BIP39 seed, unchanged schema16 binding. The exact owner is closed before
  success. No mnemonic backup or private-key getter is added.
- `public/setup.json` and `remote.json`: canonical complete wrappers, no LF;
  `ca.pem`, `tls-cert.pem`; `endpoints.json`: Version/Origin/Listen/PoolURL/
  AdmissionURL/OwnershipURL/DueUTC only, no private TLS input paths.
- `public/prepared.json`: version1, three reference hashes, complete setup/base
  setup/profile/remote/base remote IDs, absolute due and planned credential
  expiry milliseconds, leaf SPKI, completed-wallet creation/close observations,
  exact two-reward economics, and optional-RPC-copy observation. This is written
  last and synced. It is preparation evidence, not payment/host acceptance.

There are no service roots, listeners, init/rotate calls or credential records.
Run original `slugpoold init --daily-recoverable-issue` and three stopped
`rotate` commands after preparation, using runtime root `root/operator`, wallet
root `root/custody/wallet16` and protected files above. The two pool tokens and
one selected-gym admission token are the published Set79/A2 hash recipes.
Generation0→1 and expiry=`prepared.credential_expires_at_ms` are plans; real
command responses must be checked before writing the corresponding outer
IssueProfile-bound client credential files. No record can claim a rotation that
did not run.

The process uses the original30second context for wallet creation and refuses
cheap PoW before file access. SIGTERM/cancellation and close errors fail; partial
roots remain for diagnosis. There is no network, subprocess, mining, clock
substitution, old-root adoption or automated retry loop. Root must independently
check public output normalization/IDs before using the newly created stores.

The separate normal node bootstrap omits `localwalletpilot`. Preserve the old
two-block published prefix, then produce60 actual normal blocks to height62
before new work. Discrete `generate` does not require currentness; subsequent
GBT still requires a connected synchronized peer and a tip timestamp within24h.
The local wallet attachment keeps its original loopback HTTP node restriction.
Do not copy this adapter's keys into a production or seed service.
