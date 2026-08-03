# syntax=docker/dockerfile:1.7
#
# The agent-architecture runtime image: one image that runs both the
# documentation-curator and the collector. It carries the agent binary and the
# core-owned tool declarations the curator profile names under /opt/agent-core.
# Canonical profiles (config) are mounted at /profiles, never baked in. The one
# exception is the documentation-curator's staged workspace: its browser UI dist
# trees total several MB, far past the profile ConfigMap payload limit, and the
# curator's document routes need the catalog docs tree, so both are baked under
# /opt/curator-ui and an init container stages them into the curator's writable
# workspace at runtime (GH-1261, GH-1293). Build context is the repository root
# so the builder reaches agent-core and the catalog UI.

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

# busybox wget backs the curator loopback health probe; jq backs the curator
# profile's declared doc_index_jq exec tool; ca-certificates and bash round out
# the runtime the mounted profiles expect.
RUN apk add --no-cache bash ca-certificates jq

WORKDIR /work

COPY --from=builder /out/agent /usr/local/bin/agent
COPY agent-core/tools /opt/agent-core/tools

# The documentation-curator's browser UI (ux.json, docs, and monitor dist trees)
# is baked here because it is too large for the profile ConfigMap mount. The
# curator init container copies it into the writable workspace at runtime so the
# profile's literal ui/*/dist static_assets roots resolve (GH-1261). The catalog
# docs tree rides in the same staged root: the profile's document routes list
# from the workspace-relative docs/ root, and without it every /api/v1/docs
# request fails with CommandError (GH-1293).
COPY applications/catalog/agents/knowledge-manager/documentation-curator/ui \
    /opt/curator-ui/agents/knowledge-manager/documentation-curator/ui
COPY applications/catalog/docs /opt/curator-ui/docs

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
