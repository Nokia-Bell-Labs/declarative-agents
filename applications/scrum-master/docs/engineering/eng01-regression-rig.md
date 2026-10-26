---
id: eng01-regression-rig
title: "Regression Rig: A Program on Local Disk Per Run"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: introduction -->
## Introduction

The process lives in the tracker and the repositories: Sprint Goals,
due dates, deliveries, tags. A test that mocks them proves the mock. We
run every release's regression against a real program that the run builds
and removes, so each run starts from a known layout and leaves nothing
behind. Since the tracker became beads (srd042), that program needs no
service at all: a tracker repository and the code repositories, as git
repositories on local disk, are everything scrum-master reads and writes. The rig
therefore needs no network, no credentials, and no paid tier, and a full
regression runs in under a minute.

<!-- section: lifecycle -->
## Lifecycle

mage regression:up builds a run under .regression/<run-id>, which git
ignores, and records it as the current run; every integration target
reads the current run; mage regression:down removes it. mage regression
runs up, every integration target in order, and down, and down runs even
when a target fails, so a failed run leaves nothing behind but its
report. mage regression:sweep removes any run older than a day that an
interrupted regression left.

<!-- section: layout -->
## Layout

A run holds a bare tracker repository, tracker.git, whose first commit is
an empty .beads/issues.jsonl; and, for each rig project, a bare origin
under origins/ and a checkout under checkouts/, all named for the
project's last path segment. The projects are rig/system, rig/reports,
rig/product-owners/po-1, and rig/product-owners/po-2. The checkouts sit
side by side with work/, the checkout scrum-master runs in, which is how the beads
tracker finds a project's checkout (eng04-beads-sync, Choosing the
backend). Fixtures go on a seed branch apart from main, which the
targets' merges change; a target that clones a repository clones it under
clones/, so pop's worktrees land beside the clone inside the run.

<!-- section: people -->
## People

scrum-master runs as the rig's operator, with SCRUM_MASTER_TRACKER naming the run's tracker
repository and the operator's own clone of it in SCRUM_MASTER_TRACKER_CLONE. A
step another person takes, such as a release's peer read, runs scrum-master as the
reviewer with the reviewer's own clone, so the two people's writes meet
only in the tracker repository, as they would on two machines. A target
reads and writes the tracker beside scrum-master through the beads tracker in Go,
through yet another clone.

<!-- section: delivery -->
## Delivery

Work reaches a default branch only through scrum-master issue merge, as on any
beads program: the audit gate for a docs/ change, a --no-ff merge citing
the issue and what it closes, a push, and the closes. The rig's
repositories carry no magefile, so a marker file, docs/BROKEN, stands in
for a failing mage audit.

<!-- section: unit-tests -->
## Unit tests

The rig itself is plain git and the beads tracker, which their own tests
cover; the tests every scrum-master package runs on both trackers through
internal/trackertest build the same kind of program per test. mage
regression is what proves the commands end to end, through the built
binary and the environment a person's machine gives it.

<!-- section: gitlab -->
## The opt-in GitLab variant

The local rig proves the commands; it cannot prove beads under the
conditions a program meets on a hosted origin: pushes that lose a race
over the network, many people writing at once, a clone that was offline.
mage regression:gitlab does, and it is opt-in. It runs only when
GITLAB_TOKEN is set, and skips with a message that says so otherwise; it
never joins mage regression, the integration targets, or a release gate,
so the regression above stays offline and fast.

It needs GITLAB_TOKEN, a token with the api scope; GITLAB_RIG_GROUP, the
existing group the run creates its own group under, named in the
environment rather than in the repository because it is the person's
own; and optionally GITLAB_BASE_URL for an instance other than gitlab.com.
Each run applies the Terraform root infra/regression, which builds on the
program module infra/modules/program, with local state under
.regression/gitlab-<run-id>: a private group rig-<run-id> holding a tracker
repository, system, reports, and the product-owner repositories. The run
clones each repository, makes the tracker's first commit, and installs the
skills into each with scrum-master install.

