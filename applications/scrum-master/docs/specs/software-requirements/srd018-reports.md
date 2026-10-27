---
id: srd018-reports
title: "Script: reports (scrum-master reports ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Weekly status is the one place people say what the tracker cannot know:
why work mattered, what blocks them, what comes next. Written from memory
on a Friday, reports contradict the tracker and leave out work that
shipped. Read one at a time, they hide what cuts across them: the
component everyone waits on, the commitment that slipped three weeks
running.

The reports subcommands keep each report in a fixed, checkable shape and
aggregate a week or a sprint of them against the tracker. The weekly
sprint update and the stakeholder update are recurring audit issues worked
by the inner loop (implement and the reviewed merge); these
subcommands are what those issues read and what their acceptance criteria
check, so a roll-up starts from computed candidates instead of a pile of
prose.

<!-- field: goals -->
## Goals

- G1: Every report has the same machine-checkable shape, and its index cannot drift from its body.
- G2: Aggregation finds what cuts across reports and where reports contradict the tracker.
- G3: Nothing aggregated ranks or compares people.
- G4: A report is written and merged in the repository where its author worked, and each level collects what it needs from the program's repositories, so no program stands up or staffs a central reports repository (GH-2725).

<!-- req: R1 -->
## R1 - scrum-master reports init

<!-- req: R1.1 -->
### R1.1

Scaffolds the repository the program's site lives in (srd048): its README, the site's front page, and a deck template. A program whose roster is in the system repository writes its reports where the work is (R12), so that is all (GH-2725). A central reports repository, one init is given members for or that already holds reports.md, also gets a reports directory per quarter, the report schema, a summaries directory, and the configuration reports.md (config format reports-config, its slide_limit 10 by default), as before. It installs the skills for the agents --agent names, as scrum-master init does.

<!-- req: R1.2 -->
### R1.2

Is idempotent and never overwrites an existing report or summary.

<!-- req: R1.3 -->
### R1.3

Writes into a local checkout of the reports repository (--dir), whose owner commits the scaffold through the inner loop.

<!-- req: R2 -->
## R2 - scrum-master reports validate

<!-- req: R2.1 -->
### R2.1

Checks that each report's body has a Work section and a Blockers section, one entry per item, each with its key-value fields, and that its front matter (author, sprint, components, blocked-on) is derived from the body (config format report).

<!-- req: R2.2 -->
### R2.2

Exits non-zero for any report whose front matter disagrees with its body.

<!-- req: R3 -->
## R3 - scrum-master reports aggregate

<!-- req: R3.1 -->
### R3.1

For a week or a sprint, reads every report in it and groups items by component.

<!-- req: R3.2 -->
### R3.2

Computes signals: blockers shared by two or more reports, blockers older than policy.escalation.blocker_age_days, commitments that slipped in policy.escalation.slip_releases or more consecutive reports, components with an expected report missing for a week that has begun and did not end before the reports repository's first commit, when reporting began (GH-116): a week ends as the next begins, and the commit's day is UTC's, as the weeks are (GH-141), or whose reports name no Sprint Goal of theirs (the system repository, which holds none, aside), items that contradict the ledger (a report calling a Sprint Goal done that has no met event, srd006 R10, unless a sprint-end merge closed it as reported complete before track-goals runs, srd038 R2), and merged work in the tracker that no report mentions. Merged work counts only deliveries, merges that close an issue they cite other than a recurring one (a sprint-end or status issue) or one that cuts a release (kind::release, GH-141), landed no earlier than the creation of the first Sprint Goal the range's reports name: a merge from before the period began, such as an earlier run's on the same days, is not the period's work. Merge times compare as instants in UTC. A merge is named by its merge request, project!iid, or, read from a checkout's history where there is none, by its commit, project@sha (GH-141). A Sprint Goal a sprint-end merge closed as reported complete reads so in progress whatever its last report said, and a blocker a component reports on its own work is named as such, not as the component blocking itself. A cost overrun has no signal of its own: a report states it as a blocker naming the spend and the limit it passed. Narrowed to one repository, the aggregate reads only the reports of the people the reports configuration lists for it, and only they can miss one, so a team's sprint end describes its team (GH-116).

