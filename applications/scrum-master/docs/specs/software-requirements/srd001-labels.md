---
id: srd001-labels
title: "The label scheme: plane, type, kind, component, and state"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: context | form: "tree" -->
## Context

- plane:
  1. R1
  1. R2
  1. R3
  1. R4
  1. R5
- type:
  1. R6
  1. R7
  1. R8
  1. R9
- kind:
  1. R10
  1. R11
  1. R12
  1. R13
  1. R14
  1. R15
  1. R16
  1. R17
  1. R18
  1. R19
  1. R20
  1. R33
- component:
  1. R21
- op:
  1. R34
- state:
  1. srd041-states

<!-- field: problem -->
## Problem

The process has one unit of work, the issue, and every script decides what
to do with an issue by reading its labels. A program that lets each script
infer an issue's nature from its title or its author gets a different
answer from each script: the wrong people review it, a container is popped
as if it were work, a release is offered to people who do not own the
artifact, and nobody can tell why an issue sits where it does.

We define one label scheme with five axes. Plane says which kind of change
an issue makes and how fast it may move. Type marks the few issues that are
not plain work: tracking issues, the sprint issue, and recurring subscriptions. Kind
says what the work is and what finishing means. Component names the part
of the program the issue belongs to. State says where the issue is and
which one actor may move it (srd041). Admission checks
all five axes on every issue; the other scripts read them and never infer
them.

<!-- req: R1 -->
## R1 - plane::management

<!-- req: R1.1 -->
### R1.1

A plane::management issue changes intent: a road-map release, a system or subsystem SRD (structure and ownership), a theme, initiative, epic, feature, or story document, a sprint issue, or an exit decision. A change to a component SRD, a use case, a test suite, or the architecture is design work, a kind::spec issue (R33), except the system repository's docs/ARCHITECTURE.md and docs/VISION.md: they declare the subsystems and boundaries a plan moves, which planning stops for (design.md D15, srd047), so a change to them is plane::management.

<!-- req: R1.2 -->
### R1.2

Only the program lead or a sub-project lead files a plane::management issue. Scripts file exactly two kinds of plane::management issue: the sprint issue (R7), and the Sprint Goal issues tasks files for a receiving Product Owner (srd022, srd034 R3), since a Product Owner answers a Sprint Goal by selecting stories and writing the requirements they cite.

<!-- req: R1.3 -->
### R1.3

A plane::management issue never enters the backlog. It is assigned by the pop of the lead who works it, the filer or, for a Sprint Goal, the receiving Product Owner, and closed by that lead's merge, or for a Sprint Goal by track-goals when it is met (R30.1).

<!-- req: R1.4 -->
### R1.4

The management plane writes goals and maps. It does not create, move, label, or assign any plane::data issue.

<!-- req: R2 -->
## R2 - plane::control

<!-- req: R2.1 -->
### R2.1

A plane::control issue changes a rule the process runs by: the label schema, the state machine, routing, admission, scheduling, escalation, or the event vocabulary, as stored in the program's policy files.

<!-- req: R2.2 -->
### R2.2

A plane::control issue is closed only by a merge that changes a policy file and that a lead other than the author approved.

<!-- req: R2.3 -->
### R2.3

A plane::control issue never enters the backlog. It is filed by a person and assigned by that person's own pop or by its author.

<!-- req: R2.4 -->
### R2.4

Every script reads rules from the policy files at run time. A rule that exists only in script code is a defect against this requirement.

<!-- req: R3 -->
## R3 - plane::data

<!-- req: R3.1 -->
### R3.1

A plane::data issue is work: something a person or an agent builds, runs, writes, reviews, consumes, or releases. Its kind label (R10–R20) says which.

<!-- req: R3.2 -->
### R3.2

Any member of the program may file a plane::data issue, and the handoff and release scripts create them.

<!-- req: R3.3 -->
### R3.3

An unassigned open plane::data issue that passes admission is in the backlog (R24); anyone in the program may pop it.

<!-- req: R3.4 -->
### R3.4

