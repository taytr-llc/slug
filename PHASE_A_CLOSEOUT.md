# Phase A closeout: authorized release procedure

Mike authorized the master merge and seed deployment on November 15, 2026
with "Do it!" after completion of the first signed checkpoint. This is
the disposable Phase A chain, not the permanent production genesis.

This document records preflight evidence and the intended rollout/rollback.
The actual deployment ID, observed build, finality report, public sync and
monitoring results must be recorded after Railway completes the rollout;
the procedure alone is not evidence that a deployment succeeded.

## Release contents and evidence

Sets 1–11 provide the Argon2id chain, SLUG network parameters, restart fix,
deploy image, rebrand, module rename, report-only signed checkpoints,
PoW security review/fixes and full blockchain/integration verification.
Set 12 adds public checkpoint status to startup logs so a private-RPC seed
can be verified from its operational logs. No private signing material or
additional consensus change is included.

- Complete cheap-profile blockchain suite: 45.62 seconds wall time.
- Complete integration/RPC harness suite: 21.68 seconds wall time.
- Production proof/genesis vectors, normal short scenarios and all-module
  builds passed. Sets 10–11 verified the actual operator signature.
- The signed-record smoke covered pending, a valid conflicting local branch,
  normal adoption of the seed chain, finalized status and restart. It paid
  10 SLUG per mined block and preserved the original height-3 ancestry.
- GitHub exposes zero registered workflows, runs or check results for the
  release tip; CI is not claimed green. Local gates are the release evidence.
  Existing address-test Bitcoin vectors, committed-workspace workflow setup,
  slow default PoW CI targets and legacy release tooling remain maintenance
  debt, as documented in sets 6 and 9. Railway does not wait for check suites.
- Independent review found no production genesis, network magic, Argon2
  parameter or database-format change relative to the old seed build.
  Set 8's resource-exhaustion and local-corruption residuals still apply.

## Exact rollout

Merge the verified stack into `master` with one explicit merge commit,
preserving upstream history. Push only after the local gates pass. Stage
both the Railway source and startup settings, inspect the pending diff,
and commit them together. Keep the same volume, TCP endpoint and replica.

- Project: `successful-cooperation` (`62fdb666-7f67-4cf3-98f0-6177f099b580`).
- Environment: `production` (`3fd7da79-0f2f-4077-bf29-e7035d62ac57`).
- Service: `slug` (`7447ce93-4786-42cf-ae81-91309b140ff0`).
- Repository/branch: `taytr-llc/slug`, `master`.
- Build file: root `Dockerfile`, not the auxiliary GitHub workflow Dockerfile.
- Volume: `slug-volume` (`0d74c49e-b04d-4a27-b995-f8d97b3b1f9f`), unchanged at `/data`.
- Public P2P: `junction.proxy.rlwy.net:16023` → 29333. RPC remains private.
- One replica in `us-west2`.

```text
slugd --notls --rpcuser=u --rpcpass=p --datadir=/data --signedcheckpoint=/usr/share/slug/checkpoints/phase-a/height-1.json
```

The root Dockerfile builds without test tags and packages the public JSON.
The cheap profile also rejects mainnet and has a visible version suffix;
the deployed version must be normal `0.27.0`.

## Acceptance and observation

Require a successful Railway build and healthy startup against the existing
volume. Observe the expected version and report-only checkpoint status in
startup logs. A fresh normal client must download the same chain through
the public seed endpoint. Verify:

- Genesis `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`.
- Height 1 `729d92f2de29fa6415f4920df49164a8a9824971ff972d42037ee7c9d41f470f`.
- Height 61 `1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0`.
- Authority `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`.
- Checkpoint status finalized, at least 60 descendants, no conflict.

Review runtime errors, restarts/OOMs, resource usage and peer service after
deployment. The preflight baseline was about 0.184 GB memory and idle CPU;
short production hash operations legitimately allocate a 256 MiB arena.
Observe the service for 15 minutes after successful startup; do not claim
that observation window is complete before it has elapsed. No HTTP health
endpoint or new public RPC proxy is introduced.

## Rollback

Trigger rollback for a bad image/startup loop, missing volume/chain state,
wrong genesis, checkpoint conflict at the expected chain, unavailable peer
service, or sustained new resource failure. Do not delete or replace /data.

Restore **both** source/build and startup arguments:

- Old successful deployment: `280f6837-d409-4841-bfd7-d92410441120`.
- Old branch/commit: `slug-rebrand` at `0574b8ba131f7c7b9d5def07b1371604465cb2e9`.
- Old command: `slugd --notls --rpcuser=u --rpcpass=p --datadir=/data`.

The old client does not recognize `--signedcheckpoint` and its image lacks
the public JSON, so reverting only its image is insufficient. The ordinary
chain data remains compatible; the signed record is external and does not
change consensus or disk schema. Preserve the new master history and fix
forward on a new branch if a rollout issue appears.