<!-- req: R3.3 -->
### R3.3

For a sprint, adds the dashboard signals of scrum-master goals status (srd019) as further candidates.

<!-- req: R3.4 -->
### R3.4

Expresses every signal by component or by blocker, never as a comparison or ranking of people.

<!-- req: R3.5 -->
### R3.5

States progress as the ledger state of each Sprint Goal (srd006 R11) and its latest evidence, never as counts of closed issues or tasks.

<!-- req: R3.6 -->
### R3.6

With --push, files each signal that needs action as an issue in the issue body's work format in the system repository the configuration names (R6.3), unassigned, its body mentioning the person who pushed it (srd041 R9.2), and reports each issue it files with its admission, as scrum-master issue file does.

<!-- req: R3.7 -->
### R3.7

The ISO week stays aggregate's unit. With --from-tag and --to-tag, the aggregate profile's from_tag and to_tag parameters, aggregate resolves each tag to its commit date and reads the reports whose ISO week ends after the first tag's date and not after the second's, for a standalone re-run over a release.

<!-- req: R4 -->
## R4 - scrum-master reports deck

<!-- req: R4.1 -->
### R4.1

Renders an approved summary into a slide deck from the template, one slide per outline section, up to the slide_limit the reports repository's configuration sets (default 10). A summary whose outline has more sections makes deck exit non-zero naming the sections over the limit; nothing is truncated.

<!-- req: R5 -->
## R5 - Common behavior

<!-- req: R5.1 -->
### R5.1

Subcommands are in the scrum-master group reports, honour --json and --dry-run, and never prompt (srd010 R1).

<!-- req: R6 -->
## R6 - Report audits

<!-- req: R6.1 -->
### R6.1

The sprint update is a recurring issue (srd001 R8) per member per week, filed in the repository the member works in. Its Requirements name the status-report skill (R18.1) and the words it drafts from; its Acceptance Criteria are a report in the fixed shape, at status/reports in that repository (R12.1), that passes scrum-master reports validate, written by the member about their own work only. It closes by its merge, and status-issues re-creates it for the next week.

<!-- req: R6.2 | status: retired | retired_in: 5 -->
### R6.2

Retired in 05.0 (design.md D12): direction in R11.3.

<!-- req: R6.3 -->
### R6.3

A signal from aggregate that needs action is pushed as an issue in the issue body's work format, not carried in the prose of a report.

<!-- req: R7 -->
## R7 - scrum-master reports status-issues

<!-- req: R7.1 -->
### R7.1

For each person the configuration lists, ensures one status issue (the sprint update of R6.1) exists in their project for the coming ISO week. When the person's latest status issue is closed, it recurs that issue (srd010 R8.2); when there is none, it files a fresh one through scrum-master issue file.

<!-- req: R7.2 -->
### R7.2

Is idempotent: a second run in the same week files nothing. A scheduled run of the status-issues profile each week, from a person's machine or a scheduled pipeline on the release image, on any tracking system, files the next instance after the merge that closed the last one.

<!-- req: R8 -->
## R8 - scrum-master reports week

<!-- req: R8.1 -->
### R8.1

For a project and a person, returns the project's open Sprint Goal issues (Sprint Goals included) with their due dates and the person's activity since the ISO week began, or since the start of the week --date names, the week taken in UTC: branches merged and issues closed. A merge counts from the later of the week's start and the creation of the project's earliest open or recently closed Sprint Goal, so an earlier run's merges in the same week do not; a project holding no Sprint Goal of its own, the system repository, starts at its own earliest open or recently closed issue, so its planning and registration merges count, and only with no issue of its own takes the teams' earliest instead, the teams being those --control's subsystem SRDs name. The issues closed are those assigned to the person and those a counted merge closes. A merge is the person's when its Merged-by trailer, which scrum-master issue merge writes, names them, whatever git identity made the commit. These are the facts the status issue's guided script drafts from.

