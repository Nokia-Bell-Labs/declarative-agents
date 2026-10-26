# syntax=docker/dockerfile:1
#
# The image each scrum-master release publishes (eng03): the agent, composed
# with the scrum-master words, and the release's profiles, so a CI job runs a
# profile with no install step and no Go toolchain (GH-34). The build context
# is the directory mage publish builds the release into; the image copies the
# agent for its architecture and unpacks the profiles archive, so both are the
# ones on the GitHub release.

ARG BASE_IMAGE=debian:bookworm-slim

FROM ${BASE_IMAGE}
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="scrum-master" \
      org.opencontainers.image.description="the agent with the scrum-master words and profiles" \
      org.opencontainers.image.source="https://github.com/Nokia-Bell-Labs/declarative-agents" \
      org.opencontainers.image.vendor="Nokia" \
      org.opencontainers.image.licenses="BSD-3-Clause" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git git-crypt \
 && rm -rf /var/lib/apt/lists/* \
 && git config --system safe.directory '*' \
 && useradd --uid 1000 --create-home --shell /bin/sh agent \
 && install -d -o agent -g agent /work /opt/scrum-master
COPY agent_linux_${TARGETARCH} /usr/local/bin/agent
ADD --chown=agent:agent scrum-master-profiles.tar.gz /opt/scrum-master/
ENV SCRUM_MASTER_HOME=/opt/scrum-master
USER agent
WORKDIR /work
ENTRYPOINT []
CMD ["agent", "--help"]
