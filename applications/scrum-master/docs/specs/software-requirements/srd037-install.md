---
id: srd037-install
title: "Script: install (scrum-master install)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

The skills are what an agent runs: Cursor, Claude Code, or GitHub
Copilot, in the repository a developer works in. Copied by hand, a
skill falls out of step with the scrum-master it calls: a renamed flag, a new
subcommand, a refusal the skill does not expect. Every repository then
carries a slightly different process.

The skills ship as one Agent Plugins 1.0 package (agent-plugins.org),
plugin/ in this repository: a plugin.json manifest and one Agent Skills
directory per skill (agentskills.io). A client that loads Agent Plugins
can install the package as it is. scrum-master install makes the binary the one
source of the same skills for every agent: the package is embedded in scrum-master
when it is built, and scrum-master install writes each skill as an Agent Skills
directory where each agent reads skills. Every
installed file names the scrum-master version and the source checksum it came
from, so a later install updates what scrum-master wrote and keeps what a person
edited. The installed files belong to the scrum-master each developer has
installed, not to the repository, so git ignores them, and scrum-master install
--check tells the developer when they no longer match the local binary.

The skills run as declarative-agent workflows, and a profile runs only
against the declarative-agents runtime it was written for: the agent and
the agent-core declarations its paths resolve against. A release ships
both — the agent, which serves the scrum-master words in its own process,
and the profiles archive — and a program repository vendors the profiles
and those declarations into .scrum-master, so every skill runs what the
repository has checked in (GH-29, GH-34).

<!-- field: goals -->
## Goals

- G1: A scrum-master release carries exactly the skills written against its subcommands, as a conforming Agent Plugins 1.0 package.
- G2: One command places the skills where Cursor, Claude Code, or GitHub Copilot reads them.
- G3: The installed skills never enter the repository; git ignores them, and the developer learns of drift locally.
- G4: One script installs the agent and the release's profiles, verified against the release's checksums, on any machine, and a repository vendors the profiles its skills run.

<!-- req: R1 -->
## R1 - Embedding

<!-- req: R1.1 -->
### R1.1