<!-- req: R9 -->
## R9 - Weekly status issue

<!-- req: R9.1 -->
### R9.1

The weekly status issue is the sprint update of R6.1, filed in the repository each person the roster lists reports in (R12.2). A program that lists only sub-project leads receives one status issue per lead per week.

<!-- req: R9.2 -->
### R9.2

Its due date is report_due_days after the roster's reminder weekday, the day after Wednesday by default. The body mentions the person it is for, which makes that person a participant who receives the notification (srd006 R7.1) without assigning them (srd041 R9.1).

<!-- req: R9.3 -->
### R9.3

Its Requirements section is the guided script the owner works with the status-report skill (R18.1): for each open Sprint Goal issue in the repository, record its status, its problems, and complete or not complete, with the week's impact, what blocks the work, and what comes next. The output is a report in the srd018 shape that passes scrum-master reports validate.

<!-- req: R9.4 -->
### R9.4

It closes by its merge; the scheduled scrum-master reports status-issues run recurs it with the next week's due date (R7, srd001 R8.3).

<!-- req: R10 -->
## R10 - Summary on a system release

<!-- req: R10.1 -->
### R10.1

When the system repository tags a release, its sprint-end summary is already merged at docs/sprint-ends/<release>.md, reviewed by the lead (srd038 R1). The release profile's step after the tag word renders the deck from that approved summary with scrum-master reports deck, so R4.1's approved summary and this one are the same document.

<!-- req: R10.2 -->
### R10.2

The deck is committed beside the summary, as R11.1 treats forecasts: regenerated on each run and never edited by hand. The step opens no branch and no review issue and spends no model tokens; the summary's review before its merge was the approval.

<!-- req: R10.3 -->
### R10.3

Run twice on the same tag, the step leaves the committed deck unchanged and commits nothing.

<!-- req: R11 -->
## R11 - Direction: forecasts and the stakeholder update

<!-- req: R11.1 -->
### R11.1

Once track-goals runs (06.0), the system release's aggregate also renders a roadmap forecast, the remaining stories of each epic spread over future sprints at velocity in Blocked-by order, and a burn-up per epic and initiative, story points done against total points, from the hierarchy documents and the ledger alone (srd045 R7.1). Both are committed beside the dashboard, regenerated each run, never edited, and labelled as forecasts, not baselines.

<!-- req: R11.2 -->
### R11.2

Until then, reports render no forecast and no burn-up (srd045 R7.2).

<!-- req: R11.3 -->
### R11.3

The stakeholder update becomes its own recurring artifact per lead per period when 06.0 is planned: a summary, an updated risk list, and a deck, with no comparison or ranking of people. Until then the system release's deck beside its approved summary is the program's update.

<!-- req: R12 -->
## R12 - Reports in place

<!-- req: R12.1 -->
### R12.1

A report is status/reports/<YYYY-Qn>/<YYYY-Www>-<username>.md in the repository where its author worked, a team's repository or the system repository, outside docs/ so the corpus gate reads none of it as intent. It keeps the report format, which gains two optional sections, Impact and Next (config format report), so validate and aggregate read a report with or without them. It lands by the merge of its author's status issue in that same repository, through pop, approve when the policy requires it, and merge; no step names another repository.

<!-- req: R12.2 -->
### R12.2

The roster is control/reports.md in the system repository (config format reports-config), owned by the technical program manager: who reports and in which repository, the reminder weekday, the status and rollup issue titles, the program rollup's lead, and the due offsets. A person's team is the subsystem whose SRD lists their repository. A person listed in a code repository of a team whose SRD names a scrum repository reports in the scrum repository, where the team journals: its status reports beside its Sprint Goals and sprint-end summaries (GH-2738). A program that has not moved its roster keeps reports.md in a central reports repository, which status-issues and aggregate read as before; a roster that names a central repository has collect and previous read its reports too, for the release that moves them into place.

