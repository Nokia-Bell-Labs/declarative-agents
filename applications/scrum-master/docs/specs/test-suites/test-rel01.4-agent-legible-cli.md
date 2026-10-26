---
id: test-rel01.4-agent-legible-cli
title: Test Suite for Release 01.4 The Agent-Legible CLI
release: "01.4"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The suite runs offline. It inits an empty git repository with an empty
tracker repository beside it, runs the parent's corpus audit and the
Sprint Goal gate over the result, onboards a second repository, and runs
both commands again. It reads every help page, holds the pages to the
dispatch table and the corpus, runs every issue subcommand under its new
group and the old group name, and recurs a recurring issue whose earlier
instance carries the marker of an earlier tpm.

<!-- field: traces -->
## Traces

- [rel01.4-uc001-bootstrap-a-program](../use-cases/rel01.4-uc001-bootstrap-a-program.md)
- [rel01.4-uc002-agent-learns-the-cli](../use-cases/rel01.4-uc002-agent-learns-the-cli.md)

<!-- field: preconditions -->
## Preconditions

- A tpm binary built from this repository, so the templates, the skills, and the help pages are embedded.
- Temporary git repositories for the system repository, the tracker repository, and the joining repository; no network.
- The parent's clone cache, for the corpus audit over the new repository (docs/guides/corpus-audit.md).

<!-- field: test_cases -->
## Test cases

<!-- case: init writes a system repository the audit passes | status: implemented -->
### init writes a system repository the audit passes

<!-- field: use_case -->
#### Use case

[rel01.4-uc001-bootstrap-a-program](../use-cases/rel01.4-uc001-bootstrap-a-program.md)

<!-- field: integration_target -->
#### Integration target

