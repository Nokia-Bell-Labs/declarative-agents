---
id: policy
title: Process policy
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Holds every rule of the control plane that a program may tune: admission, plan scoring, escalation thresholds, release classification, extra event names, and the gate for each artifact, which binds the artifact to the tests and environments whose evidence decides done (srd008). Scripts read rules from this file at run time and hold none in code (srd001 R2.4). A change to it is a plane::control issue and a reviewed merge.

<!-- field: schema -->
## Schema

<!-- schema_entry: version -->
### version

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

Schema version of this file; 1.

<!-- schema_entry: admission -->
### admission

<!-- field: type -->
#### Type

mapping

<!-- field: description -->
#### Description

require_milestone (boolean, data issues must carry a sprint milestone), require_acceptance_criteria (boolean, build, experiment, research, and spec issues must have an Acceptance Criteria section), management_filers (list of usernames or the word leads, who may file plane::management issues; scrum-master issue file refuses anyone else), require_approval (boolean, false by default: scrum-master issue merge refuses a branch without another person's approval of its tip; off, it merges without one and records an approval when one was given), audit_command (the corpus gate scrum-master issue merge runs in the worktree before merging a docs/ change; mage audit by default), control_reviewers (count of lead approvals a plane::control merge needs), rules (bullet list naming the admission rules that apply, of one-plane, one-type, one-component, one-op, kind-on-data, data-milestone, work-sections, and management-filers; absent, every rule applies, so removing a name disables that rule, and a name scrum-master does not implement is reported on every lint; GH-130). Two checks are not rules and always apply, because admission cannot express them declaratively: a Cites line must be pinned, and the link lines keep the srd007 discipline.

<!-- schema_entry: scoring -->
### scoring

<!-- field: type -->
#### Type

mapping of term to points

<!-- field: description -->
#### Description

The terms scrum-master standup score (srd010 R6) adds up to order a person's own issues. Keys are deadline (a list of [days, points] pairs, overdue counts as 0 days), progress_gap (a list of [percent, points] pairs), type (points per kind of commitment), blocked (negative points), waiting (negative points), downstream_release_ending (points when the road-map release the issue counts toward ends within days), and consume (points for kind::consume).

<!-- schema_entry: escalation -->
### escalation

<!-- field: type -->
#### Type

mapping

<!-- field: description -->
#### Description

blocker_age_days, waiting_chain_length, slip_releases, and evidence_age_days (latest evidence for a planned requirement older than this): thresholds past which track-goals and the dashboard report a signal to the owner.

<!-- schema_entry: releases -->
### releases

<!-- field: type -->
#### Type

mapping

<!-- field: description -->
#### Description

scheme (semver), breaking (major, or major-and-minor-before-1.0), tag_format (a pattern with {artifact} and {version}).

<!-- schema_entry: architecture_checks -->
### architecture_checks

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

How the corpus gate treats an ARCHITECTURE component added or changed since the latest tag that no agreed hierarchy document cites (srd045 R6.3): warn, the default, reports it without failing the gate; refuse makes it a finding. A policy without the field warns.

<!-- schema_entry: events -->
### events

<!-- field: type -->
#### Type

list of event name

<!-- field: description -->
#### Description

Event names the program adds to the vocabulary of srd006, each with its required keys. Empty when the program adds none.

<!-- schema_entry: definition_of_done -->
### definition_of_done

<!-- field: type -->
#### Type

mapping of artifact to gate

<!-- field: description -->
#### Description

For each artifact a subsystem SRD declares, the checks srd008 runs before a release: a list of check entries, each with kind (tests, contract-tests, scorecard, peer-read, custom), environment (the name of an environment SRD), and its arguments (suite for tests; command, the command a local tests or custom check runs from the repository root). An artifact with no entry uses definition_of_done.default.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: policy
title: Process policy
---

<!-- field: config_files -->
## Config files

control/policy.md in the program repository

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

<!-- field: management_filers -->
### Management filers

leads

<!-- field: audit_command -->
### Audit command

mage audit

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

<!-- field: control_reviewers -->
### Control reviewers

1

<!-- field: scoring -->
## Scoring

<!-- field: deadline -->
### Deadline

| days | points |
| --- | --- |
| 7 | 40 |
| 14 | 35 |
| 30 | 30 |
| 60 | 20 |
| 90 | 10 |

<!-- field: progress_gap -->
### Progress gap

| percent | points |
| --- | --- |
| 40 | 25 |
| 20 | 20 |
| 10 | 15 |
| 0 | 10 |

<!-- field: type -->
### Type

<!-- field: sprint_goal -->
#### Sprint goal

15

<!-- field: stretch -->
#### Stretch

10

<!-- field: adhoc -->
#### Adhoc

5

<!-- field: blocked -->
### Blocked

-20

<!-- field: waiting -->
### Waiting

-20

<!-- field: downstream_release_ending -->
### Downstream release ending

<!-- field: days -->
#### Days

14

<!-- field: points -->
#### Points

15

<!-- field: consume -->
### Consume

10

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

<!-- field: events -->
## Events

None

<!-- field: definition_of_done -->
## Definition of done

<!-- field: default -->
### Default

| kind | environment | suite | command |
| --- | --- | --- | --- |
| tests | local | unit | go test ./... |

<!-- field: corpus -->
### Corpus

| kind | environment | command |
| --- | --- | --- |
| custom | local | mage audit |

<!-- field: shared-schema -->
### Shared-schema

| kind | environment | suite | consumers |
| --- | --- | --- | --- |
| tests | local | unit |  |
| tests | dev | subsystem-install |  |
| contract-tests | dev |  | all |

<!-- field: agent-profile -->
### Agent-profile

| kind | environment | against | threshold |
| --- | --- | --- | --- |
| scorecard | dev | current | no-regression |
| peer-read |  |  |  |
```