The data plane runs on the last committed version of the policy files and the last pushed sprint. Absence of management-plane action does not stop or change data-plane flow.

<!-- req: R4 -->
## R4 - The one-way rule

<!-- req: R4.1 -->
### R4.1

Intent flows from the management plane to the control and data planes only as tracker objects the management plane owns: the milestone, the sprint issue, the road-map, and the SRDs.

<!-- req: R4.2 -->
### R4.2

Rules flow from the control plane to the data plane only as policy files that scripts read.

<!-- req: R4.3 -->
### R4.3

State flows up only as tracker state: issues, labels, comments, merges, and tags, read by the reporting scripts. No plane sends status through a channel outside the tracker.

<!-- req: R5 -->
## R5 - Exactly one plane

<!-- req: R5.1 -->
### R5.1

Every issue in the program carries exactly one plane label.

<!-- req: R5.2 -->
### R5.2

An issue with no plane label or with more than one is labelled needs-triage by admission (R23) and stays out of the backlog.

<!-- req: R5.3 -->
### R5.3

The plane of an issue follows from what it changes, not from who filed it: an issue that edits a policy file is plane::control whoever files it.

<!-- req: R6 -->
## R6 - type::tracking

<!-- req: R6.1 -->
### R6.1

A tracking issue is an issue labelled type::tracking whose description holds a task list of member issues, one line each, written as cross-project references so members may live in any repository of the program.

<!-- req: R6.2 -->
### R6.2

A tracking issue is never popped into a worktree and never has a merge of its own. A pop request on a tracking issue is refused and lists its open members.

<!-- req: R6.3 -->
### R6.3

A tracking issue closes when it has at least one member and every member is closed. A tracking issue with no members does not close automatically.

<!-- req: R6.4 -->
### R6.4

A tracking issue may carry a later milestone than its members, so a body of work spanning several sprints shows under the sprint it is expected to finish in.

<!-- req: R6.5 -->
### R6.5

A tracking issue that spans components lives in the program's own repository; a tracking issue inside one component lives in that component's repository.

<!-- req: R6.6 -->
### R6.6

type::epic, the value's earlier name, is read as an alias of type::tracking: an issue labelled type::epic is a tracking issue, and a program's control files that list epic keep their meaning. scrum-master writes type::tracking on every issue it files, and scrum-master issue labels --migrate relabels issues that still carry type::epic (srd013 R6.3).

<!-- req: R7 -->
## R7 - type::sprint

<!-- req: R7.1 -->
### R7.1

A sprint is a group milestone named for the release it plans (srd031 R4.2), with its ISO weeks in its description, and exactly one issue labelled type::sprint and plane::management in the system repository.

<!-- req: R7.2 -->
### R7.2

The sprint issue's description is the exit checklist, one item per Sprint Goal of the release, copied from the road-map release when plan records it. Each item is ticked from that Sprint Goal's ledger, never from its issues.

<!-- req: R7.3 -->
### R7.3

Only the program lead ticks items on the exit checklist and closes the sprint issue. No script ticks or closes it.

<!-- req: R7.4 -->
### R7.4

Pushing a sprint that already exists updates its milestone dates and checklist text and creates nothing new.

<!-- req: R8 -->
## R8 - type::recurring

<!-- req: R8.1 -->
### R8.1

A recurring issue is a consumer's standing subscription to exactly one artifact. It lives in the consumer's repository, carries the consumer's component, and holds a Requires line naming the artifact and the version the consumer is on (srd007 R2).

<!-- req: R8.2 -->
### R8.2

A release of the artifact posts a release event on the recurring issue (srd006 R2). A breaking release also sets its state to needs-run (R29); a compatible release does not change its state.

<!-- req: R8.3 -->
### R8.3

Finishing a run closes the recurring issue and opens the next instance with the same title, labels, and body, with the run appended to a Previous Runs section and the Requires line updated to the version consumed.

<!-- req: R8.4 -->
### R8.4

The set of open recurring issues for an artifact is the list of its consumers. Routing may derive consumers from it instead of from a hand-written map.

