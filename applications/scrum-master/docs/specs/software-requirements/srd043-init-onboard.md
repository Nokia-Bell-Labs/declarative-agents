---
id: srd043-init-onboard
title: "Script: init, onboard, and doctor (scrum-master init, scrum-master onboard, scrum-master doctor)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- Scaffolding and enforcement are separate. init and onboard make a corpus complete when it is created; keeping it complete afterwards is the audit's job. A check that a corpus holds its required documents belongs in the parent's corpus audit, so it gates every repository that carries a charter; a required-documents key in the policy was rejected because it would duplicate the auditor in scrum-master and fire only on scrum-master's paths.
- The registration of a member repository is filed, not written. The subsystem SRD that lists a component belongs to the system repository's owner and changes through that repository's gate, so onboard files the change as an issue and never commits there (design.md D4, D7).
- The registration is a plane::management issue: it changes a subsystem SRD, which is intent. Filed through scrum-master issue file under the system repository's policy, it is refused like any management issue when the policy names who may file one and the caller is not among them; the refusal says who may.
- scrum-master reports init (srd018 R1) stays its own subcommand. The reports repository has its own layout and owner, and folding it into onboard would change an implemented command for no reader's benefit.
- The templates are embedded in the binary beside the skills and written against the parent format release scrum-master was built with, so a scrum-master release and the corpus it scaffolds always agree.
- doctor is its own command, not a second run of init or onboard. init is for an empty repository and onboard files a registration, so neither is the step to take after upgrading scrum-master; doctor reads the level from the files it finds, touches only what scrum-master owns or can repair without changing meaning, and files and commits nothing.
- Only a tpm-managed block is refreshed. The orientation files are scrum-master's account of its own process, so a newer scrum-master updates them; text a person writes around the block is theirs.
- A block is appended to an orientation file that has none, rather than the file being kept as edited. AGENTS.md at the top level is the one place every agent reads, so scrum-master stays agent-agnostic by writing there, and a repository that joins a program usually already has its own AGENTS.md and README.md; keeping them untouched would leave scrum-master's text out, and --force would replace the owner's. An owner who deletes the block gets it back on the next run. Every other skeleton file belongs to its owner once written, so it is never refreshed and R3.2 holds for it.
- Repair runs the formatter the agent carries: the agent links agent-core's format library with the words, so it formats and checks in its own process with no Go toolchain (GH-33). Outside the agent, repair builds the spec tool from the pinned release's source as mage fmt builds it (srd040 R1.1), with a Go toolchain or a tool already built at the pin.
- doctor reports the release the repository vendored rather than vendoring it. Repair changes only documents; the vendoring is the init, onboard, and upgrade profiles' step (srd037 R7.4), so doctor names the upgrade profile as the fix.
- Taking a new release is its own profile, upgrade, not a rerun of init or onboard (GH-2719). A rerun of init may seed a tracker and a rerun of onboard files a registration when none is open, which are steps of laying a repository down; upgrade composes only the three steps a new release needs, vendor, doctor, and install, each a word that keeps what a person edited, at either level.

<!-- field: problem -->
## Problem

Nothing creates a complete program repository. The corpus skeleton (VISION,
ARCHITECTURE, SPECIFICATIONS, road-map, the design constitution, the corpus
charter, and the parent pins), the control files, and the skills are
assembled by hand in every repository. A document left out is never
noticed: the audit checks the documents that exist, and the formatter
quietly unlinks an index row whose document is missing. Seeding a program
for a regression run took a dozen manual steps that scrum-master should own.

Every repository of a program carries the same corpus (design.md D7), so
the corpus can be written once, embedded in scrum-master, and laid down by one
command: init for the system repository a program starts from, onboard
for each repository that joins it.

A repository laid down this way does not explain itself. A first-time
reader finds docs/specs/themes/, initiatives/, and epics/ full of numbered
documents and nothing saying what each level is, who owns it, or which scrum-master
command reads it; the answer is in the design constitution, which nobody
reads first. An agent opening the repository is not told which scrum-master
commands and skills operate there. And a repository laid down by an older
scrum-master cannot catch up: a newer scrum-master scaffolds files the older one did not,
and documents drift from the pinned format with no formatter in reach,
since the format tools run only in this repository's mage (srd040).

