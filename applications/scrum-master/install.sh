#!/bin/sh
# Installs scrum-master from a declarative-agents release (eng03, GH-34):
# downloads the agent for this machine, the profiles archive, and the
# release's SHA256SUMS, refuses a file whose checksum differs, installs the
# agent, and unpacks the profiles.
#
#   sh install.sh [tag]
#
# With no tag it installs the latest release. AGENT_PREFIX is the directory
# the agent installs into, by default $HOME/.local/bin. SCRUM_MASTER_HOME is
# where the profiles unpack, by default $XDG_DATA_HOME/scrum-master or
# $HOME/.local/share/scrum-master. SCRUM_MASTER_RELEASE_URL is where the
# releases are published. SCRUM_MASTER_RELEASE_FORGE=gitlab reads them from a
# GitLab project instead, such as an internal fork (GH-31): the URL is then
# the project's, and GITLAB_TOKEN, when set, reads a private one.
set -eu

tag=${1:-latest}
prefix=${AGENT_PREFIX:-$HOME/.local/bin}
home=${SCRUM_MASTER_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/scrum-master}
forge=${SCRUM_MASTER_RELEASE_FORGE:-github}
base=${SCRUM_MASTER_RELEASE_URL:-https://github.com/Nokia-Bell-Labs/declarative-agents/releases}

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "install.sh: no release agent for $(uname -s); the releases build Linux and macOS" >&2; exit 1 ;;
esac
case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "install.sh: no release agent for $(uname -m); the releases build amd64 and arm64" >&2; exit 1 ;;
esac
agent=agent_${os}_${arch}
profiles=scrum-master-profiles.tar.gz

case $forge in
  github)
    if [ "$tag" = latest ]; then url=$base/latest/download; else url=$base/download/$tag; fi ;;
  gitlab)
    if [ -z "${SCRUM_MASTER_RELEASE_URL:-}" ]; then
      echo "install.sh: name the GitLab project with SCRUM_MASTER_RELEASE_URL" >&2; exit 1
    fi
    if [ "$tag" = latest ]; then url=$base/-/releases/permalink/latest/downloads; else url=$base/-/releases/$tag/downloads; fi ;;
  *) echo "install.sh: no forge $forge; use github or gitlab" >&2; exit 1 ;;
esac

fetch() {
  if [ "$forge" = gitlab ] && [ -n "${GITLAB_TOKEN:-}" ]; then
    curl -fsSL --header "PRIVATE-TOKEN: $GITLAB_TOKEN" -o "$1" "$2"
  else
    curl -fsSL -o "$1" "$2"
  fi
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fetch "$work/SHA256SUMS" "$url/SHA256SUMS"
for asset in "$agent" "$profiles"; do
  fetch "$work/$asset" "$url/$asset"
  want=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "$work/SHA256SUMS")
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$work/$asset" | awk '{ print $1 }')
  else
    got=$(shasum -a 256 "$work/$asset" | awk '{ print $1 }')
  fi
  if [ -z "$want" ] || [ "$got" != "$want" ]; then
    echo "install.sh: $asset has checksum $got, but SHA256SUMS lists ${want:-none}; refusing it" >&2
    exit 1
  fi
done

mkdir -p "$prefix"
chmod 0755 "$work/$agent"
mv "$work/$agent" "$prefix/agent"
rm -rf "$home.new"
mkdir -p "$home.new"
tar -xzf "$work/$profiles" -C "$home.new"
rm -rf "$home"
mv "$home.new" "$home"
echo "installed agent $("$prefix/agent" --version 2>/dev/null | awk '{ print $NF }') at $prefix/agent"
echo "installed the scrum-master profiles $(cat "$home/release") at $home"
case ":$PATH:" in
  *":$prefix:"*) ;;
  *) echo "add $prefix to PATH" ;;
esac
echo "next, in a system repository: agent --profile \"$home/agents/init/profile.yaml\" --core-root \"$home/agent-core\" --directory ."
