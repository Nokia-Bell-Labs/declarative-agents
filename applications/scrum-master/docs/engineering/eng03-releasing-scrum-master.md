---
id: eng03-releasing-scrum-master
title: "Releasing scrum-master: Gates, Then a Date Tag"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: introduction -->
## Introduction

A scrum-master tag is what scrum-master is installed from: a developer runs
install.sh, which installs the tag's agent and unpacks its profiles, and
a CI job runs install.sh too, or an image a program published from the
tag to its own registry. The words stamp that version
into every skill they install and every profile they vendor (srd037). A
tag that nothing checked would put an unverified agent and its profiles
into every repository that installs them.
This guideline states how we cut a scrum-master release: the gates file,
the sequence mage tag runs, the binaries it publishes and the image it
publishes when asked, and
why the release depends on nothing outside this repository. A program repository's release is a
different process (srd015, scrum-master release) and shares only the tagging code.

<!-- section: the-gates-file -->
## The gates file

release-gates.md at the repository root declares what a release must
pass. It is a config document (design D11): push is a field, ask or auto,
and stages holds one stage record per stage, in order, whose gates field
is a table of name, command, and an optional directory relative to the
root.

| Stage | Gates |
|---|---|
| audit | mage audit: the repository gate tests, agent-core's gates, then the specification-critic corpus audit |
| test | go test ./..., mage integration:install, mage crossBuild, which builds the agent for every release platform and the profiles archive, and mage outOfTree, which runs the audit, the unit tests, and the cross build on a copy of the application outside the repository, its Go module renamed to a placeholder |

Table: scrum-master's release gates.

Every gate builds against one runtime, which internal/runtime's Resolve
finds for the application (GH-2754): the agent-core beside it inside the
repository that ships agent-core; else the parent checkout
docs/external-checkouts.yaml names, a file git ignores; else the
declarative-agents release docs/external-pins.yaml pins, its source
fetched once per tag from the forge the pins file's sources entry names,
or the default one: a sources entry that is a git remote, an ssh or
scp-form address, a URL ending in .git, or a local path, is cloned at the
tag with the person's or CI's own git credentials, so a runtime pinned to
any forge, a private GitLab project among them, needs no archive URL, no
token of scrum-master's, and no package registry (GH-2761). mage outOfTree names this repository's own checkout
in its copy, so a gate, a test, or a build step that depends on the
repository's layout or on the application's module path fails the
release rather than a later move of the code.

mage regression:all is not a gate. It needs the GitLab trial group and a
token, takes minutes, and depends on gitlab.com's availability, so a
release would fail for reasons unrelated to the commit. We run it
before merging changes that touch the tracker, not at release time.
Push is ask: the person cutting the release sees the tag and its
annotation before it leaves the machine.

<!-- section: the-sequence -->
## The sequence

mage tag runs these steps, and mage tagDryRun runs the first four and
creates nothing.

| Step | What happens |
|---|---|
| Lock | A directory under the git dir (scrum-master-release.lock) holds the release, so two releases of one clone cannot overlap |
| Preflight | The release runs on main with a clean worktree; a HEAD already carrying a v0.* tag has nothing to release; gh answers as the organization account, and docker buildx is on PATH when SCRUM_MASTER_RELEASE_IMAGE names an image to publish, so a tag never lands without the means to publish it |
| Gates | Stages run in order and the gates of a stage at the same time, each line of output prefixed with the gate's name; the first stage with a failed gate stops the release and names every failed gate |
| Recheck | HEAD and the worktree must be unchanged after the gates; a gate that commits or edits files voids the run |
| Tag | v0.YYYYMMDD.N, N one past the highest revision for the day across local tags and origin's; the annotation lists the gates that passed and git log --oneline since the previous release |
| Push | push auto pushes the tag to origin; push ask asks y/N and otherwise prints the push command |
| Publish | Once the tag is on origin, mage publish runs: the release assets, then, when SCRUM_MASTER_RELEASE_IMAGE names a repository, the container image; a tag left unpushed prints mage publish as the step to run after pushing it |

