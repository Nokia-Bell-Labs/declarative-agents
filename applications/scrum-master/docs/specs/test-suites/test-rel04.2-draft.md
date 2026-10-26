---
id: test-rel04.2-draft
title: Test Suite for Release 04.2 A Planning Document Drafted in Conversation
release: "04.2"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The suite runs offline. It inits a system repository, adds hierarchy
documents to it, and runs tpm backlog list and tpm backlog check on the
result; then it breaks one document and runs check again. It checks that
check and the gate in mage audit report the same findings on the same
documents, and that the draft skill is installed for every agent within
its line limit.

<!-- field: traces -->
## Traces

- [rel04.2-uc001-draft-a-planning-document](../use-cases/rel04.2-uc001-draft-a-planning-document.md)

<!-- field: preconditions -->
## Preconditions

- A tpm binary built from this repository, so the skills and help pages are embedded.
- A temporary git repository; no network and no tracker.

<!-- field: test_cases -->
## Test cases

<!-- case: backlog list names the hierarchy | status: implemented -->
### backlog list names the hierarchy

<!-- field: use_case -->
#### Use case

[rel04.2-uc001-draft-a-planning-document](../use-cases/rel04.2-uc001-draft-a-planning-document.md)

<!-- field: integration_target -->
#### Integration target

mage integration:backlog

<!-- field: go_test -->
#### Go test

go test ./internal/backlog -run '^TestList'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- list: every hierarchy document with id, kind, title, status, parent, and path; points and rank for stories
- kind_filter: only documents of the named kind
- read_only: no tracker opened, no file written

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC8](../software-requirements/srd045-backlog.md#ac8)

<!-- case: backlog check is the hierarchy gate | status: implemented -->
### backlog check is the hierarchy gate

<!-- field: use_case -->
#### Use case

[rel04.2-uc001-draft-a-planning-document](../use-cases/rel04.2-uc001-draft-a-planning-document.md)

<!-- field: integration_target -->
#### Integration target

mage integration:backlog

<!-- field: go_test -->
#### Go test

go test ./internal/backlog -run '^TestCheck'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- valid: a pass with the document count; exit 0
- malformed: the gate's findings, file, field, and message; exit non-zero
- same_as_gate: the findings equal hierarchy.Validate's on the same documents

<!-- field: traces -->
#### Traces

- [srd045-backlog](../software-requirements/srd045-backlog.md) [AC8](../software-requirements/srd045-backlog.md#ac8)

<!-- case: the draft skill is installed within its limit | status: implemented -->
### the draft skill is installed within its limit

<!-- field: use_case -->
#### Use case

[rel04.2-uc001-draft-a-planning-document](../use-cases/rel04.2-uc001-draft-a-planning-document.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestSkillFilesStayWithinLineLimit$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- installed: draft written for Cursor, Claude Code, and GitHub Copilot
- limit: SKILL.md at most 50 lines
- agents_file: AGENTS.md lists draft at the system and the subsystem level

<!-- field: traces -->
#### Traces

- [srd046-draft](../software-requirements/srd046-draft.md) [AC1](../software-requirements/srd046-draft.md#ac1)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC2](../software-requirements/srd046-draft.md#ac2)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC3](../software-requirements/srd046-draft.md#ac3)
- [srd046-draft](../software-requirements/srd046-draft.md) [AC4](../software-requirements/srd046-draft.md#ac4)
