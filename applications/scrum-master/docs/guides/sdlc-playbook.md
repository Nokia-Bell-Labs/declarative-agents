<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# This Process and the AI-Native SDLC Playbook

Anthropic's [AI-native SDLC playbook](https://claude.com/resources/articles/the-ai-native-sdlc-playbook)
(August 2026) describes how a team reorganizes software delivery once agents
write most of the code. Its argument is that code is no longer the
bottleneck; planning, review, testing, and deployment are, because they still
move at human speed. It answers with a loop of six stages (plan, design,
build, test, deploy, maintain) in which every stage commits an artifact that
triggers the next, machines enforce the rules while agents work, and people
approve at gates. This page compares that design with ours. In short, the
two agree on the premise and on the inner loop; the playbook goes further in
governing the agent sessions themselves, and we go further in coordinating
several teams and in proving work in environments.

## Where we agree

Both designs make a committed specification the source of truth, have an
agent build from it, put a person at the gates, and run delivery as a loop
rather than a line. Most of the playbook's practices have a counterpart in
the inner loop of [ai-native-sdlc.md](ai-native-sdlc.md) or the outer loops
of [process.md](process.md).

Table 1: Playbook practices and their counterparts here.

| Playbook | This process |
|---|---|
| `intent.md`, then `spec.md`, then `plan.md`, each committed and each triggering the next stage | Initiatives (srd034), which are this process's intent, then SRDs, the requirements a Sprint Goal cites by pinned URL, then issues in the issue body's work format; the pop marker commit embeds the plan verbatim. Goals, intents, and Sprint Goals are one concept here (VISION, ARCHITECTURE decision 14) |
| Plan mode by default; nothing is implemented without an accepted plan | `tasks` breaks a story into issues, agreed with its owner, before `implement` builds anything |
| The product owner reviews and approves the agent's spec rather than writing it | The Product Owner writes requirements with an agent through specify and implement |
| Story points and refinement meetings retired | Issues are estimated in size and review complexity, with estimated against actual lines of code logged; story points stay only on epic backlog items, for velocity and forecasts (srd045), and refinement is a reviewed change to an epic rather than a meeting ([agile-mapping.md](agile-mapping.md)) |
| Parallel sessions, each in its own git worktree | A worktree per issue |
| A mistake made twice goes into `CLAUDE.md` | The retrospective files policy-change issues; rules and memory files carry corrections |
| Roles keep their names and change their activities | Scrum's accountabilities kept, their activities reassigned between people and agents |
| One source of truth per artifact, with tracker and repository linked | Specifications in the repository, work and ledgers in the tracker, linked by URL |

## What the playbook leaves out

The playbook is written for one team working in one repository. It names
many roles but no level above the team, no contract between teams, and no
environment beyond the deployment target. Those are the parts of our problem
this corpus exists for.

Table 2: What this process covers that the playbook does not.

| Concern | Playbook | This process |
|---|---|---|
| Scope | One team, one repository, one service | A system of subsystems across repositories, linked by requirements pinned to a release tag |
| Coordination between teams | None | The technical program manager plans system releases and pushes Sprint Goals to each subsystem; handoff fans releases out to the repositories that cite them |
| Done | A merged pull request that passes review and evals | The named tests pass in the named environment, recorded as evidence on the Sprint Goal |
| Record of progress | Git history and pull request metadata | Each Sprint Goal is a ledger of planned, evidence, and met events that anyone can replay |
| Environments | Autonomy tiers for dev, staging, and production | A chain from local to customer R&D, each environment an SRD with its trigger and suites, owned by a test subsystem |
| Testing software | The feature's tests and the agent evals | A test subsystem with its own owner, shipped with each release so a customer can run it |

## What we take from the playbook

The playbook is ahead of us where the agent's own behaviour is concerned.
Our controls sit in GitLab: branch protection, pipeline gates, and
least-privilege service accounts. The playbook adds controls around the
session and tests the agent's configuration like code.

Table 3: Practices to adopt, in order.

| Practice | In the playbook | For us |
|---|---|---|
| Evals of the agent configuration | 20 to 50 real tasks with accepted outcomes, run nightly and on every change to `CLAUDE.md`, skills, or hooks; configuration changes gated on pass rate; every incident becomes a permanent case | Extend the GitLab regression rig, which tests the scrum-master CLI and the skills' deterministic parts, to run sessions with our skills on recorded tasks and gate skill changes on the result |
| An agent review pass with an enforced human approval | `REVIEW.md` defines bug, security, and compliance passes; findings never approve or block; a code owner must approve; the authoring agent never can | A review pass run before scrum-master issue merge under its own identity, with the merge's approval rule (srd013 R5.5) as the enforced human approval |
| Guardrails on the session | Hooks for rules that must always hold, managed settings engineers cannot disable, an OS sandbox, credentials denied to the agent | Open; skills are advisory, so any rule we rely on needs a hook or a re-check at merge behind it |
| A loop from production signals to new work | Deterministic detection on a rolling baseline; log, diagnose, or act through a pull request by band; the agent writes the next `intent.md` | Open; our audits watch evidence age and blockers, not production signals; this belongs with the platform's observability subsystem |

The first two are cheap and build on what exists, so we take them first.
They also address the weakest part of our design, which checks the work
thoroughly but not the agents doing it.

## The production gate

The playbook's rule for deployment is that the agent "may act up to the
production gate and cannot pass it": a release manager authorizes every
production release. Our environment chain promotes a pinned set to
production PoC automatically when staging is green, and a person approves
only the handoff to customer R&D. That is acceptable while production PoC is
a proof of concept. Before it carries real operations, promotion into it
needs a person's approval, and we should decide where that approval lives:
in the promotion pipeline, in the environments repository as a reviewed
merge, or in the environment's SRD as its trigger.