<!-- field: goals -->
## Goals

- G1: One command turns an empty git repository into a program's system repository whose corpus passes the audit as written.
- G2: One command turns an existing repository into a member of a program, and files its registration with the system repository's owner.
- G3: Running either command again changes nothing, and nothing a person wrote is overwritten without --force.
- G4: A first-time reader, person or agent, learns from the repository itself what each hierarchy level is, who owns it, and which scrum-master commands and skills operate on it.
- G5: One command brings a repository an older scrum-master laid down up to date: missing files added, scrum-master's orientation text refreshed, documents repaired to the pinned format, and nothing a person wrote lost.

<!-- req: R1 -->
## R1 - scrum-master init

<!-- req: R1.1 -->
### R1.1

Writes into --dir, by default the root of the git repository holding the working directory, the system repository's corpus skeleton: docs/VISION.md, docs/ARCHITECTURE.md, docs/SPECIFICATIONS.md, docs/road-map.md with one release, docs/constitutions/design.md, docs/corpus-charter.yaml, docs/external-pins.yaml, and a system SRD, docs/specs/software-requirements/srd001-system.md, which that release lists. The parent's corpus audit loads no corpus without an SRD and no road-map without a release, so the skeleton carries one of each for the owner to rewrite. The system SRD declares the system repository a component of the program: its context names the technical program manager as owner, and its implementation_targets row names the repository at its origin's URL, path /, so issues in the system repository carry a component like any team repository's (srd001 R21). A repository with no origin gets the field as an empty list.

<!-- req: R1.2 -->
### R1.2

Every skeleton document is in the format of the parent release scrum-master was built with, holds only fields that release declares, and states in each field what the owner writes there. The skeleton as written passes the parent's corpus audit (docs/guides/corpus-audit.md) and the hierarchy gate.

<!-- req: R1.3 -->
### R1.3

Writes control/labels.md, control/policy.md, and control/states.md holding the built-in defaults, in the config formats labels, policy, and states; the states rows are the srd041 table, so the tracker a program extends reads its own file. Every command the policy names is one a program repository can run: its audit_command is scrum-master backlog check, its default definition of done is that check, not go test ./..., which a repository with no Go module cannot pass, and its definition of done gives the system's corpus release that check and a peer-read check (srd015 R1.2), so the system is never released on its author's word.

<!-- req: R1.4 -->
### R1.4

Installs the skills for the agents --agent names, through scrum-master install (srd037). --agent detect names the agents the repository already uses (.cursor; .claude or CLAUDE.md; copilot's instructions or skills under .github) and is refused when it uses none; the init and onboard profiles default to it. With --require-profiles, which both profiles pass, init and onboard refuse before their first write when no release's profiles are installed to vendor (GH-49).

<!-- req: R1.5 -->
### R1.5

Writes control/trackers.md naming bd with the tracker repository --tracker or SCRUM_MASTER_TRACKER names, when either does (config format trackers), and the program group --group names, which issue labels and plan push default to; with --group and no tracker known, it reports that the group was not recorded (GH-103). With --tracker naming a tracker repository that has no commits, makes its first commit holding the config file, which sets the files layout and the program's issue prefix, --prefix or else the last segment of --group (srd042 R3.1), and creates the program's labels there (srd013 R6). A tracker repository that already has commits is left as it is. With --tracker-branch, the tracker is that branch of the repository --tracker names, which may be the program's own repository: init makes the branch as an orphan holding only the config file when the repository lacks it, however many commits the repository's other branches hold, leaves an existing branch as it is, and writes the branch to control/trackers.md beside the repository (srd042 R3.1). Without --tracker, and with no tracker repository SCRUM_MASTER_TRACKER names, reports that no tracker was seeded, so the operator knows no issue can be filed yet.

<!-- req: R1.6 -->
### R1.6

Writes the orientation files at the system level: README.md and AGENTS.md at the root of --dir, and a README.md in docs/specs/themes/, docs/specs/initiatives/, and docs/specs/epics/, and in docs/engineering/. The root README says what the repository is, the order to read its corpus in, ARCHITECTURE and the engineering documents among it, and the chain theme, initiative, epic, feature, story with each level's owner and timeframe. Each folder README says what its level is, its owner, timeframe, parent kind, file naming, and status values, and which scrum-master commands and skills read it. The docs/engineering/ README says what an engineering document is (how the repository's part is structured, run, and deployed, and the practices its parts share), how it differs from ARCHITECTURE, the index that names it, and from the SRDs, and its file naming and sections; the ARCHITECTURE skeleton names docs/engineering/ among its related documents. AGENTS.md lists the scrum-master command groups and the skills that apply at the level, read from the binary's dispatch table and embedded skills, and the working rules: intent changes through the gate, work enters as an issue, and every figure is Mermaid in a fenced code block inside the Markdown document, with no generated image file or other diagram source under docs/. A system repository whose tracker is a branch of itself is a program of one repository: it is the system and its only component, and its README and AGENTS.md say so.

