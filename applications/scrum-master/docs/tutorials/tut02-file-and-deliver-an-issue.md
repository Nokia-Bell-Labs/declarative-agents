<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 2: File and Deliver an Issue

This tutorial is the inner loop every contributor lives in: a request
becomes an admitted issue, the issue becomes a branch and a worktree, the
work becomes commits, and one merge delivers and closes it (srd035,
srd036). It assumes a program repository from
[Tutorial 1](tut01-start-a-program.md).

Throughout, the issue workflows read the same two variables:
`SCRUM_MASTER_PROJECT` names the project and `SCRUM_MASTER_ISSUE` the
issue's number in it.

```bash
export SCRUM_MASTER_PROJECT=<project path>
```

## Step 1: File the issue with specify

In an agent session, describe the work and ask for an issue:

> File an issue: the exporter drops the last row of every CSV when the
> file ends without a newline. Fix internal/export and cover it with a
> test.

The specify skill runs (srd035). scrum-master prints the
plan-vs-architecture guide first, so the issue lands against the
architecture; the model writes the one thing scrum-master cannot — the
body, with `## Requirements` as numbered steps and
`## Acceptance Criteria`, each criterion ending
`(test: <name>; environment: <local|dev|staging>)` — and files it with
the `issue_file` word, which sets the plane, kind, and component from the
named paths. The skill asks you only for what is missing: the kind when
no path decides it, the test of a criterion that names none.

The run's report is the issue's URL, its plane, kind, and component, and
whether admission admitted it. An issue admission did not admit comes
back with the reasons; the skill fixes the body and lints again.

## Step 2: Admit it to the sprint

An issue enters a sprint by joining its milestone (srd041):

```bash
export SCRUM_MASTER_ISSUE=<the issue's number>
agent --profile .scrum-master/agents/admit/profile.yaml --core-root .scrum-master/agent-core --directory .
```

By default the issue joins the active sprint's milestone. A pop refuses a
data issue admission has not admitted, so this step comes before the
next.

## Step 3: Take the work

The implement skill runs the whole loop — pop, work, push, merge — in
one conversation (srd036). In an agent session:

> Take issue <number> and implement it.

scrum-master decides whether the issue can be taken; the model acts only
in the Work state and closes nothing itself. The steps below are the same
machine, run one profile at a time, which is also how you recover when a
step stops halfway.

### Pop: claim the issue, open its branch and worktree

```bash
agent --profile .scrum-master/agents/pop/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The pop is deterministic, no model anywhere: it claims the issue
(refusing a closed issue, someone else's, or an unadmitted data issue),
creates or reuses the branch `<iid>-<slug>` from the default branch,
opens the sibling worktree beside the checkout, and installs the
checkout's agents' skills into it, so an agent started there can finish
the issue. The worktree path is the run's report. A run that stops
partway ends in a state a re-run finishes — `ClaimedOnly`, `NoWorktree` —
reusing what is already in place.

### Work: build the change in the worktree

Work in the worktree, and only there. The issue's Requirements are the
steps and its Acceptance Criteria what must hold. Commit each issue
separately, the message holding its Requirements and Acceptance Criteria
and ending with the closes line:

```bash
git commit -m "Fix the CSV exporter's last row

...

Closes <project#iid>"
```

Then publish the branch:

```bash
git push -u origin HEAD
```

## Step 4: Approval, when the policy requires it

Under `require_approval` in the program's policy, a reviewer other than
the author reads the pushed tip against the Acceptance Criteria and
approves it from their own checkout — the approve skill posts the
approval as a comment naming the branch's tip. The merge refuses an
unapproved or outdated approval; a new push needs a new approval.

## Step 5: Merge — the one merge that closes it

From the issue's worktree:

```bash
agent --profile .scrum-master/agents/merge/profile.yaml --core-root .scrum-master/agent-core --directory .
```

Delivery is deterministic, three words (srd013 R5): the gate runs every
check with no effect — the issue and each closes open, the branch with
commits of its own, the corpus audit when `docs/` changed, the approval
when the policy requires one; the deliver word merges the branch
`--no-ff` into the default branch and pushes it; the close word closes
the issue and each closes reference on the merge commit and posts a
`reported_complete` event on each closed Sprint Goal's ledger.

A refusal names its reason and nothing merges. A merge whose closes did
not land ends in `PartiallyDelivered`, naming the commit; a re-run
finishes the delivery, and a closed issue stays closed.

## What you have

The issue is closed by a merge commit on the default branch that cites
it, and its branch and worktree are done. The ledger has the event the
weekly status reads in [Tutorial 5](tut05-weekly-status.md). To plan the
sprint this issue lived in, go to
[Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md).