The workload runs at least four workers at once, each with its own clone
of the tracker and its own checkouts, because a beads tracker is not safe
to share, and each runs one command at a time, as a person at one machine
does: disjoint creates, contended pops of the same issues, closes by
merge, and one worker that goes offline and reconciles late. After a final
sync on every clone it asserts that every clone reads the same tracker
state, every disjoint write landed, each contended issue has exactly one
taker, the earliest to push, raced creates are renumbered consistently,
and the offline worker's lost pop is reported. The semantics are srd042's
and eng04's, already tested locally; this run adds the real remote.

The run destroys its group with terraform destroy when it ends, including
when an assertion fails, so its cost is a private group for the length of
a run. mage regression:gitlabSweep destroys any run an interrupted
regression left behind. Provisioning restores the Terraform release 03.0
removed; provisioning through the GitLab API in Go was the alternative,
rejected so that the rig builds a program the same way a program is built.

<!-- section: github-rig -->
## The opt-in GitHub rig

mage integration:github proves the gh tracking system against GitHub
itself, and is opt-in like the GitLab variant: it needs gh logged in
(GH_TOKEN) and the network, so it never joins mage regression. It works
in one private repository, the one magefiles/integration_github.go
names, created on first use and reused; GITHUB_RIG_REPO names another. With
SCRUM_MASTER_TRACKER=gh it builds scrum-master and proves scrum-master issue labels writes the schema
into the repository, scrum-master issue file files an admitted issue on a dated
milestone, scrum-master issue pop opens the branch with a draft pull request,
GitHub refuses the author's own approval, and under the default policy scrum-master
issue merge merges the author's own pull request and closes the issue. A
second person's approval, and the refusals of a policy that requires one,
are proven offline in internal/github, against a fake GitHub with two
logins that merges in a real git origin, as the rig has one identity.

<!-- section: glab-rig -->
## The opt-in glab rig

mage integration:glab proves the glab tracking system against GitLab the
same way: glab logged in, the network, never mage regression. It works in
one private project, rig, under the group GLAB_RIG_GROUP names, creating
the group as a subgroup of its parent and the project on first use and
reusing them after. With SCRUM_MASTER_TRACKER=glab it proves scrum-master issue labels writes
the schema as group labels, scrum-master issue file files an admitted issue on a
dated group milestone, scrum-master issue pop opens the branch with a draft merge
request, and under the default policy scrum-master issue merge merges the author's
own merge request and closes the issue. A second person's approval, and
the refusals of a policy that requires one, are proven offline in
internal/gitlab.

<!-- section: beads-scale -->
## The opt-in beads scale run

The regression proves beads with a handful of issues. mage
integration:beadsScale measures it at a program's size, and is opt-in for
the same reason as the GitLab variant: it is slow, and it measures rather
than proves, so it never joins mage regression or a release gate. It
needs no rig; it builds scrum-master and works in a temporary directory.

For each tracker size and each number of writers, it seeds a bare tracker
repository with that many issues in one commit, each with five comments
the size of ledger events and a fifth of them closed, in the layout
BEADS_SCALE_LAYOUT names, files by default or jsonl. It times issue file, issue pop, issue lint --since with
--dry-run, and sprint-end facts against it with one writer, three runs
each, keeping the median. Then it starts the writers at once, each with
its own tracker clone and code checkout, each filing new issues and
popping its own ready ones through the rig's words harness, and records each
command's time. Afterwards it reads the tracker's main and counts the
filed issues missing from it and the IDs and IIDs that repeat, packs the
tracker repository, measures its size, and times a fresh clone.

BEADS_SCALE_LAYOUT (files or jsonl),
BEADS_SCALE_ISSUES (default 1000,5000,10000), BEADS_SCALE_WRITERS
(default 2,5,10), and BEADS_SCALE_OPS (default 10 file-and-pop pairs per
writer) set the sizes, each a comma-separated list. The report goes to
.regression/beads-scale/<UTC stamp>.json and a Markdown table beside it.
The run fails only on a lost write or a duplicate ID; times inform the
storage decision eng04 describes, and fail nothing.

<!-- field: references -->
## References

- docs/engineering/eng04-beads-sync.md (the tracker repository and choosing the backend)
- docs/specs/software-requirements/srd042-beads-tracker.md R4.2 (the rig builds fixture tracker repositories)
- docs/specs/test-suites/test-rel01.0-objectives.md (first suite to run on the rig)
- infra/regression and infra/modules/program (the GitLab variant's Terraform)
