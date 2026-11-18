# GitHub action dockerfile
# Requires docker experimental features as buildx and BuildKit so not suitable for developers regular use.
# https://docs.docker.com/develop/develop-images/build_enhancements/#to-enable-buildkit-builds

###########################
# Build binaries stage
###########################
# Public Go 1.27.4 index pinned by digest. Container qualification is separate.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ADD . /app
WORKDIR /app
# Arguments required to build binaries targetting the correct OS and CPU architectures
ARG TARGETOS TARGETARCH
# Build into a flat directory. When cross-compiling, go install would put the
# binaries into a per-platform subdirectory such as /go/bin/linux_arm64.
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

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH sh scripts/candidate-go.sh build -v -o /out/slugd . && \
    GOOS=$TARGETOS GOARCH=$TARGETARCH sh scripts/candidate-go.sh build -v -o /out/ ./cmd/...

# Verify every shipped executable before leaving the Go build stage. Retain the
# exact profile and embedded build settings alongside the runtime image.
RUN mkdir -p /build-evidence && \
    cp build-profile.json /build-evidence/build-profile.json && \
    sh scripts/candidate-go.sh verify-binary /out/* > /build-evidence/build-info.txt && \
    sha256sum /out/* > /build-evidence/binary-sha256.txt

###########################
# Build docker image stage
###########################
FROM alpine:3.24
COPY --from=build /out/ /bin/
COPY --from=build /build-evidence/ /usr/share/slug/build/
# 8333  Mainnet Bitcoin peer-to-peer port
# 8334  Mainet RPC port
EXPOSE 8333 8334
ENTRYPOINT ["slugd"]
