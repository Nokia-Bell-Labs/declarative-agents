---
id: test-rel04.1-orientation-doctor
title: Test Suite for Release 04.1 A Repository That Explains Itself
release: "04.1"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The suite runs offline. It inits an empty git repository and onboards a
second one, reads the orientation files each wrote, then turns the system
repository into one an older tpm laid down: it deletes the orientation
files, edits the text around a managed block, and moves a document out of
canonical form. It runs tpm doctor with --dry-run, then for real, then
again, and runs the parent's corpus audit over the result.

<!-- field: traces -->
## Traces

- [rel04.1-uc001-orient-and-repair](../use-cases/rel04.1-uc001-orient-and-repair.md)

<!-- field: preconditions -->
## Preconditions

- A tpm binary built from this repository, so the templates, the skills, and the help pages are embedded.
- Temporary git repositories for the system and the joining repository; no network.
- The parent's clone cache and a Go toolchain, for the repair and the corpus audit (docs/guides/corpus-audit.md).

<!-- field: test_cases -->
## Test cases

<!-- case: init and onboard write the orientation files | status: implemented -->
### init and onboard write the orientation files

<!-- field: use_case -->
#### Use case

[rel04.1-uc001-orient-and-repair](../use-cases/rel04.1-uc001-orient-and-repair.md)

<!-- field: integration_target -->
#### Integration target

mage integration:init

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^Test(InitWritesOrientation|OnboardWritesOrientation|AgentsFile)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- system: README.md, AGENTS.md, and README.md in themes/, initiatives/, epics/, and engineering/
- subsystem: README.md, AGENTS.md, and README.md in epics/, features/, stories/, and engineering/; none in themes/ or initiatives/
- agents_file: every command group and skill at the level, and the Mermaid figure rule
- audit: the parent's corpus audit and the hierarchy gate pass on both repositories

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC5](../software-requirements/srd043-init-onboard.md#ac5)

<!-- case: an existing AGENTS.md or README gets tpm's block appended | status: implemented -->
### an existing AGENTS.md or README gets tpm's block appended

<!-- field: use_case -->
#### Use case

[rel04.1-uc001-orient-and-repair](../use-cases/rel04.1-uc001-orient-and-repair.md)

<!-- field: integration_target -->
#### Integration target

mage integration:onboard

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^Test(InitAppendsToOrientationWithoutBlock|OnboardAppendsToExistingAgentsAndReadme|DoctorAppendsToOrientationWithoutBlock)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- existing_agents_and_readme: appended, the owner's text first and tpm's block after one blank line
- second_run: kept
- dry_run: appended reported, nothing written
- force: the whole file replaced
- edited_vision: kept

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC6](../software-requirements/srd043-init-onboard.md#ac6)

<!-- case: doctor brings an older repository up to date | status: implemented -->
### doctor brings an older repository up to date

<!-- field: use_case -->
#### Use case

[rel04.1-uc001-orient-and-repair](../use-cases/rel04.1-uc001-orient-and-repair.md)

<!-- field: integration_target -->
#### Integration target

mage integration:doctor

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^Test(Doctor(AddsMissing|RefreshesBlock|ReportsStaleSkills|DryRunWritesNothing|SecondRunChangesNothing|RefusesNonProgram|SubsystemGetsNoControl|ExtendsLabels|UpgradesTheSystem)|ExtendLabelsLeavesACompleteFile|CorpusPeerReadKeepsTheProgramsGate)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- missing_files: created, and nothing else
- changed_block: refreshed, with the text around it kept
- orientation_file_without_block: appended, with the owner's text first
- labels_file_missing_axes: extended with the axis and values, a recolored value kept
- system_srd_and_policy_from_an_older_init: extended with the context, the component row, and the corpus peer read, the owner's text kept
- stale_skills: reported with tpm install as the fix, none installed
- dry_run: the same report, and nothing written
- second_run: nothing created, appended, refreshed, or repaired
- not_a_program_repository: refused with the reason

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC6](../software-requirements/srd043-init-onboard.md#ac6)

<!-- case: doctor repairs documents at the pin | status: implemented -->
### doctor repairs documents at the pin

<!-- field: use_case -->
#### Use case

[rel04.1-uc001-orient-and-repair](../use-cases/rel04.1-uc001-orient-and-repair.md)

<!-- field: integration_target -->
#### Integration target

mage integration:doctor

<!-- field: go_test -->
#### Go test

go test ./internal/scaffold -run '^TestDoctorRepair(RewritesCanonical|ReportsUnrepairable|SkipsWithoutGo)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- out_of_form_document: rewritten in canonical form; the audit passes
- unrepairable_finding: reported with its fix hint; nothing else changed
- without_go: repair skipped with the reason; the rest done

<!-- field: traces -->
#### Traces

- [srd043-init-onboard](../software-requirements/srd043-init-onboard.md) [AC7](../software-requirements/srd043-init-onboard.md#ac7)
