---
id: srd048-site
title: "Skill: site"
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: design_notes -->
## Design notes

- Content first, presentation later. The site is plain Markdown, with Mermaid in fenced code blocks, which GitHub and GitLab render as they are; committing the pages through the inner loop is already a site anyone with access to the reports repository reads. A static site generator is a later rung that consumes the same files.
- One word per output family. Each family of pages, status, timeline, history, and the corpus, is written by its own word into its own folder, and the site profile's machine runs the words in order, one state each, so a run is observable a step at a time.
- Ownership by stamp. Every generated page closes with a stamp naming the scrum-master version and the checksum of the page, the pattern installed skills use (srd037). The index is written once and is the program's after, so it needs neither stamps nor managed blocks. Decided 2026-10-25 in GH-87.
- The timeline draws itself. Its page is SVG the page's own script renders, each item drawn as what it is, with no library (GH-136, replacing frappe-gantt, GH-82), so the shapes a program needs (release lines, due-date diamonds, Sprint Goal flags, actual and as-planned bars beside the plan) are not limited to a library's bars on rows.
- No settings. The site generates every view; the reports repository's reports.md gains no site settings until a program asks for them. Decided 2026-10-25 in GH-87.
- One site per program. It lives in the reports repository, built from the system repository's corpus and every team's issues and Sprint Goals, as the reports are; a team reads its part there rather than in a site of its own. Decided 2026-10-25 in GH-96, answering GH-15.
- History from the program's first corpus tag. The plan is tagged at each release, so the requirement history and the timeline's earlier plans start at the oldest corpus tag the road map names, and every corpus tag after it is a step, so an earlier program's tags the repository still holds drop out while the plan tags between releases stay; a road map naming none scopes nothing (GH-115). Commits to docs/ between tags are the merges each change names, not steps of their own. Each tag's documents are read into a snapshot kept inside the system checkout's git directory, or where SCRUM_MASTER_SITE_CACHE names, so decrypted text does not outlive the checkout.
- The site is the dashboard. srd019's track-goals renders no dashboard of its own: its ledgers and findings are read on the status pages, which render from them.
- The forge first. The first presentation rung is the forge rendering the committed pages; Docsify, Hugo, or Starlight are later rungs that read the same files, each its own decision.
- The timeline facts live in the site. They are computed by the site's timeline word and written under site/timeline/, where the gantt views read them, rather than by the reports aggregate.
- The machine grows with its words. It ships with the scaffold word alone, and each follow-on word inserts its state before Done as it lands, rather than passing through states for words that do not exist yet. Each also adds its requirement group here: the status pages (GH-83), the timeline facts and gantt views (GH-82, GH-84), the requirement history (GH-85), and the corpus pages and publication (GH-86).

<!-- field: problem -->
## Problem

Nobody outside the tracker can see a program's status, how its
requirements changed release by release, or how its epics and stories
track against the plan in time, without running the profiles. The specs
promise a dashboard (srd019), and nothing builds one.

<!-- field: goals -->
## Goals

- G1: A reader who knows no git answers what exists, what is being built and when it lands, and what is planned beyond, from rendered pages alone.
- G2: Regenerating the site never overwrites what a person wrote.

<!-- req: R1 -->
## R1 - The site word and its profile

<!-- req: R1.1 -->
### R1.1

scrum-master site writes the generated site into the reports repository's checkout by running every site word in the order the site profile's machine runs them; scrum-master site scaffold and each follow-on word run one family alone. The site profile is a model-free machine with one state per word, starting with the scaffold word, each follow-on word inserting its state before Done as it lands (design.md D10, D14).

<!-- req: R1.2 -->
### R1.2

The scaffold word writes site/index.md, linking each generated folder, when it is missing and the program's to edit after; site/index.html, the app shell GitLab Pages serves (R7.3), as a generated page, stamped, refreshed each run, and kept once edited, taking back a shell an earlier release wrote once and unstamped (GH-127); site/overview.md, the shell's home, as a placeholder until the overview word writes it (R8.3); and a page for each generated folder its word has not written yet, saying which word fills it.

<!-- req: R1.3 -->
### R1.3

The reports repository's checkout is --dir, else SCRUM_MASTER_REPORTS, else the checkout beside the one the word runs in named for --project; no program configuration names it (srd018 R1.3). The words write files and never commit or push; the pages land through the inner loop like any other change.

<!-- req: R2 -->
## R2 - What the site owns

<!-- req: R2.1 -->
### R2.1

