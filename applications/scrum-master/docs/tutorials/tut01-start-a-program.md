<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Tutorial 1: Start a Program

This tutorial takes you from an empty repository to a working program:
the agent installed, a tracker seeded, the corpus laid down, and the
skills in place. At the end you have a system repository whose check
profile passes, ready for the first issue of
[Tutorial 2](tut02-file-and-deliver-an-issue.md).

Every tutorial in this series shows two ways to run a workflow. In an
agent session (Claude Code, Cursor, or GitHub Copilot) you ask in plain
words — "initialize this repository as a program" — and the installed
skill runs. On the command line you run the same profile directly with
the `agent` binary; that is the form the steps below show, because it is
exact. The two are one machine (design.md D14).

## Prerequisites

- git, and a forge account (GitHub or GitLab) if the program is shared
- an agent the skills install for: Claude Code, Cursor, or GitHub
  Copilot

## Step 1: Install the agent and the profiles

Run the install command in the [README's Install section](../../README.md#install).
This installs the `agent` binary in `$HOME/.local/bin` and unpacks the
release's profiles in `$HOME/.local/share/scrum-master`, both verified
against the release's `SHA256SUMS`. `AGENT_PREFIX` and
`SCRUM_MASTER_HOME` change the two locations; make sure the binary's
directory is on your `PATH`. To install from a GitLab fork's releases
instead, see the README.

## Step 2: Choose a tracker

The program's facts live in a tracker. Three choices:

- **A tracker repository** — a dedicated git repository the program
  writes through beads (srd042). Create an empty repository, for example
  `<program>-tracker`, and note its remote. This is the default for a
  program of several repositories.
- **A tracker branch** — a program of one team and one repository keeps
  the tracker on an orphan branch of that repository, such as
  `scrum-master/tracker`, and `main` stays protected.
- **The forge's own tracker** — `gh` (GitHub) or `glab` (GitLab):
  scrum-master drives that CLI with your existing login. `gh`'s token
  needs the `repo` and `read:org` scopes, `glab`'s the `api` scope.

## Step 3: Run the init profile

In the program's repository — a fresh `git init` is fine — name the
tracker and yourself, write the init request, and run the profile:

```bash
export SCRUM_MASTER_TRACKER=<tracker remote, or gh, or glab>
export SCRUM_MASTER_USER=<your name in the tracker>
home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
printf '{"parameters": {"tracker": "%s", "group": "<program group>", "agent": "claude"}}' "$SCRUM_MASTER_TRACKER" > /tmp/init.json
agent --profile "$home/agents/init/profile.yaml" --core-root "$home/agent-core" --directory . --request /tmp/init.json
```

`agent` in the request names whose skills install: `cursor`, `claude`,
`copilot`, or `all`. Without it, init installs for the agents the
repository already uses and refuses when it uses none. For the tracker
branch variant, add `"tracker_branch": "scrum-master/tracker"` to the
parameters with the repository's own remote as `tracker`.

The run ends in the `Done` state. It wrote the corpus — `AGENTS.md`,
`control/`, `docs/` with `VISION.md` and `ARCHITECTURE.md` as templates,
the specification index, and the road map — vendored the release's
profiles and the agent-core declarations into `.scrum-master` with a
manifest naming the release, installed the skills where your agent reads
them, made the tracker's first commit, and created the program's labels
there.

## Step 4: Commit and push

```bash
git add -A && git commit -m "Lay down the program" && git push origin main
```

The issue workflow reads the control files from `main`, so this commit
comes before the first issue. Commit `.scrum-master` with the rest; the
installed skills themselves stay untracked, listed in a managed block in
`.git/info/exclude`.

## Step 5: Check the repository

```bash
agent --profile .scrum-master/agents/check/profile.yaml --core-root .scrum-master/agent-core --directory .
```

The check profile runs what CI runs: the corpus gate, the vendored
profiles against their manifest, and the drift doctor's report, writing
nothing. It also names any environment variable your setup still sets
under an old name. A CI job runs the same command after installing the
agent with the install script; the README's "Run in CI" section has the
job.

## Joining an existing program

A repository that joins a program runs `onboard` instead of `init`, with
the system repository named:

```bash
export SCRUM_MASTER_SYSTEM=<system repository path or remote>
export SCRUM_MASTER_SUBSYSTEM=<the subsystem this repository builds>
home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
agent --profile "$home/agents/onboard/profile.yaml" --core-root "$home/agent-core" --directory .
```

Onboard lays down the repository's corpus and orientation, registers it
in the system repository, installs the skills, and vendors the profiles,
the same way init does.

A new team often starts with its scrum repository, where it plans, before
any code repository. Onboard it with the `scrum` role, naming the team's
owner and components:

```json
{"parameters": {"role": "scrum", "owner": "Product Owner, platform", "components": "platform"}}
```

When the system repository has no subsystem SRD for the team yet, the
registration asks to create it: the owner, `scrum_repository` set to this
repository, and one row per component, declared on the scrum repository
until a code repository joins and takes the row. Each code repository then
onboards with the default `component` role.

scrum-master finds the program's system repository from any of its
checkouts. Two layouts work with nothing set:

```text
~/code/system          # side by side: every checkout in one folder
~/code/fleet

~/GITLABS/acme/tpm             # mirroring the forge's groups
~/GITLABS/acme/platform/tpm    # a scrum repository of acme/platform
```

In the second, scrum-master looks beside each folder above the checkout,
up to your home folder, and takes the system checkout whose group holds
your repository. With the system repository checked out nowhere near,
name it, and scrum-master reads its `control/trackers.md` through `glab`
or `gh`:

```bash
export SCRUM_MASTER_SYSTEM=acme/tpm
```

## Upgrading to a new release

Rerun the install script from step 1, then run the upgrade profile from
the installed profiles — the ones in `.scrum-master` are the release you
are leaving:

```bash
home="${SCRUM_MASTER_HOME:-$HOME/.local/share/scrum-master}"
agent --profile "$home/agents/upgrade/profile.yaml" --core-root "$home/agent-core" --directory .
```

It vendors the new release into `.scrum-master`, adds the files the older
release did not write, refreshes the managed blocks, moves the format pin,
repairs the documents, and rewrites the skills, keeping every file a
person edited (srd043 R5). It seeds no tracker and files no issue, so it
is safe in a system and a subsystem repository alike. Commit what it
changed and run the check profile from step 5.

## What you have

A program repository whose check profile passes, a seeded tracker with
the program's labels, and the skills installed for your agent. Next:
[file and deliver your first issue](tut02-file-and-deliver-an-issue.md).
