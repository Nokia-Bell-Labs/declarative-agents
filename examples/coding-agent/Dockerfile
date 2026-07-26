# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26-alpine
ARG GOLANGCI_LINT_VERSION=v2.12.2

FROM ${GO_IMAGE} AS builder

ARG GOLANGCI_LINT_VERSION
WORKDIR /src/agent-core

COPY agent-core/go.mod agent-core/go.sum ./
RUN go mod download

COPY agent-core/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -tags production -trimpath \
    -ldflags="-s -w" -o /out/agent ./cmd/agent
RUN GOBIN=/out go install \
    github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}

# The canonical executor runs go build, golangci-lint, and go test. Retaining the
# pinned Go image here is therefore part of the application runtime contract,
# not a smoke-only convenience. Profiles remain mounted at runtime.
FROM ${GO_IMAGE} AS runtime

ARG GOLANGCI_LINT_VERSION
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
    ripgrep \
    sed \
    tar \
    unzip

WORKDIR /work

COPY --from=builder /out/agent /usr/local/bin/agent
COPY --from=builder /out/golangci-lint /usr/local/bin/golangci-lint
COPY agent-core/tools /opt/agent-core/tools

RUN <<'EOF'
cat > /usr/local/bin/agent-entrypoint <<'SCRIPT'
#!/bin/sh
for arg in "$@"; do
    case "$arg" in
        --help|-h|help|--version|version)
            exec agent "$@"
            ;;
        --profile|--profile=*)
            has_profile=1
            ;;
    esac
done
if [ "${has_profile:-0}" != "1" ]; then
    echo "Error: --profile is required; mount profiles and pass --profile /profiles/agents/<name>/profile.yaml" >&2
    exit 1
fi
exec agent "$@"
SCRIPT
chmod +x /usr/local/bin/agent-entrypoint
EOF

RUN addgroup -S -g 10001 agent \
    && adduser -S -u 10001 -G agent agent \
    && mkdir -p /work /tmp/go-build /tmp/go-mod /tmp/golangci-lint \
    && chown -R 10001:10001 /work /tmp/go-build /tmp/go-mod /tmp/golangci-lint

ENV AGENT_CORE_HOME=/opt/agent-core \
    GOCACHE=/tmp/go-build \
    GOMODCACHE=/tmp/go-mod \
    GOLANGCI_LINT_CACHE=/tmp/golangci-lint \
    HOME=/tmp \
    PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

USER 10001:10001

LABEL org.opencontainers.image.title="Declarative Coding Agent Runtime" \
      org.opencontainers.image.description="Profile-free agent runtime with Go 1.26 and golangci-lint v2.12.2" \
      org.opencontainers.image.source="https://github.com/Nokia-Bell-Labs/declarative-agents" \
      org.opencontainers.image.version="0.1.0" \
      io.declarative-agents.golangci-lint.version="${GOLANGCI_LINT_VERSION}"

STOPSIGNAL SIGTERM

ENTRYPOINT ["agent-entrypoint"]
