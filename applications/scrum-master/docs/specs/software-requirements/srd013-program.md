---
id: srd013-program
title: "Script: issue (scrum-master issue ...)"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- The group is named for its object. Every subcommand acts on an issue, and the group was named program before release 01.4, which said nothing about any of them.
- pop keeps its name. People pull their work or a person assigns it, and popping an issue is the verb for taking it in the skills (implement), the vision, and the states (srd041); under the issue group the command reads as what it does.
- The old name breaks cleanly. Every caller is a skill embedded in the same scrum-master (srd037), so an alias would only keep stale skills running; instead scrum-master program runs nothing and names the fix.
- recur moves here from standup (srd010 R8.2, retired). It closes and reopens a recurring issue, wherever the issue lives and whoever runs it, which is the issue lifecycle, not a person's morning. The marker it writes into an issue body is tracker data, so its text does not change.

<!-- field: problem -->
## Problem

Every issue that enters a program has to carry the labels and sections the
other scripts read, and every merge has to tell its reviewer what to
look at. Done by hand, admission means something different in each
repository, and an issue missing its acceptance criteria reaches implement
with nothing to build against.

The issue subcommands are the small set of issue operations every
repository shares: the labels admission reads, admission, filing, showing,
taking an issue into a worktree, and delivering it by a reviewed merge. Together with
the specify and implement skills (srd035, srd036) they are the inner
loop on GitLab.
Tracking Sprint Goals against evidence, verifying what was built, closing
drained tracking issues belong to track-goals (srd019), and the dashboard
is the site the site skill generates (srd048), which shows track-goals' ledgers;
recording a release plan belongs to plan.

<!-- field: goals -->
## Goals

- G1: Admission is a script, so needs-triage means the same thing in every repository.
- G2: Filing an issue and merging its branch each have one subcommand, used by every skill.
- G3: Every issue is taken the same way, claimed by the person who works it, on its own branch, in its own worktree.

<!-- req: R1 -->
## R1 - scrum-master issue lint

<!-- req: R1.1 -->
### R1.1

Checks one issue, or every issue changed since a date, against the label schema and policy.admission. The checks are named rules the policy declares (config format policy, admission.rules; every rule when it declares none), so a program disables one by removing its name: exactly one plane, a component listed by a subsystem SRD on every issue except the sprint issue, once a subsystem SRD declares one (srd001 R21.2), exactly one kind on data issues, listed values only, Requirements and Acceptance Criteria sections where policy requires them, each criterion naming a test and an environment (design.md D8), and a management or control issue filed by someone policy allows. Two checks are not rules and always apply: every Cites line pinned (srd007 R5.3) and the srd007 link-line discipline.

<!-- req: R1.2 -->
### R1.2

Sets needs-triage on an issue that fails, with a comment listing each failure unless its latest admission comment lists the same; clears needs-triage on an issue that passes, and when its latest admission comment is a failure, comments that it is now admitted. It changes no other label and never the assignee (srd041 R9.4).

<!-- req: R1.3 -->
### R1.3

Runs in CI on issue creation and edit where the tracker allows, and on a schedule otherwise.

<!-- req: R2 -->
## R2 - Common behavior

<!-- req: R2.1 -->
### R2.1

Subcommands are in the scrum-master group issue, honour --json and --dry-run, and never prompt (srd010 R1).

<!-- req: R2.2 -->
### R2.2

scrum-master program, the group's name before release 01.4, runs no subcommand: it exits non-zero naming scrum-master issue and scrum-master install, so a skill installed from an earlier scrum-master fails with its fix in the message.

<!-- req: R3 -->
## R3 - scrum-master issue file

<!-- req: R3.1 -->
### R3.1

Creates an issue in a given project with the given title, body, plane, kind, component, and milestone, unassigned, and runs lint on it before returning (R1). It never sets an assignee; a person who wants the issue pops it with implement (srd041 R9.1).

<!-- req: R3.2 -->
### R3.2

With a parent tracking issue given, appends the new issue to its Members section (srd001 R6); with a parent issue given, attaches the new issue as its sub-issue.

<!-- req: R3.3 -->
### R3.3

