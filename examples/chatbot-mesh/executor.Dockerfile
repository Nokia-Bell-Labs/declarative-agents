# Executor image (srd006, srd003 R1.3 sanctioned divergence): the agent-core runtime
# plus the helm and kubectl CLIs and the chart at /chart, so the executor's exec words
# can run `helm upgrade chatbot-mesh /chart` and `kubectl rollout status` in-cluster.
# Build context is examples/chatbot-mesh:
#   docker build -f executor.Dockerfile -t <registry>/chatbot-mesh-executor:0.1.0 .
ARG RUNTIME_IMAGE=ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0
ARG HELM_VERSION=v3.16.3
ARG KUBECTL_VERSION=v1.31.0
ARG TARGETARCH=amd64

FROM alpine:3.20 AS tools
ARG HELM_VERSION
ARG KUBECTL_VERSION
ARG TARGETARCH
RUN apk add --no-cache curl tar && \
    curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" | tar xz -C /tmp && \
    install -m 0755 "/tmp/linux-${TARGETARCH}/helm" /usr/local/bin/helm && \
    curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod 0755 /usr/local/bin/kubectl

FROM ${RUNTIME_IMAGE}
COPY --from=tools /usr/local/bin/helm /usr/local/bin/helm
COPY --from=tools /usr/local/bin/kubectl /usr/local/bin/kubectl
# The chart the executor's helm_upgrade word installs, at the /chart path its exec
# args reference.
COPY helm /chart