<!-- req: R2 -->
## R2 - scrum-master onboard

<!-- req: R2.1 -->
### R2.1

Writes into --dir, a checkout of the repository joining the program, the same corpus skeleton as init at the subsystem's level: a subsystem SRD, docs/specs/software-requirements/srd001-subsystem.md, in place of the system SRD, every field naming the Product Owner as the one who writes it, and no control files, which only the system repository holds.

<!-- req: R2.2 -->
### R2.2

Files one issue in the system repository that --system names, through scrum-master issue file (srd013 R3), asking that the subsystem SRD --subsystem names list the repository in its components table, with the repository's URL and path. Its criterion names the program policy's audit command as the test, and it carries the system repository's component when the system SRD declares one. It never commits to the system repository.

<!-- req: R2.3 -->
### R2.3

Files no second registration while one for the same repository is open or the subsystem SRD already lists it, and says which.

<!-- req: R2.4 -->
### R2.4

Writes the orientation files at the subsystem level, as R1.6 states them: README.md and AGENTS.md at the root, and a README.md in docs/specs/epics/, docs/specs/features/, and docs/specs/stories/, the levels a team's repository holds (srd045 R2.2), and in docs/engineering/, and none in themes/ or initiatives/.

<!-- req: R3 -->
## R3 - Common behavior

<!-- req: R3.1 -->
### R3.1

init, onboard, and doctor honour --json and --dry-run, never prompt (srd010 R1), and refuse a --dir that is not a git repository.

<!-- req: R3.2 -->
### R3.2

Each file is reported as created, kept, appended or refreshed (R3.5), or overwritten. A file that exists is kept and reported unless --force replaces it, so a second run changes nothing.

<!-- req: R3.3 -->
### R3.3

The templates are embedded in the scrum-master binary beside the skills. init and onboard write into docs/external-pins.yaml the release of this repository the corpus follows: the running scrum-master's own release, or, for a build that is not a release, the newest release with published binaries (GH-190). A rerun that finds the file pinning another release moves that pin, keeping the rest of the file, and reports the file repinned, so the documents follow the release taken; doctor and the check profile report a pin that differs from the release vendored in .scrum-master as drift (GH-91).

<!-- req: R3.4 -->
### R3.4

The templates name no person, organization, product, or project (design.md D6); what a program supplies, its subsystems, components, and environments, the owner writes into the skeleton.

<!-- req: R3.5 -->
### R3.5

An orientation file holds scrum-master's text between a begin marker and an end marker. A run whose text for the block differs from the file's replaces the block, keeps every line outside it, and reports the file refreshed; a person's text outside the block is theirs. An orientation file that exists without a block gets the block appended after its text, separated by one blank line, and is reported appended. No other file carries a block.

<!-- req: R4 -->
## R4 - scrum-master doctor

<!-- req: R4.1 -->
### R4.1

