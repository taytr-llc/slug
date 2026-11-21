# Bounded Linux operator package

Candidate package; [native image/component qualification passes](../../CHANGESET_82_NOTES.md)
for the original package at `2d79de95`. The corrected source below requires its
own image/host qualification. The public source export has been checked locally. This packages the existing finite operator;
it adds no capacity, migration, account login or node supervisor. No private
files are copied into the build context and no service is deployed by this recipe.

The selected sources are chain `359a2853ea092b17a832dd1fc824c5d47aec5f12` and wallet `dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`. Preserve sibling `slug/` and `slugwallet/` directories: `slug/desktop/go.work` selects all paired modules, and its `go.mod` replaces the wallet with `../../slugwallet`. Build from `slug/desktop`, using local Go1.27.4 and the unchanged `scripts/candidate-go.sh` compiler profile. Do not build from the seed's existing Docker context.

Linux's protected-file ACL checks use `x/sys/unix.Fgetxattr`, not a C library. The existing Linux qualification script already builds `slugpoold` with `CGO_ENABLED=0`; only its race tests and `slugpilot` build select CGO. This image therefore uses static CGO0 headless binaries and `scratch`, with no GCC, glibc/musl, libacl, GTK or Wails runtime dependency. Darwin's CGO requirement does not transfer to Linux. Both shipped commands now pass native image execution; the earlier operator-only build remains separate evidence.

## Public build context

Run the following in a native Linux amd64 build environment. `chain_checkout`
and `wallet_checkout` denote the paired source checkouts; `package_draft`
denotes this directory. The context contains Dockerfile, .dockerignore, two
tracked-source archives and a public context manifest. The manifest is excluded
from the Docker context. No working-tree files, runtime directories or private
credentials enter it; published test fixtures already committed to Git remain
part of the public source archive.

```sh
context_parent=$(mktemp -d)
context_dir="$context_parent/public-context"
python3 "$package_draft/export-context.py" --chain-checkout "$chain_checkout" \
  --wallet-checkout "$wallet_checkout" --output "$context_dir"
chain_archive_sha=$(sha256sum "$context_dir/chain.tar" | cut -d' ' -f1)
wallet_archive_sha=$(sha256sum "$context_dir/wallet.tar" | cut -d' ' -f1)
docker build --platform linux/amd64 \
  --build-arg CHAIN_ARCHIVE_SHA256="$chain_archive_sha" \
  --build-arg WALLET_ARCHIVE_SHA256="$wallet_archive_sha" \
  --tag slug-operator:359a2853 "$context_dir"
```

The archive commit checks are part of the build procedure; OCI labels alone do not authenticate a source tree. The Dockerfile verifies both supplied archive hashes, the candidate compiler, production wire selection and final binaries, and refuses any source-file change during compilation. It retains dependency closure, workspace, binary hashes and build settings under `/usr/share/slug/build/`. Root must retain the archive hashes, build logs and resulting image digest. The exact public Go Alpine image digest comes from the selected chain Dockerfile; the native Linux image job successfully resolved and used it. Module downloads still need the pinned checksums and reachable public module sources or a separately verified cache; no module/version update is proposed. This is a reproducible selection recipe, not a claimed byte-identical image result.

The branch-scoped [package workflow](../../.github/workflows/hosted-pool-package.yml)
builds this image on native Linux amd64. It requires ten existing component
tests and all five Linux ACL subcases, plus two ownership HTTPS tests and nine
refusal subcases under UID/GID10001, then executes both
shipped commands' help paths in the final unprivileged, network-disabled image.
Its public evidence distinguishes those checks from a real hosted service,
mounted-volume acceptance, full PID1 shutdown and scheduled payment.

## Protected runtime layout

Use one non-root UID/GID, proposed `10001:10001`, for the process and every selected file/root. Real and effective UID must match. Private directories and operator/wallet roots must be0700; selected input files must be regular, single-link,0400 or0600, with no symlink ancestry or extended POSIX ACL. Even public Setup/certificate files are read through these protected-input rules. The volume must support the original file locking, atomic/synchronized writes and ACL queries; `ENOTSUP` is not treated as permission to skip the ACL check. Docker secret mounts commonly use root ownership or symlinks and are not automatically compatible. Do not weaken the checks or change ownership under a running owner.

Keep two explicit host locations: a writable persistent data tree containing only the original runtime and wallet, and a protected read-only inputs directory containing the selected public Setup, TLS chain/key, node RPC configuration, passphrase files and three online authority seeds. Prepare these through the existing operator custody workflow, outside the image. Keep the offline material, recovery-word backup and checkpoint signing key off the host/image. Never mount the complete provisioning output wholesale.

