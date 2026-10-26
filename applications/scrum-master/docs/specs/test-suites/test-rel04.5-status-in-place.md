---
id: test-rel04.5-status-in-place
title: Test Suite for Release 04.5 Status Reports in Place
release: "04.5"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The Go tests run against the in-memory tracker and a beads tracker over
real git repositories on local disk, and against GitHub's and GitLab's
APIs in memory; the rig lays a system repository down with the init
profile beside a team's and runs the words and the reports-collect
profile over them. Together they check that reports are filed, written,
and collected where the work happened, that rollups cite their sources,
and that the site, plan, and doctor read and move what they should.

<!-- field: traces -->
## Traces

- [rel04.5-uc001-report-in-place](../use-cases/rel04.5-uc001-report-in-place.md)
- [rel04.5-uc002-roll-up-in-place](../use-cases/rel04.5-uc002-roll-up-in-place.md)

<!-- field: preconditions -->
## Preconditions

- git; the repository's agent, built from agent-core.

<!-- field: test_cases -->
## Test cases

<!-- case: a status report is filed, written, and read back in place | status: implemented -->
### a status report is filed, written, and read back in place

<!-- field: use_case -->
#### Use case

[rel04.5-uc001-report-in-place](../use-cases/rel04.5-uc001-report-in-place.md)

<!-- field: integration_target -->
#### Integration target

mage integration:statusInPlace

<!-- field: go_test -->
#### Go test

go test ./internal/reports -run '^TestStatusIssuesInPlaceFileReportsAndRollups$|^TestPreviousAndPath$|^TestControlRoster$|^TestInitWithoutARosterLaysDownTheSiteOnly$'
go test ./internal/install -run '^TestSkillsRenderFromTheirProfiles$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- filed: one status issue per person in the repository they report in, one team rollup issue per subsystem for its owner, and the program rollup issue, each due after the last; nothing on a second run
- report: the report merged at status/reports in the team's repository, read back by previous with its Next, Blockers, and commit
- skill: status-report renders from its profile, every word it uses has a model-free profile

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [AC10](../software-requirements/srd018-reports.md#ac10)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC12](../software-requirements/srd018-reports.md#ac12)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC15](../software-requirements/srd018-reports.md#ac15)

<!-- case: a rollup collects in place, cites its sources, and reaches its readers | status: implemented -->
### a rollup collects in place, cites its sources, and reaches its readers

<!-- field: use_case -->
#### Use case

[rel04.5-uc002-roll-up-in-place](../use-cases/rel04.5-uc002-roll-up-in-place.md)

<!-- field: integration_target -->
#### Integration target

mage integration:statusInPlace

<!-- field: go_test -->
#### Go test

go test ./internal/reports -run '^TestCollectReadsReportsWhereTheyWereWritten$|^TestAggregateReadsACollectionAcrossRepositories$|^TestRollupFormat$|^TestRollupFormatExampleValidates$'
go test ./internal/github -run '^TestHostFilesReadTheDefaultBranchWithNoCheckout$'
go test ./internal/gitlab -run '^TestHostFilesReadTheDefaultBranchWithNoCheckout$'
go test ./internal/site -run '^TestStatusShowsTheLatestProgramRollup$'
go test ./internal/reports -run '^TestLatestProgramRollupReadsAsAnAggregate$'
go test ./internal/scaffold -run '^TestDoctorMovesTheRoster$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- collect: a team's reports and the program's reports and team rollups, each with repository and commit, read from the host with no checkout or from a checkout, writing nothing
- aggregate: a blocker shared across repositories, a missing report naming its open status issue, and unreported merged work, comparing no people
- rollup: a rollup citing listed sources validates; one citing an unlisted source or none fails
- readers: the site's status pages show the latest program rollup
- move: doctor moves a central roster into control/reports.md once, naming the central repository

<!-- field: traces -->
#### Traces

- [srd018-reports](../software-requirements/srd018-reports.md) [AC11](../software-requirements/srd018-reports.md#ac11)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC13](../software-requirements/srd018-reports.md#ac13)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC14](../software-requirements/srd018-reports.md#ac14)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC16](../software-requirements/srd018-reports.md#ac16)
- [srd018-reports](../software-requirements/srd018-reports.md) [AC17](../software-requirements/srd018-reports.md#ac17)
