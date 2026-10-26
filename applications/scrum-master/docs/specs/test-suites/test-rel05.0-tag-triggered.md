---
id: test-rel05.0-tag-triggered
title: Test Suite for Release 05.0 Tags Trigger Handoff and the Summary
release: "05.0"
status: specified
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them.
The tag stages push a downstream tag and a system tag through the release
profile and prove the reference-update issue and the deck rendered beside
the approved summary, each idempotent.

<!-- field: traces -->
## Traces

- [rel05.0-uc001-reference-update](../use-cases/rel05.0-uc001-reference-update.md)
- [rel05.0-uc002-release-summary](../use-cases/rel05.0-uc002-release-summary.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository, a reports repository, and the Product Owner repositories, with a period of fixture reports and Sprint Goals, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.
- The release profile and the handoff profile run with the rig's tracking systems: its bd tracker repository, and gh and glab fakes for the cross-system cases.

<!-- field: test_cases -->
## Test cases

<!-- case: A downstream tag files one reference-update issue upstream | status: specified -->
### A downstream tag files one reference-update issue upstream

<!-- field: use_case -->
#### Use case

[rel05.0-uc001-reference-update](../use-cases/rel05.0-uc001-reference-update.md)

<!-- field: integration_target -->
#### Integration target

mage integration:referenceUpdate

<!-- field: go_test -->
#### Go test

go test ./internal/handoff -run '^TestDownstreamTagFilesReferenceUpdateOnce$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- upstream_issues: one per upstream repository listing or closed by the release, filed by the release profile's handoff step
- body: the pinned old tag, the new tag, files and fields to change, handoff marker
- second_run: nothing filed, with the first issue open or closed
- open_issue_for_earlier_tag: takes the new tag and a comment; no sibling filed
- unreachable_tracker_or_missing_checkout: nothing filed, non-zero exit, the release run names the handoff profile, the tag stands
- tracking_systems: the above on bd, gh, and glab, fakes where the rig has no live system, and once with upstream and downstream on different systems
- references_edited_by_script: none

<!-- field: traces -->
#### Traces

- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC7](../software-requirements/srd014-handoff.md#ac7)
- [srd014-handoff](../software-requirements/srd014-handoff.md) [AC6](../software-requirements/srd014-handoff.md#ac6)
- [srd007-links](../software-requirements/srd007-links.md) [AC5](../software-requirements/srd007-links.md#ac5)

<!-- case: A system tag renders the deck from the approved summary | status: specified -->
### A system tag renders the deck from the approved summary

<!-- field: use_case -->
#### Use case

[rel05.0-uc002-release-summary](../use-cases/rel05.0-uc002-release-summary.md)

<!-- field: integration_target -->
#### Integration target

mage integration:releaseSummary

<!-- field: go_test -->
#### Go test

go test ./internal/reports -run '^TestSystemTagRendersDeckFromApprovedSummaryOnce$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- source: the merged sprint-end summary at docs/sprint-ends/RELEASE.md
- deck: committed beside the summary, one slide per outline section, within slide_limit
- over_limit: non-zero exit naming the sections over slide_limit, nothing truncated, the tag stands
- branches_or_review_issues: none
- model_tokens: 0
- second_run: nothing committed

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [AC8](../software-requirements/srd018-reports.md#ac8)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC4](../software-requirements/srd018-reports.md#ac4)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC5](../software-requirements/srd018-reports.md#ac5)
