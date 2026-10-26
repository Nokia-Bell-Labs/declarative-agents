---
id: eng04-beads-sync
title: "Beads Sync: One Tracker Repository, Pull Before Read, Push After Write"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: introduction -->
## Introduction

srd042 puts the program's facts in one tracker repository that every
working clone synchronizes with through git. A hosted tracker gives
every reader the same view because there is one server; a repository
gives every reader the same view only if the discipline around pulling
and pushing is stated and held by the tools, not by memory. This
guideline states that discipline: when scrum-master synchronizes, how a conflict
resolves, and what offline operation promises.

<!-- section: the-tracker-repository -->
## The tracker repository

The tracker repository holds the beads database and nothing else: no
code, no documents, no CI. Issues are .beads/issues.jsonl, bd's own
interchange file; milestones are .beads/milestones.jsonl and the label
schema .beads/labels.jsonl beside it, each record belonging to a group or
project path, merged a record at a time with the later writer winning
when both clones changed one. Every program repository's configuration
document names it by URL. Access to it is access to the program's facts,
so it carries the same visibility as the most restricted repository it
tracks. A working clone keeps its local beads state as a cache under the
code repository; the cache is disposable and never the thing another
clone reads.

A program of one repository keeps the beads database on a dedicated
branch of its code repository instead, such as scrum-master/tracker,
which control/trackers.md's tracker_branch names. The branch is an
orphan: it shares no history with the code and holds only .beads/, and
it is never merged into the code's branches. Every rule below applies to
that branch as it does to a tracker repository's main: the working clone
is a single-branch clone of it, kept apart from the code's checkout, and
fetches, merges, and pushes touch no other branch. The code's main stays
protected while the tracker branch takes direct pushes from every member.

<!-- section: pull-before-read-push-after-write -->
## Pull before read, push after write

Every scrum-master command that reads the tracker pulls the tracker repository
first; every command that writes pushes immediately after the write, in
the same invocation. A command is not done until its push lands. The
rule is in scrum-master, not in the skills: a skill cannot forget it, and a
person running scrum-master by hand gets it for free. Two consequences follow.
First, a command's output always reflects the tracker as of its own
start, named by the tracker repository commit it pulled. Second, the
window for conflicting writes is the length of one command, not the
length of a working session.

A push the tracker repository rejects because another write landed first
re-pulls, merges by the rules below, and pushes again, after a randomized,
growing delay so writers that collided do not collide again in lockstep.
When the attempts run out the command fails and nothing was written: the
clone drops that write's commit, so no later push can land it, and writes
queued offline before it stay queued. The opt-in GitLab rig holds this
under four writers at once (eng01-regression-rig).

<!-- section: conflicts -->
## Conflicts

Two clones that write between each other's synchronizations produce a
merge in the tracker repository, and the merge resolves by rules, not
by hand. Field edits on different issues merge trivially. Field edits
on the same issue merge field-wise, last writer wins, except for the
two fields the state machine guards: a state transition replays through
the srd041 table, and a replay the table refuses is dropped with the
refusal recorded as a comment on the issue; an assignee conflict keeps
the earlier assignment, and the later actor's pop is reported back to
them as lost to an earlier taker. Comments are append-only and never
conflict. A conflict the rules cannot resolve stops the push and
reports, which in practice means a malformed database, not a
disagreement about facts.

<!-- section: offline -->
## Offline

A clone that cannot reach the tracker repository still works: reads
serve the cache and say so, naming the last pulled commit and its time
(srd042 R3.3); writes land in the cache and are pushed by the next
successful synchronization of any command. What offline operation never
does is silently present the cache as current, or let two offline
sessions believe they both hold an assignment: an assignment taken
offline is provisional, is marked so in the pop's own output, and
confirms or reports as lost when scrum-master tracker reconcile runs back online; reconcile also lists the queued writes it pushed and names the tracker commit after the push. Every command's output names the tracker commit it read, and says when that was the cache.

<!-- section: scale -->
## Scale

