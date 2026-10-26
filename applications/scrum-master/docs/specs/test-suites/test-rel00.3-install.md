---
id: test-rel00.3-install
title: Test Suite for Release 00.3 Install the Skills
release: "00.3"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: overview -->
## Overview

The suite runs offline in a temporary git repository. It installs the
embedded skills for each agent, confirms git ignores exactly the installed
files, re-installs, edits a file, removes a skill from the embedded set,
and checks, proving every path, header, stamp, and exit status.

<!-- field: traces -->
## Traces

- [rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: preconditions -->
## Preconditions

- A tpm binary built from this repository, so the plugin/ package is embedded.
- A temporary git repository as --dir; no network.

<!-- field: test_cases -->
## Test cases

<!-- case: Install writes each agent's files | status: implemented -->
### Install writes each agent's files

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./internal/install -run '^TestInstall(Cursor|Claude|Copilot)PathsAndHeaders$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- cursor: .cursor/skills/<name>/SKILL.md
- claude: .claude/skills/<name>/SKILL.md
- copilot: .github/skills/<name>/SKILL.md
- front_matter: the skill's own Agent Skills front matter, the stamp after it
- stamp: tpm version and checksum on every file
- list: every embedded skill with its description

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC1](../software-requirements/srd037-install.md#ac1)
- [srd037-install](../software-requirements/srd037-install.md) [AC2](../software-requirements/srd037-install.md#ac2)

<!-- case: Re-install keeps edits and removes only unedited stale skills | status: implemented -->
### Re-install keeps edits and removes only unedited stale skills

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./internal/install -run '^Test(Reinstall(ChangesNothing|KeepsEditedFile|ForceReplaces|RemovesUneditedStaleSkill)|InstallMigratesCommandFiles)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- second_install: nothing changed
- edited_file: kept and reported; replaced with --force
- stale_skill: removed when unedited, kept when edited
- earlier_layout: stamped command files in .cursor/commands, .claude/commands, .github/prompts removed when unedited, kept and reported when edited

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC3](../software-requirements/srd037-install.md#ac3)

<!-- case: Check fails on drift and passes after install | status: implemented -->
### Check fails on drift and passes after install

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./internal/install -run '^TestCheck(FailsOnMissingOutdatedEdited|PassesAfterInstall)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- missing_outdated_edited_stale: non-zero exit naming each file; nothing written
- after_install: exit 0
- writes_outside_dir: none; outside the agents' directories only the block in .git/info/exclude
- flags: --json and --dry-run honoured; no prompt

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC4](../software-requirements/srd037-install.md#ac4)
- [srd037-install](../software-requirements/srd037-install.md) [AC5](../software-requirements/srd037-install.md#ac5)

<!-- case: The installed push and pop commands stay within the limit | status: implemented -->
### The installed push and pop commands stay within the limit

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^TestSkillFilesStayWithinLineLimit$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- issue_push_lines: at most 50
- issue_pop_lines: at most 50

<!-- field: traces -->
#### Traces

- [srd035-specify](../software-requirements/srd035-specify.md) [AC4](../software-requirements/srd035-specify.md#ac4)
- [srd036-implement](../software-requirements/srd036-implement.md) [AC3](../software-requirements/srd036-implement.md#ac3)

<!-- case: Git ignores the installed files and nothing else | status: implemented -->
### Git ignores the installed files and nothing else

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage integration:install

<!-- field: go_test -->
#### Go test

go test ./internal/install -run '^(TestInstallIgnoresOwnedFiles|TestCheckFailsOnUnignored|TestInstallRefusesNonRepository|TestInstallInWorktreeUsesTheCloneExclude)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- exclude_block: .git/info/exclude lists every stamped file, sorted, between the tpm install markers; a linked worktree uses the clone's file
- git_status: no change after an install, in the checkout and in a linked worktree
- non_repository: --dir outside git is refused and nothing is written
- own_command: a command without the stamp in the same directory is not ignored
- other_lines: lines outside the block unchanged; a removed stale skill drops out
- check_unignored: --check names a stamped file the block does not list

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC6](../software-requirements/srd037-install.md#ac6)
- [srd037-install](../software-requirements/srd037-install.md) [AC4](../software-requirements/srd037-install.md#ac4)

<!-- case: The skills are a conforming Agent Plugins 1.0 package | status: implemented -->
### The skills are a conforming Agent Plugins 1.0 package

<!-- field: use_case -->
#### Use case

[rel00.3-uc001-install-skills](../use-cases/rel00.3-uc001-install-skills.md)

<!-- field: integration_target -->
#### Integration target

mage audit

<!-- field: go_test -->
#### Go test

go test ./magefiles -run '^Test(PluginManifestConforms|SkillsConformToAgentSkills|ManifestRulesRejectBrokenManifests|SkillRulesRejectBrokenSkills)$'

<!-- field: expected_outputs | form: "tree" -->
#### Expected outputs

- manifest: plugin/plugin.json with the 1.0.0 $schema, a valid name, and only the specification's fields
- skills: plugin/skills/<name>/SKILL.md, one level deep, name equal to the directory, description of 1 to 1024 characters
- broken_package: mage audit fails, naming the rule

<!-- field: traces -->
#### Traces

- [srd037-install](../software-requirements/srd037-install.md) [AC1](../software-requirements/srd037-install.md#ac1)
