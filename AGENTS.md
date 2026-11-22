<!-- managed:begin: the init and onboard profiles write the text down to managed:end and refresh it on a rerun; write your own notes outside it -->
# Agent instructions

This is a program's system repository, run with the agent and the AI-native
scrum process's profiles, vendored in .scrum-master. Read README.md first:
it says what the repository holds, how the planning hierarchy nests, and
which skill serves each step of the process.

## Rules

- Every change starts as an issue: file it with the specify skill, take it with the pop or implement skill, and deliver it with the merge skill. Never commit to main directly.
- A change under docs/ is a change of intent: the corpus gate must pass before it merges.
- Run a workflow through its skill: each runs the agent over its profile in .scrum-master, and the profile's words fetch the facts and write the tracker. Never read or edit tracker files by hand. The words do the workflow, and the model writes only language (docs/constitutions/design.md D10).
- Every document follows docs/constitutions/design.md: Markdown with marker tags, in the format of the parent release docs/external-pins.yaml pins. After taking a new release, run the upgrade profile to vendor it, add missing files, repair the format, and refresh the skills; the check skill reports what drifted.
- Every figure is Mermaid in a fenced code block inside the Markdown document that shows it. Put no generated image file, and no other diagram source, under docs/.
- When control/trackers.md names a tracker_branch of this repository, it is a program of one repository: the tracker lives on that branch. Never check it out, merge it, or base a branch on it.

## Skills

The init or onboard profile installs them for your agent; the upgrade profile updates them to a new release.

- backlog: List and check the planning hierarchy, print a document's template or the components ARCHITECTURE declares, route a guide, render the architecture status, and record each story's issues, each with the model-free profile that runs one scrum-master word.
- check: Check a program repository the way CI does (srd045, srd043): the corpus gate, the vendored profiles against their manifest, and the drift doctor reports, each a word, writing nothing.
- checkouts: Lay the program's checkouts down from the system checkout (GH-2812): clone every repository the registry names at its group path beside the system checkout, and fetch and fast-forward the ones already there.
- design: Change the intended state ahead of implementation (srd047): a design decision becomes one issue whose only changes are to docs/ARCHITECTURE.md and docs/engineering/, with its Design Decisions.
- draft: Draft one planning document, a theme, initiative, epic, feature, or story, or an engineering document, from a description, in conversation with its owner, then check it and offer its children (srd046).
- implement: Take an issue, or a story's issues, into a branch and worktree, build them, and finish with the one merge that closes them (srd036).
- init: Lay down a program's system repository and install its skills (srd034, srd037): the corpus, the control files, the orientation, and the skills, then the profiles they run, vendored. Run it once with the agent to start a program; rerun it to check the repository and take a new release.
- issue: File, list, lint, link, comment on, close, clean up after, admit, approve, assign, give back, name a reviewer for, and recur the program's issues, and keep its labels, each with the model-free profile that runs one scrum-master word.
- merge: Deliver an issue by the one merge that closes it (srd013 R5): the gates, the --no-ff merge pushed to the default branch, and the closes, each a scrum-master word.
- onboard: Add a repository to a program and install its skills (srd034, srd037): its corpus and orientation here, its registration in the system repository, the skills, then the profiles they run, vendored. Run it once with the agent in a joining repository, SCRUM_MASTER_SYSTEM and SCRUM_MASTER_SUBSYSTEM naming where it joins; rerun it to check the repository.
- peer-read: Record a reviewer's peer read of a release (srd015): the gate's peer- read check passes once a person other than the releaser has read the release issue.
- people: Name who is on which scrum and epic and who signs off on which scrum in control/people.md, take them off, or list them, each with the model-free profile that runs one scrum-master word.
- plan: Plan the next release in the system repository, choosing its initiative items, publishing the plan as a tag, dating the period, and filing its sprint-end issues; or, in a team's repository, run Sprint Planning from the ranked stories (srd031, srd034 R3.1, srd045 R5).
- pop: Claim an issue and open its own branch and worktree (srd013 R7): the claim, the branch, the worktree, and the skills install, each a word.
- reconcile: Sweep the program for drift from the system checkout (GH-2822): every registered repository checked against the forge — unreachable, or redirected as a moved project is — and against what its documents declare: a checkout whose origin names another project, a parent or initiative pinned to what is not there, and the repository's doctor drift, run dry. Where the tracking system's host lists the group's projects, one the registry does not name is a finding too. Use on an existing installation that may have drifted, as when the forge's groups and repositories move through infrastructure tooling.
- release: Release an artifact at the version its commits call for (srd015): the next version, its gate, and its tag.
- reports: File the week's status and rollup issues, lay down a reports repository, and collect, aggregate, validate, or read back weekly reports, each with the model-free profile that runs one scrum-master word.
- roll: Roll the installed scrum-master release across the program from the system checkout (GH-2813): for every registered repository behind it, an upgrade issue and a merge request taking the release — the upgrade profile's steps committed on a branch — never a push to a default branch.
- setup: Get a fresh clone of any of a program's repositories ready to work in (srd043): the program's other repositories laid down beside it, the system repository among them, and the skills installed.
- site: Write the program's generated site into its reports repository (srd048): plain Markdown pages a forge renders as they are, one family of pages per word, each in its own folder under site/.
- specify: Turn one request into one admitted issue in this repository (srd035).
- sprint-end: Work the period's sprint-end issue, summarizing the period, asking the owner what to change, merging the summary, and releasing on confirmation (srd038 R1, R6).
- status-report: Work a person's weekly status issue in the repository they worked in: gather the week, ask only what the tracker cannot know, write the report the person approves, and merge it (srd018 R12, R14; GH-2725).
- status-rollup: Work a rollup issue: collect a team's or the program's period from the repositories where it was written, aggregate it into signals, ask the lead only for judgment on each, write the rollup they approve, and merge it (srd018 R13, R15, R17; GH-2725).
- tasks: Break a level into the units below it (srd022), a release into one Sprint Goal per team or a story chosen at Sprint Planning into the issues that execute it.
- tracker: Export, migrate, or sync the program's tracker, each with the model-free profile that runs one scrum-master word.
- upgrade: Bring a program repository to the scrum-master release installed on this host (srd043 R5): the profiles vendored, the repository repaired, and the skills refreshed.
<!-- managed:end -->