<!-- req: R13 -->
## R13 - scrum-master reports collect

<!-- req: R13.1 -->
### R13.1

For a level and a period of ISO weeks, lists the level's repositories from the system repository's subsystem SRDs (implementation_targets): for a team, its subsystem's repositories, its scrum repository among them, named by --subsystem or by the subsystem whose SRD lists --project; for the program, the system repository and every subsystem's, a sub-program's at its interface: a team whose scrum repository is a program's system repository (it holds control/trackers.md) contributes its program rollup as that team's rollup, named for its subsystem, and none of its repositories' reports or open status issues, which that rollup stands for, and nobody listed in it misses a report at this level (GH-2742). A program with no subsystem SRDs takes the repositories its roster names.

<!-- req: R13.2 -->
### R13.2

Reads from each repository's default branch the period's merged reports, and for the program the teams' rollups, each with its repository, path, and the commit that last changed it: through the host's API when the tracking system serves files without a checkout (gh, glab), so it clones no team repository, else from a checkout of the repository beside this one, never from another repository's checkout taken for it. A repository it cannot read is named with the reason. It writes nothing.

<!-- req: R13.3 -->
### R13.3

Lists each repository's status and rollup issues of the period still open, which feed the missing-report signal (R17.1).

<!-- req: R14 -->
## R14 - scrum-master reports previous and path

<!-- req: R14.1 -->
### R14.1

previous returns a person's latest report before the week in the repository they report in, looking back a quarter, with its repository, path, and commit, and its Work, Blockers, Impact, and Next, or says there is none.

<!-- req: R14.2 -->
### R14.2

path names where the week's report goes, status/reports/<YYYY-Qn>/<YYYY-Www>-<username>.md, or with --level a rollup's, and the branch its issue's work goes on.

<!-- req: R15 -->
## R15 - Rollups

<!-- req: R15.1 -->
### R15.1

A rollup is status/rollups/<YYYY-Qn>/<YYYY-Www>-<subsystem>.md in the repository holding a team's Sprint Goals, or status/rollups/<YYYY-Qn>/<YYYY-Www>-program.md in the system repository (config format rollup): headlines, progress by component and Sprint Goal as the ledger states it (srd006), never issue counts, risks and asks with owners, the next period, and its sources, every claim citing its report or rollup by repository and commit.

<!-- req: R15.2 -->
### R15.2

validate checks every report and rollup under status/, a rollup against the rollup format by where it lives, and exits non-zero for a rollup whose claim cites a source its Sources do not list or names none.

<!-- req: R16 -->
## R16 - status-issues against the roster

<!-- req: R16.1 -->
### R16.1

With --control naming the system repository, status-issues files for the coming ISO week each listed person's status issue in the repository they report in, due report_due_days after the reminder weekday; one team rollup issue per subsystem in its scrum repository, none for a sub-program, whose program rollup stands for it and whose result says so (GH-2742), and none for a subsystem no listed person reports in, which has nothing to roll up and whose result says so (GH-2779), which holds the team's Sprint Goals (the one its SRD's scrum_repository names, else its first component's, GH-2738), mentioning the subsystem's owner, due team_rollup_due_days working days later; and one program rollup issue in the system repository, mentioning the program lead, due program_rollup_due_days working days after that. Each recurs as R7 does.

<!-- req: R16.2 -->
### R16.2

Is idempotent: a second run in the same week files nothing.

<!-- req: R17 -->
## R17 - aggregate over a collection

<!-- req: R17.1 -->
### R17.1

With --control and a level, aggregate reads the level's collection (R13) in place of a reports checkout, or a collection --collected names: every R3.2 signal is computed across the level's repositories, a shared blocker across reports in different repositories, a missing report naming its still-open status issue, and merged work in any of them that no report mentions, reporting having begun with the earliest week the collection holds a report or status issue for. It still never ranks or compares people (G3).

