<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# This Process and the Scrum Guide

This page is for a reader who knows the [Scrum Guide
(2020)](https://scrumguides.org/scrum-guide.html) and wants to check one
claim: this process is Scrum. Every accountability, event, artifact, and
commitment the Guide defines has a named counterpart here, or a deviation
stated with its reason. The tables below are the whole claim, including the
work-item hierarchy and link types agile tools use; the sections after them
explain the deviations. [process.md](process.md) shows the same
process in Scrum's three time horizons, and this page is its index.

Two facts frame every row. First, this process runs a program of several
Scrum teams, not one: as in Nexus, a program-level goal is broken into each
team's goals, and a level above the teams exists (the system, owned by the
technical program manager). Second, scripts and agents do the
administrative work around each event, so a person is asked only for the
judgments the tracker cannot hold (design.md D5). Neither changes what an
event is for; both change who does the bookkeeping.

## Accountabilities

Table 1: The Scrum Team's accountabilities.

| Scrum Guide | Here | Fit |
|---|---|---|
| Scrum Team: small, cross-functional, self-managing | A level's owner and the people who pull its issues; nobody assigns work, people pop issues for themselves (srd041 R9.1) | Aligned |
| Developers: create the Increment, own the Sprint Backlog, hold to the Definition of Done, adapt daily | Component owners: turn each selected story into issues with tasks, pull and build them, review every merge, release (roles.md developer) | Aligned |
| Product Owner: maximizes value, owns the Product Backlog and the Product Goal, orders the backlog | Subsystem owner: writes the team's epics, features, and user stories, orders the Product Backlog by rank, selects stories at Sprint Planning, judges the Increment (roles.md product_owner) | Aligned |
| Scrum Master: accountable for the team's effectiveness, the events, and removing impediments | Owns the test subsystem: environments, suites, evidence, and audits; facilitates the Retrospective (roles.md scrum_master) | Deviation, explained below |
| (none: the Guide has no role above one team) | Technical program manager: owns the system level, its themes, initiatives, and cross-team epics, the label schema, and the policy (roles.md technical_program_manager) | Addition, explained below |

## Events

Table 2: The Scrum events.

| Scrum Guide | Here | Fit |
|---|---|---|
| The Sprint: fixed length, one month or less | A three-week period ending in a road-map release: a group milestone and one type::sprint issue holding the exit checklist (srd001 R7) | Aligned |
| Sprint Planning: why, what, and how for the Sprint | tasks (srd022) files each team's Sprint Goal; plan (srd031 R6) lists the ranked Product Backlog with velocity and writes the stories the Product Owner chooses onto the Sprint Goal; tasks turns each story into issues | Aligned |
| Daily Scrum: Developers inspect progress toward the Sprint Goal and adapt the Sprint Backlog | daily-scrum (srd020): the facts are shown, and only blockers and interrupts are asked; until it is built, scrum-master standup (srd010) | Aligned, with status recitation removed |
| Sprint Review: inspect the Increment with stakeholders and adapt the Product Backlog | sprint-review (srd032): each Sprint Goal's ledger and evidence shown, the Product Owner's verdict recorded, follow-up items pushed; until it is built, the sprint-end issue (srd038), which also records which stories are done | Aligned |
| Sprint Retrospective: plan ways to increase quality and effectiveness | sprint-retrospective (srd033): the period's metrics read, at most three plane::control issues that change the policy; until it is built, a policy change is a plane::control issue | Aligned |
| Backlog refinement: an ongoing activity, not an event | A change to a story, its rank, or a document above it, merged through the audit gate (srd045 R5.1, srd031 R6.2) | Aligned |

## Artifacts and commitments

Table 3: Each artifact and the commitment that belongs to it.

| Scrum Guide | Here | Fit |
|---|---|---|
| Product Backlog | A team's agreed user stories that are not done, with the features and epics above them (srd045 R5.1) | Aligned |
| Product Backlog order | Each story's rank, set by the Product Owner; plan's ranking proposes, the owner decides (srd045 R5.1, srd031 R6.4) | Aligned |
| Product Goal | An initiative: a multi-team outcome for several quarters (srd034 R1) | Aligned |
| Sprint Backlog | The stories the Sprint Goal lists, and the issues that implement them (srd034 R3.5) | Aligned |
| Sprint Goal | The Sprint Goal issue each team receives for the sprint, citing the initiative items it serves and the environment where they count (srd034 R3.2) | Aligned; one per team, as in Nexus |
| Increment | A release tag, and the pinned set of releases promoted together (srd008, srd015) | Aligned |
| Definition of Done | The release gate in control/policy.md (definition_of_done) and the reviewed merge today; every listed story's criteria passing in their named environment, recorded as evidence, once track-goals runs (srd008, srd019) | Aligned, and stricter |

## The work-item hierarchy

Agile tools break strategy into work in the same levels. Each level here is
a document the corpus gate checks, with at most one parent (design.md
hierarchy_links, srd045); the issue and its tasks are the tracker's. How
this hierarchy relates to the architecture, and where the two meet, is
[plan-vs-architecture.md](plan-vs-architecture.md).

Table 4: The vertical hierarchy.

| Agile level | Here | Where | Timeframe |
|---|---|---|---|
| Theme | Theme document (design.md theme) | system repository | months to years |
| Initiative | Initiative document, the Product Goal; an objective document from before 04.0 is read as one (srd034) | system repository | several quarters |
| Epic | Epic document, child of an initiative (design.md epic) | system repository for a cross-team epic, owned by the technical program manager; a team's repository, owned by its Product Owner, when every component it names is that team's | several sprints |
| Feature (optional) | Feature document, child of an epic (design.md feature) | the team's scrum repository | two to four sprints |
| User Story | Story document: as a, I want, so that; points; rank; its own acceptance criteria; the SRD items it proves (design.md story) | the team's scrum repository | one sprint |
| Sprint backlog item | The issue that implements a story, with an Implements line (srd007 R6.5) | tracker | days |
| Task | A task inside the issue (srd001 R9.2) | tracker | hours to days |