[Railway's volume documentation](https://docs.railway.com/volumes) says volumes
mount as root and are unavailable during pre-deploy. A mounted volume does not
inherit the image's UID10001 ownership. A separate ownership and protected-input
preparation must complete before this process starts; no root fallback or
startup chmod/chown is included. Qualify the actual mount and ownership
arrangement; the generic Docker bind example below does not establish provider
compatibility.

The offline provisioner is available as `/usr/local/bin/slugpool-provision`. Its fresh command is `create --daily-recoverable-issue` with the existing five explicit policy/root/certificate/passphrase-file selections; `resume` and `verify` retain the same original operation. The operator image does not run provisioning automatically or generate any private material on startup. An already selected wallet and online roles must exist before `slugpoold init`.

## Explicit operator launch

The example paths below must name independently selected, already protected files. Bash array contents contain paths only, never secret values. `--daily-recoverable-issue` is the sole mode selector; do not combine it with `--daily`, including explicit false variants. No source, payee, quota or TLS pin is inferred from an environment variable.

```sh
operator_files=(
  --daily-recoverable-issue
  --setup-file /inputs/setup.json
  --root /data/runtime --wallet-root /data/wallet
  --wallet-public-passphrase-file /inputs/public-passphrase
  --wallet-unlock-passphrase-file /inputs/unlock-passphrase
  --admission-seed-file /inputs/admission.seed
  --publication-seed-file /inputs/publication.seed
  --cutoff-seed-file /inputs/cutoff.seed
  --rpc-config-file /inputs/node-rpc.json
  --tls-certificate-file /inputs/server-chain.pem
  --tls-key-file /inputs/server-key.pem
)
container_options=(
  --read-only --user 10001:10001 --cap-drop ALL
  --security-opt no-new-privileges:true --restart=no
  --stop-timeout=180
  --mount type=bind,src=/srv/slug-operator-data,dst=/data
  --mount type=bind,src=/srv/slug-operator-inputs,dst=/inputs,readonly
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=64m,mode=1777
)
docker run --name slug-operator-init "${container_options[@]}" \
  slug-operator:359a2853 init "${operator_files[@]}"
# Only after the original init result and exact selected identity are verified:
docker run --name slug-operator-bounded "${container_options[@]}" \
  --publish "$selected_public_port:$selected_ingress_port" \
  slug-operator:359a2853 serve "${operator_files[@]}"
```

Bind the image by its recorded digest for execution. Port values must match the selected public transport and ingress listen configuration. Publish only ingress. Keep pool/admission/ownership upstreams on their configured literal loopback endpoints. The operator terminates its own TLS1.3/HTTP1.1 and verifies Host/SNI/certificate selection; use raw TCP forwarding. An HTTP edge that terminates TLS or inserts forwarded-identity headers is not a compatible substitute. No `$PORT` override or automatic repinning is introduced.

Use a separately managed node with verified HTTPS RPC on its private hostname
and an explicit CA in the protected RPC configuration. The operator ownership
reader requires that explicit CA for every non-loopback endpoint; normal TLS
hostname verification and network/genesis checks still apply. Plain HTTP is
limited to literal loopback IPs. The desktop ownership reader retains its
loopback-only rule. Node RPC remains private and authenticated. `slugpoold`
does not launch or supervise a node.

The original `138553b1` image failed hosted initialization because the operator
ownership reader still inherited the desktop address restriction. The scoped
correction and real TLS regression checks are recorded in
[Set82's evidence](../../docs/evidence/set82/operator-https-fix/report.json).
The first hosted failure is retained separately; the corrected image needs its
own hosted payment proof.

Consume stdout continuously and retain public logs; blocked output is an error. PID1 is the original operator process, so SIGTERM reaches its existing joined shutdown. The180s container stop grace is a proposed supervisor allowance, not a product deadline extension or a promise of clean shutdown. Prove it on the selected host. Preserve containers/logs and all persistent roots on failure; do not auto-reset or restart uncertain owners. Local `status`, `rotate`, `revoke` and `checkpoint` commands need the original runtime stopped and exclusively available; a parallel `status` container is not a live health check. Use existing serve status plus authenticated service observations for the bounded test.

## Remaining target execution

The native Linux CI image build, UID10001 component controls and final command
execution pass. The exact public evidence is linked above. The local Docker
daemon is unavailable; no local container execution is claimed. Still qualify
the selected provider volume/ownership, direct TLS, private HTTPS node access,
an original worker receipt, scheduled payment to its selected wallet, full
serve/PID1 shutdown and exact stopped reopen. Retain compiler/image/source
identities and clean joins. Existing finite reservations and D84 remain in force;
general continuation is not a prerequisite. Size the host for normal 256 MiB
Argon2id verification and measured peak memory.

A2's local scheduled-payment proof is useful prior evidence. It does not establish remote DNS/TLS, Linux volume behavior, host resource capacity or image shutdown. The failed extended-offline B1 catch-up remains a separately recorded limitation; this package makes no extended-offline recovery or continuous public-operation claim. D74 stays the final release review.
