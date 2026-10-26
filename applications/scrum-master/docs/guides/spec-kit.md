<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# Why This Process Alongside Spec Kit

[Spec Kit](https://github.com/github/spec-kit) is GitHub's open-source
toolkit for spec-driven development, and it is widely used: a team writes a
constitution, specifies a feature, clarifies it, plans it, breaks it into
tasks, checks the artifacts against each other, and lets an agent implement
it. Anyone who knows it will ask why this corpus exists. This page answers
that question. In short, Spec Kit solves the problem of one developer
turning one feature into code in one repository, and it solves it well. Our
problem is a program: several teams, several repositories, people who
review and approve, and a system that has to run in environments we do not
own. Spec Kit stops where that problem starts.

## Where we agree

The premise is the same. The specification is the source of truth, it says
what and why before how, and an agent builds from it. Every Spec Kit command
has a counterpart in the inner loop described in
[ai-native-sdlc.md](ai-native-sdlc.md).

Table 1: Spec Kit commands and their counterparts here.

| Spec Kit | Purpose | This process |
|---|---|---|
| `/constitution` | Governing principles | `docs/constitutions/design.md`, written once with `bootstrap` |
| `/specify` | What to build and why | SRDs: problem, goals, requirements, acceptance criteria |
| `/clarify` | Structured questions about what is unclear | Skills ask bounded questions; tasks proposes and people agree before anything is filed |
| `/plan` | Technology and architecture | `ARCHITECTURE.md` and each issue's Design Decisions; how the plan relates to the architecture is [plan-vs-architecture.md](plan-vs-architecture.md) |
| `/tasks` | A breakdown into steps | `tasks`: a release into Sprint Goals, a story into issues |
| `/analyze` | Consistency and coverage across artifacts | The audit |
| `/implement` | Build the tasks | `implement` |

If the inner loop were all we needed, we would adopt Spec Kit and stop.

## What Spec Kit leaves out

Spec Kit has no concept of a person other than the one at the keyboard, of
a second repository, or of anything that happens after the code is written.
Each gap below is a part of our problem it does not attempt.

Table 2: What this process covers that Spec Kit does not.

| Concern | Spec Kit | This process |
|---|---|---|
| Who decides | One developer and an agent | Four roles, each owning a level: the technical program manager the system, the Product Owner a subsystem, the Developers the components, the Scrum Master the test subsystem |
| Who takes work | Whoever runs `/implement` | Pull, never push: an item gets an assignee only when that person pops it |
| Review | Left to the user | A person other than the author reviews a merge; with the policy's `require_approval`, merge waits for that approval |
| Scope | One feature in one repository | A system of subsystems across repositories, linked by URLs pinned to a release tag |
| Done | The tasks are implemented | The named tests pass in the named environment |
| Environments | None | local, dev, staging, production proof of concept, then customer R&D, each declared as an SRD with its trigger and suites |
| Cadence | None | A system release every three weeks, Sprint Goals pushed downstream, unmet Sprint Goals carried to the next release |
| Record | `tasks.md` in the feature branch | Issues in the tracker; each Sprint Goal is a ledger of planned, evidence, and met |
| Testing software | The feature's own tests | A test subsystem with its own owner, shipped with the release so a customer can run it |

Three of these decide the matter for us. The first is review. The framework
we adopted moves Developers from writing code to reviewing it, and it
requires every generated change to be read by a person for security and
architecture; a toolkit whose last step is `/implement` has nowhere to put
that. The second is the boundary between teams. When one team's agent
changes an interface another team's agent is building against, the break
happens between repositories, and only a process that holds the contracts
above both repositories and tests them continuously catches it. The third is
the customer. Our software ends in a customer's R&D environment, so done
cannot mean implemented; it has to mean proven, by tests we ship, in
environments we hand over.

## How the two coexist

We do not ask teams to give up Spec Kit. This process constrains what
crosses a repository's boundary, not how a team works inside it. A team may
run Spec Kit as its inner loop as long as its repository honours the
contract the levels depend on.

Table 3: What any inner loop must provide.

| Obligation | Why |
|---|---|
| Requirements cite upstream requirements by URL pinned to a release tag | track-goals follows the citation to decide which Sprint Goal a requirement serves |
| Each acceptance criterion names its test and its environment | Evidence can be collected without a person interpreting the spec |
| Work is visible as tracker issues | daily-scrum, sprint-review, and track-goals read the tracker, not files on a branch |
| A person other than the author reviews a merge, and approves it first when the policy requires approval | The Definition of Done |
| Releases are tags | Promotion pins sets of tags |

A Spec Kit repository meets the first two by writing them into `spec.md`,
the third by filing its `tasks.md` items as issues, and the last two with
ordinary repository settings.

## What we take from Spec Kit

Spec Kit's spec template does three things our SRD format does not, and we
intend to adopt them.

Table 4: Practices to borrow from the Spec Kit template.

| Practice | In Spec Kit | For us |
|---|---|---|
| Inline ambiguity markers | `[NEEDS CLARIFICATION: …]` inside a requirement, resolved by `/clarify` | The audit fails while any marker remains, which gives us a clarify gate without a new skill |
| Given/When/Then scenarios | Acceptance scenarios under each user story | Each scenario maps onto one test, which makes "names its test" concrete |
| Assumptions | Defaults chosen where the input was silent | A reviewer can challenge a default instead of discovering it in the code |

Spec Kit also runs `/analyze` before `/implement` as a required step. We run
the audit, but nothing yet blocks implement on it; that ordering is worth
copying.

## What it costs us

Maintaining our own process is not free. Spec Kit has a large community,
support for many agents, and maintainers whose job it is; ours has one
corpus and the people who use it. We keep that cost down in two ways. The
inner loop is not ours to reinvent: specify and implement are thin scripts
over scrum-master, and Spec Kit is an acceptable substitute inside any
repository. What this corpus specifies is only the layer Spec Kit does not
attempt: levels, Sprint Goals, evidence, environments, and review. If Spec
Kit grows that layer, we should read it closely and drop whatever of ours it
replaces.