The binary embeds the plugin package plugin/ at build time: its plugin.json and every skills/<name>/SKILL.md. Each SKILL.md conforms to the Agent Skills specification (front matter name equal to its directory name, lowercase letters, digits, and single hyphens, at most 64 characters; description of 1 to 1024 characters; no other keys than the specification's) and is at most 50 lines (design.md D10).

<!-- req: R1.2 -->
### R1.2

scrum-master install --list names every embedded skill with its description.

<!-- req: R1.3 -->
### R1.3

plugin/ conforms to Agent Plugins 1.0: plugin.json at its root is a JSON object with only the specification's fields, $schema https://agent-plugins.org/schemas/1.0.0/plugin.schema.json, and a name of 1 to 64 characters of a-z, 0-9, hyphens, and periods, alphanumeric at both ends, without doubled hyphens or periods; skills sit exactly one level under skills/; no path leaves the package. mage audit fails a package that does not conform.

<!-- req: R1.4 -->
### R1.4

The binary also embeds plugin/agents/<name>/: the declarative-agents profile of a skill whose workflow is a profile (design.md D14). scrum-master install renders that skill's SKILL.md from the profile's machine — the purpose's first paragraph is the description, the states between the initial and the terminal ones the numbered steps in the machine's order, the terminal states its ending — and refuses a name that is both an embedded SKILL.md and a profile. The rendered skill installs, stamps, checks, and uninstalls as any other (R2 to R6).

<!-- req: R1.5 -->
### R1.5

Every profile ships and is vendored, so CI and a coding agent with no model run any of them by its path, but install writes a roster, not a skill per profile (GH-2749): each installed skill's description sits in the agent's context on every turn, and near-alike names route a request to a step rather than its workflow. The roster is the workflows a person starts, init, onboard, upgrade, check, pop, merge, implement, draft, design, specify, plan, tasks, sprint-end, release, peer-read, status-report, status-rollup, and site, each its rendered skill, and four group skills, issue, backlog, reports, and tracker, each naming its single-word profiles with each one's purpose and the command line that runs it. A profile reachable from no installed skill, as a workflow, a group's member, or a step a workflow runs, fails the build; a skill an earlier release installed that the roster drops is removed as any stale skill is (R5).

<!-- req: R2 -->
## R2 - Targets

<!-- req: R2.1 -->
### R2.1

--agent names cursor, claude, or copilot, repeatable, or all, or detect, the agents the repository already uses as init's --agent detect finds them (srd043 R1.4), refused when it uses none (GH-2719). --dir is the repository to install into, by default the root of the git repository holding the working directory.

<!-- req: R2.2 -->
### R2.2

Each agent receives every skill as an Agent Skills directory with the skill's own SKILL.md: Cursor in .cursor/skills/<name>/, Claude Code in .claude/skills/<name>/, and GitHub Copilot in .github/skills/<name>/.

<!-- req: R3 -->
## R3 - Version stamp and updates

<!-- req: R3.1 -->
### R3.1

Every installed SKILL.md carries a comment naming the scrum-master version and the checksum of the text scrum-master wrote, less the comment itself, on the first line after its front matter, which must open the file.

<!-- req: R3.2 -->
### R3.2

A later install rewrites a file whose content still matches its stamp, keeps a file a person edited and reports it, and replaces an edited file only with --force.

<!-- req: R3.3 -->
### R3.3

A skill no longer embedded is removed only when its installed file is unedited; an edited one is reported and kept. A file scrum-master stamped in an earlier layout (.cursor/commands, .claude/commands, .github/prompts) is removed the same way.

<!-- req: R4 -->
## R4 - Check

<!-- req: R4.1 -->
### R4.1

scrum-master install --check writes nothing, names each installed file that is missing, outdated against the binary, edited, stale (a stamped file whose skill is no longer embedded), or unignored (a stamped file the exclude block does not list), and exits non-zero when any is.

<!-- req: R5 -->
## R5 - Common behavior

<!-- req: R5.1 -->
### R5.1

Honours --json and --dry-run and never prompts (srd010 R1).

<!-- req: R5.2 -->
### R5.2

Writes nothing outside --dir and the clone's git directory. Outside the agents' skills directories it writes only its block in .git/info/exclude, which git applies and never commits, so an install changes no tracked file. init, onboard, and reports init list the skill files they installed under skills and what the install did to that block apart, under exclude, so no entry of the skills names no skill (GH-2781).

<!-- req: R6 -->
## R6 - Ignored by git

<!-- req: R6.1 -->
### R6.1

scrum-master install keeps one block in the clone's .git/info/exclude, the path git rev-parse --git-path info/exclude names, so every linked worktree of the clone shares it. The block sits between the lines "# skill install: the skill files the init and onboard profiles install" and "# end skill install", or the "# tpm install" lines an earlier release wrote, which a run rewrites, and lists exactly the files scrum-master owns (those carrying its stamp), one anchored path per line, sorted. A file without the stamp is never listed, so a command the repository owns stays tracked. scrum-master install and --check refuse a --dir that is not a git repository.

<!-- req: R6.2 -->
### R6.2

Each install rewrites the block from the files scrum-master owns after the run, drops a removed stale skill, and leaves every line outside the block as it was. With no exclude file, it creates one holding only the block.

<!-- req: R6.3 -->
### R6.3

The installed files are untracked, so a new worktree of the clone has none; scrum-master issue pop installs the checkout's agents into each worktree it creates (srd013 R7.4), and the shared exclude block already lists their paths.

<!-- req: R7 -->
## R7 - The agent and the profiles

<!-- req: R7.1 -->
### R7.1

A release ships the agent, composed with the scrum-master words (GH-27), for each platform the runtime publishes, and the profiles archive — the profiles, agent-core's tools tree, and the release — under one SHA256SUMS (GH-34). install.sh installs the agent and unpacks the profiles into SCRUM_MASTER_HOME, by default under the user's data directory. Inside this repository, the sibling is the release: the agent builds from agent-core into the release cache and nothing downloads (GH-190).

<!-- req: R7.2 -->
### R7.2

install.sh holds the agent and the profiles archive to the SHA-256 the release's SHA256SUMS lists for each before it installs either; a mismatch is refused naming the asset and both checksums, and nothing is written. A host the release publishes no agent for is refused naming the platforms it publishes.

<!-- req: R7.3 -->
### R7.3

The agent runs the words and the corpus gate's shape check in its own process (GH-33), so a machine with the agent and no Go toolchain runs every profile and the gate.

<!-- req: R7.4 -->
### R7.4

scrum-master install --profiles vendors the release's profiles and the agent-core declarations they read into the repository's .scrum-master, with a manifest of the release and each file's digest (GH-29). It copies from SCRUM_MASTER_HOME, or this repository's own tree inside the repository that ships agent-core; a rerun keeps a file a person edited, rewrites one the release changed, and removes one the release dropped. scrum-master install --profiles --check holds every vendored file to the manifest and fails on one edited or missing (GH-35).

<!-- req: R7.5 -->
### R7.5

The vendoring reports the release and each file's action, created, updated, unchanged, kept, or removed. It honours --json and --dry-run; a dry run writes nothing.

<!-- field: non_goals -->
## Non-goals

- Publishing a release itself; that is the release of scrum-master, its assets and container image (eng03).
- Linking agent-core into the words' own module; the agent links the words at build time instead (GH-27), so the application depends on none of agent-core's packages.
- Model credentials; the profiles that need them name them (GH-135), and scrum-master stores none.
- Agents other than Cursor, Claude Code, and GitHub Copilot.
- Per-repository templating; repository-specific values come from scrum-master at run time.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

plugin/ is embedded; every SKILL.md conforms to Agent Skills and is at most 50 lines; --list names each skill with its description; mage audit fails a manifest or skill that breaks Agent Plugins 1.0 or Agent Skills; a skill whose workflow is a profile installs a SKILL.md rendered from the profile's machine, its states the numbered steps; install writes the roster of workflows and group skills, every profile still shipped, refuses a profile no installed skill reaches, and removes the skills a repository installed per profile before.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)
- [R1.5](#r15)

<!-- ac: AC2 -->
### AC2

Installing for cursor, claude, and copilot writes each skill as an Agent Skills directory in .cursor/skills, .claude/skills, or .github/skills, into the repository root by default; with detect, only for the agents the repository uses, and a repository that uses none is refused.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC3 -->
### AC3

Every installed file carries the version and checksum stamp; a second install changes nothing; an edited file is kept and reported unless --force; a skill no longer embedded, or a file of the earlier commands layout, is removed only when unedited.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC4 -->
### AC4

--check writes nothing and exits non-zero naming each missing, outdated, edited, stale, or unignored file, and passes after an install.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)

<!-- ac: AC5 -->
### AC5

install honours --json and --dry-run, never prompts, and writes nothing outside --dir, the agents' skills directories, and its block in .git/info/exclude.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)

<!-- ac: AC6 -->
### AC6

After an install, git ignores every installed file and nothing else: git status shows no change, in the main checkout and in a linked worktree; a command without the stamp in the same directory is not ignored; lines outside the block are unchanged; a directory that is not a git repository is refused; a worktree scrum-master issue pop creates receives the checkout's installed agents and stays clean.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)

<!-- ac: AC7 -->
### AC7

A release carries the agent for each published platform and the profiles archive under one SHA256SUMS; install.sh, with no Go on PATH, installs both at the tag, the init profile runs from them with only the agent, and a tampered download is refused naming both checksums with nothing written; the vendored profiles validate against the vendored core root, and a rerun of the vendoring changes nothing.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
- [R7.4](#r74)
- [R7.5](#r75)