A data issue filed without a milestone and without a type gets the sprint: the active milestone of the project or its ancestor groups whose start date is on or before today and whose due date is on or after it. A milestone whose sprint has ended, its sprint-end issue in the project closed or its release cut, the release issue closed in the project or the system's project (as a reports repository has no sprint-end issue of its own, GH-141), no longer counts: the issue goes on the next milestone by start date, so a retrospective's actions land in the next sprint. An issue that cuts a release (kind::release) is the exception: it belongs to the sprint whose release it cuts, so that sprint's milestone still counts for it after the sprint-end. With none, or more than one, it is filed without a milestone and the result says why, naming what ended a sprint whose dates still cover today (GH-141); a milestone given explicitly always wins. scrum-master issue milestone puts an issue already filed in a milestone its project finds and lints it again. A kind given explicitly makes a data issue of that kind, whatever plane its paths alone would give.

<!-- req: R4 -->
## R4 - scrum-master issue show

<!-- req: R4.1 -->
### R4.1

Returns one issue with its labels, its state per srd041 R1–R32, its sub-issues and their states, its link lines (Upstream, Requires, Cites, Blocked-by, Blocks, Depends-on, Relates-to, Duplicates, Implements; srd007) with track-goals' explanation of each (srd019), its tracking issue if any, and its event comments, including its ledger events for a Sprint Goal.

<!-- req: R5 -->
## R5 - scrum-master issue merge

<!-- req: R5.1 -->
### R5.1

Merges a worktree branch into the default branch of the repository's main checkout with a merge commit whose message cites the issue and every open issue named with --closes (one another merge already closed is reported, not cited, since every cited issue reads as closed by the merge), pushes it, and closes each issue on that merge: the merge it made is the one each records, though an earlier merge cites the same reference. scrum-master issue close, which is not shown the merge, takes the first merge citing the issue made after the issue was created (srd042 R2.3). A branch reads as already merged only through the merge that brought it in, one whose second parent's history holds the branch's tip and whose first parent's does not; a branch with no commits over the base and no such merge is refused, so an earlier merge citing the same reference, as a reused issue number leaves, is never taken for its delivery (GH-2801).

<!-- req: R5.2 -->
### R5.2

Refuses when the branch has no commits ahead of the default branch, when the issue is closed, or when an issue named with --closes is malformed or closed.

<!-- req: R5.3 -->
### R5.3

For a branch that changes docs/, runs the corpus gate, the policy's audit_command, in the worktree first, and merges nothing when it fails (srd034 R2.2). Without --control the policy is the system repository's, the checkout holding the trackers document beside this one (srd042 R1.3), so a team repository's merge is held to the program's policy; with no system repository beside it, the project's own control files; the built-in mage audit becomes scrum-master backlog check in a repository with no magefiles, which could not run it. The merge's output names the audit it ran, or says no docs/ change meant none. The JSON result always names the audit command and the reason it ran or did not: docs/ changed, or no docs/ change.

<!-- req: R5.4 -->
### R5.4

With --issue-project, merges in the given project and closes the issue in the other by its full reference; the weekly status report merges in the reports repository and closes the status issue in the owner's repository (srd018 R6.1).

<!-- req: R5.5 -->
### R5.5

With the policy's admission.require_approval set, refuses unless the issue carries an approval of the exact commit it would merge, from a person other than the issue's assignee, or, for an issue nobody took, other than the person merging (srd008 R1.4). The default policy requires none: merge merges without an approval and names the approver when the tip carries one. An approval names the commit its reviewer read, so a commit made after it needs a new one. On a tracking system whose host reviews branches, such as gh, the approval is the host's review of that commit, and the host merges; a review the host's own branch protection requires still applies.

<!-- req: R5.6 -->
### R5.6

scrum-master issue approve posts the approval as the person running it: a comment on the issue naming the tip of the branch at its origin. It refuses the issue's assignee, and posts nothing twice for the same commit.

<!-- req: R5.7 -->
### R5.7

merge decomposes into two words the merge profile composes (GH-201, design.md D14): scrum-master issue merge-branch — the gates, the --no-ff merge citing the issue and each closes, and the push, closing nothing — and scrum-master issue close-delivered, which closes the issue and each closes reference on the commit the merge word reports and posts a reported_complete event on each closed Sprint Goal (srd038 R2.2). scrum-master issue merge runs the same code as one command, and scrum-master issue merge --dry-run is the gate word, running every gate and writing nothing. A close that fails after the push is the machine's PartiallyDelivered state, finished by a re-run of close-delivered with the named commit.

<!-- req: R5.8 -->
### R5.8

