---
id: srd041-states
title: "States: where an issue is and who may move it"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

| from | to | fired_by | assignee | condition |
| --- | --- | --- | --- | --- |
| open | needs-triage | lint | none | an axis label is missing, doubled, or unlisted |
| open | ready | lint | none | admitted, unassigned, plane data, backlog kind |
| open | taken | pop | set | the author takes the issue as they file it |
| needs-triage | ready | lint | none | labels fixed, admitted, unassigned, backlog kind |
| needs-triage | taken | lint | none | labels fixed on an issue its author already took |
| ready | taken | pop | set | a person pops it |
| taken | ready | person | clear | the assignee releases it back |
| taken | in-progress | person | none | the assignee starts it |
| in-progress | taken | person | none | the assignee stops it |
| any open | +waiting | track-goals | none | a Blocked-by, Upstream, or Requires dependency is unsatisfied, or a dependency or Cites line is unresolvable (srd007 R3.1) |
| +waiting | -waiting | track-goals | none | every dependency is satisfied |
| taken or in-progress | +blocked | person | none | the assignee states a blocker in a comment |
| +blocked | -blocked | person | none | the assignee clears it |
| open recurring | +needs-run | handoff | none | a breaking release of its artifact is tagged |
| any open | closed | merge | none | a merge that closes the issue lands, whatever state the issue is in (R7.1) |
| open Sprint Goal | closed | track-goals | none | its ledger replays to met and the met event is posted; a Sprint Goal is a Sprint Goal |
| any open | closed | person | none | declined, duplicate (with a Duplicates line naming the original, srd007 R6.4), or answered, with the reason as the last comment |
| closed | open | person | none | reopened; admission runs again |

<!-- field: problem -->
## Problem

An issue's labels say what it is (srd001); its state says where it is and
which one actor may move it next. A program that lets any script move an
issue gets an issue nobody can explain: assigned by a script, closed by a
checklist, or offered to people while it waits on something. This SRD
states each state, how it is represented in the tracker, the table of
transitions with the one actor that fires each, and the invariants every
script keeps.

<!-- req: R1 -->
## R1 - open

<!-- req: R1.1 -->
### R1.1

An issue that exists and has not yet been checked by admission. It has no state label. Admission runs on every new issue, so open is transient for a data issue; an issue of another plane has no admitted state (ready is plane data, R3.1) and stays open until it is popped or closed.

<!-- req: R2 -->
## R2 - needs-triage

<!-- req: R2.1 -->
### R2.1

Label needs-triage. Set by admission when an issue is missing a required axis label, carries two values on one axis, or uses a value the program does not list (srd001 R5, R20, R21).

<!-- req: R2.2 -->
### R2.2

Cleared only by admission, when a later check passes. A person fixes the labels; admission clears the flag.

<!-- req: R2.3 -->
### R2.3

An issue in needs-triage is never in the backlog.

<!-- req: R3 -->
## R3 - ready

<!-- req: R3.1 -->
### R3.1

Open, unassigned, admitted, plane data, and of a kind that enters the backlog (srd001 R10 to R20). Represented by those tracker fields; there is no ready label.

<!-- req: R3.2 -->
### R3.2

A ready issue may be popped by anyone in the program. No script offers it to a person or assigns it.

<!-- req: R4 -->
## R4 - taken

<!-- req: R4.1 -->
### R4.1

Open and assigned to exactly one person, with no status label. Represented by the assignee field.

<!-- req: R4.2 -->
### R4.2

A person may hold an issue in taken without starting it. It is no longer ready.

<!-- req: R5 -->
## R5 - in-progress

<!-- req: R5.1 -->
### R5.1

Label status::in-progress on a taken issue. Set and cleared by the assignee.

<!-- req: R5.2 -->
### R5.2

No script caps how many issues a person has in progress. How much a team carries at once is its own working agreement, judged at the retrospective from evidence age and cycle time.

<!-- req: R6 -->
## R6 - blocked

<!-- req: R6.1 -->
### R6.1

Label blocked. An overlay state on a taken or in-progress issue, set and cleared by its assignee, with a comment saying what it waits for and from whom.

<!-- req: R6.2 -->
### R6.2

Blocked is a person's statement about something the tracker cannot see; waiting is track-goals' statement about something it can. A dependency the tracker can express is a link, not a block.

