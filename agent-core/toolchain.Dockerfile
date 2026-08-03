# syntax=docker/dockerfile:1.7
# agent-core-toolchain (GH-1368): the agent-core runtime plus the Go toolchain and
# golangci-lint. Applications whose executor profiles declare `go build`, `go test`,
# and `golangci-lint` exec words select this image by chart values. The toolchain is
# a capability a profile declares, not a property of one application, so this
# generic layer replaces the per-app coding-agent-runtime.
ARG RUNTIME_IMAGE=ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0
ARG GO_IMAGE=golang:1.26-alpine
ARG GOLANGCI_LINT_VERSION=v2.12.2

FROM ${GO_IMAGE} AS toolchain
ARG GOLANGCI_LINT_VERSION
RUN GOBIN=/out go install \
    github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}

FROM ${RUNTIME_IMAGE}
ARG GOLANGCI_LINT_VERSION
USER root
# git and make are the only tools the Go executor contract needs beyond agent-core's
# base runtime set (srd003 R1.2 already ships bash/coreutils/git/grep/sed/tar/... in
# the production agent-core image); apk add is a no-op for packages the base already
# carries and backfills them when the base is the minimal local agent-core.
RUN apk add --no-cache git make
# The Go toolchain and linter the executor's exec words run.
COPY --from=toolchain /usr/local/go /usr/local/go
COPY --from=toolchain /out/golangci-lint /usr/local/bin/golangci-lint
# Writable caches under /tmp so the read-only-root serving pods can run go and
# golangci-lint as their non-root UID.
RUN mkdir -p /tmp/go-build /tmp/go-mod /tmp/golangci-lint \
    && chmod 0777 /tmp/go-build /tmp/go-mod /tmp/golangci-lint

ENV GOCACHE=/tmp/go-build \
    GOMODCACHE=/tmp/go-mod \
    GOLANGCI_LINT_CACHE=/tmp/golangci-lint \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

LABEL org.opencontainers.image.title="Agent Core Toolchain" \
      org.opencontainers.image.description="agent-core runtime with Go 1.26 and golangci-lint" \
      org.opencontainers.image.source="https://github.com/Nokia-Bell-Labs/declarative-agents" \
      io.declarative-agents.golangci-lint.version="${GOLANGCI_LINT_VERSION}"

USER 10001:10001