<!-- req: R9 -->
## R9 - Plain issue

<!-- req: R9.1 -->
### R9.1

An issue with no type label is a plain issue: one commitment, with one component, one kind, one plane, and a milestone.

<!-- req: R9.2 -->
### R9.2

A plain issue's steps are child tasks under it. Tasks are not issues; they carry no labels of their own and are never in the backlog.

<!-- req: R9.3 -->
### R9.3

A plain issue is worked in one worktree and closed, with all its tasks, by one merge.

<!-- req: R9.4 -->
### R9.4

A plain issue may be a member of at most one tracking issue.

<!-- req: R10 -->
## R10 - kind::build

<!-- req: R10.1 -->
### R10.1

Builds or changes software. Finished when the merged branch's tests pass and the actual size is recorded against the estimate.

<!-- req: R10.2 -->
### R10.2

Filed by a person. Plane data (R3). Enters the backlog when unassigned and admitted.

<!-- req: R11 -->
## R11 - kind::experiment

<!-- req: R11.1 -->
### R11.1

Runs a procedure to produce a measurement. Finished when the result and the command that reproduces it are committed in the worktree.

<!-- req: R11.2 -->
### R11.2

Filed by a person. Plane data (R3). Enters the backlog.

<!-- req: R12 -->
## R12 - kind::research

<!-- req: R12.1 -->
### R12.1

Produces a written result: a report, a section, a survey, or an analysis. Finished when the document is committed with a Result section stating what was found or decided.

<!-- req: R12.2 -->
### R12.2

Filed by a person. Plane data (R3). Enters the backlog.

<!-- req: R13 -->
## R13 - kind::review

<!-- req: R13.1 -->
### R13.1

Reads another person's artifact and responds. Finished when a response is recorded on the artifact or in the worktree and the issue closes with a verdict.

<!-- req: R13.2 -->
### R13.2

Filed by a person or by handoff when an artifact's producer asked for review. Plane data (R3). Enters the backlog.

<!-- req: R14 -->
## R14 - kind::consume

<!-- req: R14.1 -->
### R14.1

Adapts a consumer to a released artifact. Finished as build is finished, with the Requires line updated to the consumed version (srd007 R2).

<!-- req: R14.2 -->
### R14.2

Created by handoff on a breaking release, unassigned. Plane data (R3). Enters the backlog.

<!-- req: R15 -->
## R15 - kind::handoff

<!-- req: R15.1 -->
### R15.1

Decides what an arriving artifact means for the receiving side. Finished when the decision and its reasons are recorded in the receiving repository.

<!-- req: R15.2 -->
### R15.2

Created by handoff when a repository citing the released interface asked for a handoff decision (srd014 R9.1, from 06.0). Plane data (R3), or plane management (R1) when the decision changes a road-map release or an SRD. Enters the backlog only when plane data.

<!-- req: R16 -->
## R16 - kind::release

<!-- req: R16.1 -->
### R16.1

Publishes a version of an artifact for others to depend on. Finished when the artifact's gate passed and a version tag exists; the tag is what handoff reads.

<!-- req: R16.2 -->
### R16.2

Filed by the artifact's producer. Plane data (R3). Does not enter the backlog; a release is cut by the side that owns the artifact.

<!-- req: R17 -->
## R17 - kind::defect

<!-- req: R17.1 -->
### R17.1

Reports that a released or merged artifact is wrong. Finished when the producer fixes it or declines it with a reason.

<!-- req: R17.2 -->
### R17.2

Filed only by a person, in the producer's repository. Plane data (R3). Enters the backlog of the producer's component after the producer accepts it.

<!-- req: R18 -->
## R18 - kind::request

<!-- req: R18.1 -->
### R18.1

Asks a producer for something the artifact does not yet provide. Finished when the producer delivers it or declines it with a reason.

<!-- req: R18.2 -->
### R18.2

Filed only by a person, in the producer's repository. Plane data (R3). Enters the backlog of the producer's component after the producer accepts it.

<!-- req: R19 -->
## R19 - kind::question

