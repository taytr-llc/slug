# SLUG node deploy image (Railway-compatible).
#
# Minimal standard Dockerfile replacing btcd's upstream one, whose
# VOLUME instruction Railway rejects ("docker VOLUME ... is not
# supported, use Railway Volumes").
#
# Builds the slugd and slugctl binaries from this repo (go.work ties the
# 10 modules together) and runs slugd. There is intentionally NO VOLUME
# instruction here: Railway attaches its own volume at /data via the
# dashboard, and the node's --datadir=/data start flag points there.
#
# 29333  SLUG mainnet peer-to-peer port (expose this via TCP proxy)
# 29332  SLUG mainnet RPC port (keep private inside the container)
#
# NOTE: the Railway service start command must invoke slugd (not btcd):
#   slugd --notls --rpcuser=u --rpcpass=p --datadir=/data

FROM golang:1.27-alpine AS build

WORKDIR /app
COPY . .
RUN go build -o /out/slugd . && go build -o /out/ ./cmd/...

FROM alpine:3.24

COPY --from=build /out/ /bin/

EXPOSE 29333 29332

ENTRYPOINT ["slugd"]
