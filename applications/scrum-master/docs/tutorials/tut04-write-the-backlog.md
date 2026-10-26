<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 4: Write the Backlog

This tutorial writes the planning hierarchy: a theme broken into
initiatives, an initiative into epics, an epic into features and stories,
each a document in the system repository (srd045, srd046). It assumes a
program from [Tutorial 1](tut01-start-a-program.md). The release planning
of [Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md) chooses from
what this tutorial writes.

## The hierarchy in one paragraph

The planning hierarchy breaks strategy into work: vision, theme,
initiative, epic, feature, story, and the issue that executes a story.
Each level is a document under `docs/specs/` with the fields its level
requires, linked to its parent: themes, initiatives, and epics in the
system repository, and features (`docs/specs/features/`) and stories
(`docs/specs/stories/`) in the owning team's repository. The hierarchy
gate holds the tree to that form. A document cites only components the repository's `ARCHITECTURE.md`
declares — a component that is not there needs a design decision first.
How the plan tree relates to the architecture is
[plan-vs-architecture.md](../guides/plan-vs-architecture.md).

## Step 1: See what is there

In an agent session, or directly:

```bash
agent --profile .scrum-master/agents/backlog-list/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The backlog-list profile names every theme, initiative, epic, feature,
and story with its kind, and a story's points and rank. On a fresh
program it is empty, and the first draft starts the tree. Two more words
help before writing: backlog-template prints a level's template, and
backlog-components lists the components `ARCHITECTURE.md` declares, which
a document may cite.

## Step 2: Draft a document in conversation

The draft skill turns a description into one document at any level, in
conversation with its owner (srd046). In an agent session:

> Draft a theme from these notes: our platform should let operators
> declare network intents instead of configuring devices; over the next
> two years we move provisioning, assurance, and optimization onto that
> model.

The skill surveys the hierarchy first, so the draft lands under the right
parent with the fields its level requires. Then it converses: it says
what it understood the item to be, proposes its id, title, and parent,
names any document it overlaps and a scope too large or small for the
level, and asks only what the description leaves open, at most five
questions at a time, each with a recommended answer — a theme's horizon
and outcomes; an initiative's horizon, milestones, and owning teams; an
epic's owner, horizon, dependencies, and non-goals; a feature's owner,
boundary, and tests; a story's as-a / I-want / so-that, points and rank,
each criterion's test and environment.

It writes the document with status `to_be_agreed`, runs the
`backlog_check` word, fixes every finding, and revises with you until the
document says what you mean. It never estimates, ranks, or assigns a
person — you give points and rank — and it never commits: the document
reaches `main` through the repository's own issue.

## Step 3: Walk down the tree

When a draft is done, the skill offers the next level down — initiatives
under a theme, epics under an initiative, features or stories under an
epic — and drafts each child the same way. A backlog deep enough to plan
from is a theme, its initiatives, and at least one epic whose stories
carry points and rank: that is what Sprint Planning consumes.

## Step 4: Check the tree

```bash
agent --profile .scrum-master/agents/backlog-check/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The backlog-check profile holds every document to its gate: the form of
each level, the links between them, and the citations into the
architecture. It writes nothing; a finding names the document and what it
lacks.

## Step 5: Land the documents through an issue

The drafts are working-tree changes in the system repository. They land
like any other change to `docs/`: file a `plane::management` issue with
specify naming the documents, take it with implement, commit the drafts
in its worktree, and merge ([Tutorial 2](tut02-file-and-deliver-an-issue.md)).
The merge's audit gate checks the corpus, this hierarchy included, before
anything lands on `main`.

## Epics that span teams

A team owns an area: its subsystem and the components its subsystem SRD
lists. An epic whose work falls to more than one team lives in the system
repository, and the technical program manager owns it; each team's Product
Owner then drafts the stories for that team's part in the team's own
repository, with the epic as parent by its pinned URL. An epic whose
components all belong to one team lives in that team's repository, and its
Product Owner owns it. The draft skill offers the right owner for the
repository it writes in (`backlog template --kind epic --dir .` shows it).

Name in each document's `architecture:` the components it changes. They
are what tie the work to the teams:

- backlog-check warns when a team's story names a component its subsystem
  does not list, and says whose it is; when an epic in the system
  repository names a Product Owner; and when every component a
  system-repository epic names is one team's. Set `area_checks: refuse` in
  `control/policy.md` to make these findings instead of warnings.
- The epic's progress counts the stories in every team repository the
  subsystem SRDs name, read through the tracker, and the architecture page
  and the site name any repository that could not be read.
- When a release is planned, goals-plan recommends every team whose
  components the release item's agreed epics name, one Sprint Goal each,
  unless the item's `delivered_by` names the team. You confirm or change
  each recommendation.

## When planning stops at the architecture

A draft that needs a component `ARCHITECTURE.md` does not declare stops
with "this needs a design decision". That is the design skill's job
(srd047): a design decision becomes one issue whose only changes are to
`docs/ARCHITECTURE.md` and `docs/engineering/`, delivered through the
same issue workflow, and the backlog document cites the component once it
is declared. Intent changes ahead of implementation, and the plan follows
the architecture, never around it.

## What you have

A planning tree the hierarchy gate passes, agreed with its owners, on
`main` through audited merges — the backlog
[Tutorial 3](tut03-plan-a-release-and-run-the-sprint.md) plans releases
and sprints from.