<!-- req: R19.1 -->
### R19.1

Asks a producer something that needs an answer, not a change. Finished when the answer is recorded and the asker closes it.

<!-- req: R19.2 -->
### R19.2

Filed only by a person. Plane data (R3). Does not enter the backlog; it is addressed to the producer, who answers it.

<!-- req: R19.3 -->
### R19.3

A question that needs no tracking is a question event comment (srd006), not an issue. The kind is for questions that block work.

<!-- req: R20 -->
## R20 - Rules across kinds

<!-- req: R20.1 -->
### R20.1

Every plane::data issue carries exactly one kind label. An issue with none or more than one is labelled needs-triage (R23).

<!-- req: R20.2 -->
### R20.2

No script creates a defect, request, or question. A script cannot know that something is wrong or missing.

<!-- req: R20.3 -->
### R20.3

A producer may decline a defect, request, or question. Declining closes the issue with the reason as its last comment and is not a failure of the process.

<!-- req: R20.4 -->
### R20.4

The kind routes pull: it decides who takes an issue, whether it is offered, and how it is counted. It does not change how the work runs. Every kind is popped, built by implement against the issue's Requirements and Acceptance Criteria, and closed by one merge or one recorded answer.

<!-- req: R21 -->
## R21 - The component axis

<!-- req: R21.1 -->
### R21.1

Component values come from the subsystem SRDs in the system repository: each component a subsystem SRD declares (design.md subsystem_srd) is one label component::<name>. The system SRD scrum-master init writes declares the system repository the same way (srd043 R1.1). This corpus defines no value, and no policy file lists them a second time.

<!-- req: R21.2 -->
### R21.2

Every issue other than a sprint issue carries exactly one component label. An issue with none or with more than one is labelled needs-triage by admission (R23) and stays out of the backlog. Until a subsystem SRD declares a component, as in a program scrum-master init has just laid down, there is no value to choose, and an issue with no component label is admitted.

<!-- req: R21.3 -->
### R21.3

Routing reads the component to choose the repository a downstream issue is created in, from the repository URL its subsystem SRD declares; track-goals reads it to group evidence.

<!-- req: R21.4 -->
### R21.4

Reporting groups progress, cycle time, and evidence age by component, never by person.

<!-- req: R21.5 -->
### R21.5

A component label that no subsystem SRD declares is treated as missing.

<!-- req: R22 -->
## R22 - open

<!-- req: R22.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R1.1"] -->
### R22.1

Retired in 01.2 (design.md D12).

<!-- req: R23 -->
## R23 - needs-triage

<!-- req: R23.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R2.1"] -->
### R23.1

Retired in 01.2 (design.md D12).

<!-- req: R23.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R2.2"] -->
### R23.2

Retired in 01.2 (design.md D12).

<!-- req: R23.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R2.3"] -->
### R23.3

Retired in 01.2 (design.md D12).

<!-- req: R24 -->
## R24 - ready

<!-- req: R24.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R3.1"] -->
### R24.1

Retired in 01.2 (design.md D12).

<!-- req: R24.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R3.2"] -->
### R24.2

Retired in 01.2 (design.md D12).

<!-- req: R25 -->
## R25 - taken

<!-- req: R25.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R4.1"] -->
### R25.1

Retired in 01.2 (design.md D12).

<!-- req: R25.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R4.2"] -->
### R25.2

Retired in 01.2 (design.md D12).

<!-- req: R26 -->
## R26 - in-progress

<!-- req: R26.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R5.1"] -->
### R26.1

Retired in 01.2 (design.md D12).

<!-- req: R26.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R5.2"] -->
### R26.2

Retired in 01.2 (design.md D12).

<!-- req: R27 -->
## R27 - waiting

<!-- req: R27.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R10.1"] -->
### R27.1

Retired in 01.2 (design.md D12).

<!-- req: R27.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R10.1"] -->
### R27.2

Retired in 01.2 (design.md D12).

<!-- req: R27.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R10.1"] -->
### R27.3