Once a delivery has landed, what pop opened for the issue is removed (GH-2796): scrum-master issue clean-up, from the main checkout, removes the worktree that has the branch checked out, deletes the branch with git branch -d, and deletes it on origin. It keeps a worktree holding a change git status shows, and removes nothing for a branch the main checkout has not merged, naming each with the reason; nothing it keeps fails the delivery, and a second run finds nothing and says so. The merge profile runs it after close-delivered; scrum-master issue merge runs it unless --clean-up=false, which a skill that runs words in the worktree after its merge passes, running the clean-up last.

<!-- req: R6 -->
## R6 - scrum-master issue labels

<!-- req: R6.1 -->
### R6.1

Creates or updates the program's group labels from control/labels.md in the program repository (config format labels), or from the built-in defaults when the file is absent: every plane, type, kind, and state value, with its color and description. With --control it also creates one component::<name> label per component the control project's subsystem SRDs declare (srd001 R21.1).

<!-- req: R6.2 -->
### R6.2

Is idempotent: a second run changes nothing. It never deletes or renames a label, so issues keep the labels they carry.

<!-- req: R6.3 -->
### R6.3

With --migrate and --control naming the system repository, also relabels issues for a value srd001 renamed: every issue, open or closed, labelled type::epic in the system repository and in every repository its subsystem SRDs declare gains type::tracking and loses type::epic (srd001 R6.6). It is opt-in, writes nothing with --dry-run, and is idempotent. It never deletes or renames the group label itself and never edits the program's control files; it reports when control/labels.md still lists epic, for a plane::control issue to change.

<!-- req: R7 -->
## R7 - scrum-master issue pop

<!-- req: R7.1 -->
### R7.1

Claims one issue for the caller: assigns the caller and no one else (srd041 R9.1). Refuses an issue assigned to someone else, a closed issue, and a plane::data issue that admission has not admitted (srd041 R2). An issue a person assigned the caller (R10.1) is theirs to pop like any other. It refuses a type::tracking issue, naming why: it lists or mirrors work and is never popped. With --issue-project, the issue is claimed there and the branch, which --branch may name, opens in --project: a weekly status issue's report lands in the reports repository (srd018). A plane::management or plane::control issue that admission has not admitted is popped, and pop reports the reasons it fails.

<!-- req: R7.2 -->
### R7.2

Creates the branch <iid>-<slug> from the default branch, the slug taken from the title, and adds a local git worktree for it beside the repository's checkout; reuses both when they already exist.

<!-- req: R7.3 -->
### R7.3

Returns the issue, its branch, and its worktree path, so the skill and the person work there and nowhere else.

<!-- req: R7.4 -->
### R7.4

Installs into the worktree the skills of every agent installed in the checkout (srd037), and reports which; a failed install is reported without undoing the pop, and --dry-run installs nothing.

<!-- req: R7.5 -->
### R7.5

pop decomposes into four words the pop profile composes (GH-201, GH-204, design.md D14): scrum-master issue claim — the assignment with pop's refusals, the atomic transaction the word boundary protects, naming the branch to open — scrum-master issue open-branch, scrum-master issue add-worktree, and scrum-master issue install-skills (srd036 R2.3). scrum-master issue pop runs the same code as one command. Each word reuses what is already in place, so a word that fails after the claim is a partial machine state a re-run of the pop finishes.

<!-- req: R8 -->
## R8 - scrum-master issue recur

<!-- req: R8.1 -->
### R8.1

scrum-master issue recur, which the recur skill runs, closes a recurring issue with a run summary, naming the skill, not the command, in what it writes on the issue (GH-141), appends the run to its Previous Runs section, updates its Requires version, and opens the next instance (srd001 R8.3). With --summary (or --summary-file), the run's result is the run summary it closes the issue with and the Previous Runs line in place of the outcome, with how the instance closed after it, so a run that merges no file still carries its result forward. An issue a merge already closed is not closed again; recur only appends the run and opens the next instance, and it opens none when the next instance already exists. The next instance is admitted as scrum-master issue file admits an issue, under the policy of --control (the project's own by default), so a recurred data issue starts ready like a freshly filed one.

<!-- req: R8.2 -->
### R8.2

The marker recur writes into the next instance's body, naming the issue it recurs, keeps the text it had as scrum-master standup recur, so an instance opened before release 01.4 is still found and never opened twice.

<!-- req: R9 -->
## R9 - scrum-master issue comment