mage integration:init

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^TestInit(WritesSkeleton|WritesControlFiles|InstallsSkills)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- corpus: VISION, ARCHITECTURE, SPECIFICATIONS, road-map with one release, design constitution, charter, pins, system SRD
- control: labels.md and policy.md holding the defaults
- skills: installed for the named agents
- audit: the parent's corpus audit and the Sprint Goal gate pass on the new repository

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC1](../software-requirements/srd043-init-onboard.md#ac1)

<!-- case: init seeds an empty tracker once | status: implemented -->
### init seeds an empty tracker once

<!-- field: use_case -->
#### Use case

[rel01.4-uc001-bootstrap-a-program](../use-cases/rel01.4-uc001-bootstrap-a-program.md)

<!-- field: integration_target -->
#### Integration target

mage integration:init

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^TestInit(SeedsEmptyTracker|LeavesTrackerWithCommits)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- empty_tracker: one first commit holding an empty issues file, and the program's labels
- tracker_with_commits: unchanged

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC2](../software-requirements/srd043-init-onboard.md#ac2)

<!-- case: onboard files one registration and commits nothing upstream | status: implemented -->
### onboard files one registration and commits nothing upstream

<!-- field: use_case -->
#### Use case

[rel01.4-uc001-bootstrap-a-program](../use-cases/rel01.4-uc001-bootstrap-a-program.md)

<!-- field: integration_target -->
#### Integration target

mage integration:onboard

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^TestOnboard(WritesMemberSkeleton|FilesOneRegistration|FilesNoSecondRegistration)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- member_corpus: the skeleton without control files
- registration: one issue in the system repository naming the repository, its URL and path, and the subsystem SRD
- system_repository_commits: none
- second_run: no issue filed, and the reason

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC3](../software-requirements/srd043-init-onboard.md#ac3)

<!-- case: init and onboard change nothing on a second run | status: implemented -->
### init and onboard change nothing on a second run

<!-- field: use_case -->
#### Use case

[rel01.4-uc001-bootstrap-a-program](../use-cases/rel01.4-uc001-bootstrap-a-program.md)

<!-- field: integration_target -->
#### Integration target

mage integration:init

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^Test(Scaffold(RerunChangesNothing|KeepsEditedFile|ForceReplaces|RefusesNonRepository|DryRunWritesNothing)|ScaffoldWritesParentPin|TemplatesNameNoProject)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- second_run: every file reported kept
- edited_file: kept and reported without --force, replaced with it
- not_a_repository: refused
- pins: the templates' parent release
- templates: no person, organization, product, or project named

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC4](../software-requirements/srd043-init-onboard.md#ac4)

<!-- case: help pages teach the process | status: retired -->
### help pages teach the process

<!-- field: use_case -->
#### Use case

[rel01.4-uc002-agent-learns-the-cli](../use-cases/rel01.4-uc002-agent-learns-the-cli.md)

<!-- field: integration_target -->
#### Integration target

mage integration:issueGroup

<!-- field: go_test -->
#### Go test

go test ./internal/help -run '^TestHelp(ProcessPage|GroupPages|ConceptPages|ListAndUnknown|JSON|PageLength)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- process_page: the planes, the ten release-cycle steps with the group or skill serving each, the corpus and control locations, the pointer to VISION
- group_pages: one per group
- concept_pages: Sprint Goal, admission, approval, milestone, tracker, evidence, release, recurring issue
- page_length: at most 60 lines
- unknown_page: non-zero exit listing the pages

<!-- field: traces -->
#### Traces

- [srd044-help](../software-requirements/srd044-help.md) [AC1](../software-requirements/srd044-help.md#ac1)
- [srd044-help](../software-requirements/srd044-help.md) [AC2](../software-requirements/srd044-help.md#ac2)

<!-- case: help cannot drift from the binary or the corpus | status: retired -->
### help cannot drift from the binary or the corpus

<!-- field: use_case -->
#### Use case

[rel01.4-uc002-agent-learns-the-cli](../use-cases/rel01.4-uc002-agent-learns-the-cli.md)

<!-- field: integration_target -->
#### Integration target

mage audit

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestHelpPages(NameRealCommandsAndSRDs|CoverEveryGroup|ListGroupSubcommands)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- missing_command: mage audit fails naming the page and the command
- missing_srd: mage audit fails naming the page and the SRD
- group_without_page: mage audit fails naming the group

<!-- field: traces -->
#### Traces

- [srd044-help](../software-requirements/srd044-help.md) [AC3](../software-requirements/srd044-help.md#ac3)

<!-- case: the issue group serves every subcommand and the old name refuses | status: implemented -->
### the issue group serves every subcommand and the old name refuses

<!-- field: use_case -->
#### Use case

[rel01.4-uc002-agent-learns-the-cli](../use-cases/rel01.4-uc002-agent-learns-the-cli.md)

<!-- field: integration_target -->
#### Integration target

mage integration:issueGroup

<!-- field: go_test -->
#### Go test

go test ./words -run '^Test(IssueGroupServesSubcommands|ProgramGroupRefusesWithPointer)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- issue_group: file, lint, labels, pop, approve, merge, close, recur
- program_group: non-zero exit naming tpm issue and tpm install, nothing run

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC2](../software-requirements/srd013-program.md#ac2)

<!-- case: recur opens the next instance once across the move | status: implemented -->
### recur opens the next instance once across the move

<!-- field: use_case -->
#### Use case

[rel01.4-uc002-agent-learns-the-cli](../use-cases/rel01.4-uc002-agent-learns-the-cli.md)

<!-- field: integration_target -->
#### Integration target

mage integration:issueGroup

<!-- field: go_test -->
#### Go test

go test ./internal/standup -run '^TestRecur(OpensNextInstanceOnce|FindsEarlierMarker)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- recurring_issue: closed with the run logged and Requires updated
- next_instance: opened once; a second recur opens none
- earlier_marker: an instance an earlier tpm standup recur opened is found

<!-- field: traces -->
#### Traces

- [srd013-program](../software-requirements/srd013-program.md) [AC9](../software-requirements/srd013-program.md#ac9)