Retired in 01.2 (design.md D12).

<!-- req: R28 -->
## R28 - blocked

<!-- req: R28.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R6.1"] -->
### R28.1

Retired in 01.2 (design.md D12).

<!-- req: R28.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R6.2"] -->
### R28.2

Retired in 01.2 (design.md D12).

<!-- req: R29 -->
## R29 - needs-run

<!-- req: R29.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R10.2"] -->
### R29.1

Retired in 01.2 (design.md D12).

<!-- req: R29.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R10.2"] -->
### R29.2

Retired in 01.2 (design.md D12).

<!-- req: R30 -->
## R30 - closed

<!-- req: R30.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R7.1"] -->
### R30.1

Retired in 01.2 (design.md D12).

<!-- req: R30.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R7.2"] -->
### R30.2

Retired in 01.2 (design.md D12).

<!-- req: R31 -->
## R31 - Transitions

<!-- req: R31.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R8.1"] -->
### R31.1

Retired in 01.2 (design.md D12).

<!-- req: R31.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R8.2"] -->
### R31.2

Retired in 01.2 (design.md D12).

<!-- req: R31.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R8.3"] -->
### R31.3

Retired in 01.2 (design.md D12).

<!-- req: R32 -->
## R32 - Invariants

<!-- req: R32.1 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R9.1"] -->
### R32.1

Retired in 01.2 (design.md D12).

<!-- req: R32.2 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R9.2"] -->
### R32.2

Retired in 01.2 (design.md D12).

<!-- req: R32.3 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R9.3"] -->
### R32.3

Retired in 01.2 (design.md D12).

<!-- req: R32.4 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states R9.4"] -->
### R32.4

Retired in 01.2 (design.md D12).

<!-- req: R33 -->
## R33 - kind::spec

<!-- req: R33.1 -->
### R33.1

Changes requirements or design documents under docs/: a component SRD, a use case, a test suite, the architecture, an engineering guideline, or a constitution. Requirements change during design as well as during planning, so this is ordinary work.

<!-- req: R33.2 -->
### R33.2

Any member of the program files it, and it is in the backlog like every other plane::data issue (R3).

<!-- req: R33.3 -->
### R33.3

It carries Requirements and Acceptance Criteria, and its branch changes docs/ and merges only when the corpus audit passes (srd034 R2.2).

<!-- req: R34 -->
## R34 - The op axis

<!-- req: R34.1 -->
### R34.1

Every issue scrum-master files carries one op::<operation> label naming the operation that filed it: op::file (scrum-master issue file), op::plan (scrum-master plan push), op::goals (scrum-master goals push), op::onboard (scrum-master onboard), op::report (an actionable signal from scrum-master reports), op::status (a weekly status issue), and op::recur (the next instance scrum-master issue recur opens).

<!-- req: R34.2 -->
### R34.2

The label is set when the issue is created and never changed: an issue outlives the operation that filed it, so the operation is a label and the issue's ID stays opaque. A next instance is filed by recur, so it carries op::recur in place of the label its previous instance had.

<!-- req: R34.3 -->
### R34.3

Admission fails an issue with more than one op:: label and requires none, so issues filed before the axis existed are still admitted. A program whose control/labels.md predates the axis gets it from scrum-master doctor (srd043 R4.2), and its labels in the tracker from scrum-master issue labels.

<!-- field: goals -->
## Goals

- G1: Every issue declares its plane, kind, and component with exactly one label each; type and state are set only where they apply.
- G2: Each plane has its own filing and closing rules, and information crosses planes in one direction per kind.
- G3: The objects that are not plain work are recognizable from one type label, and the hierarchy stays flat at tracking issue, issue, and task.
- G4: Every kind of work travels the same inner loop (specify, implement, and the reviewed merge); the kind decides who pulls it and what finishing means, not how it runs.
- G5: The process reads component values without defining them.

<!-- field: non_goals -->
## Non-goals

