# SLUG Change Set 4: Minimal Railway-Compatible Dockerfile

Branch `slug-deploy`, one commit on top of change set 3
(`slug-chain-fixes`, tip 30acf83), which stacks on change sets 2
and 1 and master (dd169fe). The bundle `deploy.bundle` contains
the full history from master, all four change sets included, and
was verified with `git bundle verify`.

Commit:
- 322135b Replace upstream Dockerfile with minimal Railway-compatible build

## What changed and why

Only the root `Dockerfile` changed (16 insertions, 36 deletions).
No Go code, no chain parameters, no consensus behavior.

Railway rejected btcd's upstream Dockerfile at validation. The
exact error from the Railway build log:

> dockerfile invalid: docker VOLUME at Line 43 is not supported, use Railway Volumes

The upstream file's only actual blocker was its
`VOLUME ["/root/.btcd"]` instruction. The replacement is a minimal
standard two-stage build that also drops the upstream quirks
(digest-pinned `FROM`, `--platform=$BUILDPLATFORM`, variable-based
final `FROM $ARCH/alpine`) so nothing else trips Railway's
validator:

```dockerfile
FROM golang:1.27-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o /out/ . ./cmd/...

FROM alpine:3.24
COPY --from=build /out/ /bin/
EXPOSE 29333 29332
ENTRYPOINT ["btcd"]
```

(A brief comment header in the file itself records that it is the
SLUG/Railway deploy image and why it looks the way it does.)

**There is intentionally NO `VOLUME` instruction in this
Dockerfile.** Railway rejects `VOLUME` outright. Persistence is
Railway's job instead: a Railway Volume attached at `/data` in the
dashboard, and the node's `--datadir=/data` start flag (see the
Railway settings below) points the chain data at it.

The build uses the repo workspace: the root `go.work` ties the
repo's 10 Go modules together, and `go build . ./cmd/...` from the
repo root builds the node (`btcd`), the CLI (`btcctl`), and the
helper tools from the local modules.

## Verification, what actually ran

Environment: Ren's VM, Go 1.27.4. Docker is NOT installed on this
VM, so the image build itself could not be tested locally; the
first real image build is Railway's. What was verified is the
exact build command the Dockerfile runs:

1. **Equivalent local build: pass.** From the repo root on
   `slug-deploy`: `go build -o /tmp/dockerfile-check/ . ./cmd/...`
   exited 0 in ~7 s and produced all five binaries:
   `btcd` (19 MB), `btcctl` (12 MB), `addblock`, `findcheckpoint`,
   `gencerts`.
2. **Binary smoke check: pass.** `/tmp/dockerfile-check/btcd
   --version` prints `btcd version 0.27.0`.
3. **No VOLUME line: confirmed** by grep on the committed
   Dockerfile.

## For Mike: fetch and push the branch

```sh
# put deploy.bundle somewhere on the Mac, then:
cd ~/slug/slug
git fetch ~/slug/deploy.bundle slug-deploy:slug-deploy
git checkout slug-deploy
git push -u origin slug-deploy
```

## Railway settings that pair with this Dockerfile

- **Service source:** GitHub repo `taytr-llc/slug`, branch
  **`slug-deploy`**. Railway builds the root Dockerfile
  automatically.
- **Start command** (arguments to the `btcd` entrypoint):
  `--notls --rpcuser=u --rpcpass=p --datadir=/data`
- **Volume:** attach a Railway Volume mounted at **`/data`**
  (this is what makes the chain data survive redeploys; the
  Dockerfile deliberately declares no volume of its own).
- **Networking:** TCP proxy to container port **29333** (the SLUG
  P2P port). That public endpoint is what other nodes peer with.
  Port 29332 (RPC) is exposed in the image for completeness but
  should NOT be proxied publicly; RPC stays private inside the
  container.
- The seed node does not mine, so no `--miningaddr` is needed in
  the start command.

Once the service is up, both other nodes join with a runtime
`addnode "<railway-host>:<railway-port>" add` against their local
RPC; no restart needed on either side.
