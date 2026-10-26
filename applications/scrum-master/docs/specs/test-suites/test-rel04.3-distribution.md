---
id: test-rel04.3-distribution
title: Test Suite for Release 04.3 tpm Ships to Machines and to CI
release: "04.3"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The Go tests run offline: the release tests against a fixture tree and a
fake toolchain, and the install-script test against a local release
server serving a release built from a clone of this repository — the
agent and the profiles archive under SHA256SUMS. The integration targets
build the release from this repository; mage integration:image also needs
docker. Together they check that a release ships the agent, which serves
the scrum-master words, and the profiles, that install.sh installs both
held to SHA256SUMS and the init profile runs from them with only the
agent installed, that doctor reports the vendored release and repairs
without Go, and that the image carries the agent and the profiles (GH-34;
the runtime this suite once fetched into a cache retired with it).

<!-- field: traces -->
## Traces

- [rel04.3-uc001-install-without-go](../use-cases/rel04.3-uc001-install-without-go.md)
- [rel04.3-uc002-ci-job-on-the-image](../use-cases/rel04.3-uc002-ci-job-on-the-image.md)

<!-- field: preconditions -->
## Preconditions

- This repository, with agent-core beside the application, which the release builds the agent from.
- For the Go tests: a temporary directory and a local release server; no network.
- For the integration targets: docker with buildx for mage integration:image.

<!-- field: test_cases -->
## Test cases

<!-- case: the release ships the agent and the profiles | status: implemented -->
### the release ships the agent and the profiles

<!-- field: use_case -->
#### Use case

[rel04.3-uc001-install-without-go](../use-cases/rel04.3-uc001-install-without-go.md)

<!-- field: integration_target -->
#### Integration target

mage integration:image

<!-- field: go_test -->
#### Go test

go test ./internal/release -run '^TestBuildWritesEveryPlatformAndItsSums$|^TestProfilesArchiveIsTheTreesAndTheRelease$|^TestPublishCompletesAPartialRelease$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- assets: the agent for darwin and linux on amd64 and arm64, and the profiles archive, under one SHA256SUMS
- archive: agents/, agent-core/tools/, and release, the tag, written the same at every build
- stamped: the agent reports the tag, and the words it serves stamp it into what they install

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7)

<!-- case: install.sh installs the agent and the profiles, held to the checksums | status: implemented -->
### install.sh installs the agent and the profiles, held to the checksums

<!-- field: use_case -->
#### Use case

[rel04.3-uc001-install-without-go](../use-cases/rel04.3-uc001-install-without-go.md)

<!-- field: integration_target -->
#### Integration target

mage integration:initProfile

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestInstallScriptInstallsAVerifiedRelease$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- installed: the agent at the tag in AGENT_PREFIX and the profiles at the tag in SCRUM_MASTER_HOME, with no Go on PATH
- init: the init profile runs from the installed profiles with only the agent, and vendors the release
- tampered_download: refused naming the asset and both checksums; nothing written
- next: install.sh names the init profile's command

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7)

<!-- case: doctor reports the vendored release and repairs without Go | status: implemented -->
### doctor reports the vendored release and repairs without Go

<!-- field: use_case -->
#### Use case

[rel04.3-uc001-install-without-go](../use-cases/rel04.3-uc001-install-without-go.md)

<!-- field: integration_target -->
#### Integration target

mage integration:doctor

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^TestDoctorReportsTheVendoredRelease$|^TestRepairPrebuilt'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- vendored: the release .scrum-master holds, from its manifest
- none: a repository that vendored none told to run the init or onboard profile
- prebuilt: with no Go on PATH and a spec tool built at the pin, a document repaired to canonical form
- in_agent: in the agent the check runs in process, with no Go

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8)

<!-- case: the image carries the agent and the profiles | status: implemented -->
### the image carries the agent and the profiles

<!-- field: use_case -->
#### Use case

[rel04.3-uc002-ci-job-on-the-image](../use-cases/rel04.3-uc002-ci-job-on-the-image.md)

<!-- field: integration_target -->
#### Integration target

mage integration:image

<!-- field: go_test -->
#### Go test

go test ./internal/release -run '^TestImage|^TestPublishPushes|^TestPublishKeepsAPushed'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- agent: the image's agent is the release's for linux on the image's architecture
- profiles: the release's profiles under SCRUM_MASTER_HOME
- no_go: the image carries no Go toolchain and no mage, and runs as a non-root user
- init: the init profile runs over a mounted checkout, and a vendored profile validates against the vendored core root
- check: the vendored check profile, a CI job's run, passes over the checkout (GH-35)

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8)