- Defining the content of any plane's files; road-maps and SRDs belong to each repository.
- Permissions in the tracker itself; the plane rules are enforced by admission and review, not by tracker roles.
- A fourth plane. Telemetry and dashboards are views over tracker state, not a plane.
- Tracking issues as a native tracker object (a tracker's epics); the process uses only features of the free tier.
- A goal type; a Sprint Goal is an ordinary issue whose events are its ledger (srd006 R11).
- Nesting tracking issues or nesting worktrees.
- Estimating effort per kind.
- A separate workflow per kind; every kind ends in one merge or one recorded answer on the same issue loop.
- Sub-kinds; a program that needs finer distinctions uses its own labels outside the process axes.
- Naming, ordering, or describing any component.
- Ownership of a component by a person; people declare which components they will pull, the component does not name people.
- Hierarchies of components.
- Priority or ordering; scheduling is a policy, not a state.
- States of a branch under review (draft, review, approved); the issue carries only its approval comment (srd013 R5.6).
- Per-component state machines; one machine serves every component.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A management-plane issue filed by a non-lead, or a script-created management issue other than the sprint issue or a tasks Sprint Goal issue, is flagged by admission; no management-plane issue appears in the backlog; each is assigned only by the pop of the lead who works it; no management-plane action creates, labels, or assigns a data-plane issue.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

A control-plane issue closes only through a merge that changes a policy file with a second lead's approval; control-plane issues never appear in the backlog; every script's rules resolve to a policy-file entry.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 -->
### AC3

A data-plane issue filed by any member or created by handoff or release, unassigned and admitted, appears in the backlog for matching roles; with no management-plane action for a full sprint, data-plane issues continue to be created, offered, taken, and closed.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)

<!-- ac: AC4 -->
### AC4

