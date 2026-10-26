---
id: rel04.3-uc002-ci-job-on-the-image
title: A CI Job Runs tpm and the Pinned Runtime on the Release's Image
status: implemented
priority: P2
test_suite: test-rel04.3-distribution
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A program runs tpm in GitLab CI: a release gate, a scheduled run, or an
agent workflow. The job names the release's image by digest,
ghcr.io/nokia-bell-labs/declarative-agents/scrum-master:<tag>, and runs with no install step and no Go
toolchain. tpm in the image finds the runtime at the release it pins in
the cache the image carries, tpm doctor reports it, and repairs a corpus
at tpm's pin with the spec tool built into the image.

<!-- field: actor -->
## Actor

The technical program manager or a Product Owner, who writes the CI job; the job runs as the service user SCRUM_MASTER_USER names.

<!-- field: trigger -->
## Trigger

A pipeline in a program repository starts a job on the image.

<!-- field: preconditions -->
## Preconditions

- A scrum-master release whose image is published (eng03), pulled from ghcr or from the program's registry mirror.
- Tracker credentials, and any model credentials a profile names, in masked CI variables.
- A program repository at the parent release the image's tpm pins, for the repair step.

<!-- field: flow -->
## Flow

- F1: The runner pulls the image by the digest the release's notes list and starts the job on it.
- F2: The job runs tpm backlog check over its checkout.
- F3: tpm doctor --dry-run reports the runtime it found in the image's cache, checked against the release's checksums, and the repair it would make with the image's spec tool, with no Go toolchain.
- F4: The job runs agent --validate-config on a profile against the image's runtime and source root.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd037-install](../software-requirements/srd037-install.md) [R7.1](../software-requirements/srd037-install.md#r71), [R7.3](../software-requirements/srd037-install.md#r73) | S1 traces the runtime and spec tool the image carries in the shared cache. |
| T2 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.5](../software-requirements/srd043-init-onboard.md#r45), [R4.7](../software-requirements/srd043-init-onboard.md#r47) | S2 traces doctor's report and repair in the image. |
| T3 | [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7) | S1 traces this acceptance criterion. |
| T4 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

The image's tpm reports the release, and the runtime and spec tool it carries are the release tpm pins, found in the cache directory tpm install --runtime fills.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R7.1](../software-requirements/srd037-install.md#r71)
- [srd037-install](../software-requirements/srd037-install.md) [R7.3](../software-requirements/srd037-install.md#r73)
- [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7)

<!-- sc: S2 -->
### S2

On the image, with no Go toolchain, tpm doctor reports the runtime with its path and repairs a corpus at tpm's pin.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.5](../software-requirements/srd043-init-onboard.md#r45)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.7](../software-requirements/srd043-init-onboard.md#r47)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8)

<!-- field: out_of_scope -->
## Out of scope

- Building and publishing the image; that is the release of tpm (eng03).
- The profiles a job runs; tpm ships them once the skills become declarative-agent workflows (GH-135).
- Repairing a corpus pinned to another parent release on the image; that needs Go, which the image does not carry.

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a tpm subcommand, the runtime's own validation, or the runner's, and spends no model tokens (design.md D10).
- A2: Writes a job makes are attributed to the service user SCRUM_MASTER_USER names, and srd041's approval rule holds, so a job never approves its own merge.
