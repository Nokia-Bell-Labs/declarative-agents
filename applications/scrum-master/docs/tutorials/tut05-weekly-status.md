<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 5: Weekly Status

This tutorial sets up the weekly status loop: a reports repository, one
status issue per person each week, each person's report gathered from
facts, and the week aggregated into signals the next plan reads (srd018).
It assumes a program from [Tutorial 1](tut01-start-a-program.md) whose
members deliver through [Tutorial 2](tut02-file-and-deliver-an-issue.md).

## Step 1: Lay down the reports repository, once

Create a repository for the program's reports and, in its checkout, run
the reports-init profile with the system repository named:

```bash
export SCRUM_MASTER_SYSTEM=<system repository path or remote>
agent --profile .scrum-master/agents/reports-init/profile.yaml --core-root .scrum-master/agent-core --directory .
```

On a fresh checkout without vendored profiles, run it from the installed
profiles under `$SCRUM_MASTER_HOME` instead, as Tutorial 1 ran init. The
reports init word writes the repository's config — the team who report
and the reminder weekday — and the vendor word vendors the release's
profiles into `.scrum-master`, which the installed skills run. The report
format the repository checks is laid down with it; the reports-validate
profile holds existing reports to it.

## Step 2: Open the week's status issues

The program manager's weekly step, in the reports checkout:

```bash
export SCRUM_MASTER_REPORTS=<the reports checkout's path>
agent --profile .scrum-master/agents/status-issues/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The status-issues word ensures one status issue per listed person for
the coming ISO week, filed in that person's own project: it recurs the
person's last status issue when that one is closed, files a fresh one
when there is none, and files none twice — a rerun on the same week adds
nothing (srd018 R7). Run it on a schedule, at the start of each week.

## Step 3: Each person works their status issue

A report is written from facts, not memory. In the person's own project:

```bash
export SCRUM_MASTER_PROJECT=<the person's project path>
agent --profile .scrum-master/agents/status/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The week word gathers the open Sprint Goals and what the person merged
and closed this week: the merges whose `Merged-by` trailer names them,
which Tutorial 2's merge wrote. In an agent session, "what did I do this
week?" runs the same profile.

The status issue is then worked like any issue
([Tutorial 2](tut02-file-and-deliver-an-issue.md)): its Acceptance
Criteria ask for a report in the fixed shape that passes the
reports-validate profile, written by the person about their own work
only, and it closes by its merge. Next week's status-issues run recurs
it.

## Step 4: Validate the reports

In the reports checkout:

```bash
agent --profile .scrum-master/agents/reports-validate/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The validate word holds every report to the report format, naming each
one that does not parse; a report the aggregate cannot read is a report
the program never sees.

## Step 5: Aggregate the week into signals

The program manager's reading step, in the reports checkout:

```bash
agent --profile .scrum-master/agents/aggregate/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The aggregate word reads the period's reports into progress per Sprint
Goal and the signals that need action, by component; with push it files
each actionable signal once as an issue. The aggregate's JSON output is
an input to release planning: `plan_candidates` in
[Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md) reads it, so a
slipped Sprint Goal surfaces where the next release is chosen.

## The cadence, in one list

- **Once**: reports-init.
- **Weekly, program manager**: status-issues at the week's start;
  aggregate at its end.
- **Weekly, each person**: status, then the report, merged through
  their status issue.
- **Each period**: the aggregate feeds plan, and the sprint-end of
  [Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md) summarizes the
  period from the same reports.

## What you have

A reports repository with a weekly rhythm: issues that ask for the
reports, reports written from gathered facts, and an aggregate that turns
them into signals the program acts on. With all five tutorials done, the
loop is closed: the backlog (Tutorial 4) feeds the plan (Tutorial 3), the
plan becomes issues delivered one merge at a time (Tutorial 2), and the
status (this tutorial) tells the next plan what actually happened.