Runs in --dir, by default the root of the git repository holding the working directory, and reads the repository's level from the files it finds: system when control/policy.md or the system SRD is there, subsystem when the subsystem SRD is. A directory that holds neither is refused with the reason.

<!-- req: R4.2 -->
### R4.2

Writes every file init (at system level) or onboard (at subsystem level) writes that is missing, control/states.md among them, and appends or refreshes the orientation files' blocks (R3.5). At system level it extends control/labels.md with the axes and values scrum-master's defaults list that it lacks, keeping every value, color, and description it has and inserting rather than re-rendering, and names scrum-master issue labels as what creates them in the tracker. It also adds what init now writes into the system SRD and control/policy.md when they lack it: the SRD's context first and its implementation_targets last (R1.1), keeping every line the owner wrote, and the corpus release's peer-read gate when the definition of done names no corpus gate (R1.3). Every other existing file is kept; doctor offers no --force. It files no issue, installs no skill, and touches no tracker.

<!-- req: R4.3 -->
### R4.3

Reports, without changing them, the skills installed for each agent that scrum-master install --check finds missing, outdated, edited, or stale (srd037 R4.1), and names scrum-master install as the fix.

<!-- req: R4.4 -->
### R4.4

Repairs the corpus documents, the hierarchy documents among them: rewrites each in the canonical form of the parent release docs/external-pins.yaml pins, with that release's own formatter built as srd040 R1.1 builds it, then reports each document repaired and each finding the formatter cannot repair, with its fix hint. A repair never changes what a field says and never moves the pin.

<!-- req: R4.5 -->
### R4.5

In the agent, formats with the formatter the agent carries (srd037 R7.3). Elsewhere uses a spec tool already built at the pin without building it; with no such tool, and without a Go toolchain or the pinned release's source, skips the repair, says which is missing, and does the rest.

<!-- req: R4.6 -->
### R4.6

A dry run reports what doctor would write, append, refresh, extend, and repair and changes nothing. Both runs name the documents under docs/ git does not track, which the format check cannot read. A second run reports nothing created, appended, refreshed, extended, or repaired.

<!-- req: R4.7 -->
### R4.7

Reports the release whose profiles the repository vendored, from .scrum-master's manifest (srd037 R7.4), or that it vendored none, with the upgrade profile (R5) as the fix (GH-34). It also names each tracker clone kept under the user cache that follows no origin: one whose origin is a local path that no longer exists (GH-116), or one made again there since, holding none of the clone's history (GH-141). It removes none, since a clone may hold the only copy of a tracker's commits, pushed or not; the person copies what they need and removes it. Each is drift, so the check profile fails on it (GH-141). The report changes nothing, so a dry run gives it too. Opening a tracker made again at a cached clone's origin sets that clone aside, renamed beside it, and clones the new tracker, so the old one's history is never read as the new one's; a clone whose origin is simply gone is read as it is (GH-141).

<!-- req: R5 -->
## R5 - The upgrade profile

<!-- req: R5.1 -->
### R5.1

The upgrade profile brings a program repository to the release install.sh installed on the host, in three words and in this order: scrum-master install --profiles vendors the release (srd037 R7.4); scrum-master doctor repairs the repository at it (R4), writing the missing files, refreshing the managed blocks, and moving the format pin as a rerun does (R3.3); and scrum-master install --agent detect rewrites the skills of the agents the repository uses (srd037 R2.1). No model acts in it (design.md D10, D14).

<!-- req: R5.2 -->
### R5.2

It serves a system and a subsystem repository alike, seeds no tracker, files no issue, and touches no tracker. Each word keeps a file a person edited, so a second run changes nothing.

<!-- req: R5.3 -->
### R5.3

Each word that refuses ends the run in a state that names what stands: with no release installed, the vendoring refuses and nothing is written; a directory that is not a program repository is refused by doctor after the vendoring; a repository that uses no agent scrum-master knows is reported with the profiles vendored and the repository repaired. The refusal's reason, as printed, is the run's last output.

<!-- field: non_goals -->
## Non-goals

