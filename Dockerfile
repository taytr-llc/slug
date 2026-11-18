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

# Public Go 1.27.4 index pinned by digest. Container qualification is separate.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build

WORKDIR /app
COPY . .
# Run the bounded normal-profile gate in the build stage, without diagnostic
# tags. This tests the build-stage platform; cross-target runtime qualification
# is separate. Preserve successful output in the shipped build evidence.
RUN mkdir -p /build-evidence && \
    printf '%s\n' 'Normal-profile build-stage tests only; cross-target runtime qualification is separate.' > /build-evidence/test-scope.txt && \
    if sh scripts/candidate-go.sh test -count=1 -timeout=10m ./gymresult ./wire/... ./chaincfg/... > /build-evidence/normal-profile-tests.txt 2>&1; then \
        cat /build-evidence/normal-profile-tests.txt; \
    else \
        cat /build-evidence/normal-profile-tests.txt; exit 1; \
    fi

RUN sh scripts/candidate-go.sh build -o /out/slugd . && \
    sh scripts/candidate-go.sh build -o /out/ ./cmd/...

# Verify every shipped executable before leaving the Go build stage. Retain the
# exact profile and embedded build settings alongside the runtime image.
RUN mkdir -p /build-evidence && \
    cp build-profile.json /build-evidence/build-profile.json && \
    sh scripts/candidate-go.sh verify-binary /out/* > /build-evidence/build-info.txt && \
    sha256sum /out/* > /build-evidence/binary-sha256.txt

FROM alpine:3.24

COPY --from=build /out/ /bin/
COPY --from=build /build-evidence/ /usr/share/slug/build/
# Public signed records are available for explicit --signedcheckpoint imports.
COPY checkpoints/ /usr/share/slug/checkpoints/

EXPOSE 29333 29332

ENTRYPOINT ["slugd"]