<!-- req: R18 -->
## R18 - The status skills, their readers, and the move

<!-- req: R18.1 -->
### R18.1

The status-report skill works a person's status issue in the repository they worked in: its words gather the week (reports week), last week's Next and Blockers (reports previous), and the path (reports path); the model shows them, asks one question at a time only what the tracker cannot know (each open Sprint Goal's status, problems, and completeness, the week's impact, the blockers, whether last week's commitments landed, and what comes next), drafts the report the person approves, writes it at the path, checks it with reports validate, and commits it; the machine pushes and merges it on the status issue. Each word has a model-free profile.

<!-- req: R18.2 -->
### R18.2

The status-rollup skill works a rollup issue, its level the repository it runs in: in a team's repository the team rollup, in the system repository the program rollup. Its words collect the level (reports collect) and aggregate it (reports aggregate); the model proposes a disposition for each signal (headline, risk, ask, fyi, follow-up, or drop) and asks the lead only to confirm or change it, drafts the rollup the lead approves in the rollup format, every claim citing its source by repository and commit, writes it at the path reports path names, checks it with reports validate, commits it, and files the follow-up signals with reports aggregate --push; the machine pushes and merges it on the rollup issue.

<!-- req: R18.3 -->
### R18.3

The site's status pages show the latest program rollup and the signals over the program's collected reports (srd048 R4.1); plan's candidates read the latest program rollup's progress as they read the aggregate's (srd031); and sprint-end facts aggregate the team's or the program's reports collected where they were written when the system repository holds the roster, a central reports checkout only otherwise (srd038).

<!-- req: R18.4 -->
### R18.4

init writes the program's empty roster at control/reports.md (srd043 R1.3); doctor, and so the upgrade profile, moves a central reports repository's roster there once, naming that repository so collect and previous read its reports for the release that moves them (srd043 R4.2).

<!-- field: non_goals -->
## Non-goals

- Writing a report's or a rollup's judgment for its author: the status-report and status-rollup skills gather the facts with words, ask the person only for judgment, and write only what they approve (R18; design.md D10).
- Per-person productivity metrics of any kind.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

init lays down the site's home, its README, front page, and deck template, for a program whose roster is in the system repository, and a central reports repository's layout, configuration, and deck template where it is given members or one exists, once, never overwriting existing reports or summaries.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)

<!-- ac: AC2 -->
### AC2

validate passes a well-formed report and fails, non-zero, any report whose front matter disagrees with its body.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC3 -->
### AC3