Table: What mage tag does.

mage publish publishes the tag at HEAD and nothing else. Each part it
finds already published (an asset on the GitHub release, the image at
the tag in the registry) is checked and kept, so rerunning it after a
failed upload or push completes the release rather than repeating it.

The /gh-release-push skill calls mage tag on main. A failed gate is
never fixed inside the release: we file an issue, merge the fix, and
release the green main.

<!-- section: release-binaries -->
## Release binaries

mage publishDryRun builds the assets and names what mage publish would
upload and push, creating nothing. mage integration:image builds the
image for this machine's architecture and checks it without pushing. The
first part of mage publish builds the agent at the tag with
CGO_ENABLED=0 for linux and darwin on amd64 and arm64, composed with the
scrum-master words (GH-27) and stamped with the tag, writes the profiles
archive, scrum-master-profiles.tar.gz — the profiles, agent-core's tools
tree, and the release — writes SHA256SUMS over them, creates the tag's
GitHub release, and uploads the assets to it (GH-34). The agents are named
agent_<os>_<arch>, the convention the declarative-agents releases follow,
so one naming rule and one checksum check serve both. install.sh
downloads the agent for the machine and the profiles archive from a named
release, or the latest, verifies both against SHA256SUMS, installs the
agent, and unpacks the profiles; the README gives that, and the init
profile a program starts from.

The release is created with the gh account magefiles/tag.go names
(releaseAccount, overridden by SCRUM_MASTER_RELEASE_ACCOUNT), the
account that publishes this repository's releases.

<!-- section: the-container-image -->
## The container image

The image is opt-in (GH-2721). This repository's releases publish none:
the project pushes no container to the organization's registry, and a CI
job installs the agent with install.sh. A program that wants a ready image
publishes one to a registry it controls, from a checkout at the tag, by
setting SCRUM_MASTER_RELEASE_IMAGE to the image's repository and running
mage publish, which keeps the assets already on the release and adds the
image. With SCRUM_MASTER_RELEASE_IMAGE unset, mage publish says no image is
published and pushes nothing, and the preflight does not ask for docker
buildx.

With it set, the second part builds one image for linux/amd64 and
linux/arm64 with docker buildx from the Dockerfile at the repository root
and pushes it as <SCRUM_MASTER_RELEASE_IMAGE>:<tag>, and moves latest to it. Its
build context is the directory the first part built the assets into, and
it copies agent_linux_<arch> and unpacks the profiles archive from there,
so the image's agent and profiles are the ones on the GitHub release;
Dockerfile.dockerignore admits nothing else. The image holds the agent,
the profiles under SCRUM_MASTER_HOME, git, git-crypt, and CA
certificates. The agent runs the words and the corpus gate's shape check
in its own process, so the image carries no Go toolchain and no mage; it
runs as a non-root user and works in the directory a CI job mounts its
checkout into, where the job runs the repository's vendored check
profile (GH-35).

A push to ghcr.io logs in as the release account with its gh token, which
needs the write:packages scope and a registry namespace that lets the
account create the package. buildx attaches an SBOM
attestation to the image index, and its provenance is recorded at
mode=min so it names no machine or person. The index carries the source,
description, and license annotations, which link the package to this
repository on ghcr; the Dockerfile's labels repeat them on each
platform's image. The image is published after
the tag is pushed, so its digest cannot be in the tag annotation; mage
publish adds it to the GitHub release's notes, which pipelines pin from;
latest is for people. An image already at the tag is kept.

The image is not signed. Pinning by digest already holds a job to the
image that was published, and the digest's trust is the GitHub release,
the same trust the release binaries and their SHA256SUMS rest on. A
signature adds only a key held apart from the GitHub account, which one
maintainer holding both does not give, and no consumer verifies one yet.
Signing from a person's machine would also need a key pair to keep
secret, since keyless signing there records that person's identity in a
public transparency log. Signing returns when a consumer requires it,
keyless from a CI identity once releases publish from a pipeline.