<!-- req: R9.1 -->
### R9.1

scrum-master issue comment posts the person's text, from --body or --body-file, on an issue as the person running it, through the tracking system serving the project, so a skill writes to an issue only through scrum-master, such as a completed issue's Actual LOC line. It refuses an issue that does not exist, an empty body, and both body flags at once; a dry run posts nothing.

<!-- req: R10 -->
## R10 - scrum-master issue assign

<!-- req: R10.1 -->
### R10.1

scrum-master issue assign makes --person the issue's assignee and leaves the issue for them to pop (srd041 R9.1); any person may run it, and the tracking system's own permissions stand. It refuses a closed issue, and, once work has started, refuses and names the assignee, who gives it back first (R10.2), so nobody loses work in progress unknowingly. Work has started when the tracking system records it, as beads' taken and in-progress do, and otherwise when the issue's branch <iid>-<slug> exists in --project; --issue-project names the issue's project when it is not --project. It reports an error when the tracking system took the write without assigning the person. --dry-run changes nothing, and a second identical run changes nothing.

<!-- req: R10.2 -->
### R10.2

scrum-master issue unassign is the assignee giving an issue back: it clears their assignment, and an issue they had started on beads is ready again (srd041 R9.2). It refuses anyone else and names the assignee. An issue given back keeps its branch, which the next assignee's pop reuses, and can be assigned again.

<!-- req: R10.3 -->
### R10.3

scrum-master issue list --mine lists the caller's open issues, leaving out tracking issues, which hold no work, in --project, or, with no --project, in the system repository and every repository its subsystem SRDs declare, taken from --control or the system repository beside --dir. Issues an open blocker holds back are listed apart.

<!-- req: R10.4 -->
### R10.4

The assign and unassign skills each run their one word in a model-free profile; no other profile declares either word, and a gate holds this. The assign skill confirms the issue and the person before it runs.

<!-- req: R11 -->
## R11 - Who signs off

<!-- req: R11.1 -->
### R11.1

Who signs off on an issue is, most specific first: the person its Reviewer line names (config format issue-body); the reviewer the document its Implements line names carries in its frontmatter, then that document's parents' up to its epic; the reviewer control/people.md names for the scrum whose repository holds the issue; and the policy's admission.default_reviewer. Who signs off on a document is found the same way from its own frontmatter: the vision, the architecture, the road map, an SRD, a use case, or a planning document, each of which carries it beside its authors. With no name from any of these, nobody is required and R5.5 stands.

<!-- req: R11.2 -->
### R11.2

A merge needs the approval of the tip by whoever signs off on its issue, and by whoever signs off on each such document the branch changes, even with admission.require_approval off, and only their approval counts for them. A document's reviewer is added to the issue's, so an issue's Reviewer line replaces the default but not a changed document's reviewer. Signing off on a document means the document is agreed; a change to its issues record (srd045 R4.4), its issue items (srd045 R2.7), or its people block alone changes nothing agreed and needs nobody, so a reviewer who is away can be replaced. merge reads who signs off from each document at its repository's default branch and the control files at --ref, not from the branch, except that a document the branch adds says it from the branch, and its refusal names everyone still needed and any earlier commit they approved. An inherited reviewer who wrote the work cannot approve it, so that requirement falls back to anyone but the assignee, or anyone but the merger when nobody took the issue; a Reviewer line naming them is refused, for a person to change.

<!-- req: R11.3 -->
### R11.3

A merge commit carries one Approved-by trailer per approver, beside its Merged-by trailer, so the site reads who signed off from git alone (srd048 R8.9). It is the only record a sign-off adds.

<!-- req: R11.4 -->
### R11.4

scrum-master issue reviewer writes an issue's Reviewer line, replacing another only with --force, and --clear drops it; it refuses a closed issue and the issue's assignee. scrum-master backlog reviewer writes a document's reviewer into its frontmatter, by id or path, replacing another only with --force, and --clear drops it. scrum-master people review names who signs off on a scrum in control/people.md, moving a scrum another person holds only with --force, and people unreview takes them off. A scrum or document has one reviewer, and a control/people.md naming two for a scrum is refused. --dry-run changes nothing, and a second identical run changes nothing.

<!-- req: R11.5 -->
### R11.5

The reviewer, backlog-reviewer, people-review, and people-unreview skills each run their one word in a model-free profile; no other profile declares any of the four, and a gate holds this. An agent never names a reviewer or approves; each skill confirms with the person before it runs.