aggregate groups a week's or sprint's items by component and reports every listed signal, adds dashboard signals for a sprint, phrases no signal as a comparison of people, reports a team whose reports name no Sprint Goal of its own, counts no merge closing only recurring issues as unreported work, with --push files each actionable signal as an unassigned issue, and with --from-tag and --to-tag reads the ISO weeks between the two tags' dates.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R3.4](#r34)
- [R3.5](#r35)
- [R3.6](#r36)
- [R3.7](#r37)

<!-- ac: AC4 -->
### AC4

deck renders an approved summary within the slide limit and exits non-zero naming the sections over it, truncating nothing; every subcommand honours --json and --dry-run and never prompts.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R5.1](#r51)

<!-- ac: AC5 -->
### AC5

The sprint update exists only as a recurring issue whose acceptance criteria name validate; it closes by its merge and is re-created for the next week; an actionable signal becomes a pushed issue.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.3](#r63)

<!-- ac: AC6 -->
### AC6

status-issues leaves exactly one status issue per listed person for the coming week, recurring the last closed one or filing a fresh one, and files nothing on a second run; week returns the open Sprint Goal issues with due dates and the person's activity since the week began, the system repository's from its own first issue, with the issues its counted merges closed.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R8.1](#r81)

<!-- ac: AC7 -->
### AC7

Each listed person receives one weekly status issue due the day after the reminder weekday, which mentions them without assigning them; worked with implement it produces a report with status, problems, and complete or not complete per open Sprint Goal that passes scrum-master reports validate, and it is re-created with the next due date.

<!-- field: traces -->
#### Traces

- [R9.1](#r91)
- [R9.2](#r92)
- [R9.3](#r93)
- [R9.4](#r94)

<!-- ac: AC8 -->
### AC8

A system release tag renders the deck from the merged, lead-reviewed sprint-end summary and commits it beside the summary, with no branch, review issue, or model step; a second run on the same tag commits nothing.

<!-- field: traces -->
#### Traces

- [R10.1](#r101)
- [R10.2](#r102)
- [R10.3](#r103)

<!-- ac: AC9 -->
### AC9

Release 06.0's planning writes the forecast requirement items from R11.1 and the stakeholder update's from R11.3; until then the interim rules of R11.2 and R11.3 hold.

<!-- field: traces -->
#### Traces

- [R6.2](#r62)
- [R11.1](#r111)
- [R11.2](#r112)
- [R11.3](#r113)

<!-- ac: AC10 -->
### AC10

With the roster in the system repository, status-issues leaves exactly one status issue per listed person in the repository they report in, asking for the report at status/reports in that repository, one team rollup issue per subsystem for its owner, and one program rollup issue for the lead, each due after the last, and files nothing on a second run; the roster reads its due offsets and titles with their defaults.

<!-- field: traces -->
#### Traces

- [R12.1](#r121)
- [R12.2](#r122)
- [R16.1](#r161)
- [R16.2](#r162)

<!-- ac: AC11 -->
### AC11

collect for a team returns its subsystem's merged reports and for the program every repository's reports and the teams' rollups, each with its repository and commit, a week outside the period left out, the open status issues listed, through gh and glab with no checkout and through a checkout beside this one, and writes nothing.

<!-- field: traces -->
#### Traces

- [R13.1](#r131)
- [R13.2](#r132)
- [R13.3](#r133)

<!-- ac: AC12 -->
### AC12

previous returns a person's last report before the week with its commit, blockers, impact, and next, and says when there is none; path names the week's report and rollup paths and the report's branch.

<!-- field: traces -->
#### Traces

- [R14.1](#r141)
- [R14.2](#r142)

<!-- ac: AC13 -->
### AC13

validate passes a rollup whose every claim cites a listed source, and fails one whose file name disagrees with its front matter, whose claim cites an unlisted source or none, or whose team level names no subsystem; the rollup format's example passes.

<!-- field: traces -->
#### Traces

- [R15.1](#r151)
- [R15.2](#r152)

<!-- ac: AC14 -->
### AC14

aggregate over a collection across two repositories reports a blocker shared between them, a missing report naming its open status issue, and unreported merged work, and phrases none of them as a comparison of people.

<!-- field: traces -->
#### Traces

- [R17.1](#r171)

<!-- ac: AC15 -->
### AC15

status-report and status-rollup render from their profiles, every word they use has a model-free profile, and every profile validates; a status issue worked with status-report merges a report at status/reports in its repository that passes reports validate, and no other repository is written; a team rollup and then a program rollup worked with status-rollup each merge in their own repository, pass reports validate, and cite every claim by repository and commit.

<!-- field: traces -->
#### Traces

- [R18.1](#r181)
- [R18.2](#r182)

<!-- ac: AC16 -->
### AC16

The site's status pages show the latest program rollup the system repository holds; plan's candidates read the latest program rollup's progress; sprint-end facts aggregate the reports collected in place when the roster is in the system repository.

<!-- field: traces -->
#### Traces

- [R18.3](#r183)

<!-- ac: AC17 -->
### AC17

init writes an empty roster at control/reports.md; doctor moves a central reports repository's roster there once, naming that repository, reports it on a dry run without moving it, and moves nothing a second time or where a roster is in place.

<!-- field: traces -->
#### Traces

- [R18.4](#r184)
