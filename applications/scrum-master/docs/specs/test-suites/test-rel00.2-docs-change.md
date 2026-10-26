---
id: test-rel00.2-docs-change
title: Test Suite for Release 00.2 Changes Under docs/
release: "00.2"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

Each run starts with mage regression:up creating the repositories below in
a fresh rig run on local disk and ends with mage regression:down removing them. The
suite works a kind::spec design change and a plane::management planning
change through the same audit gate, and proves who may file each.

<!-- field: traces -->
## Traces

- [rel00.2-uc001-spec-change](../use-cases/rel00.2-uc001-spec-change.md)
- [rel00.2-uc002-planning-change](../use-cases/rel00.2-uc002-planning-change.md)

<!-- field: preconditions -->
## Preconditions

- mage regression:up builds a rig run under .regression/<run-id>, a beads tracker repository with a system repository and one Product Owner repository, and writes their paths as JSON for the integration targets (eng01).
- mage regression:down removes the run after the targets run, including when they fail.
- The system repository carries a docs/ corpus that passes mage audit, and a malformed fixture that fails it.

<!-- field: test_cases -->
## Test cases

<!-- case: A design change is a kind::spec issue anyone files | status: implemented -->
### A design change is a kind::spec issue anyone files

<!-- field: use_case -->
#### Use case

[rel00.2-uc001-spec-change](../use-cases/rel00.2-uc001-spec-change.md)

<!-- field: integration_target -->
#### Integration target

mage integration:docsChange

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestSpecIssueIsAdmittedForAnyMember$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- labels: plane::data and kind::spec for a request naming a component SRD
- filed_by: a non-lead, admitted into the backlog

<!-- field: traces -->
#### Traces

- [srd001-labels](../software-requirements/srd001-labels.md) [AC24](../software-requirements/srd001-labels.md#ac24)
- [srd035-specify](../software-requirements/srd035-specify.md) [AC1](../software-requirements/srd035-specify.md#ac1)

<!-- case: The audit gates the docs/ change | status: implemented -->
### The audit gates the docs/ change

<!-- field: use_case -->
#### Use case

[rel00.2-uc001-spec-change](../use-cases/rel00.2-uc001-spec-change.md)

<!-- field: integration_target -->
#### Integration target

mage integration:docsChange

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestMergeDeliversOnBeads$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- failing_audit: findings reported, nothing delivered
- passing_audit: the merge runs mage audit, merges, and closes the issue

<!-- field: traces -->
#### Traces

- [srd036-implement](../software-requirements/srd036-implement.md) [AC2](../software-requirements/srd036-implement.md#ac2)
- [srd013-program](../software-requirements/srd013-program.md) [AC5](../software-requirements/srd013-program.md#ac5)
- [srd034-initiatives](../software-requirements/srd034-initiatives.md) [AC2](../software-requirements/srd034-initiatives.md#ac2)

<!-- case: A planning change is lead-only and kept out of the backlog | status: implemented -->
### A planning change is lead-only and kept out of the backlog

<!-- field: use_case -->
#### Use case

[rel00.2-uc002-planning-change](../use-cases/rel00.2-uc002-planning-change.md)

<!-- field: integration_target -->
#### Integration target

mage integration:docsChange

<!-- field: go_test -->
#### Go test

go test ./internal/program -run '^TestManagementIssueRefusedForNonLead$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- non_lead: refused
- lead: plane::management issue, not in the backlog, popped and merged through the same gate

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)
- [srd001-labels](../software-requirements/srd001-labels.md) [AC1](../software-requirements/srd001-labels.md#ac1)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC1](../software-requirements/srd036-implement.md#ac1)
- [srd013-program](../software-requirements/srd013-program.md) [AC7](../software-requirements/srd013-program.md#ac7)