<!-- req: R11.6 -->
### R11.6

On a tracking system whose host reviews branches, the person who signs off on the issue is asked on its review, when the review opens and when issue reviewer names them, unless they are its assignee or the person running the word. What stops the request is reported and fails nothing, since the merge, not the request, holds the sign-off. A tracking system without one, such as beads, shows them in the listings alone.

<!-- req: R11.7 -->
### R11.7

scrum-master issue list --reviewing lists the open issues the caller signs off on and is not assigned, leaving out tracking issues, taken as issue list --mine takes its repositories (R10.3).

<!-- field: non_goals -->
## Non-goals

- Working a story's issues in one branch; that is implement at the story level (srd036 R5).
- Choosing what goes in a release; that is plan.
- Tracking Sprint Goals, verifying merged work, or closing tracking issues; that is track-goals (srd019). Rendering the dashboard; that is the site (srd048).
- Offering work to a person, or an agent assigning it (srd041 R9.1).

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

lint sets needs-triage with a listed reason on each failing issue, including a criterion without test or environment and an unpinned Cites line, clears it on passing ones, comments the admission of one that failed before and posts a failure again after it, changes nothing else, and runs on creation or on a schedule.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)

<!-- ac: AC2 -->
### AC2

Every issue subcommand honours --json and --dry-run and never prompts; scrum-master program runs nothing, exits non-zero, and names scrum-master issue and scrum-master install.

<!-- field: traces -->
#### Traces

