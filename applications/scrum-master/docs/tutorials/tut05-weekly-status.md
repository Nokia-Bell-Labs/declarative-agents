<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 5: Weekly Status

This tutorial sets up the weekly status loop. Each person writes their
report in the repository where they worked; each team's lead rolls the
team's reports up there; the technical program manager rolls the teams
up in the system repository. Nothing lives in a central reports
repository, and every claim in a rollup cites the report it rests on
(srd018 R12 to R18). It assumes a program from
[Tutorial 1](tut01-start-a-program.md) whose members deliver through
[Tutorial 2](tut02-file-and-deliver-an-issue.md).

## Step 1: Fill in the roster, once

The roster is `control/reports.md` in the system repository, owned by the
technical program manager. Init wrote it empty; list who reports and the
repository each one works in. Someone listed in a code repository of a team
whose subsystem SRD names a `scrum_repository` reports in that scrum
repository, where the team plans
([Tutorial 4](tut04-write-the-backlog.md)):

```markdown
---
id: reports
title: Reports configuration
---

<!-- field: team -->
## Team

| username | project |
| --- | --- |
| alice | group/team-a |
| bob | group/team-b |
| tpm-1 | group/system |

<!-- field: reminder_weekday -->
## Reminder weekday

wednesday

<!-- field: program_lead -->
## Program lead

tpm-1
```

A person's team is the subsystem whose SRD lists their repository, so
the subsystem SRDs decide who is rolled up together, and each
subsystem's owner writes its team rollup. The roster's other fields, its
issue titles and due offsets, have defaults; the reports-config format
lists them. The roster lands like any change to the system repository:
through an issue, popped and merged.

A program that kept its reports in a central reports repository moves
its roster with the upgrade profile, which runs doctor: with that
repository checked out beside the system checkout, doctor writes its
`reports.md` into `control/reports.md`, naming the old repository so its
reports stay readable for the release that moves them.

## Step 2: Open the week's issues

The program manager's weekly step, from the system checkout, on a
schedule:

```bash
printf '{"parameters": {"control": "<system project path>"}}' > /tmp/status.json
agent --profile .scrum-master/agents/status-issues/profile.yaml --core-root .scrum-master/agent-core --directory . --request /tmp/status.json
```

It files, once for the coming week:

- each person's status issue in the repository they report in, due the
  day after the reminder weekday;
- one team rollup issue per subsystem in its repository, for its owner,
  due the next working day;
- one program rollup issue in the system repository, for the program
  lead, due the working day after that.

A second run the same week files nothing.

## Step 3: Each person writes their report

In an agent session in the repository you work in:

> Take my status issue and write my weekly status.

The status-report skill takes the issue, then gathers the facts with
words: your week's Sprint Goals, merges, and closes (`reports week`),
what you said last week you would do next and what blocked you
(`reports previous`), and where the report goes (`reports path`). It
shows them, then asks you one question at a time only what the tracker
cannot know: each Sprint Goal's status and problems, why the week's work
mattered, what blocks you, whether last week's commitments landed, and
what comes next. It drafts the report with Work, Blockers, Impact, and
Next, revises it until you approve it, writes it at
`status/reports/<quarter>/<week>-<you>.md`, checks it with
`reports validate`, and merges it on your status issue, a reviewer
approving it first if the repository's policy requires one. No other
repository is written.

## Step 4: Each team's lead rolls the team up

In an agent session in the team's repository, the subsystem's owner:

> Take the team rollup issue and roll up the week.

The status-rollup skill collects the week's merged reports of the
subsystem's repositories (`reports collect --level team`), each with its
repository and the commit that last changed it, read through GitHub's or
GitLab's API with no clone, or from a checkout beside yours. It
aggregates them into signals (`reports aggregate`): blockers shared
across reports, blockers older than the policy allows, slipping
commitments, missing reports with their open status issues, reports that
contradict the ledger, and merged work no report mentions. It proposes a
disposition for each signal (headline, risk, ask, fyi, follow-up, or
drop) and asks you only to confirm or change it, then drafts the rollup:
headlines, progress per Sprint Goal as its ledger states it, risks and
asks with owners, and the next period, every claim citing its report as
`repository@commit`. Once you approve it, it is written at
`status/rollups/<quarter>/<week>-<subsystem>.md`, checked, merged on the
rollup issue, and each follow-up signal is filed as an issue.

## Step 5: The program manager rolls the program up

In an agent session in the system repository:

> Take the program rollup issue and roll up the week.

The same skill, at the program level, collects the teams' rollups and
the system repository's own reports (`reports collect --level program`),
opening a raw report only to cite a signal, so a lead's judgment is not
repeated or overridden. The program rollup lands at
`status/rollups/<quarter>/<week>-program.md` in the system repository.

## Where it shows up

- **The site**: the status pages show the latest program rollup and the
  signals over the program's collected reports
  ([Tutorial 1](tut01-start-a-program.md), the README's site section).
- **The next plan**: plan's candidates read the latest program rollup's
  progress, so a slipped Sprint Goal surfaces where the next release is
  chosen ([Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md)).
- **The sprint end**: its facts aggregate the team's or the program's
  reports where they were written.

## Without the skills

Every step is a word with a model-free profile you can run yourself, from
the repository it concerns, its parameters in a request file:

```bash
printf '{"parameters": {"control": "<system>", "level": "team", "project": "<team repository>"}}' > /tmp/collect.json
agent --profile .scrum-master/agents/reports-collect/profile.yaml --core-root .scrum-master/agent-core --directory . --request /tmp/collect.json
```

The others are `status` (your week), `reports-previous`, `reports-path`,
`aggregate`, and `reports-validate`; each skill's page lists the
parameters each takes.

## The cadence, in one list

- **Once**: the roster in `control/reports.md`.
- **Weekly, program manager**: status-issues at the week's start; the
  program rollup when the team rollups are in.
- **Weekly, each person**: status-report, merged where they worked.
- **Weekly, each team's lead**: status-rollup for the team.
- **Each period**: the program rollup feeds plan, and the sprint-end of
  [Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md) summarizes the
  period from the same reports.

## What you have

A weekly rhythm with no repository to stand up or staff: reports written
where the work happened, rolled up by the people who lead it, every claim
traceable to the commit it came from, and the program's view on the site
and in the next plan. With all five tutorials done, the loop is closed:
the backlog (Tutorial 4) feeds the plan (Tutorial 3), the plan becomes
issues delivered one merge at a time (Tutorial 2), and the status (this
tutorial) tells the next plan what actually happened.
