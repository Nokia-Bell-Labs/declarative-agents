<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# scrum-master

A technical program manager as a toolchain: `scrum-master`, a Go CLI that does Scrum's administrative work against a GitLab tracker, and the agent skills that drive it.

Most of what Scrum asks of people outside the work itself is moving facts the tracker already holds: standup updates, board changes, status reports, release bookkeeping. `scrum-master` reads and writes those facts directly — objectives, sprint ends, weekly reports, release gates and tags — so people spend their process time on judgment, not administration. The repository is specification-driven: the documentation corpus under `docs/` is the source of truth, and release gates hold the code to it.

## Layout

| Path | Role |
|---|---|
| `words/` | The words: every step a program can do, which the agent runs in its own process |
| `internal/` | Tracker, planning, reporting, and release logic |
| `plugin/` | Agent Plugins 1.0 package: the profiles in `plugin/agents/`, and the skills rendered from them, which the init and onboard profiles install |
| `docs/` | The specification corpus: SRDs, use cases, test suites, constitutions, guides |
| `infra/` | Terraform module and the regression-rig root |
| `magefiles/` | Build, gates, corpus audit, and integration targets |

## Install

Each release of this repository publishes the agent for Linux and macOS on amd64 and arm64, built with the scrum-master words, and the profiles archive — the profiles and the agent-core declarations they read — in one `SHA256SUMS`, on its [GitHub release](https://github.com/Nokia-Bell-Labs/declarative-agents/releases). Install the latest, or name a tag, with the script, which checks both against `SHA256SUMS`, installs the agent in `$HOME/.local/bin` (`AGENT_PREFIX` changes that), and unpacks the profiles in `$HOME/.local/share/scrum-master` (`SCRUM_MASTER_HOME` changes that):

```bash
curl -fsSL https://raw.githubusercontent.com/Nokia-Bell-Labs/declarative-agents/main/applications/scrum-master/install.sh | sh
```

No Go toolchain is needed: the agent runs the words, and the corpus gate's shape check, in its own process.

To install from a GitLab fork's releases, such as an internal one, set `SCRUM_MASTER_RELEASE_FORGE=gitlab` and `SCRUM_MASTER_RELEASE_URL` to the project's URL, with `GITLAB_TOKEN` for a private project; `docs/engineering/eng03-releasing-scrum-master.md` says how the fork publishes them.

## Tutorials

Five walkthroughs under `docs/tutorials/` take a new program from nothing to a running process: [start a program](docs/tutorials/tut01-start-a-program.md), [file and deliver an issue](docs/tutorials/tut02-file-and-deliver-an-issue.md), [plan a release and run the sprint](docs/tutorials/tut03-plan-a-release-and-run-the-sprint.md), [write the backlog](docs/tutorials/tut04-write-the-backlog.md), and [weekly status](docs/tutorials/tut05-weekly-status.md). The guides under `docs/guides/` explain the model behind them, starting with [the process](docs/guides/process.md) and [plan-vs-architecture](docs/guides/plan-vs-architecture.md).

## Start a program with the agent

A program repository runs its workflows with the agent: each installed skill runs `agent --profile .scrum-master/agents/<name>/profile.yaml --core-root .scrum-master/agent-core --directory .` over the profiles the repository vendored. The one workflow a person runs directly is `init` in a system repository, or `onboard` in a repository joining a program: it lays the repository down, installs the skills, and vendors the profiles and the agent-core declarations they read into `.scrum-master`, with a manifest naming the release. Commit `.scrum-master` with the rest; the check profile reports what drifted, and the upgrade profile takes a new release (below).

Run `init` from the installed profiles (`SCRUM_MASTER_SYSTEM` and `SCRUM_MASTER_SUBSYSTEM` name where an `onboard` joins):

```bash
home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
agent --profile "$home/agents/init/profile.yaml" --core-root "$home/agent-core" --directory .
```

A run's request file (`--request`, `{"parameters": {...}}`) names the agents whose skills install (`agent`, default `all`) and, for `init`, the tracker to seed (`tracker`, `group`, `prefix`).

A profile that refuses exits non-zero. Its reason is the last `scrum-master:` line before `terminal state: failed`, and `final machine state` names the step it stopped at; nothing after that step ran. The workflows that act on one issue (pop, approve, merge, close, comment, admit, recur, peer-read, and release) all read it the same way: `SCRUM_MASTER_ISSUE` is its number in the project `SCRUM_MASTER_PROJECT` names, and the release words also take it whole as `project#iid`. The goals and plan workflows cite documents under `SCRUM_MASTER_WEB_BASE`, which defaults to the scheme and host of the checkout's origin.

## Upgrade to a new release

Rerun the install script to put the new release on the machine, then run the upgrade profile in each program repository. Run it from the installed profiles, since the ones the repository vendored are the release it is leaving:

```bash
curl -fsSL https://raw.githubusercontent.com/Nokia-Bell-Labs/declarative-agents/main/applications/scrum-master/install.sh | sh
home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
agent --profile "$home/agents/upgrade/profile.yaml" --core-root "$home/agent-core" --directory .
```

The profile vendors the installed release into `.scrum-master`, runs doctor to add the files the release lays down that are missing, refresh the managed blocks, move the format pin in `docs/external-pins.yaml`, and repair the documents to its format, and rewrites the skills of the agents the repository uses. It works in a system and a subsystem repository alike, seeds no tracker, files no issue, and keeps every file a person edited, so a second run changes nothing. Commit what it changed; the check profile confirms the repository is in order. A CI job moves by the release tag it installs, or the image digest it pins.

## Run in CI

A CI job runs the repository's vendored check profile with the agent over its checkout: the corpus gate, the vendored profiles held to their manifest, and doctor's drift report, writing nothing. The releases publish no container image, so the job installs the agent with the install script, pinned to a release tag, on any image with sh, curl, and git:

```yaml
scrum-master-check:
  image: <an image with sh, curl, and git>
  before_script:
    - curl -fsSL https://raw.githubusercontent.com/Nokia-Bell-Labs/declarative-agents/main/applications/scrum-master/install.sh | sh -s -- <tag>
    - export PATH="$HOME/.local/bin:$PATH"
  script:
    - agent --profile .scrum-master/agents/check/profile.yaml --core-root .scrum-master/agent-core --directory .
```

A program that would rather pull a ready image publishes one to its own registry: from a checkout at the release's tag, with docker buildx and a login to that registry, `SCRUM_MASTER_RELEASE_IMAGE=<registry>/scrum-master mage -d magefiles publish` builds the image for linux/amd64 and linux/arm64 from the release's binaries and pushes it — the agent, built with the scrum-master words, and the release's profiles under `SCRUM_MASTER_HOME`, with git and git-crypt, no Go toolchain, running as a non-root user — and the job names it by digest as its `image:`, with no install step. Tracker credentials (`SCRUM_MASTER_TRACKER`, `SCRUM_MASTER_USER`) and any model credentials a profile names come from masked CI variables; `SCRUM_MASTER_USER` names a service user, never a person.

## Weekly status

Each person writes their weekly report in the repository they worked in with the status-report skill, which gathers the week with words and asks only what the tracker cannot know. Each team's lead rolls the team's reports up there with the status-rollup skill, and the technical program manager rolls the teams up in the system repository; every rollup claim cites the report it rests on by repository and commit. Who reports, and where, is the roster in the system repository's `control/reports.md`, and the status-issues profile files the week's status and rollup issues from it. [Tutorial 5](docs/tutorials/tut05-weekly-status.md) walks through the loop; a program that kept a central reports repository moves its roster with the upgrade profile.

## The program's site

The site profile writes the program's site into the reports repository's `site/` folder (srd048), the repository that is now the site's home only, as reports are written where the work is: the status pages (road map, story-point roll-up, architecture status, Sprint Goals, signals, open issues), the timeline (each initiative, epic, feature, story, Sprint Goal, and issue on the sprint axis, now and as planned at every corpus tag, as mermaid gantt charts and one self-contained HTML timeline), the requirement history between corpus tags with the merge behind each change, and the corpus documents as pages. The facts behind every page are JSON under `site/data/` and `site/timeline/`. Every generated page is stamped; a page a person edits is kept, never replaced, and `site/index.md` is the program's once written.

Read it on GitLab Pages: `site/index.html`, the app shell the scaffold writes, renders the generated Markdown in the browser. The Pages URL opens on the program's overview: the interactive timeline, a tile per initiative with its progress, and the releases. The navigator on the left lists the status pages, the plan from every theme down to every story, and the program's repositories on the forge. Search covers every page, every story included. Each theme, initiative, epic, feature, and story has its own page linking up to its parent, down to its children, and out to the Sprint Goals and issues that serve it, so you can follow the work in either direction. One `pages` job publishes the folder as it is:

```yaml
pages:
  stage: deploy
  script:
    - rm -rf public && cp -r site public
  artifacts:
    paths:
      - public
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

The shell loads Docsify and mermaid from pinned CDN URLs. Like every generated page it is refreshed each run and kept once edited, so an air-gapped GitLab can vendor those libraries into `site/` and point the shell at them. The repository view stays the no-tooling fallback: GitHub and GitLab render the Markdown and its mermaid charts as they are, so a reader with repository access needs nothing else. Run the profile from a reports checkout with the system repository checked out beside it:

```bash
agent --profile "$SCRUM_MASTER_HOME/agents/site/profile.yaml" --core-root "$SCRUM_MASTER_HOME/agent-core" --directory .
```

It writes into the working directory unless `SCRUM_MASTER_REPORTS` names another checkout. When the system repository encrypts its documents with git-crypt and the reports repository does not encrypt `site/`, the corpus pages are withheld, with a page saying why; encrypt `site/` in the reports repository, or set `SCRUM_MASTER_SITE_DECRYPTED=1` to copy them anyway. Each corpus tag's documents are read into a snapshot under the system checkout's `.git/scrum-master/site`, or where `SCRUM_MASTER_SITE_CACHE` names.

The site never commits: its pages land like any other change, through an issue that is filed, popped, and merged. In CI, regenerate it on a schedule and on every corpus tag, so the history gains a step at each plan:

```yaml
scrum-master-site:
  image: <an image with sh, curl, and git>
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - if: $CI_COMMIT_TAG =~ /^corpus\/v/
  before_script:
    - curl -fsSL https://raw.githubusercontent.com/Nokia-Bell-Labs/declarative-agents/main/applications/scrum-master/install.sh | sh -s -- <tag>
    - export PATH="$HOME/.local/bin:$PATH"
  script:
    - git clone "$SYSTEM_REPOSITORY" ../system
    - home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
    - agent --profile "$home/agents/site/profile.yaml" --core-root "$home/agent-core" --directory .
```

The job runs in the reports repository; on a corpus tag in the system repository, trigger it with a pipeline trigger. The changed pages then land through the issue workflow, filed, popped, and merged by the issue-file, pop, and merge profiles with the job's service user.

## Seed a program repository from an architecture document

A program's system repository can be generated from one document that describes the system the program builds: its purpose, layers, parts, and a build order. The init profile lays down the corpus, and a coding agent given the brief below writes the program's intent on top of it. Keep the seed in the repository (for example under `seed/`), so the program can be regenerated whenever the seed or the release changes.

The tracker repository comes first, so every step after it is recorded there. The init profile's output is the one commit made to `main` directly: the issue workflow reads the control files from `main`, so they must be there before the first issue is filed. The generated program then lands like any other change, as an issue that is filed, popped, approved by a second person, and merged.

The init profile writes thirteen files that need no further work, besides the vendored profiles in `.scrum-master`: `AGENTS.md`, `control/labels.md`, `control/policy.md`, `control/states.md`, `docs/SPECIFICATIONS.md`, `docs/constitutions/design.md`, `docs/corpus-charter.yaml`, `docs/external-pins.yaml`, `docs/road-map.md`, `docs/specs/software-requirements/srd001-system.md`, and a `README.md` in each of `docs/specs/themes/`, `initiatives/`, and `epics/`. It writes `docs/VISION.md` and `docs/ARCHITECTURE.md` as templates, and appends a managed block between `managed:begin` and `managed:end` markers to the repository's `README.md`. The agent writes the two templates and the planning hierarchy.

1. Create an empty tracker repository for the program (for example `<program>-tracker`). Then, in the program's repository, with the seed committed, lay down the corpus, seed the tracker, and commit the init profile's output:

   ```bash
   export SCRUM_MASTER_TRACKER=<tracker remote> SCRUM_MASTER_USER=<your name in the tracker>
   home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
   printf '{"parameters": {"tracker": "%s", "group": "<program group>", "agent": "<cursor, claude, copilot, or all>"}}' "$SCRUM_MASTER_TRACKER" > /tmp/init.json
   agent --profile "$home/agents/init/profile.yaml" --core-root "$home/agent-core" --directory . --request /tmp/init.json
   git add -A && git commit -m "Lay down the program" && git push origin main
   ```

   Without `agent`, the init profile installs the skills of the agents the repository already uses (a `.cursor` or `.claude` directory, `CLAUDE.md`, or copilot's instructions under `.github`), and refuses when it uses none. It refuses, writing nothing, when no release's profiles are installed. The init profile makes the tracker's first commit and creates the program's labels there. Without a tracker in its request it says that no tracker was seeded, and no issue can be filed until one is.

   The environment variables were named `TPM_` until GH-38. A `TPM_` name is still read where its `SCRUM_MASTER_` name is unset, and the check skill names each one a setup still sets, so a CI job can move at its own pace.

   A repository can use GitHub or GitLab instead of a tracker repository: name it `gh` or `glab` in the system repository's `control/trackers.md`, or run with `SCRUM_MASTER_TRACKER=gh` or `SCRUM_MASTER_TRACKER=glab`. scrum-master then drives that CLI with your existing login: `gh`'s token needs the `repo` and `read:org` scopes, and `glab`'s the `api` scope (`gh auth status` and `glab auth status` show them); the check skill reports a missing or logged-out CLI.

   A program of one team and one repository can keep its tracker on a branch of that repository instead of a tracker repository of its own. Name the repository's own remote as `tracker` and a dedicated branch as `tracker_branch` in the init request, for example `{"parameters": {"tracker": "<the repository's remote>", "tracker_branch": "scrum-master/tracker", "group": "<program group>"}}`. The init profile makes the branch as an orphan holding only the tracker's config file, never merged into `main`, and writes it to `control/trackers.md`; `SCRUM_MASTER_TRACKER_BRANCH` replaces it for one person's run. Every member pushes to the tracker branch directly, while `main` stays protected: on GitHub, a ruleset or branch protection rule that targets `main` only, with none on `scrum-master/tracker`; on GitLab, `main` protected and `scrum-master/tracker` left unprotected, or protected with Developers and Maintainers allowed to push.

2. File the generation issue with the specify skill and pop it into its own branch and worktree with the pop skill. The issue names the documents it changes; a planning document among them makes it a `plane::management` issue. Its body:

   ```bash
   cat > /tmp/generate.md <<'EOF'
   ## Requirements

   1. Generate the program from <seed path>: docs/VISION.md, docs/ARCHITECTURE.md, and the planning hierarchy under docs/specs/.

   ## Acceptance Criteria

   - The hierarchy gate passes (test: scrum-master backlog check; environment: local)
   EOF
   agent --profile .scrum-master/agents/pop/profile.yaml --core-root .scrum-master/agent-core --directory .   # with SCRUM_MASTER_PROJECT and SCRUM_MASTER_ISSUE set
   ```

   The issue needs no `component::` label: the program's subsystem SRDs declare no component until the hierarchy and its SRDs exist. The pop workflow reports the worktree it opened, a sibling of the checkout. Steps 3 to 5 run there.

3. In that worktree, give a coding agent this brief, with the angle-bracket placeholders filled in:

   > You are generating the program for `<program name>` in this repository, which the init profile has just laid down. Read `<seed path>` (the system description), and use the backlog templates the draft skill shows for the document formats; `docs/constitutions/design.md` declares the fields each document may hold. Write only the files listed here and change nothing else.
   >
   > **Files to write.**
   > 1. `docs/VISION.md`: fill every field the init profile laid down (executive summary, problem, what this does, why we build this, success criteria as named `key: outcome` items, not).
   > 2. `docs/ARCHITECTURE.md`: fill every field the init profile laid down. The overview has a summary and a layer diagram as an inline Mermaid block, marking each part built, partly built, or deferred in this program. Interfaces are `interface:` nodes for each interface the program owns. Components are `component:` nodes, one per layer and per service, each with capabilities and ownership, plus one for the deferred parts. Design decisions are numbered `decision:` nodes, each saying what it rejected. Also fill technology choices, project structure, implementation status, and related documents. Draw the delivery chain, if the program has one, as a second Mermaid block.
   > 3. The planning hierarchy under `docs/specs/`: one theme (`themes/th001-<name>.md`), one initiative (`initiatives/in001-<name>.md`), and the epics (`epics/ep001-<name>.md` onward).
   >
   > **Program parameters (these are not in the seed).**
   > - Platform and hosting: `<cloud and delivery tools; the interfaces the program owns>`.
   > - Platforms to integrate rather than rebuild: `<existing systems, and the capabilities each already provides>`. Wherever a capability might duplicate one, a requirement item says that platform's coverage is confirmed first.
   > - Teams that own epics: `<team names>`. An epic's owner is `Product Owner, <team>`; the theme's and initiative's owner is `technical program manager`.
   > - Cadence: `<sprint length and alignment, weeks off>`. The theme's horizon is `<ISO week range>`. The initiative's horizon is `<ISO week range>`, and it has one requirement group per sprint, due on `<each sprint's due date>`.
   > - Sprint milestones, following the seed's build order: `<one observable milestone per sprint>`.
   > - Theme: `<the strategic area>`. Its requirement groups follow `<the seed's principles or practices>`.
   > - Epics, platform setup first: `<one epic per major part of the system, each with its owning team>`. Each epic has a horizon longer than one sprint, `depends_on` links to the epics it needs, and acceptance criteria that name a test and an environment (`local`, `dev`, or `staging`).
   > - Deferred: `<the parts of the system this program does not build>`, and the documents say so.
   >
   > **Rules.** Every hierarchy document has status `to_be_agreed`. Every acceptance criterion traces only to items in its own document. Leave `delivered_by` out until the teams' requirement documents exist. Diagrams are inline Mermaid, with no image files. Write no personal names and no links to private repositories. When the files are written, run `scrum-master backlog check` and fix every finding.

4. Run the check in the worktree, and fix every finding until it passes:

   ```bash
   agent --profile .scrum-master/agents/check/profile.yaml --core-root .scrum-master/agent-core --directory .
   ```

5. Commit the generated program and push its branch. The merge runs the hierarchy gate as its audit, since a program repository has no `mage audit`. When the program's policy sets `require_approval`, someone other than the issue's assignee first approves the exact commit:

   ```bash
   git add -A && git commit -m "Generate the program from the seed" && git push -u origin HEAD
   SCRUM_MASTER_USER=<reviewer> agent --profile .scrum-master/agents/approve/profile.yaml --core-root .scrum-master/agent-core --directory .   # the reviewer, under require_approval
   agent --profile .scrum-master/agents/merge/profile.yaml --core-root .scrum-master/agent-core --directory .
   ```

   The merge pushes to `main` and closes the issue.

6. Tag the merge, so later runs and regenerations can be compared against it. In the program's checkout:

   ```bash
   git pull --ff-only origin main
   git tag -a "baseline/$(date -u +%Y%m%dT%H%M%SZ)" -m "Program generated from the seed"
   ```

Regenerating from a changed seed is the same loop from step 2: one issue, one branch, one merge.

Every hierarchy document starts as `to_be_agreed`; agreeing them is the program's first planning decision, and the plan skill reads only agreed documents. To add or rework one document at a time afterwards, use the `draft` skill, which drafts any level from a description in conversation with its owner.

## Build and test

```
go test ./...
mage -d magefiles -l
```

The agent this repository runs is agent-core's, built with the words (`mage -d magefiles integration:popProfile` builds it into the release cache); the rig drives the words through its harness, `magefiles/words`.

Releases are cut by `mage -d magefiles tag`, which runs the gates in `release-gates.md`, tags `v0.YYYYMMDD.N`, and once the tag is pushed publishes the agent and the profiles with `mage -d magefiles publish` (`docs/engineering/eng03-releasing-scrum-master.md`). The corpus audit requires a clone cache of the parent specification repository; `docs/guides/corpus-audit.md` describes it.

Issue tracking lives in the repository via [beads](https://github.com/steveyegge/beads); the GitHub tracker is unused.

## License

[MIT No Attribution (MIT-0)](LICENSE).