The generated folders are site/status/, site/timeline/, site/history/, and site/corpus/, beside reports/, summaries/, and deck/ (srd018 R1.1). Each belongs to one word, which on every run writes its pages and removes the pages it generated there before and no longer generates.

<!-- req: R2.2 -->
### R2.2

Every generated page closes with a stamp, a comment naming the scrum-master version that wrote it and the checksum of the page before it, in Markdown and HTML alike. A word replaces a page whose stamp still matches; a page with no stamp, or whose stamp no longer matches because a person edited it, is kept and reported with the reason, never replaced or removed.

<!-- req: R2.3 -->
### R2.3

site/index.md is written only when missing, or when it is still the placeholder scrum-master reports init lays down, and is the program's to edit after; the site never rewrites an index the program changed (GH-107). The reports repository's reports.md holds no site settings: the site writes every view.

<!-- req: R3 -->
## R3 - Timeline facts

<!-- req: R3.1 -->
### R3.1

The timeline word writes site/timeline/facts.json: each release of the road map with its sprint, the days of the ISO weeks its milestone names (2026-W43..W45, a second week without its year in the first's year or the next), else the dates of the program's milestone of that title; and an item for each initiative, epic, feature, story, Sprint Goal, and other issue of the program's repositories with its kind, title, parent, architecture citations, state, release and tag, planned days, actual days, and points done over total. The working tree's facts include the documents of the team checkouts beside the system's that serve its documents through a parent pinned by URL, each named <checkout>/<id> under the document it serves, done as its team's sprint-end summary records, and selected by its team's Sprint Goals under that name; a tag's facts are the system's plan alone.

<!-- req: R3.2 -->
### R3.2

A story is planned in the sprint of the release whose Sprint Goal selected it, else of the release whose sprint-end summary records it done, else over its nearest horizon; it is done when a summary records it, its actual days ending the day that summary merged, read from local git. Any other document is planned over its own horizon, else over its stories' planned days, lands in the first release whose sprint ends on or after its planned end, and is done when every agreed story under it is. A Sprint Goal's actual days run from the day its issue opened to the day it closed, read from the tracker; any other issue is planned in the sprint of the release its milestone names, runs from the day it opened to the day it closed, is done once closed, and is parented on the story it implements when its description names one (GH-96). An agreed item not done is active once its planned days begin, a story only once a release takes it, since one no Sprint Goal selects is unplanned however early its horizon begins (GH-141); any other item not done is future. Today is the day in UTC, as every stamp the site writes is (GH-141).

<!-- req: R3.3 -->
### R3.3

The word also writes facts@<tag>.json for each corpus release tag of the system repository, the tags the policy's tag format makes for the corpus artifact, listed from local git alone: the plan as the corpus held it at the tag, read through textconv, with the actual days of now, so a gantt view shows an item's slippage against any earlier plan without a baseline kept by hand. A tag's corpus is read once and kept by its commit and the textconv filters that read it, so the words of one run, separate processes, read each tag once. It writes site/timeline/index.md, listing each item, until the gantt views draw them. With no tracker reachable it leaves out the Sprint Goals and the milestones named by title, and says so.

<!-- req: R4 -->
## R4 - Status pages

<!-- req: R4.1 -->
### R4.1

The status word writes, under site/status/, an index page and one page each for the road map (each release, its status, tag, milestone, sprint, and items), the story-point roll-up (points done over total for each theme, initiative, epic, and feature, srd045 R3.3, with the team checkouts' agreed stories beside the system's counted in the documents they serve, up to the theme, as backlog status counts them, GH-141), the architecture status (the page the architecture-status word renders, srd045 R8.5, one source, its docs-relative links pointed at the corpus copies, or at the forge when the corpus is withheld, under SCRUM_MASTER_WEB_BASE when it is set (GH-141), since the page is written for docs/ and nothing sits beside it under site/status/, GH-122), the Sprint Goals (each one's ledger, the status its owner last reported, and outcome, srd038, GH-141), the signals (the weekly reports' aggregate, srd018 R3), and the open issues (each by plane, kind, state, and op). The road map page embeds the interactive road map (R5.3) above its table, and the timeline's index the interactive timeline above its own; the forge strips the figures and the tables carry the same facts (GH-131). Each page is plain Markdown under a generated-file header, and the facts behind it are under site/data/status/ as JSON.

<!-- req: R4.2 -->
### R4.2

The Sprint Goals and signals cover the sprint of the release whose sprint holds the day the word runs, else the latest begun. With no tracker reachable, the Sprint Goals, signals, and open issues pages say so and list nothing; with no reports.md in the reports checkout, the signals page says there are no reports.

<!-- req: R5 -->
## R5 - Gantt views

<!-- req: R5.1 -->
### R5.1

The gantt word draws the timeline facts, and computes none, under site/timeline/gantt/: program.md, a mermaid gantt chart of the initiatives, and a page per initiative whose epics are sections holding their features and stories as tasks. Each chart is calendar days with weekly ticks labeled by ISO week, the releases as milestones at their sprints' ends, and each task done, active, or plain for future.

<!-- req: R5.2 -->
### R5.2

No chart draws more than 30 tasks: a view with more is split into several charts, a section kept whole where it fits.

<!-- req: R5.3 -->
### R5.3

program.html is one self-contained page whose own script draws the plan as SVG, the plan now and each corpus tag's inlined, making no request of its own (GH-136). Its rows are the plan's tree beside the chart, row for row: themes, initiatives, epics, features, and stories under the documents they serve, a toggle on each row with children folding its descendants, themes and initiatives open on the timeline and themes alone on the road map, what a reader opens or closes kept across views and zoom (GH-133). Each item is drawn as what it is: the sprint grid, three weeks aligned to the program's earliest release sprint, under a header of months and of each column's ISO weeks; a release as a vertical line at its sprint's end, labelled with its version and status, solid once released and dashed until then; a theme, an initiative, or a feature as a summary bracket over its planned days; an initiative's requirement groups as diamonds at their due dates, labelled R1, R2; an epic as a bar over its planned days, coloured by kind and filled by its points done over total, its title on it where it fits; a story as a block over its sprint, solid when done, outlined while active, faded when future; a Sprint Goal not as a row but as a flag on each epic whose stories it selects, at its release's sprint end, coloured done, active, or future; an issue not as a row, its opened-to-closed days giving its story's actual span; any item's actual days as a thin bar beneath its plan, patterned while still open; the plan at a chosen corpus tag as a dashed ghost behind each item, so slippage shows; a dependency, an item's depends_on and blocked_by links or an issue's Depends-on and Blocked-by lines, as a grey elbow arrow from the end of what it waits on to its start, or, where the plan has the dependant start before that ends, an amber connector into its start (GH-135); and today as a red line. Week, Sprint, and Quarter buttons set the scale, the timeline opening at Sprint on today and the road map at Quarter from its start; a legend names every shape; hovering any shape names it, and clicking a planning item opens its page in the site, in the top window so the embedded chart navigates the app, a Sprint Goal or an issue its tracker page. Under ?embed=1 it drops its heading (GH-128) and, after each draw, tells the page holding it its height, which the app shell gives its frame, so the chart scrolls with the page rather than inside its frame (GH-136).

<!-- req: R6 -->
## R6 - Requirement history

<!-- req: R6.1 -->
### R6.1

The history word compares the corpus at each pair of consecutive corpus release tags, listed from local git and read through textconv: each requirement item (a req record whose id has a dot) and acceptance criterion of every document under docs/ is added when the later tag holds it and the earlier does not, removed in the reverse, retired when the later tag first marks it with status retired or retired_in rather than removed and added, and reworded when its text differs beyond whitespace; an initiative item the road map delivers in another release is moved, one it comes to deliver placed, and one it stops delivering taken off (GH-141), each a change to the initiative made in the road map: it is on both their pages, and its merges are the road map's (GH-116). Each change names the merges on the main line between the two tags that changed its document, each by its commit, subject, and the user its Merged-by trailer names, else its git author.

<!-- req: R6.2 -->
### R6.2

It writes, under site/history/, an index of the steps, newest first, a page per step listing its changes by document, and under site/history/docs/ a page per document gathering its changes across every step, beside a JSON copy under site/data/history/. The walk over tag pairs is the ground srd018 R3.7's aggregate between two tags can stand on; that aggregate is not built here.

<!-- req: R7 -->
## R7 - Publication

<!-- req: R7.1 -->
### R7.1

The corpus word copies every Markdown document under the system repository's docs/ to the same place under site/corpus/, with an index naming each by its title, the machine's last working state. A relative link that lands inside docs/ is kept, since the site holds its target at the same relative place; one that leaves docs/ points at the document on the forge, from the system repository's origin as configured, or is left as written when the origin is no forge a browser opens. A document gone from docs/ is removed from the site. A document the requirement history names closes with a History section, its changes across the corpus tags newest first, linking its page in the history (GH-96). When the system repository encrypts its documents at rest (git-crypt, by their filter attribute) and the reports repository does not encrypt site/, the word copies none of them: site/corpus/ holds one page saying why, which prunes any copied before, and the run's notes say so, unless SCRUM_MASTER_SITE_DECRYPTED=1 opts in (GH-115). The status, timeline, and history pages carry document ids, titles, and merge subjects, never a document's text.

<!-- req: R7.2 -->
### R7.2

The reports repository is where the site lives: scrum-master reports init lays down site/index.md and a README that says how the site is read and refreshed. The baseline is the forge: the pages are committed through the inner loop like any other change, and GitHub and GitLab render them and their mermaid charts as they are, so a reader needs access to the reports repository and nothing else. In CI, a scheduled job on the release image (eng03) runs the site profile and lands the change through the issue workflow; the site never commits. A static site generator can later read the same files and is not built here. The system and team repositories' READMEs point readers at the site.

<!-- req: R7.3 -->
### R7.3

The reading surface is GitLab Pages (presentation rung ii, GH-119): a pages CI job publishes site/ as public/, unbuilt, and site/index.html, the app shell the scaffold writes (R1.2), renders the Markdown beside it in the browser, with client routing, the navigator (R8.2) as an expandable tree whose top-level sections open and whose current page's branch opens on every route, docked from 520 pixels wide and behind a labelled Menu button below that (GH-131), and from 769 pixels wide divided from the page by a divider the reader drags, moves with the arrow keys, or resets with a double-click, the page reflowing to the width and the width kept in the reader's browser (GH-2723), search, and mermaid, from Docsify and mermaid at versions the shell pins on a CDN, opening on the overview (R8.3); the interactive timeline (R4.2) and the JSON facts open from it as the files they are. An air-gapped GitLab vendors the pinned libraries into site/ by editing the shell, which the scaffold then keeps. On a private Pages site, whose sign-in session lapses while the app stays open, a page that comes back 404 because its request was redirected to sign-in reloads the whole page once, a minute's guard against looping, so the session renews and the reader lands on the page (GH-139). A page read before the session lapsed renders again from the app's memory without asking the server, while its embedded timeline is fetched anew; a timeline frame that loads anything but the timeline renews the session the same way (GH-2723). The forge's repository view (R7.2) stays the no-tooling fallback, since the shell changes no Markdown.

<!-- req: R8 -->
## R8 - Navigating the program

<!-- req: R8.1 -->
### R8.1

The hierarchy word writes a page per theme, initiative, epic, feature, and story under site/hierarchy/, a team's under its checkout's name, from the timeline facts, whose items carry each document's horizon, owner, rank, a story's statement, its source on its own repository's forge, and its path there, and each Sprint Goal's selected stories. Each page states the document's kind, state, horizon, target release, points done over total, and owner, quotes a story's statement, and links Up to its parent, Down to its children in rank order, to the Sprint Goals selecting it or a story under it, to the issues implementing a story, each on the tracker when its address opens in a browser, and to its source on the forge and, for a system document, the site's corpus copy; every link between two pages is reciprocated. Each level has one colour, the interactive timeline's, and each page names its document's level with a labelled chip in that colour, the levels of its Up and Down links likewise, and opens with a breadcrumb linking the plan and each document above it (GH-2723). The folder's index is the plan as cards: a legend saying what each level is and its timeframe, as the planning hierarchy table init writes says them, then a band per theme with its horizon, what lies under it, and its points, holding a card per initiative with its state, horizon, target release, and points, holding a subcard per epic with what lies under it and its points, and a band for the documents no theme holds; each card links its page, relative to the index so the forge follows it, and the shell routes it (GH-2723). The nodes are JSON under site/data/hierarchy/ (GH-126).

<!-- req: R8.2 -->
### R8.2

The hierarchy word also writes site/_sidebar.md, the navigator the shell loads on every page: the overview, the program's index as About, the generated folders, the timeline opening into the program's sprints (R8.4), the plan's tree from every theme to every story, each named with its level's one-letter chip and titled with its own title, and each of the program's repositories with a checkout beside the reports checkout, the system's and those its subsystem SRDs declare, and the reports repository named by its repository rather than a worktree's folder, linked on its forge when its configured origin opens in a browser; a checkout the program does not declare is someone else's and is left out (GH-141). The shell searches every page the navigator links, so every story (GH-127).

<!-- req: R8.3 -->
### R8.3

The overview word writes site/overview.md, the shell's home: the program's name, the interactive timeline embedded (R4.2, without its heading), a tile per initiative with its level's chip, its state, a bar of points done over total, horizon, target release, and epics, linking its page, and the releases. The tiles are HTML the shell styles and routes; on the forge they read as plain blocks (GH-128).

<!-- req: R8.4 -->
### R8.4

The hierarchy word writes a page per release sprint under site/sprints/, titled by the ISO weeks the sprint spans and its release, with the folder's index listing them. Each holds the sprint's days and its release's status and tag; a table of the epics active in it, whose planned days overlap the sprint or a story of which is in it, with state, points, planned days, and the count of their stories in it; a table of the stories planned into it, within its days or placed in its release, with their epic, state, and points; and the Sprint Goals committed for its release, with their state and the stories they select. Every planning item links its page, every Sprint Goal its tracker where a browser opens it (GH-139).

<!-- req: R8.5 -->
### R8.5

No generated page links what does not exist, by the app's resolution: relative to the page, a folder link to its index.md, a bare one to its .md. A folder the site links has its own index page, and a document the system repository does not hold is named without a link (GH-139).

<!-- field: non_goals -->
## Non-goals

- A static site generator or a Node build; the shell renders the committed Markdown in the browser, and a generator is a later rung reading the same files.
- Committing or pushing the pages; the inner loop lands them.
- Settings for which views to generate.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The site word writes the index and each generated folder's page in a reports checkout; a second run changes nothing; an edited index and a page a person wrote are kept and reported; the site profile's machine runs the scaffold word to Done.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [R1.2](#r12)
- [R1.3](#r13)
- [R2.1](#r21)
- [R2.2](#r22)
- [R2.3](#r23)

<!-- ac: AC2 -->
### AC2

On a program with one done, one active, and one future epic, the timeline facts give each item its planned and actual days, state, parent, citations, and points; the facts at two corpus tags whose plans differ in one epic's horizon differ in that epic's planned days alone, and a tag of another name is no release; the ISO week ranges read across year boundaries; an issue is planned by its milestone's release, runs from opening to closing, and sits under the story it implements; a tag's corpus read twice is read once.

<!-- field: traces -->
#### Traces

- [R3.1](#r31)
- [R3.2](#r32)
- [R3.3](#r33)

<!-- ac: AC3 -->
### AC3

On a fixture program the status pages render as plain Markdown against golden files, each page's facts beside it; the architecture page is the architecture-status word's page; the roll-up's numbers are the hierarchy roll-up's, and a story a summary records done moves them; the open issues are listed by plane, kind, state, and op.

<!-- field: traces -->
#### Traces

- [R4.1](#r41)
- [R4.2](#r42)

<!-- ac: AC4 -->
### AC4

On the fixture program the program's and the initiative's charts are valid mermaid gantt, against golden files, with each state and each release's milestone; an initiative with more than 30 tasks is split; the HTML page holds the library and both plans, names no URL it would fetch, and the earlier plan moves the epic that slipped.

<!-- field: traces -->
#### Traces

- [R5.1](#r51)
- [R5.2](#r52)
- [R5.3](#r53)

<!-- ac: AC5 -->
### AC5

On a fixture repository with three corpus tags, the step between the first two lists exactly an added, a reworded, a retired, and a moved item, each with its merge and the user who merged it; a document's page gathers its changes from both steps; everything is read from local git.

<!-- field: traces -->
#### Traces

- [R6.1](#r61)
- [R6.2](#r62)

<!-- ac: AC6 -->
### AC6

On a fixture program, the agent runs the site profile end to end, its machine passing every word's state to Done, and writes the tree of pages, facts, and corpus a golden tree names; the corpus pages' links resolve within the site or at the forge, and a document the history names closes with its history; reports init lays down the site's index and the README. The reader test holds on the rendered pages: a reader who knows no git answers what exists (the architecture page's What exists), what is being built and when it lands (its What is being built, the road map, and the timeline), and what is planned beyond (What is intended and the later releases), from the site alone.

<!-- field: traces -->
#### Traces

- [R7.1](#r71)
- [R7.2](#r72)
- [R7.3](#r73)

<!-- ac: AC7 -->
### AC7

On a fixture program with a team checkout, every theme, initiative, epic, feature, and story has a page whose links up and down are reciprocated, a story's page names the Sprint Goal selecting it and the issue implementing it, and a team story links its own repository; every page names its level with its chip and links each document above it; the plan's index is a band per theme holding a card per initiative holding a subcard per epic, each linking its page; the navigator lists the plan's tree, each document with its level's chip, and the repositories; the overview embeds the timeline with one tile per initiative; and the shell is refreshed, kept once edited, and taken back from an earlier release's unstamped copy.

<!-- field: traces -->
#### Traces

- [R8.1](#r81)
- [R8.2](#r82)
- [R8.3](#r83)
- [R8.4](#r84)
- [R8.5](#r85)
- [R1.2](#r12)
