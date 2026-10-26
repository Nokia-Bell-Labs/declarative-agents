---
id: test-rel01.3-tag-triggered
title: Test Suite for Release 01.3 Tags Trigger Handoff and the Summary
release: "01.3"
status: retired
retired_in: "5.0"
replaced_by: ["test-rel05.0-tag-triggered"]
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Retired in 05.0: release 01.3 was renumbered 05.0 on 2026-10-22, and this document continues as test-rel05.0-tag-triggered. This file stays so that pinned references to it resolve.

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them.
The tag stages push a downstream tag and a system tag and prove the
reference-update issue and the summary's branch and review issue, each idempotent.

<!-- field: traces -->
## Traces

- [rel01.3-uc001-reference-update](../use-cases/rel01.3-uc001-reference-update.md)
- [rel01.3-uc002-release-summary](../use-cases/rel01.3-uc002-release-summary.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository, a reports repository, and the Product Owner repositories, with a period of fixture reports and Sprint Goals, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.
- The tag pipeline reaches the rig's tracker repository.

<!-- field: test_cases -->
## Test cases

<!-- case: A downstream tag files one reference-update issue upstream | status: specified -->
### A downstream tag files one reference-update issue upstream

<!-- field: use_case -->
#### Use case

[rel01.3-uc001-reference-update](../use-cases/rel01.3-uc001-reference-update.md)

<!-- field: integration_target -->
#### Integration target

mage integration:referenceUpdate

<!-- field: go_test -->
#### Go test

go test ./internal/handoff -run '^TestDownstreamTagFilesReferenceUpdateOnce$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- upstream_issues: one per upstream repository listing or closed by the release
- body: old and new tags, files and fields to change, handoff marker
- second_run: nothing filed
- references_edited_by_script: none

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- case: A system tag opens one summary for review | status: specified -->
### A system tag opens one summary for review

<!-- field: use_case -->
#### Use case

[rel01.3-uc002-release-summary](../use-cases/rel01.3-uc002-release-summary.md)

<!-- field: integration_target -->
#### Integration target

mage integration:releaseSummary

<!-- field: go_test -->
#### Go test

go test ./internal/reports -run '^TestSystemTagOpensOneSummaryMergeRequest$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- range: reports since the previous system release tag
- merge_request: one, with the computed aggregate and deck, linked to the stakeholder update issue
- model_tokens_in_ci: 0
- deck_slides: within the configured limit
- second_run: no second branch or review issue

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC5](../software-requirements/srd018-reports.md#ac5)
