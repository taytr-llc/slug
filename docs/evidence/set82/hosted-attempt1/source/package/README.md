# Disposable operator-volume preparation

Source-only packaging draft. No build, Docker execution, host change, service restart or payout is claimed here. The qualified scratch operator image and product sources are unchanged. This admin image is only for the new test operator volume; the separate root-authored `../node/` helper owns node-volume bootstrap.

## Public build context

Root assembles a new context containing only:

- `Dockerfile.admin`, `.dockerignore`, `bootstrap.sh` from this directory.
- `chain.tar`, `wallet.tar` from the existing `deploy/pool/export-context.py` at its exact pinned commits. Use its public tracked-source export, not a working-tree tar.
- `fixture-src/{go.mod,main.go,selection.go,files.go,tls.go}` from the separately reviewed fixture-preparer source manifest. No other source directory is copied.
- `fixture/{reference-v1.json,path-c-reference.json,base-reference.json}` from the exact three public references named in `../INPUT_PLAN.md`.
- A root-generated `fixture-inputs.sha256`, with exactly those five source files and three reference files, using their absolute build destinations: `/src/slug/desktop/hostedfixture/<name>` and `/fixture/<name>`. It must contain no other path. Retain this manifest and its SHA256 outside the image too.

Build native Linuxamd64 with `-f Dockerfile.admin --target admin`, the original chain/wallet archive SHA arguments, `FIXTURE_MANIFEST_SHA256`, and `OPERATOR_IMAGE=<qualified-operator-repository>@sha256:<recorded-digest>`. Do not use an unverified mutable image tag. The Dockerfile checks every fixture input, the three independent reference hashes, candidate compiler/normal wire profile and before/after build sources. It creates a separate workspace including the new helper module without rewriting the original `go.work`. Build evidence is retained under `/usr/share/slug/{operator-build,fixture-build}`.

The temporary admin stage reuses the pinned Go Alpine image's existing BusyBox tools. It installs no packages or SSH server. It contains public source dependencies and synthetic fixture recipes, never a copied operational database or real custody material. Root must review the final helper manifest and freeze this context before building; the preparer is being authored separately.

## Fixed actions and provider handoff

Deploy this admin image on the selected **fresh operator** volume mounted at `/data`, with automatic restart disabled and default action `stage`. It refuses a missing mount, symlink, non-root-owned mount or any existing entry except an empty, real, root-owned `lost+found` (which it does not modify). It changes only that fresh mount's mode to0755 and exclusively creates two0700 directories owned10001:10001:

```
/data/hosted-selection/       # protected input selection, outside fixture root
/data/hosted-test/            # initially empty; fixture preparer owns children
```

The original protected-input code opens every ancestor with `O_RDONLY|O_DIRECTORY`, so the earlier0711 suggestion was insufficient for UID10001.0755 is for the non-secret mount ancestor only; all selected input parents, wallet/runtime roots and file modes retain their existing protections. The prior wrapper and documentation are preserved under `before-mount-read-access/`.

It then `exec`s BusyBox `su` to a fixed UID10001 action that verifies real/effective UID/GID and sleeps for at most1800s. This is a preparation window with no product owner or listener, not a service supervisor. No root process is intended to remain after the UID transition; verify actual process credentials in the first host run. A failure preserves all partial state. There is no reset/resume/retry action; an existing tree is never recursively chowned or erased.

Select the real UTC due only after images, node and routes are ready. Through authenticated Railway SSH, run a fixed shell as10001 to exclusively create `hosted-selection/selection.json` and `hosted-selection/node-rpc.json` as0400, with0700 parent and no links/ACL. The selection is public; the RPC file contains only the explicitly published test credential and selected node CA. Transfer their exact bytes from the separately bound public selection artifacts, with no substitution under old IDs. No general credential upload is added to the product or wrapper. Capture only byte identities/public selections, not file contents of generic protected paths.

Provider SSH may start as the admin image's configured root UID even after PID1 has dropped privileges. Therefore the preparation invocation is explicitly:

```
su -s /bin/sh -c 'exec /usr/local/bin/host-bootstrap prepare' slug
```

`prepare` refuses root and calls only:

```
slug-host-fixture prepare --root /data/hosted-test \
  --selection-file /data/hosted-selection/selection.json \
  --reference-dir /usr/share/slug/fixture \
  --node-rpc-file /data/hosted-selection/node-rpc.json
```

The preparer validates the complete protected inputs and requires its root empty. It creates `private/`, `public/` and `custody/wallet16`, closes the original schema16 wallet and returns public selection evidence. Original `slugpoold init` and stopped `rotate` operations then run explicitly as10001, using path arguments from that evidence; the wrapper does not start or fake them. Confirm enough time remains in the fixed admin window before starting each bounded operation. Do not restart it to extend an admitted operation.

After successful public selection checks, original init/credential commands and clean joins, stop the admin deployment and replace it with the exact qualified scratch operator image and original `serve` arguments. The final image remains10001, has no admin shell/tool or startup repair, and reopens the same runtime/wallet/input paths. Remount/redeployment occurs only with every original owner joined. A failed or interrupted preparation remains an inspected failed attempt; no automatic root repair is part of serve.

## Separate node prerequisite

The root-authored node helper has its own exclusive `/data/node-test` bootstrap and UID handoff. Do not run this operator wrapper on the node volume first. Preserve the original public height2 chain prefix: prefer root-exported raw block1/2 submitted to a fresh normal node, followed by genuine normal `generate60` to height62, with exact public hashes recorded. The three fixture reference JSON files contain configuration/derivation and modeled work, not an already identified raw baseline1/2 export. No runtime database or private-wallet directory is needed in this operator build context.

For the already prepared node root whose first serve was refused under0711, `node-access.sh classify` is read-only. Its separate `serve` action requires that exact mounted0711 root, original10001/0700 child, nine bounded single-link0600 inputs, absent data/log directories and no live node/helper. It only changes the mount to0755, verifies original input metadata, then drops UID and calls original `serve`. It never retries bootstrap or regenerates certificates. Run it only in the explicitly stopped, failed-pre-child generation; established node data refuses this one-shot handoff. Root must explicitly include this script in that narrow successor image/context; it is not a default repair action in the operator admin Dockerfile.

The existing isolated-pilot flag remains absent. The separate node package now has a fixture-only mTLS relay: its PID1 runs as10001 after bootstrap, owns the original normal `slugd` child, and forwards only a verified exact client leaf to the child's loopback P2P listener. The Mac bridge verifies CA, hostname and server SPKI. This is not the earlier proposal in which PID1 simply execs `slugd`. On SIGTERM the relay closes/joins its sockets, then signals and waits for the original child; TERM30s/KILL5s timeouts remain failures. The original operator image is still independent and unchanged. See [host shutdown evidence](../HOST_SHUTDOWN_EVIDENCE.md) for the distinction between source-supported cleanup and actual hosted evidence.