A CI job reads tracker and model credentials from masked CI variables
(SCRUM_MASTER_TRACKER, SCRUM_MASTER_USER, and what each profile names), and its writes are
attributed to the service user SCRUM_MASTER_USER names, never to a person; srd041's
approval rule still holds, so CI never approves its own merge. The README
shows a GitLab CI job that installs the agent with install.sh at a pinned
tag, and the image a program may publish to its own registry instead.

<!-- section: releasing-from-a-gitlab-fork -->
## Releasing from a GitLab fork

A fork of this repository on GitLab, such as an internal one, releases the
same assets from its own project, and the image when it names one, so a program inside a network
that cannot reach GitHub installs from it and no package is published
from the upstream public repository (GH-31). mage tag and mage publish
read four environment variables in place of this repository's GitHub
values:

| Variable | Names |
|---|---|
| SCRUM_MASTER_RELEASE_FORGE | gitlab, where the release is published through glab |
| SCRUM_MASTER_RELEASE_REPO | the fork's project, group/name |
| SCRUM_MASTER_RELEASE_ACCOUNT | the account releases are made as, which glab must answer as |
| SCRUM_MASTER_RELEASE_IMAGE | the image in the fork's container registry, when the fork publishes one |

Table: The settings a GitLab fork releases with.

glab reads GITLAB_TOKEN, the organization's token, and docker logs in to
the fork's registry with it. A GitLab release link records no digest, so
a rerun holds each asset already on the release to the release's own
SHA256SUMS, and refuses one it cannot verify. A program installs from the
fork with SCRUM_MASTER_RELEASE_FORGE=gitlab and SCRUM_MASTER_RELEASE_URL
naming the project; install.sh sends GITLAB_TOKEN to read a private one.

<!-- section: versions -->
## Versions

The agent and the words take their version from the tag: agent-core and
the application are nested modules, so their build info never carries
the repository's tag, and mage publish stamps it into both with -ldflags
(GH-18). The words stamp that version into every skill they install and
the profiles archive's release file names it. Nothing passes the version
in by hand; a local build reports dev.

Beside its own version, a release stamps the runtime release it was built
against, internal/version's Runtime (GH-2755): its own tag when the agent
builds from the agent-core beside the words, else the release
docs/external-pins.yaml pins. A new program corpus pins that runtime
release, so scrum-master's version and the runtime's can move apart.

The repositories the code names are each set once, in internal/identity:
the runtime it builds on, and its own home, where releases, install.sh,
and issues live (GH-2755). install.sh's default release page, plugin.json's
repository, the Dockerfile's source label, and the README's install
command are held to it by a test, which also fails on either repository
name or the module path written as a string anywhere else in Go; the
tutorials link the README's Install section rather than repeat it.

<!-- section: no-dependency-outside-the-repository -->
## No dependency outside the repository

The declarative-agents ecosystem has a shared release runner and a
release-manager agent, but only in a private repository that the
environment scrum-master deploys to cannot reach. scrum-master therefore
implements the same contract in internal/release, in plain Go, with
no agent: every step is a rule, and design.md D10 puts rules in code.
The audit agent still comes from the public declarative-agents
release the corpus pins (docs/guides/corpus-audit.md).

internal/release holds the gates file, the stage runner, and the
tagging (NextDateTag, RemoteTags, Release.Tag), each behind a git
function so tests run against real temporary repositories with a bare
origin. magefiles/tag.go only wires the repository root to it, and
scrum-master release (srd015) reuses the tagging for program repositories.

<!-- field: references -->
## References

- release-gates.md
- Dockerfile
- install.sh
- internal/release
- docs/specs/software-requirements/srd015-release.md (a program repository's release)
- docs/specs/software-requirements/srd037-install.md (the version the words stamp)
- docs/constitutions/design.md D10