Every value a script reads to decide routing, admission, or scheduling comes from a tracker object or a policy file; no status is read from or written to anything outside the tracker.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)
- [R4.3](#r43)

<!-- ac: AC5 -->
### AC5

An issue with zero or two plane labels is labelled needs-triage and is absent from the backlog; an issue that edits a policy file and carries plane::data is flagged regardless of its author.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC6 -->
### AC6

A tracking issue with cross-project members shows their progress in its task list; popping it is refused with its open members listed; it closes when its last member closes and not before; an empty tracking issue stays open; a cross-component tracking issue lives in the program repository; an issue labelled type::epic is treated as a tracking issue.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)
- [R6.4](#r64)
- [R6.5](#r65)
- [R6.6](#r66)

<!-- ac: AC7 -->
### AC7

Pushing a sprint creates one milestone and one sprint issue whose checklist matches the release's Sprint Goals; pushing it again changes only dates and text; no script ticks or closes the sprint issue.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
- [R7.4](#r74)

<!-- ac: AC8 -->
### AC8

A breaking release marks each subscribing recurring issue needs-run and a compatible release only comments; finishing a run opens the next instance with the run logged and the version updated; listing open recurring issues for an artifact yields its consumers.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)
- [R8.4](#r84)

<!-- ac: AC9 -->
### AC9

A plain issue carries one component, kind, plane, and milestone; its tasks are never in the backlog; one merge closes it with its tasks; no plain issue appears in two tracking issues.

<!-- field: traces -->
#### Traces

- [R9.1](#r91)
- [R9.2](#r92)
- [R9.3](#r93)
- [R9.4](#r94)

<!-- ac: AC10 -->
### AC10

Build, experiment, and research issues are filed by people, ready when unassigned, and close only when their finishing artifact (passing tests and size, result with reproduction command, document with a Result section) is in the merged branch.

<!-- field: traces -->
#### Traces

- [R10.1](#r101)
- [R10.2](#r102)
- [R11.1](#r111)
- [R11.2](#r112)
- [R12.1](#r121)
- [R12.2](#r122)

<!-- ac: AC11 -->
### AC11

Review issues filed by a person or by handoff close with a verdict; consume issues created by handoff on a breaking release are unassigned and ready and close with the Requires line at the consumed version.

<!-- field: traces -->
#### Traces

- [R13.1](#r131)
- [R13.2](#r132)
- [R14.1](#r141)
- [R14.2](#r142)

<!-- ac: AC12 -->
### AC12

A handoff issue records a decision in the receiving repository and is ready only when plane data; a release issue is never ready and is finished only when the gate passed and the tag exists.

<!-- field: traces -->
#### Traces

- [R15.1](#r151)
- [R15.2](#r152)
- [R16.1](#r161)
- [R16.2](#r162)

<!-- ac: AC13 -->
### AC13

Defect and request issues exist only when a person filed them in the producer's repository, enter the producer's backlog only after acceptance, and close as delivered or declined with a reason.

<!-- field: traces -->
#### Traces

- [R17.1](#r171)
- [R17.2](#r172)
- [R18.1](#r181)
- [R18.2](#r182)
- [R20.3](#r203)

<!-- ac: AC14 -->
### AC14

A question issue is filed by a person, never ready, and closed by its asker after an answer is recorded; untracked questions are event comments.

<!-- field: traces -->
#### Traces

- [R19.1](#r191)
- [R19.2](#r192)
- [R19.3](#r193)

<!-- ac: AC15 -->
### AC15

Every data-plane issue has exactly one kind or is labelled needs-triage; no defect, request, or question has a script as its author; issues of every kind are popped, built by implement, and closed the same way, and differ only in who is offered them and how they are counted.

<!-- field: traces -->
#### Traces

- [R20.1](#r201)
- [R20.2](#r202)
- [R20.4](#r204)

<!-- ac: AC16 -->
### AC16

With a program list of components, every issue except the sprint issue has exactly one listed component label or is labelled needs-triage and absent from the backlog; an unlisted component label counts as missing.

<!-- field: traces -->
#### Traces

- [R21.1](#r211)
- [R21.2](#r212)
- [R21.5](#r215)

<!-- ac: AC17 -->
### AC17

A downstream issue is created in the repository its component maps to; component values come only from subsystem SRDs; reports group by component and contain no per-person metric.

<!-- field: traces -->
#### Traces

- [R21.3](#r213)
- [R21.4](#r214)

<!-- ac: AC18 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC1"] -->
### AC18

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R22.1](#r221)
- [R23.1](#r231)
- [R23.2](#r232)
- [R23.3](#r233)

<!-- ac: AC19 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC2"] -->
### AC19

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R24.1](#r241)
- [R24.2](#r242)
- [R25.1](#r251)
- [R25.2](#r252)
- [R26.1](#r261)
- [R26.2](#r262)

<!-- ac: AC20 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC3"] -->
### AC20

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R27.1](#r271)
- [R27.2](#r272)
- [R27.3](#r273)
- [R28.1](#r281)
- [R28.2](#r282)

<!-- ac: AC21 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC6"] -->
### AC21

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R29.1](#r291)
- [R29.2](#r292)

<!-- ac: AC22 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC4"] -->
### AC22

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R30.1](#r301)
- [R30.2](#r302)
- [R32.3](#r323)

<!-- ac: AC23 | status: retired | retired_in: 1.2 | replaced_by: ["srd041-states AC5"] -->
### AC23

Retired in 01.2 (design.md D12).

<!-- field: traces -->
#### Traces

- [R31.1](#r311)
- [R31.2](#r312)
- [R31.3](#r313)
- [R32.1](#r321)
- [R32.2](#r322)
- [R32.4](#r324)

<!-- ac: AC24 -->
### AC24

A kind::spec issue filed by any member is admitted into the backlog with Requirements and Acceptance Criteria, and its branch changing docs/ merges only when the corpus audit passes.

<!-- field: traces -->
#### Traces

- [R33.1](#r331)
- [R33.2](#r332)
- [R33.3](#r333)

<!-- ac: AC25 -->
### AC25

Each operation's filed issue carries its op:: label, a recurred instance carries op::recur and no other op:: label, and an issue with two op:: labels fails admission while one with none is admitted.

<!-- field: traces -->
#### Traces

- [R34.1](#r341)
- [R34.2](#r342)
- [R34.3](#r343)
