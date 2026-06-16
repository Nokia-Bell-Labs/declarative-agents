# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26-alpine
ARG RUNTIME_IMAGE=alpine:3.22

FROM ${GO_IMAGE} AS builder

ARG AGENT_CORE_REPO=https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core.git
ARG AGENT_CORE_REF=main

WORKDIR /src

RUN apk add --no-cache ca-certificates git openssh-client

# Private HTTPS GitLab clones can pass a .netrc with:
#   docker build --secret id=git_credentials,src=$HOME/.netrc .
RUN --mount=type=secret,id=git_credentials,target=/root/.netrc \
    git clone --branch "${AGENT_CORE_REF}" --depth 1 "${AGENT_CORE_REPO}" .

RUN go mod download

# Build the binary before tests so integration-style tests can invoke bin/agent.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin/agent ./cmd/agent

RUN go test ./...

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/agent ./cmd/agent

FROM ${RUNTIME_IMAGE} AS runtime

RUN apk add --no-cache \
    bash \
    ca-certificates \
    coreutils \
    diffutils \
    findutils \
    gawk \
    git \
    grep \
    gzip \
    make \
    openssh-client \
    sed \
    tar \
    unzip

WORKDIR /work

COPY --from=builder /out/agent /usr/local/bin/agent

# Runtime YAML assets used by profiles mounted from evaluation repositories.
COPY --from=builder /src/tools /opt/agent-core/tools
COPY --from=builder /src/agents/generator /opt/agent-core/agents/generator
COPY --from=builder /src/agents/evaluator /opt/agent-core/agents/evaluator

RUN addgroup -S agent \
    && adduser -S agent -G agent \
    && mkdir -p /work \
    && chown -R agent:agent /work

ENV AGENT_CORE_HOME=/opt/agent-core \
    HOME=/tmp \
    PATH=/usr/local/bin:/usr/bin:/bin

USER agent:agent

LABEL org.opencontainers.image.title="Agent Core" \
      org.opencontainers.image.description="Declarative Grammar Machine agent runtime with shared evaluator/generator configuration" \
      org.opencontainers.image.source="https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core" \
      org.opencontainers.image.vendor="Nokia"

STOPSIGNAL SIGTERM

ENTRYPOINT ["agent"]
