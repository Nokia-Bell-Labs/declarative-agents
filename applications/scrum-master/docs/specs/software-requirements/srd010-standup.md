---
id: srd010-standup
title: "Script: standup (scrum-master standup ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

A person's morning needs the facts of their work in one place: what
closed, what is overdue, what became unblocked, what happened on their
issues overnight, and what to do today. The tracker holds all of it, but
spread across projects, comments, to-dos, and time logs. Without a script
that gathers it, people either skip the standup or rebuild it from memory.

The standup subcommands fetch those facts and write the small set of changes a
person makes in a day (hours, start, block, done). They never ask anything;
the daily-scrum skill (srd020) holds the conversation.

<!-- field: goals -->
## Goals

- G1: One call returns everything the morning needs about the caller's own work.
- G2: Every daily write a person makes has one subcommand.
- G3: Scoring uses only policy terms, so two people's plans are computed the same way.

<!-- req: R1 -->
## R1 - Common behavior

<!-- req: R1.1 -->
### R1.1

Subcommands are in the scrum-master group standup. Each reads the person config, the program's label schema, and the policy file, and reaches the tracker through the tracker's Go client with the token of the tracker CLI's existing login.

<!-- req: R1.2 -->
### R1.2

Every subcommand accepts --json (machine output) and --dry-run (report the writes it would make and make none).

<!-- req: R1.3 -->
### R1.3

No subcommand prompts. A subcommand missing an input exits non-zero naming the input.

<!-- req: R1.4 -->
### R1.4

Subcommands act only on issues assigned to the caller, except fetch and activity, which read, and adhoc, which creates.

<!-- req: R2 -->
## R2 - scrum-master standup fetch

<!-- req: R2.1 -->
### R2.1

Returns the caller's open and recently closed issues across the group: identifier, title, labels, milestone and its dates, due date, time spent, time estimate, task progress, and state per srd041 R1–R32.

<!-- req: R2.2 -->
### R2.2

Marks each issue done-since (closed since last run), overdue (open, due date past), and due-soon (open, due within 7 days).

<!-- req: R3 -->
## R3 - scrum-master standup activity

<!-- req: R3.1 -->
### R3.1

Returns the caller's pushes, merged requests, and closed issues across the group since a given date, defaulting to last run.

<!-- req: R4 -->
## R4 - scrum-master standup inbox

<!-- req: R4.1 -->
### R4.1

Returns the caller's pending tracker to-dos and every event comment (srd006) on issues the caller authored or is assigned since last run, with event name, keys, issue, and author.

<!-- req: R5 -->
## R5 - scrum-master standup unblocked

<!-- req: R5.1 -->
### R5.1

Returns the caller's issues whose waiting flag track-goals cleared since last run (srd007 R3.2), and those still waiting with the dependency each waits on.

<!-- req: R6 -->
## R6 - scrum-master standup score

<!-- req: R6.1 -->
### R6.1

Scores each open parent issue assigned to the caller with the terms and points of policy.scoring: deadline, progress gap against the milestone window, type, blocked, and waiting. Tasks are not scored.

<!-- req: R6.2 -->
### R6.2

Returns issues grouped focus (60 and up), progress (30 to 59), and time-permitting (under 30), each with the terms that produced its score.

<!-- req: R7 -->
## R7 - Daily writes

<!-- req: R7.1 -->
### R7.1

scrum-master standup log adds time spent to an issue and writes a dated note with the given text.

<!-- req: R7.2 -->
### R7.2

scrum-master standup start sets status::in-progress (srd041 R5); scrum-master standup stop clears it.

<!-- req: R7.3 -->
### R7.3

scrum-master standup block sets blocked with the given note as a comment; scrum-master standup unblock clears it (srd041 R6).

<!-- req: R7.4 -->
### R7.4

scrum-master standup done closes an issue only when its assignee has no unmerged branch for it and the issue's kind has no merge finishing condition (question, handoff with a recorded decision); every other close is by merge (srd041 R9.3).

<!-- req: R8 -->
## R8 - scrum-master standup adhoc

<!-- req: R8.1 -->
### R8.1

scrum-master standup adhoc creates an issue in the caller's tracker project with the next ad-hoc identifier, the given title, due date, estimate, source, and urgency, through scrum-master issue file (srd019). It sets no assignee; the caller pops it with implement (srd041 R9.1).

<!-- req: R8.2 | status: retired | retired_in: 1.4 | replaced_by: ["srd013-program R8.1"] -->
### R8.2

Retired in 01.4: recur moved to the issue group as scrum-master issue recur (srd013 R8).

<!-- req: R9 -->
## R9 - Last run

<!-- req: R9.1 -->
### R9.1

scrum-master standup finish records the time of a completed morning run in the person config, which inbox, unblocked, and fetch use as since.

<!-- field: non_goals -->
## Non-goals

- Offering or taking work; taking is implement.
- Any per-person comparison or ranking across people.
- Asking the person anything.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Every standup subcommand reads config through the Go client, honours --json and --dry-run, never prompts, and writes only to the caller's own issues except adhoc creation.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R1.4](#r14)

<!-- ac: AC2 -->
### AC2

fetch returns every field listed for the caller's issues with done-since, overdue, and due-soon flags; activity returns pushes, merges, and closes since the given date.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R3.1](#r31)

<!-- ac: AC3 -->
### AC3

inbox returns pending to-dos and every event comment since last run on the caller's issues; unblocked returns issues cleared since last run and the dependency of each still waiting.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R5.1](#r51)

<!-- ac: AC4 -->
### AC4

score uses only policy terms, scores parent issues only, and groups them at 60 and 30 with each term shown.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)

<!-- ac: AC5 -->
### AC5

log, start, stop, block, unblock, and done perform exactly their srd041 R8 transitions; done refuses an issue that must close by merge.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
- [R7.4](#r74)

<!-- ac: AC6 -->
### AC6

adhoc creates an unassigned, linted issue with the next identifier; finish records last run and later calls use it.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R9.1](#r91)

<!-- ac: AC7 | status: retired | retired_in: 1.4 | replaced_by: ["srd013-program AC9"] -->
### AC7

Retired in 01.4: recur moved to the issue group (srd013 AC9).

<!-- field: traces -->
#### Traces

- [R8.2](#r82)
