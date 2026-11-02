---
id: srd007-release-annotation
title: Release Annotation Verification
status: planned
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

A tag's annotation is the record of what a release contained, and mage
publish copies it into the notes of the tag's release. The tag target creates a
lightweight ref on the gated commit and the operator's session annotates
it afterwards, which makes the summary the one release step nothing
checks. The record shows it degrading: of five recent releases, three
carry an annotation whose subject is the tag name itself. Nothing stands
between a degenerate annotation and the push, so the empty record ships
and cannot be distinguished later from a release nobody summarized on
purpose.

A pushed tag is also not yet installable. install.sh downloads from the tag's
release, the tag target creates only the tag, and mage publish in
applications/scrum-master creates the release. Nothing required that second
step, so tags reached origin that no install could use: a pinned install
failed with a 404 and an unpinned one silently installed an older release.

<!-- field: goals -->
## Goals

- G1: Refuse a release tag whose annotation is missing or degenerate before it can be pushed.
- G2: Name one reason per failure, so the remedy is evident from the refusal.
- G3: Mutate nothing, so verification can run any number of times at any point.
- G4: Place the check where every release path passes: the runbook and the release skill, after annotation and before push.
- G5: Refuse to treat a pushed tag as released until install.sh can install it, and stop the next release while the newest tag cannot be installed.

<!-- field: applicability -->
## Applicability

<!-- field: baseline -->
### Baseline

R1 through R4 apply to every tag on the v0 release line. The check runs
on demand against one named tag; it gates no build and joins no audit.
R5 applies after the push: it reads the forge the release is published on, and
R5.3 is the one rule the tag target itself enforces.

<!-- field: capability_gates | form: "empty map" -->
### Capability gates

None

<!-- req: R1 -->
## R1 - The tag is an annotated object

<!-- field: applicability -->
### Applicability

runnable_module

<!-- req: R1.1 -->
### R1.1

A tag that does not exist shall be refused, and a tag that is a
lightweight ref rather than an annotated tag object shall be
refused, each with its own named reason.

<!-- field: evidence -->
#### Evidence

A missing tag and a lightweight tag each fail with a reason
naming what was found; an annotated tag passes this rule.

<!-- req: R2 -->
## R2 - The annotation is not degenerate

<!-- field: applicability -->
### Applicability

runnable_module

<!-- req: R2.1 -->
### R2.1

An annotation whose subject equals the tag name, or whose subject
is empty, shall be refused.

<!-- field: evidence -->
#### Evidence

A tag annotated with its own name fails naming the rule; the
degenerate releases in the current history fail it.

<!-- req: R2.2 -->
### R2.2

An annotation whose subject and body together fall under a stated
minimum substance bar shall be refused. The bar is stated in one
place and the refusal quotes it.

<!-- field: evidence -->
#### Evidence

A one-word annotation fails; a summary naming what shipped
passes; the bar appears once in the implementation.

<!-- req: R3 -->
## R3 - The tag sits on the released line

<!-- field: applicability -->
### Applicability

runnable_module

<!-- req: R3.1 -->
### R3.1

A tag not reachable from the local main shall be refused. An
absent or unresolvable main is itself a named failure rather than
a pass.

<!-- field: evidence -->
#### Evidence

A tag on a side branch fails naming reachability; deleting main
in a scratch repository produces the named failure, not success.

<!-- req: R4 -->
## R4 - Verification mutates nothing

<!-- field: applicability -->
### Applicability

runnable_module

<!-- req: R4.1 -->
### R4.1

The check shall create, edit, and delete no ref and no file, and
shall exit non-zero only through its named refusals.

<!-- field: evidence -->
#### Evidence

The ref set and the working tree are byte-identical across a run,
passing or failing.

<!-- req: R4.2 -->
### R4.2

The release runbook and the release skill shall run the check
after annotating and before any push, and stop on refusal.

<!-- field: evidence -->
#### Evidence

The recurring release issue's workflow carries the step between
annotation and push, and the skill's push step is guarded by it
where the target exists.

<!-- req: R5 -->
## R5 - The pushed tag is released

<!-- field: applicability -->
### Applicability

runnable_module

<!-- req: R5.1 -->
### R5.1

mage tag:released shall refuse a tag with no release, a tag whose release is a
draft, and a release lacking an asset install.sh downloads, each with its own
named reason and the remedy, and shall change nothing on the forge. A release
with no draft state, as on GitLab, is checked for its existence and assets.

<!-- field: evidence -->
#### Evidence

A missing release, a draft, and a release lacking an asset each fail naming what
was found; a visible release holding every asset passes; no call creates,
uploads to, or edits a release.

<!-- req: R5.2 -->
### R5.2

mage tag:publish shall refuse a tag that R1 through R3 refuse and a tag not at
HEAD, since the release is built from HEAD, before publishing anything. It shall
then run mage publish and the R5.1 check, in that order, and fail if either
fails.

<!-- field: evidence -->
#### Evidence

A lightweight tag and a tag behind HEAD publish nothing; a tag at HEAD runs the
publish and then the check; a failed publish or an incomplete release fails the
target.

<!-- req: R5.3 -->
### R5.3

mage tag shall refuse to start, before running any gate and creating no tag,
while the newest v0.YYYYMMDD.N tag on origin fails the R5.1 check. On creating a
tag it shall name the steps that remain, ending with mage tag:publish.

<!-- field: evidence -->
#### Evidence

An unpublished newest tag stops the release naming that tag; the newest tag is
chosen numerically and tags of other shapes are ignored; a repository with no
release tags passes; a failure to list the tags is a named failure.

<!-- field: non_goals -->
## Non-goals

- Writing or improving the summary. The session that releases owns the words; this check only refuses their absence.
- Gating mage tag, the audit, or any build on the annotation. That check runs at the push boundary on demand.
- Verifying pushed history retroactively. Existing degenerate tags are the evidence, not the workload, and R5.3 reads only the newest tag.
- Publishing a tag from inside mage tag. The tag is annotated and pushed after the target returns, so its release can only follow.
- Changelog files or any release artifact beyond the assets mage publish already produces.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A missing tag, a lightweight tag, a subject equal to the tag name, an
empty subject, and an annotation under the substance bar are each
refused with their own named reason.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC2 -->
### AC2

A tag not reachable from main is refused, and an absent main is a
named failure rather than a pass.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)

<!-- ac: AC3 -->
### AC3

A run changes no ref and no file, whether it passes or refuses.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)

<!-- ac: AC4 -->
### AC4

The runbook and the skill place the check between annotation and
push.

<!-- field: traces -->
#### Traces

- [R4.2](#r42)

<!-- ac: AC5 -->
### AC5

A missing release, a draft release, and a release lacking an asset are each
refused by name with the remedy, and the check changes nothing on the forge.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)

<!-- ac: AC6 -->
### AC6

mage tag:publish refuses before publishing, publishes and then checks, and fails
when either step fails; mage tag stops before its gates while the newest tag on
origin has no complete release.

<!-- field: traces -->
#### Traces

- [R5.2](#r52)
- [R5.3](#r53)

<!-- field: use_case_traces -->
## Use case traces

- rel18.0-uc001-release-annotation

<!-- field: test_suite_traces -->
## Test suite traces

- test-rel18.0-release-annotation
