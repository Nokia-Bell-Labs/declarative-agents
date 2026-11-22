---
id: policy
title: Policy
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: version -->
## Version

1

<!-- field: admission -->
## Admission

<!-- field: require_milestone -->
### Require milestone

true

<!-- field: require_acceptance_criteria -->
### Require acceptance criteria

true

<!-- field: require_approval -->
### Require approval

false

<!-- field: audit_command -->
### Audit command

scrum-master backlog check

<!-- field: rules -->
### Rules

- one-plane
- one-type
- one-component
- one-op
- kind-on-data
- data-milestone
- work-sections
- management-filers

<!-- field: escalation -->
## Escalation

<!-- field: blocker_age_days -->
### Blocker age days

14

<!-- field: waiting_chain_length -->
### Waiting chain length

3

<!-- field: slip_releases -->
### Slip releases

2

<!-- field: evidence_age_days -->
### Evidence age days

7

<!-- field: releases -->
## Releases

<!-- field: scheme -->
### Scheme

semver

<!-- field: breaking -->
### Breaking

major-and-minor-before-1.0

<!-- field: tag_format -->
### Tag format

{artifact}/v{version}

<!-- field: architecture_checks -->
## Architecture checks

warn

<!-- field: area_checks -->
## Area checks

warn

<!-- field: definition_of_done -->
## Definition of done

<!-- field: corpus -->
### Corpus

| kind | environment | suite | command |
| --- | --- | --- | --- |
| tests | local | hierarchy | scrum-master backlog check |

<!-- field: default -->
### Default

| kind | environment | suite | command |
| --- | --- | --- | --- |
| tests | local | hierarchy | scrum-master backlog check |
