---
id: rel04.3-uc001-install-without-go
title: A Developer with No Go Toolchain Installs tpm and Readies the Pinned Runtime
status: implemented
priority: P1
test_suite: test-rel04.3-distribution
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: summary -->
## Summary

A developer joins a program on a machine with no Go toolchain. They
install tpm from a release, with install.sh or by downloading the binary
for their machine and checking it against the release's SHA256SUMS
(eng03). They run tpm install --runtime, which fetches the
declarative-agents release tpm pins, verifies it, and reports where the
agent and its source are. tpm doctor in their program repository then
names the runtime it found, and repairs the corpus with the spec tool
when that tool was built, or says why it cannot.

<!-- field: actor -->
## Actor

A Developer, or a Product Owner, working in a program repository on their own machine.

<!-- field: trigger -->
## Trigger

The developer needs tpm, and the runtime its profiles run on, on a machine that has neither.

<!-- field: preconditions -->
## Preconditions

- A scrum-master release whose binaries and SHA256SUMS are on its GitHub release (eng03).
- Network access to github.com for the downloads; no Go toolchain is required.
- A program repository laid down by tpm init or tpm onboard, for the doctor step.

<!-- field: flow -->
## Flow

- F1: The developer installs tpm from the release, with install.sh or a checked manual download, and tpm --version names the release.
- F2: tpm install --runtime fetches the pinned release's agent for the host and the release's source into the release's cache directory, holding each binary to SHA256SUMS.
- F3: Without Go, tpm install --runtime reports the spec tool as not built and why; with Go, it builds it there.
- F4: tpm install --runtime reports the release, the agent's path, the source root, and the spec tool; a second run downloads nothing.
- F5: tpm doctor in the program repository reports the runtime it found, and repairs the corpus with a spec tool built at the repository's pin, or skips the repair and says why.

<!-- field: touchpoints -->
## Touchpoints

| id | target | reason |
| --- | --- | --- |
| T1 | [srd037-install](../software-requirements/srd037-install.md) [R7.1](../software-requirements/srd037-install.md#r71), [R7.2](../software-requirements/srd037-install.md#r72), [R7.3](../software-requirements/srd037-install.md#r73), [R7.5](../software-requirements/srd037-install.md#r75) | S1 traces the runtime fetch, its verification, and its report. |
| T2 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.5](../software-requirements/srd043-init-onboard.md#r45), [R4.7](../software-requirements/srd043-init-onboard.md#r47) | S2 traces doctor's runtime report and repair without Go. |
| T3 | [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7) | S1 traces this acceptance criterion. |
| T4 | [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8) | S2 traces this acceptance criterion. |

<!-- field: success_criteria -->
## Success criteria

<!-- sc: S1 -->
### S1

tpm install --runtime leaves the pinned release's agent for the host, verified against its SHA256SUMS, and the release's source in the release's cache directory, reports their paths and the spec tool's state, and downloads nothing on a second run.

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [R7.1](../software-requirements/srd037-install.md#r71)
- [srd037-install](../software-requirements/srd037-install.md) [R7.2](../software-requirements/srd037-install.md#r72)
- [srd037-install](../software-requirements/srd037-install.md) [R7.3](../software-requirements/srd037-install.md#r73)
- [srd037-install](../software-requirements/srd037-install.md) [R7.5](../software-requirements/srd037-install.md#r75)
- [srd037-install](../software-requirements/srd037-install.md) [AC7](../software-requirements/srd037-install.md#ac7)

<!-- sc: S2 -->
### S2

tpm doctor names the runtime it found with its path, or names tpm install --runtime as the fix when there is none, and repairs the corpus without Go when a spec tool is built at the pin.

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.5](../software-requirements/srd043-init-onboard.md#r45)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [R4.7](../software-requirements/srd043-init-onboard.md#r47)
- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC8](../software-requirements/srd043-init-onboard.md#ac8)

<!-- field: out_of_scope -->
## Out of scope

- Publishing the binaries; that is the release of tpm (eng03).
- Model credentials; the profiles that need them name them (GH-135).
- Running a profile; tpm ships none until the skills become declarative-agent workflows (GH-135).

<!-- field: assumptions -->
## Assumptions

- A1: Every step is a tpm subcommand or a download and spends no model tokens (design.md D10).
- A2: The declarative-agents release publishes agent binaries for darwin and linux on amd64 and arm64; on any other platform tpm install --runtime refuses and names those.
