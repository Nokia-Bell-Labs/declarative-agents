---
id: srd042-beads-tracker
title: "Beads tracker: the program's facts live in a repository"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

| GitLab concept | beads concept |
| --- | --- |
| issue | issue |
| scoped label | label, same axis grammar |
| milestone | release field |
| assignee | assignee |
| note | comment |
| merge request | local merge; closing commit cited on the issue |
| project path | repository field on the issue |
| group hierarchy | one tracker repository, queries scoped by field |

<!-- field: problem -->
## Problem

Every fact the program runs on — issues, states, labels, comments,
milestones — lives in a hosted GitLab instance. That buys a web UI and
merge requests, and costs three things the program feels daily: an
external service to provision and keep reachable (the Terraform roots,
service accounts, and CI wiring exist only to shape it), tracker text
stored and indexed outside the repository, and a hard dependency that
makes every scrum-master command a network call. The skills already merge
branches locally, and the tracker data is small, structured, and
line-oriented. This SRD states the requirements for beads as the
program's tracker: issues as data in a dedicated shared repository,
synchronized through git, read and written by scrum-master through the same
tracker boundary the GitLab client serves today.

<!-- req: R1 -->
## R1 - The tracker boundary holds

<!-- req: R1.1 -->
### R1.1

scrum-master's commands reach a tracking system only through the tracker
interface, in two halves every tracking system supplies as one package:
the issue half (issues, their notes and the ledger events in them,
milestones, labels, the current user, assignment) and the code half (a
project's files, tags, branches, and merged history). A branch's approval
and merge go through one delivery; beads' is the local one, an approval
comment naming the commit and a --no-ff merge in the project's checkout
(srd013 R5.5). No command, skill, or gate knows which tracking system
serves it.

<!-- req: R1.2 -->
### R1.2

The state machine of srd041 is backend-neutral: every transition, actor,
and invariant holds unchanged under beads. A state is read from issue
fields, never inferred.

<!-- req: R1.3 -->
### R1.3

A program names each repository's tracking system in control/trackers.md
(config format trackers): a default, bd with its tracker repository, gh,
or glab, and overrides for the projects or groups that use another. A
repository has exactly one system, so project#iid names one issue; a
group's entry serves every project under it that no closer entry names.
SCRUM_MASTER_TRACKER replaces the default for one person's run, never an override,
and with no such file it alone names the system every project uses. scrum-master reads the file from the checkout, else from the one sibling
checkout that holds it, worktrees of one repository counting as one, else,
where checkouts mirror the forge's group layout, from the one checkout
beside a folder above it, nearest first, up to the user's home folder,
whose origin's group holds this checkout's repository, so another
program's is never taken; two at one level are refused, naming both
(GH-2743). With none, a run that names its system repository, by --system,
--control, or SCRUM_MASTER_SYSTEM, reads the file from it through the
forge the checkout's origin is on, gh for github.com and glab for any
other host; else it refuses, naming the folders searched and both
remedies. That checkout is the system repository's, and a word that reads
the policy without --control reads it there, from any repository of the
program. A project's files are read from the checkout whose origin is that
project, found the same way: at each of those folders, nearest first, at
each trailing part of the project's path, longest first, the full group
path, the path below each group, and last the final segment, so
checkouts laid out by their group path below a common folder find each
other and two projects sharing a last segment can sit side by side
(GH-2803); a checkout whose origin is another project is never read for
it, and the lookup goes on to the next candidate, and a project with no checkout is an error naming
the folders searched, never another repository's files or an empty set
(GH-2744). A checkout's own project is its origin's, whatever its folder
is called, so a worktree pop opens, named for its branch, names the same
project as its main checkout; only a checkout with no forge origin is
named by its main checkout's folder (GH-2752).

<!-- req: R1.4 -->
### R1.4

The gh system serves a repository's issues, notes, labels, and milestones
from GitHub through the gh CLI and the person's existing login; scrum-master keeps
no token. GitHub has no due date on an issue and no start date on a
milestone, so each is a Due: or Start: line of the description. A group's
labels and milestones go to each of its repositories, as GitHub has none
of its own. A branch is reviewed as a pull request pop opens as a draft
with one empty commit, an approval is the pull request's review of the
head commit, and merge merges it on GitHub and closes the issue on that
merge commit. Files, tags, branches, and merged history come from the
local checkout, as for bd; a reader of many repositories, scrum-master
reports collect, reads a repository's files on its default branch, with
the commit that last changed each, through GitHub's API, so it needs no
checkout of them (srd018 R13.2, GH-2725).

<!-- req: R1.5 -->
### R1.5

The glab system serves a repository's issues, notes, and award emoji from
GitLab through the glab CLI and the person's existing login, and its
group's milestones and labels as GitLab's own group milestones and group
labels; due dates and start dates are GitLab's fields. A branch is
reviewed as a merge request pop opens as a draft, an approval is GitLab's
approval pinned to the head commit with a note naming that commit, since
GitLab's approvals do not record one, and merge merges it on GitLab,
waiting while GitLab still checks one that just left draft, and closes
the issue on that merge commit. As for gh, scrum-master reports collect reads a
repository's files on its default branch, with the commit that last
changed each, through GitLab's API, with no checkout (srd018 R13.2,
GH-2725).

<!-- req: R2 -->
## R2 - Mapping

<!-- req: R2.1 -->
### R2.1

