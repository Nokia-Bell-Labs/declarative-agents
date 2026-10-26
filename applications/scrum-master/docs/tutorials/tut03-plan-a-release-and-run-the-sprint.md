<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 3: Plan a Release and Run the Sprint

This tutorial is the outer loop: the technical program manager plans the
next release in the system repository, the plan becomes one Sprint Goal
per team, each team's Product Owner runs Sprint Planning, and the
sprint-end closes the period (srd031, srd022, srd038). It assumes a
program from [Tutorial 1](tut01-start-a-program.md) with a backlog from
[Tutorial 4](tut04-write-the-backlog.md) — in this process the backlog
documents exist before the release that schedules them.

The model behind these steps is
[plan-vs-architecture.md](../guides/plan-vs-architecture.md): the
architecture declares intended state, release tags hold actual state, and
the plan is the ordered gap between the two.

## Step 1: Plan the release

In an agent session in the system repository:

> Plan the next release.

The plan skill runs (srd031). Every step is a word; the model asks you
only three things — the items, the version, and the end date:

1. It files a `plane::management` planning issue with specify and takes
   it with implement, so the plan itself lands through the issue
   workflow.
2. The `plan_candidates` word gathers the ranked initiative items,
   slipped items first, and the held-back items with what they lack —
   fed, when a reports repository exists, by the week's aggregate
   (Tutorial 5).
3. You choose the items and confirm or change the recommended version,
   end date, and tag.
4. The `plan_write` word writes the release into `docs/road-map.md`; the
   skill commits it and merges the planning issue as implement does.
5. On `main`, the `release_gate` and `release_tag` words gate and tag the
   corpus (`--artifact corpus`). That tag publishes the plan: the road
   map at that commit is the release's plan of record.
6. The `plan_push` word dates the period: the release's milestone in
   every group it serves, the sprint issue with its exit checklist, one
   sprint-end issue per releasing repository, and a planned event on each
   open Sprint Goal the release moves.

The run's report is the published plan: the tag, the milestone, the
sprint issue, and the sprint-end issues.

## Step 2: Break the release into Sprint Goals

Still in the system repository, the tasks skill breaks the release into
one Sprint Goal per team (srd022):

> Run tasks for release <version>.

The `goals_plan` word derives the plan from the road map's release; for
each outcome with an empty repository the skill asks which candidate
repository serves it, and nothing else; the `goals_push` word files one
Sprint Goal per team, each a document in the system repository before it
is an issue (srd034). A refused outcome is fixed in the initiative or the
subsystem SRDs, and the skill run again.

## Step 3: Sprint Planning in the team's repository

Once tasks filed the team's Sprint Goal, the Product Owner runs Sprint
Planning in the team's repository:

> Plan the team's sprint.

The plan skill's Sprint Planning path shows the backlog — the stories by
rank with their points, the measured velocity, and the recommended
stories; it asks for an assumed velocity when none is measured yet, and
which stories enter the sprint, and nothing else. The `plan_backlog` word
records the chosen stories on the Sprint Goal.

## Step 4: Break each story into issues

For each chosen story, the tasks skill files the issues that execute it:

> Run tasks for story <id>.

It reads the story and the issues already implementing it, proposes the
issues the story still needs — each a change a reviewer reads in one
sitting, in build order, with Requirements, Acceptance Criteria naming a
test and an environment, `Estimated LOC:`, `Implements: <story>`, and
`Blocked-by:` where an issue must close first — and files each one you
agree to on the sprint's milestone, showing its reference and lint
verdict. A body the lint marks needs-triage is fixed and filed again.

## Step 5: Work the sprint

The sprint is Tutorial 2, repeated: pop, work, merge, each issue closed
by its merge. Each person's week flows into the status reports of
[Tutorial 5](tut05-weekly-status.md).

## Step 6: Close the period with sprint-end

The plan filed one sprint-end issue per releasing repository. When the
period ends, its owner takes it:

> Take the sprint-end issue and close the period.

The sprint-end skill works that issue (srd038): the `sprint_end_facts`
word gathers the Sprint Goals' ledger, the period's reports, and the
releases; the model drafts the period's summary — the one model-written
text — and asks the owner what to change, which Sprint Goals are
complete, and which stories are done; the summary lands at
`docs/sprint-ends/<release>.md`, the `sprint_end_stories` word records
the done stories, and the merge closes the sprint-end issue and the
Sprint Goals the owner reported complete. On the owner's confirmation,
the `release_gate` and `release_tag` words release the artifact at the
recommended version.

## What you have

A tagged plan, a milestone and sprint issue for the period, one Sprint
Goal per team with its chosen stories, the issues that execute them, and
at the end a merged period summary, closed Sprint Goals, and a release
tag. The next plan starts from what this period's reports and ledger say,
which is where [Tutorial 5](tut05-weekly-status.md) comes in.
