<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 6: Lay Out a Program on Its Forge Group

This tutorial lays a program of several teams out on its forge group: a
system repository for the plan's top, a repository per initiative, and a
subgroup per pod with its scrum and code repositories. At the end the
registry — the system repository's subsystem SRDs — declares every
repository and what it holds, one command lays the checkouts down on any
machine, and the site files every page where nothing collides. It builds
on [Tutorial 1](tut01-start-a-program.md), which made the system
repository, and [Tutorial 4](tut04-write-the-backlog.md), which wrote a
backlog.

## The template

A program's plan spans three kinds of repository, mirrored by its forge
group:

```
acme/                        group
├── program                  system repository: control/, themes, initiatives, cross-team epics
├── <theme>/                 a subgroup per theme
│   └── <initiative>         the initiative's repository: its epics and features, its ARCHITECTURE
└── pods/
    └── <pod>/               a subgroup per pod (team)
        ├── tpm              the pod's scrum repository: stories, Sprint Goals
        └── <component>      code repositories
```

- **The system repository** (`acme/program`) holds the plan's top —
  themes, initiatives, and cross-team epics — with `control/` and the
  registry. Tutorial 1 laid it down.
- **An initiative repository** (`acme/analytics/model-evaluation`)
  holds one initiative's epics and features and its own ARCHITECTURE.
  Its subsystem SRD carries the context item `initiative: in001-…`, so
  the site draws its epics and features under that initiative, and the
  repository gets no scrum page: project management lives here, scrums
  do not (srd048 R8.6).
- **A pod's scrum repository** (`acme/pods/platform/tpm`) holds the
  team's user stories, Sprint Goals, sprint-end summaries, and rollup.
  Its SRD names it with `scrum_repository`; the pod's **code
  repositories** sit beside it as the SRD's components (srd045 R3.4).

A story in a pod's scrum repository pins its parent — a feature or epic
in an initiative repository, or one in the system repository — by URL,
and the site attaches it there (srd048 R3.1). On the site, every
repository's pages file under its path below the group
(`site/hierarchy/analytics/model-evaluation/ep001`,
`site/scrums/pods/platform/tpm`), so two pods may both call their scrum
repository `tpm`; the system's documents stay at the top
(`site/hierarchy/in001`).

The template is one layout, not the only one: a small program keeps its
epics, features, and even stories in the system repository, as Tutorial
4 wrote them, and grows into subgroups when teams need repositories of
their own. The registry says what each repository holds either way.

## Step 1: The group and the repositories

On the forge, create the group and its repositories to match the plan:
the system repository at the group's top, a subgroup per theme with a
repository per initiative, and `pods/<pod>/` subgroups, each with a
scrum repository and the pod's code repositories.

## Step 2: An initiative repository joins

In the initiative's repository, run onboard with the initiative role,
naming the initiative document the system repository holds:

```bash
scrum-master onboard --system acme/program --subsystem srd010-model-evaluation \
  --role initiative --initiative in001-reliable-agents
```

This writes the repository's corpus skeleton and files one registration
issue in the system repository asking for the subsystem SRD with the
`initiative` context item — or, where the SRD already lists the
repository, to set the item on it (srd043 R2.2). Nothing is committed to
the system repository; the SRD's owner merges the registration. Write
the initiative's epics under `docs/specs/epics/` and features under
`docs/specs/features/`, parents local (`- parent: ep100-…`); a top epic
needs no parent — the `initiative` item attaches it.

## Step 3: A pod joins

In the pod's scrum repository, and then in each code repository:

```bash
scrum-master onboard --system acme/program --subsystem srd020-platform \
  --role scrum --owner "Product Owner, platform"
scrum-master onboard --system acme/program --subsystem srd020-platform \
  --role component --path /
```

The first registration creates the SRD naming the scrum repository,
where the pod plans; each code repository then takes a components-table
row. The pod's stories pin their parents in the initiative repository by
URL:

```markdown
- parent: https://gitlab.example.com/acme/analytics/model-evaluation/-/blob/main/docs/specs/features/ft100-suite.md
```

## Step 4: Lay the checkouts down

On any machine, clone the system repository at its group path and let
scrum-master fetch the rest of the registry (srd043 R6):

```bash
git clone git@gitlab.example.com:acme/program.git acme/program
cd acme/program && scrum-master checkouts
```

Every registered repository is cloned at its group path beside the
system checkout, and the ones already there are fast-forwarded; a
checkout on another branch or holding changes is kept with its reason.

## Step 5: Read the program on its site

Run the site words in the reports checkout (Tutorial 5) and open the
site. The plan's tree shows each initiative with its repository's epics
and features under it, the initiative's page names the repository and
links its ARCHITECTURE, each pod's scrum page sits at its group path
with its Sprint Goals and stories, and the roll-up counts a pod's story
in the feature it pins, the initiative above it, and the theme (srd048
R8.1, R8.6).

## Where this leaves you

The group mirrors the plan: the registry declares every repository and
what it holds, `scrum-master checkouts` reproduces the layout anywhere,
and the site reads the whole program without a collision. A new
initiative is a repository and one registration; a new pod is a
subgroup, one scrum registration, and a component registration per code
repository.