- Creating repositories, groups, or permissions on a hosting service; the host or its provisioning does that before init runs.
- Enforcing that a corpus stays complete; that is the audit's. doctor adds what is missing when it is run and gates nothing.
- doctor installing or upgrading skills; that is scrum-master install (srd037), which the upgrade profile runs after doctor (R5).
- Installing the agent and the release's profiles on the host; that is install.sh (srd037), which the upgrade profile reads from.
- The reports repository; that is scrum-master reports init (srd018 R1).
- Moving an existing corpus to a newer format release; a person moves the pin in docs/external-pins.yaml and scrum-master doctor repairs at it.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

init in an empty git repository writes VISION, ARCHITECTURE, SPECIFICATIONS, a road-map with one release, the design constitution, the charter, the pins, and the system SRD in the parent's format, the default labels and policy, and the skills for the named agents; the parent's corpus audit and the Sprint Goal gate pass on the result.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

init with an empty tracker repository makes its first commit and creates the labels there; with a tracker repository that has commits it changes nothing there; with no tracker repository given or named by SCRUM_MASTER_TRACKER it reports that none was seeded; with --tracker-branch it makes that branch as an orphan in a repository that has commits, leaves the repository's other branches as they are, and writes the branch to control/trackers.md.

<!-- field: traces -->
#### Traces

- [R1.5](#r15)

<!-- ac: AC3 -->
### AC3

onboard in a member checkout writes the skeleton without control files and files one registration issue in the system repository, with nothing committed there; a second onboard files none and says why.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC4 -->
### AC4

Both commands honour --json and --dry-run, never prompt, refuse a directory that is not a git repository, report each file as created, kept, or overwritten, keep an existing file unless --force, change nothing on a second run, and write the templates' parent release into the pins; no template names a person, organization, product, or project.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)

<!-- ac: AC5 -->
### AC5

init writes README.md, AGENTS.md, and README.md in themes/, initiatives/, epics/, and engineering/; onboard writes README.md, AGENTS.md, and README.md in epics/, features/, stories/, and engineering/ and none in themes/ or initiatives/; AGENTS.md names every command group and skill at its level and the Mermaid figure rule; the parent's corpus audit and the hierarchy gate pass on both results.

<!-- field: traces -->
#### Traces

- [R1.6](#r16)
- [R2.4](#r24)

<!-- ac: AC6 -->
### AC6

On a repository an older scrum-master laid down, doctor adds exactly the missing files, refreshes a changed block and keeps the text around it, appends the block to an orientation file that has none, extends control/labels.md with the axes and values it lacks while keeping a recolored value and a program's own value, adds the system SRD's component and the corpus peer read an older init did not write, reports stale skills without installing them, refuses a directory that is not a program repository, writes nothing on a dry run, and changes nothing on a second run.

<!-- field: traces -->
#### Traces

- [R3.5](#r35)
- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)
- [R4.6](#r46)

<!-- ac: AC7 -->
### AC7

doctor rewrites a document whose fields stray from canonical form so the audit passes, reports a finding it cannot repair with its fix hint and changes nothing else, and without Go on PATH skips the repair, says why, and does the rest.

<!-- field: traces -->
#### Traces

- [R4.4](#r44)
- [R4.5](#r45)

<!-- ac: AC8 -->
### AC8

In the agent, or with no Go on PATH and a spec tool built at the pin, doctor repairs a document to canonical form; with neither, it skips the repair and says why. doctor names the release the repository vendored, or a repository that vendored none with the upgrade profile as the fix, and vendors nothing.

<!-- field: traces -->
#### Traces

- [R4.5](#r45)
- [R4.7](#r47)

<!-- ac: AC9 -->
### AC9

On a system and on a subsystem repository an older release laid down, the upgrade profile vendors the installed release, repairs the repository at it, moves the pin, and rewrites the skills of the agents it uses, seeding no tracker and filing no issue; a second run changes nothing. With no release installed it refuses and writes nothing; in a directory that is not a program repository it ends refused after the vendoring; in a repository that uses no agent it ends with the skills not installed and the rest done.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)