Progress rolls up through parent links from story points: a feature, epic,
or initiative reports the points of its done stories over all of them
(srd045 R3.3). A cross-team epic counts its stories in every team
repository the subsystem SRDs name, and names a repository it cannot read
(srd045 R3.4).

A scrum owns an area, its subsystem and the components its SRD lists,
while planned work is a separate tree. Each team's Product Owner writes
the stories for its part of a cross-team epic in the team's repository,
with the epic as parent by pinned URL, naming only components its area
lists; backlog check warns when a document strays outside its area
(srd045 R6.5, R6.6). goals plan routes a release item to every team whose
components its agreed epics name, unless the item's delivered_by names one
(srd022 R3.1).

A team's scrum repository is where it plans and journals: its features and
stories, its Sprint Goal for each release, its sprint-end summaries, its
weekly status reports, and its team rollup. By default it is the team's
first component repository, so planning and code live together. A team
whose work spans several code repositories can name a repository of its
own for planning with `scrum_repository` in its subsystem SRD's context;
its code repositories then hold only code, and an issue there implements a
story in the scrum repository through its pinned `Implements:` link
(design.md subsystem_srd).

The levels repeat. A program can be one team of a larger program: the
larger program's subsystem SRD names, as that team's scrum repository, the
smaller program's system repository. To the level above it is a team: it
receives one Sprint Goal per release, its program rollup is that team's
rollup, and its initiatives and epics pin the upper documents they serve.
Inside it is a whole program with its own TPM, teams, and site. Each site
shows its own level in full and only the interface of the level below: the
upper site counts and links the documents the sub-program serves it with,
read from the sub-program's published site, and points to the
sub-program's site for its teams and stories; the sub-program's site links
up to the parent's. A program declares its site and its parent in
`control/program.md` (config format program; srd048 R8.7).

## Links

Table 5: The horizontal links, on issues as body lines and on documents as context items (srd007 R6, R7).

| Agile link | Here | Blocks the item? |
|---|---|---|
| Parent / child | parent context item; an issue's parent is its story through Implements | no |
| Blocks / is blocked by | Blocks: and Blocked-by:, one link from either end; Upstream counts as Blocked-by | yes, sets waiting |
| Depends on (concurrent) | Depends-on: | no, reported |
| Relates to | Relates-to: | no, reported |
| Duplicates | Duplicates:; the duplicate closes as duplicate, a duplicate document is retired | no |
| (release dependency) | Requires: ARTIFACT >= VERSION | yes, sets waiting |

## Complementary practices

The Scrum Guide leaves estimation, epics, and forecasting to the team. The
process uses the common practices, and keeps each one's usual meaning.

Table 6: Practices outside the Guide.

| Practice | Here |
|---|---|
| Epics | Epic documents, children of initiatives (srd045). The tracker's grouping of issues is a tracking issue, type::tracking (srd001 R6) |
| Story points | On user stories, relative and never of a person (design.md story) |
| Velocity | Story points of a team's stories done per sprint, read from the ledger or, before track-goals, from the sprint-end reports (srd045 R5.3) |
| Burn-up and release forecast | Regenerated from story points and the ledger beside the dashboard, never a baseline (srd045 R7, srd018 R11) |
| User stories | Story documents with their statement, points, rank, and acceptance criteria naming a test and an environment (design.md story) |

## Deviations and their reasons

### The Scrum Master owns the test subsystem

The Guide's Scrum Master is accountable for the team's effectiveness:
coaching, making the events happen, and removing impediments. Most of a
Scrum Master's day in practice is not that; it is administration: running
standups, updating the board, counting points, chasing blockers. Here that
administration is scripts. The events run as skills, the board is the
tracker, and blockers are a label and a Daily Scrum question that end in
issues (srd020).

What is left is the part of effectiveness that is a system to own rather
than a meeting to run: the environments, suites, and audits that tell the
team whether its work is done. The Scrum Master owns that, the evidence the
Review inspects, and the Retrospective, where the team changes its rules.
Coaching stays a person's work; there is no Scrum Master agent.

### The technical program manager

The Guide describes one team and has no role above it. A program of
several teams needs one: someone owns the system's requirements and
interfaces, writes the program's themes and initiatives, declares each
system release, and files each team's Sprint Goal. Nexus calls the equivalent
integration work the Nexus Integration Team; SAFe splits it between the
Release Train Engineer and Solution Management. Here it is one role, and
it owns clarity and process, not the teams' delivery: it never assigns
work, and it orders the program's backlog the way a Product Owner orders a
team's.

### Stories cite specifications

The Product Owner writes user stories as agile teams do, and each story's
criteria name the test and the environment that prove them. A program also
keeps SRDs, the specifications of its system and subsystems, as many teams
keep PRDs beside their epics; a story cites the SRD items it proves, so a
specification change is traceable to the stories it affects (srd045 R4.2).

## The pillars

Scrum rests on transparency, inspection, and adaptation. Transparency: the
tracker is the one source of truth, every object is reachable from an issue,
and the dashboard and forecasts are regenerated from it rather than kept by
hand. Inspection: a Sprint Goal is met only by evidence recorded on its
ledger, and the Review inspects that ledger. Adaptation: the
Retrospective's output is a change to the versioned policy, and each
sprint's stories are chosen again from the backlog as it now stands.