- [R2.1](#r21)
- [R2.2](#r22)

<!-- ac: AC3 -->
### AC3

file creates an unassigned, linted issue (or a tracking-issue member, or a sub-issue of a parent issue) and never sets an assignee; show returns the issue with state, sub-issues, explained links, tracking issue, and events; a data issue filed without a milestone carries the one active sprint covering today, or none with the reason, and a release issue filed after its sprint-end still carries that sprint.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)
- [R4.1](#r41)

<!-- ac: AC4 -->
### AC4

merge merges the branch with a merge commit citing the issue and the named issues still open, pushes it, and closes each on it, recording that merge though an earlier one cites the same reference, runs the corpus gate first for a docs/ change, and refuses an empty branch or a closed issue; with --issue-project it merges in one project and closes the issue in another.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.4](#r54)

<!-- ac: AC5 -->
### AC5

A merge of a branch that changes docs/ runs mage audit first and merges nothing when it fails, whatever its issue's plane or kind.

<!-- field: traces -->
#### Traces

- [R5.3](#r53)

<!-- ac: AC6 -->
### AC6

labels creates every listed label once, from control/labels.md or the defaults, and with --control each declared component's label; a second run changes nothing; no label is deleted or renamed; with --migrate, issues labelled type::epic in the system repository and its subsystems' repositories are relabelled type::tracking once, nothing is written on a dry run, and a control/labels.md that still lists epic is reported, not edited.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)
- [R6.3](#r63)

<!-- ac: AC7 -->
### AC7

pop assigns only the caller, refuses an issue assigned to someone else, a closed issue, and a non-admitted data issue, creates or reuses the branch <iid>-<slug> and its worktree, and returns both, with the checkout's installed skills installed in the worktree.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)
- [R7.4](#r74)

<!-- ac: AC8 -->
### AC8

Under a policy with require_approval, merge refuses with no approval, with only the assignee's, and with an approval of an earlier commit, and merges with another person's approval of the tip; under the default policy it merges an unapproved branch, closes its issue, and names an approver only of the tip; approve refuses the assignee and posts once per commit.

<!-- field: traces -->
#### Traces

- [R5.5](#r55)
- [R5.6](#r56)

<!-- ac: AC9 -->
### AC9

recur closes a recurring issue with its run logged and Requires updated and opens the next instance once, admitted as a filed issue is, closes again nothing a merge closed, and finds an instance whose marker an earlier scrum-master standup recur wrote.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)

<!-- ac: AC10 -->
### AC10

comment posts the person's text on an issue once, on every tracking system, refuses an empty body, both body flags, and an issue that does not exist, and a dry run posts nothing.

<!-- field: traces -->
#### Traces

- [R9.1](#r91)

<!-- ac: AC11 -->
### AC11

A run of the merge profile with the pinned runtime over a popped issue with committed work gates, merges, pushes, and closes it, ending in Done; a branch with nothing over the base ends in Refused with scrum-master's reason; merge-branch closes nothing and close-delivered finishes a delivery from the commit it names.

<!-- field: traces -->
#### Traces

- [R5.7](#r57)
- [R5.8](#r58)

<!-- ac: AC12 -->
### AC12

A run of the pop profile with the pinned runtime over an admitted issue claims it, opens its branch and worktree, and installs the skills, ending in Done; a closed issue ends in Refused with scrum-master's reason; the words reuse what exists, so a re-run changes nothing.

<!-- field: traces -->
#### Traces

- [R7.5](#r75)

<!-- ac: AC13 -->
### AC13

After a merge, the issue's worktree, its branch, and origin's branch are gone and the result says so; a worktree with an uncommitted change is kept with its branches, as is a branch not merged, each with the reason; a dry run removes nothing; a second run finds nothing left.

<!-- field: traces -->
#### Traces

- [R5.8](#r58)

<!-- ac: AC14 -->
### AC14

issue assign sets the assignee on GitHub, GitLab, and beads and leaves the state; the assignee's pop takes the issue and another person's is refused; assigning after work has started is refused naming the assignee; a tracker that took the write without assigning is an error; --dry-run and a second run change nothing.

<!-- field: traces -->
#### Traces

- [R10.1](#r101)

<!-- ac: AC15 -->
### AC15

issue unassign clears the caller's own assignment from any open state and refuses anyone else's; an issue given back can be assigned again even with its branch still there.

<!-- field: traces -->
#### Traces

- [R10.2](#r102)

<!-- ac: AC16 -->
### AC16

issue list --mine lists only the caller's open issues, across the named project or the program's repositories, with blocked ones apart.

<!-- field: traces -->
#### Traces

- [R10.3](#r103)

<!-- ac: AC17 -->
### AC17

A gate fails when a profile other than assign or unassign declares its word, and when either has a model.

<!-- field: traces -->
#### Traces

- [R10.4](#r104)

<!-- ac: AC18 -->
### AC18

With nothing named, merge behaves as R5.5 says; with a Reviewer line, a document's own reviewer or its parents', a scrum's reviewer, or a default, the lookup finds them in that order, and a parent pinned by URL is followed to its repository.

<!-- field: traces -->
#### Traces

- [R11.1](#r111)

<!-- ac: AC19 -->
### AC19

With require_approval off, merge refuses without a required reviewer's approval of the tip and names who is needed and any earlier commit they approved; another person's approval does not stand in; a branch changing a document needs its reviewer as well as the issue's; a branch that renames a document's reviewer while changing it is held to the name on the default branch; a change to a document's issues record or people block alone needs nobody; a document the branch adds is held to the reviewer it names; an inherited reviewer who wrote the work falls back to anyone else, and a Reviewer line naming them is refused. This holds on beads, GitHub, and GitLab.

<!-- field: traces -->
#### Traces

- [R11.2](#r112)

<!-- ac: AC20 -->
### AC20

A merge commit carries Approved-by for each approver beside Merged-by.

<!-- field: traces -->
#### Traces

- [R11.3](#r113)

<!-- ac: AC21 -->
### AC21

issue reviewer writes the line, replaces another only with --force, drops it with --clear, and refuses a closed issue and its assignee; backlog reviewer writes a document's frontmatter, finding it by id or path, and refuses a document that carries none; people review and unreview change control/people.md, moving a held scrum only with --force; a control/people.md naming two reviewers for one scrum is refused; --dry-run and a second run change nothing.

<!-- field: traces -->
#### Traces

- [R11.4](#r114)

<!-- ac: AC22 -->
### AC22

A gate fails when a profile other than a word's own declares issue reviewer, backlog reviewer, people review, or people unreview, and when any of the four has a model.

<!-- field: traces -->
#### Traces

- [R11.5](#r115)

<!-- ac: AC23 -->
### AC23

Pop and issue reviewer ask the reviewer on GitHub and GitLab; a host that refuses is a note and fails nothing, and neither the assignee nor the caller is asked.

<!-- field: traces -->
#### Traces

- [R11.6](#r116)

<!-- ac: AC24 -->
### AC24

issue list --reviewing lists the open issues the caller signs off on, by a Reviewer line or a document's reviewer, and leaves out those assigned to them.

<!-- field: traces -->
#### Traces

- [R11.7](#r117)
