# syntax=docker/dockerfile:1.7
#
# The agent-architecture runtime image: one profile-free image that runs both
# the documentation-curator and the collector. It carries the agent binary and
# the core-owned tool declarations the curator profile names under
# /opt/agent-core; the canonical profiles are mounted at /profiles, never baked
# in. Build context is the repository root so the builder reaches agent-core.

ARG GO_IMAGE=golang:1.26-alpine
ARG RUNTIME_IMAGE=alpine:3.22

FROM ${GO_IMAGE} AS builder

WORKDIR /src/agent-core

COPY agent-core/go.mod agent-core/go.sum ./
RUN go mod download

COPY agent-core/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -trimpath \
    -ldflags="-s -w" -o /out/agent ./cmd/agent

FROM ${RUNTIME_IMAGE} AS runtime

# busybox wget backs the curator loopback health probe; ca-certificates and bash
# round out the runtime the mounted profiles expect.
RUN apk add --no-cache bash ca-certificates

WORKDIR /work

COPY --from=builder /out/agent /usr/local/bin/agent
COPY agent-core/tools /opt/agent-core/tools

RUN addgroup -S -g 10001 agent \
    && adduser -S -u 10001 -G agent agent \
    && mkdir -p /work /data /tmp \
    && chown -R 10001:10001 /work /data

ENV AGENT_CORE_HOME=/opt/agent-core \
    HOME=/tmp \
    PATH=/usr/local/bin:/usr/bin:/bin

USER 10001:10001

LABEL org.opencontainers.image.title="Declarative Agent Architecture Runtime" \
      org.opencontainers.image.description="Profile-free agent runtime carrying agent-core and core-owned tools" \
      org.opencontainers.image.source="https://github.com/Nokia-Bell-Labs/declarative-agents" \
      org.opencontainers.image.version="0.1.0"

STOPSIGNAL SIGTERM

ENTRYPOINT ["agent"]