A GitLab issue maps to a beads issue: title, description, state, one
assignee, labels, and comments carry over one to one. Its link lines
(srd007 R6.1) also become bd's dependency records on the dependent
issue, so bd sees the edges: Blocked-by as blocks, Depends-on and
Relates-to as related, and Blocks as a blocks record on its target; the
lines stay in the description, and two clones' records merge as a set. The scoped labels
of srd001 become beads labels with the same axis grammar; the lint of
srd013 enforces exclusivity exactly as it does on GitLab Free.

<!-- req: R2.2 -->
### R2.2

A milestone maps to an issue field holding the release or sprint
identifier. Anything srd031 writes to a milestone today it writes to
that field.

<!-- req: R2.3 -->
### R2.3

A merge request maps to nothing: delivery is a local merge to main whose
closing commit cites the issue id, and the closing actor of srd041 R7
becomes the merge commit observed by the implement skill, which closes
the issue and records the commit hash as the closing comment.

<!-- req: R2.4 -->
### R2.4

A project path (group/subgroup/repo) maps to a repository field on the
issue. One tracker holds every repository's issues; a query scoped to a
repository replaces a query scoped to a project.

<!-- req: R3 -->
## R3 - The shared tracker repository

<!-- req: R3.1 -->
### R3.1

The program's beads database lives on one branch of one git repository,
the tracker repository: a dedicated repository's main branch, or a
dedicated branch of a code repository, which control/trackers.md's
tracker_branch names and which is never merged into that repository's
other branches, so a program of one repository keeps its code and its
tracker in one remote (config format trackers). It is the
single source of truth for program facts; a working clone's local beads
state is a cache. Its .beads/config.yaml sets the program's issue
prefix, once, when scrum-master init seeds it: every issue's ID is that prefix and
eight hex digits, and scrum-master when the file sets none. An ID is never renamed.
The file's layout key names how issues are stored: files keeps each issue
in .beads/issues/<id>.json, with an index under .beads/index/ from each
repository's IIDs to IDs, and without the key every issue is in
.beads/issues.jsonl; both are read and written, and scrum-master tracker migrate,
which the tracker-migrate profile runs, moves the second to the first.

<!-- req: R3.2 -->
### R3.2

Every scrum-master command that reads the tracker synchronizes from the tracker
repository before reading; every command that writes synchronizes to it
after writing. The protocol, its conflict rules, and its offline
behavior are eng04-beads-sync.

<!-- req: R3.3 -->
### R3.3

A report produced from a clone that could not synchronize says so,
naming the time of its last successful synchronization: the last pull, or
the last push of its own writes, whichever came later.

<!-- req: R4 -->
## R4 - What leaves

<!-- req: R4.1 -->
### R4.1

The platform Terraform (groups, projects, memberships, recurring-issue
seeding) is retired; recurring issues (srd001 R8) are seeded by scrum-master
against the tracker repository.

<!-- req: R4.2 -->
### R4.2

The regression rig builds fixture tracker repositories instead of
GitLab subgroups; eng01 is rewritten when 03.0 is planned.

<!-- field: goals -->
## Goals

- G1: Every program fact lives in a repository the program owns, synchronized through git, with no hosted tracker in the loop.
- G2: scrum-master behaves identically over beads and GitLab while both exist, because both serve one tracker interface.

<!-- field: non_goals -->
## Non-goals

- The synchronization protocol's mechanics and conflict rules; those are eng04-beads-sync.
- A web UI over the tracker repository.
- Migrating historical GitLab data; a program adopts beads at a release boundary with open issues re-filed.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Every scrum-master command passes its tests against a fixture beads tracker with
no GitLab reachable, and the srd041 state machine replays identically on
both backends; a team checkout finds control/trackers.md in the system
checkout beside it though a worktree of the system repository sits there
too, and a scrum checkout whose folders mirror the forge's groups finds
the system's above it with no environment variable, or through the forge
when the run names the system; the one-component rule gives one verdict
whether the control project's SRDs are read from disk or through the
tracker, and lint says when it waived the rule.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)

<!-- ac: AC2 -->
### AC2

An issue created on GitLab and its beads mapping carry the same title,
state, labels, assignee, and comments; a closed issue on beads cites the
closing commit; a repository-scoped query returns the same issues a
project-scoped query returns today.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)
- [R2.4](#r24)

<!-- ac: AC3 -->
### AC3

A command run against a stale clone synchronizes before reading; a write
reaches the tracker repository before the command reports success; a
command that cannot synchronize says so and names the last good
synchronization. A tracker on a branch of a code repository is read and
written on that branch alone, and no tracker commit reaches the code's
main.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC4 -->
### AC4

Release 03.0 ends with no Terraform platform root, no GitLab client, and
a rig that provisions fixture tracker repositories.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)

<!-- ac: AC5 -->
### AC5

On the gh system an issue files, lints, and closes in GitHub with its due
date and milestone read back; pop opens the branch with a draft pull
request; the author cannot approve it; under the default policy merge
lands the author's own pull request on GitHub and closes the issue on that
commit; under a policy with require_approval, another person's approval
of the head lets merge land it, and an approval of an earlier commit is
refused.

<!-- field: traces -->
#### Traces

- [R1.4](#r14)

<!-- ac: AC6 -->
### AC6

On the glab system an issue files, lints, and closes in GitLab under a
group milestone its project finds; pop opens the branch with a draft
merge request; under the default policy merge lands the author's own
merge request on GitLab and closes the issue on that commit; under a
policy with require_approval, another person's approval of the head lets
merge land it, and an approval of an earlier commit is refused.

<!-- field: traces -->
#### Traces

- [R1.5](#r15)
