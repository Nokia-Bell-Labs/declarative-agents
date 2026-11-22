<!-- managed:begin: the init and onboard profiles write the text down to managed:end and refresh it on a rerun; write your own notes outside it -->
# Epics

An epic is a large body of work under an initiative, typically one to
three months long. A Product Owner owns it. An epic that spans teams
lives here. An epic one team owns lives in that team's subsystem
repository. An epic is not a unit of work: the stories under it are
planned into sprints, and its progress is the share of their story points
done.

## Documents here

- Name: `epNNN-short-name.md`, numbered in order and never reused.
- Format: an SRD in the pinned parent release's Markdown format, with the fields docs/constitutions/design.md lists for this level; the draft skill drafts one in that layout.
- Status: `to_be_agreed` while it is drafted, `agreed` once it passes review, and `retired` when it is replaced. Only agreed documents are planned and rolled up. A retired document keeps its path, because links pin it.
- Parent: an initiative. Features and stories name an epic as theirs.
- Changes go through an issue and the corpus gate, never a direct edit on main.

## What reads them

- The plan skill lists the stories under an epic in each team's Product Backlog.
- Features and stories name an epic as their parent; its progress is the story points of its done stories over those of all its agreed stories.

See README.md at the repository's root for the whole hierarchy.
<!-- managed:end -->
