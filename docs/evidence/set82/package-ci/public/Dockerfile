# Candidate package; qualification is recorded separately. Use the archive context
# described in README.md. No working tree, private input or runtime tree COPY.
# Builder digest is inherited unchanged from slug's pinned node Dockerfile.
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG CHAIN_ARCHIVE_SHA256
ARG WALLET_ARCHIVE_SHA256
ENV GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT= \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOMAXPROCS=2 \
    GOWORK=/src/slug/desktop/go.work
WORKDIR /src
COPY chain.tar wallet.tar /archives/
RUN test "$BUILDPLATFORM" = linux/amd64 && test "$TARGETPLATFORM" = linux/amd64 && \
    printf '%s\n' "$CHAIN_ARCHIVE_SHA256" "$WALLET_ARCHIVE_SHA256" | grep -Eq '^[0-9a-f]{64}$' && \
    test "${#CHAIN_ARCHIVE_SHA256}" = 64 && test "${#WALLET_ARCHIVE_SHA256}" = 64 && \
    printf '%s  %s\n' "$CHAIN_ARCHIVE_SHA256" /archives/chain.tar "$WALLET_ARCHIVE_SHA256" /archives/wallet.tar | sha256sum -c - && \
    tar -xf /archives/chain.tar && tar -xf /archives/wallet.tar && \
    test -f slug/desktop/go.work && test -f slugwallet/go.mod && \
    mkdir -p /out /build-evidence && \
    printf '%s\n' 'slug=138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c' 'slugwallet=dfc5d9dd7e0f604486fbac812d0413fce1f45cd6' > /build-evidence/source-revisions.txt && \
    printf '%s  %s\n' "$CHAIN_ARCHIVE_SHA256" chain.tar "$WALLET_ARCHIVE_SHA256" wallet.tar > /build-evidence/source-archives.sha256 && \
    find slug slugwallet -type f -print0 | sort -z | xargs -0 sha256sum > /build-evidence/source-before.sha256
WORKDIR /src/slug/desktop
RUN sh ../scripts/candidate-go.sh check && \
    go env -json GOVERSION GOOS GOARCH CGO_ENABLED GOWORK GOFLAGS GOEXPERIMENT > /build-evidence/go-env.json && \
    go work edit -json > /build-evidence/workspace.json && \
    go list -mod=readonly -f '{{range .GoFiles}}{{println .}}{{end}}' github.com/taytr-llc/slug/wire/v2 > /build-evidence/wire-files.txt && \
    grep -Fx pow_production.go /build-evidence/wire-files.txt && \
    ! grep -Fx pow_testpow.go /build-evidence/wire-files.txt
RUN go list -mod=readonly -deps -json ./cmd/slugpoold ./cmd/slugpool-provision > /build-evidence/dependencies.json && \
    sh ../scripts/candidate-go.sh build -mod=readonly -p=1 -trimpath -buildvcs=false -tags= -o /out/slugpoold ./cmd/slugpoold && \
    sh ../scripts/candidate-go.sh build -mod=readonly -p=1 -trimpath -buildvcs=false -tags= -o /out/slugpool-provision ./cmd/slugpool-provision && \
    sh ../scripts/candidate-go.sh verify-binary /out/slugpoold /out/slugpool-provision > /build-evidence/build-info.txt && \
    go tool nm /out/slugpoold > /build-evidence/operator-symbols.txt && \
    go tool nm /out/slugpool-provision > /build-evidence/provision-symbols.txt && \
    ! grep -E 'ForTestClock|UseOwnedCalendarOffsetForTest|UseCalendarClockForTest|UseOwnedClockForTest|UseAuthorityClockForTest' /build-evidence/*-symbols.txt
# Exercise the real Linux ACL and lifetime controls as the runtime UID. These
# are component checks; provider volume and full serve/SIGTERM remain separate.
RUN mkdir -p /checks/cache /checks/tmp && cp -a /go/pkg/mod /checks/mod && \
    chown -R 10001:10001 /checks && chown 10001:10001 /build-evidence
ENV GOCACHE=/checks/cache GOMODCACHE=/checks/mod GOTMPDIR=/checks/tmp TMPDIR=/checks/tmp
USER 10001:10001
RUN test "$(id -u)" = 10001 && test "$(id -g)" = 10001 && id > /build-evidence/check-user.txt && \
    if sh ../scripts/candidate-go.sh test -mod=readonly -count=1 -timeout=3m -json ./pooloperator \
      -run '^(TestOperatorPrivateInputOwnedReadAndLifetime|TestOperatorPrivateInputAdmissionRefusals|TestOperatorPrivateInputOriginalIdentityRefusals|TestOperatorPrivateInputLinuxACLRefusal|TestOperatorFilesLockAndIdentity|TestOperatorCommandBlockedOutputJoins|TestOperatorRuntimeCloseJoinsAdmittedLocalAction)$' \
      > /build-evidence/operator-tests.jsonl; then :; else cat /build-evidence/operator-tests.jsonl; exit 1; fi
WORKDIR /src/slug
RUN if sh scripts/candidate-go.sh test -mod=readonly -count=1 -timeout=3m -json ./internal/operatorauthority \
      -run '^TestOperatorAuthorityExtendedACLRefusal$' > /build-evidence/authority-tests.jsonl; then :; else cat /build-evidence/authority-tests.jsonl; exit 1; fi && \
    if sh scripts/candidate-go.sh test -mod=readonly -count=1 -timeout=3m -json ./poolingress \
      -run '^(TestRecoverableIngressOriginalTLSAndJoin|TestIngressCloseCancelsAndJoins)$' > /build-evidence/ingress-tests.jsonl; then :; else cat /build-evidence/ingress-tests.jsonl; exit 1; fi
USER 0:0
WORKDIR /src
RUN find slug slugwallet -type f -print0 | sort -z | xargs -0 sha256sum > /build-evidence/source-after.sha256 && \
    cmp /build-evidence/source-before.sha256 /build-evidence/source-after.sha256 && \
    cp slug/build-profile.json /build-evidence/build-profile.json && \
    sha256sum /out/slugpoold /out/slugpool-provision > /build-evidence/binary-sha256.txt && \
    test -s /etc/ssl/certs/ca-certificates.crt && \
    mkdir -p /runtime-root/etc/ssl/certs /runtime-root/data /runtime-root/tmp && \
    cp /etc/ssl/certs/ca-certificates.crt /runtime-root/etc/ssl/certs/ && \
    printf '%s\n' 'slug:x:10001:10001:SLUG operator:/data:/sbin/nologin' > /runtime-root/etc/passwd && \
    printf '%s\n' 'slug:x:10001:' > /runtime-root/etc/group && \
    chmod 0700 /runtime-root/data && chown 10001:10001 /runtime-root/data && chmod 1777 /runtime-root/tmp

FROM scratch
LABEL org.opencontainers.image.revision="138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c" \
      org.slug.wallet.revision="dfc5d9dd7e0f604486fbac812d0413fce1f45cd6" \
      org.slug.build.profile="slug-go1274-no-cse-v1" \
      org.slug.qualification="candidate package; see exact CI evidence; hosted execution pending"
COPY --from=build /runtime-root/ /
COPY --from=build /out/ /usr/local/bin/
COPY --from=build /build-evidence/ /usr/share/slug/build/
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
USER 10001:10001
WORKDIR /data
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/slugpoold"]
CMD ["help"]