The tracker keeps its issues in one of two layouts, which
.beads/config.yaml names. With layout: files, the layout scrum-master init seeds,
each issue is one line of JSON in .beads/issues/<id>.json. A write
rewrites only the files of the issues it changes, so its diff stays with
its own issue, and two writers of different issues change different
files, which git merges by itself. Two writers of the same issue conflict
on its one line, and that file merges by the rules above; an issue each
side created with the same IID is renumbered on the side that pushes
second. Beside the issues, .beads/index/<repository>/<iid> holds the ID
of the issue numbered iid there, the repository path escaped into one
directory name. A command that names an issue reads its entry and then
that one file, and a create numbers its issue from the entries' names, so
neither reads the other issues; an entry that is missing or names another
issue sends the lookup back to reading them all. Entries only appear, so
git merges them, and when both sides created the same IID the entry keeps
theirs and the renumbered issue gets a new one. Without the key the
tracker keeps bd's JSONL in
.beads/issues.jsonl and every write rewrites the whole file, so a write's
cost grows with the tracker. scrum-master tracker migrate moves a JSONL tracker to
the files layout, with its index, in one commit, and is run once, when
no one is writing; on a files tracker that has no index it builds one;
scrum-master tracker export writes either layout as JSONL for bd. The
tracker-migrate and tracker-export profiles run the two.

scrum-master issue file records a new issue and its admission, the lint state and
any note, in one write, so filing races once. In the files layout an
update reads and writes only its issue and its index entry. Every writer
pushes the same branch, so a push can be rejected because another writer
pushed first. A rejected write is not merged: its commit is dropped and
the write replays on the newer tracker, reading it afresh, so a create
numbers its issue after the other writer's instead of colliding with it,
and a pop that finds the issue taken reports the race lost. Each replay
takes the same path as a first try, up to 60 times with a jittered delay
that starts at 0.2 seconds and doubles to at most 2, about two minutes in
all. Only a write committed offline, which cannot replay against a
repository it cannot reach, merges by the rules above when it syncs.
mage integration:beadsScale (eng01)
measures both layouts at thousands of issues and several writers.

<!-- section: choosing-the-backend -->
## Choosing the backend

SCRUM_MASTER_TRACKER, set to the tracker repository's URL, selects the beads
backend, and scrum-master requires it. In scrum-master's code a tracking system is the two
halves of the tracker interface, tracker.Issues and tracker.Forge, and
program's delivery holds a branch's approval and merge; beads supplies
both halves and the local delivery. The tracker clone lives at
SCRUM_MASTER_TRACKER_CLONE, or under the user cache directory, cloned on first use.
The person is SCRUM_MASTER_USER, or git's user.name. SCRUM_MASTER_WEB_BASE is the web root repositories are cited under, which goals and plan need to write a pinned citation; when it is unset, they take the scheme and host of the checkout's origin as configured. Issues come from the tracker
repository; files, branches, tags, and comparisons come from git: a
project's checkout is the sibling of the command's checkout named for the
project's last path segment, as repositories sit side by side under their
owner, and a project with no such sibling is served by the command's own
checkout and its origin. Delivery is a merge commit on the default
branch citing project#iid, made by scrum-master issue merge after the audit gate
for a docs/ change and, when the policy requires approval, only on
another person's approval of that very commit, posted with scrum-master issue
approve (srd013 R5.5); scrum-master issue close finds it and closes the issue on it, and refuses a fast-forward or an empty
branch, because neither is a delivery a reviewer can point to. A merge
also closes every other issue its message cites, such as the Sprint Goal a
sprint-end merge closes; scrum-master issue close closes such an issue, which has
no pop branch, on the first merge commit that cites it, matching the
number exactly. A project's merged history, what reports read as merged branches,
is the merge commits on its default branch: each numbered by its place in
the branch's first-parent history, by its author on its commit date, and
closing every project#iid its message cites.

<!-- section: what-this-replaces -->
## What this replaces

The GitLab instance was the serializer: every write landed on one
server in one order. This guideline moves that job to git plus the
rules above, which is a weaker serializer with a longer window, bought
deliberately for the absence of the server. The program accepts the
trade because its write pattern is a handful of actors making small,
mostly disjoint writes, with the one genuinely contended field —
assignee — already guarded by the pop discipline of srd041 R9.

<!-- field: references -->
## References

- srd042-beads-tracker: the requirements this guideline serves
- srd041-states: the state machine conflicts replay through
- eng01-regression-rig: the rig this protocol's fixtures will replace when 03.0 is planned