<!-- req: R7 -->
## R7 - closed

<!-- req: R7.1 -->
### R7.1

The tracker's closed state. Reached by the merge of a request that closes the issue, by a person declining it with a reason as the last comment, or, for a Sprint Goal issue, by track-goals when its ledger replays to met (srd019).

<!-- req: R7.2 -->
### R7.2

Reopening is a person's action and returns the issue to open, so admission checks it again.

<!-- req: R8 -->
## R8 - Transitions

<!-- req: R8.1 -->
### R8.1

The transitions table is the complete state machine, held as data in control/states.md (config format states): scrum-master embeds the default — the table this document shows — writes it at init, and the beads tracker reads the program's own rows from main, so a program extends its table, an overlay row of its own among them, without a scrum-master change (GH-130). A script that is asked to make a change the rows do not list refuses and reports the attempted transition, and a file that does not parse refuses every transition with its reason.

<!-- req: R8.2 -->
### R8.2

Each row names the actor that fires it, one of person, pop, merge, track-goals, handoff, or lint. pop is the person's own implement. lint is admission.

<!-- req: R8.3 -->
### R8.3

Each row states its effect on the assignee, one of none, set, or clear.

<!-- req: R9 -->
## R9 - Invariants

<!-- req: R9.1 -->
### R9.1

Only pop sets an assignee, and pop assigns only the person who runs it.

<!-- req: R9.2 -->
### R9.2

Only a person clears their own assignment. No script changes an assignee except pop.

<!-- req: R9.3 -->
### R9.3

Only a merge, a person declining with a reason, or track-goals on a met Sprint Goal closes an issue.

<!-- req: R9.4 -->
### R9.4

Only track-goals sets or clears waiting, and only admission sets or clears needs-triage.

<!-- req: R10 -->
## R10 - Direction

<!-- req: R10.1 -->
### R10.1

Label waiting, an overlay on ready, taken, or in-progress, is set and cleared only by track-goals (srd019) from the issue's Upstream, Requires, and Cites lines (srd007): set while a dependency is unsatisfied, cleared when all are satisfied, and reported as unblocked when cleared.

<!-- req: R10.2 -->
### R10.2

Label needs-run, an overlay on a recurring issue, is set by handoff (srd014) when a breaking release of its artifact is tagged; an unassigned recurring issue in needs-run is ready, and the next instance opens without it.

<!-- req: R10.3 -->
### R10.3

Until track-goals and handoff run, neither label is set: a dependency is a link a person reads, and a producer announces a breaking release on its own (design.md D12).

<!-- field: goals -->
## Goals

- G1: Every transition between states has exactly one actor that may fire it; only the person taking work sets its assignee, and only delivery or a person's decision closes it.
- G2: A state is read from tracker fields and labels, never inferred from a title or an author.

<!-- field: non_goals -->
## Non-goals

- The label axes and admission; those are srd001.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Every new issue is checked by admission; an issue with a bad axis label carries needs-triage, is never offered, and loses the flag only when admission passes it.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC2 -->
### AC2

An open, unassigned, admitted data issue of a backlog kind is ready, may be popped by anyone, and has no ready label; once assigned it is no longer ready; no script caps in-progress issues.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R4.1](#r41)
- [R4.2](#r42)
- [R5.1](#r51)
- [R5.2](#r52)

<!-- ac: AC3 -->
### AC3

Blocked is set and cleared only by the assignee with a comment saying what it waits for.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)

<!-- ac: AC4 -->
### AC4

An issue closes only by a closing merge, a person's decline with a reason, or track-goals posting met on a Sprint Goal; closing a Sprint Goal's sub-issues alone never closes it; a reopened issue is checked by admission again.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R9.3](#r93)

<!-- ac: AC5 -->
### AC5

Every row of the transitions table names exactly one actor and an assignee effect; every row whose actor is not pop or person has assignee none; a script asked for an unlisted transition refuses.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)
- [R9.1](#r91)
- [R9.2](#r92)
- [R9.4](#r94)

<!-- ac: AC6 -->
### AC6

Release 06.0's planning writes waiting and needs-run from R10; until then the interim rule of R10.3 holds.

<!-- field: traces -->
#### Traces

- [R10.1](#r101)
- [R10.2](#r102)
- [R10.3](#r103)
