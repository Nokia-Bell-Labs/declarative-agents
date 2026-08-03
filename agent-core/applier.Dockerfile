# Shared applier image (GH-1368): agent-core plus the helm and kubectl CLIs, so an
# applier's exec words can run `helm upgrade <release> /chart` and
# `kubectl rollout status` in-cluster. One recipe serves every application's
# applier -- the applications differ by the chart their release delivers, not by a
# per-app image build.
#
# The chart is NOT baked into this image. Each app delivers its own chart to the
# applier pod as a mounted volume at /chart (a packaged chart tarball in a
# ConfigMap, unpacked into an emptyDir by an init container), so the chart bytes
# travel with the Helm release rather than an image rebuild.
#
# Build context is the agent-core tree; RUNTIME_IMAGE selects the agent-core base:
#   docker build -f applier.Dockerfile --build-arg RUNTIME_IMAGE=<agent-core> \
#     -t <registry>/applier:0.1.0 .
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
