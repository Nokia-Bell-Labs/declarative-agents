---
id: design
title: Documentation Design Constitution
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

We use this constitution when creating or changing VISION, ARCHITECTURE, SRDs, use cases, test suites, semantic models, config-format specs, and the other documents of this repository. The repository specifies a project-agnostic AI-native scrum process and the skills that implement it; the goal is a corpus that people can read and tools can validate.

<!-- field: articles -->
## Articles

<!-- article: D1 -->
### D1 - Specification-driven development

Specifications lead implementation. A new skill or a change to one starts with an SRD, a use case, or an explicit documentation issue before any script changes. A script change points back to the requirement it implements.

<!-- article: D2 -->
### D2 - Every file is Markdown with tags

Every file is Markdown with marker tags. Structured artifacts use the format the pinned declarative-agents release declares (its eng10): VISION, ARCHITECTURE, SPECIFICATIONS, road-map, SRDs, use cases, test suites, semantic models, config formats, constitutions including this one, engineering guidelines, and a program's hierarchy documents (themes, initiatives, epics, features, and stories), which are SRDs in that format. A document holds only the fields the pinned release declares, in its order; data the process needs that no field is named for goes in the declared field the schema below names for it. scrum-master's configuration files are config documents (D11). Prose a person reads start to finish is Markdown under docs/guides, except a program repository's orientation files, which the init and onboard profiles write where a first-time reader looks: README.md and AGENTS.md at the root and a README.md in each hierarchy folder and in docs/engineering/ (srd043 R1.6, R2.4). Four files are not Markdown because a consumer outside this corpus requires their format: docs/external-checkouts.yaml, which mage audit writes for one run and git ignores; docs/corpus-charter.yaml, which the corpus audit executes; plugin/plugin.json, which the Agent Plugins 1.0 standard names; and .golangci.yml, which the repository's lint gate reads by path. A program repository the init profile lays down also keeps docs/external-pins.yaml, pinning the release of this repository its corpus follows (srd043 R3.3); this corpus needs none, because the format library and the audit are siblings in this repository (GH-190). The declarative-agents profiles under plugin/agents/ are YAML for the same reason: the pinned runtime reads them (D14).

<!-- article: D3 -->
### D3 - Traceability

Vision states goals and boundaries. Architecture states components and flows. SRDs define numbered requirements, one SRD per skill. Use cases exercise SRDs through touchpoints that cite requirement groups. Test suites validate use cases and acceptance criteria. A change keeps that chain intact.

<!-- article: D4 -->
### D4 - Issue-first process

The process has one unit of work, the issue. Documents describe every object (sprint, tracking issue, task, recurring issue, comment event, link) by how it relates to an issue and by the labels an issue carries. A document that introduces a second unit of work is wrong.

<!-- article: D5 -->
### D5 - Facts are fetched, judgment is asked

Every SRD separates what a script derives from the tracker from what a skill asks a person. A requirement that has a script assign, rank, or estimate a person is refused. Scripts are deterministic and idempotent; skills hold the questions.

<!-- article: D6 -->
### D6 - Project agnostic

No document names a person, an organization, a product, or a project. Examples use placeholder components and repositories. Anything a consuming project supplies (its subsystems, its components, its environments) is declared in that project's own SRDs, never as content of this corpus.

<!-- article: D7 -->
### D7 - Same corpus at every level

Every repository of a program, at system, subsystem, component, or test-subsystem level, carries the same docs/ corpus: SRDs, a road-map, and this constitution, checked by the same audit. Levels differ only by which SRDs their owner writes. Repository layout is declared, not inferred: a subsystem SRD names its components by repository URL and path, and an environment is an SRD in the test-subsystem repository.

<!-- article: D8 -->
### D8 - Acceptance criteria name their evidence

Every acceptance criterion names the test that proves it and the environment where that proof counts (local, dev, staging, production PoC, or a customer R&D environment, each defined by an environment SRD). A criterion without a named test and environment cannot be met and is reported by the audit. Done means the named test passed in the named environment on a recorded version, not that an issue closed.

<!-- article: D9 -->
### D9 - Cross-repository references are pinned

A reference to a requirement, interface, or SRD in another repository is a URL pinned to a release tag, for example https://host/group/repo/-/blob/v1.4.0/docs/specs/software-requirements/srd003-ingest.md#R2.1. An unpinned reference, or one whose tag or anchor does not resolve, is an audit failure. References inside one repository use the srdNNN Rn form.

<!-- article: D10 -->
### D10 - Code does the workflow; the model does language

Every step a program can do is a word: fetching facts, deriving state, filing and linking issues, validating files, computing aggregates, tagging, and posting events. A skill is a thin script, at most 50 lines, that runs words with --json and relays the person's choices among the options the words compute. The model does three kinds of work only: turning loose conversation into a structured file, combining several files into one summary, and iterating on that content with the person. A specification that gives the model any other step is refused, and the step moves into a word. This extends D5: facts are fetched by code, judgment is asked, and language is the only thing the model writes.

<!-- article: D11 -->
### D11 - Configuration is Markdown with tags

A configuration file scrum-master reads is a config document: Markdown with frontmatter (id, title) and marker tags, read by internal/corpus and decoded into the settings it holds. Each setting is a field, a field marker above a heading; a level 2 field is a top-level setting, and a field one level below another is a setting inside it. A field whose children are fields holds a mapping. A field whose children are records, a kind marker with an id above a heading, holds a list: each record is a mapping of its child fields with its id as name. A field with no children holds a value: a table is a list of mappings, one per row keyed by the header; a bullet list is a list of values; any other text is one value. Values read as integers, true or false, or text, and None is empty. The parent declares no type for these files, so a gate test in mage audit reads every config document in the repository, and each config format names its fields (schema.config_document).

<!-- article: D12 -->
### D12 - Future releases hold direction

An SRD whose release is not the next one holds its problem, goals, non-goals, and one requirement group of direction; its detailed requirement items are written when its release is planned, as its use cases are. Items written earlier and never built retire in place (D6) rather than wait to mislead the planning of the release before them. A requirement that depends on behavior an unbuilt SRD specifies states its interim rule, the behavior that holds until that SRD is built, as srd038 R2 does for Sprint Goals closing before track-goals runs. An SRD that is partly built keeps its built items live and reduces only the rest.

<!-- article: D13 -->
### D13 - The trunk is the monorepo

Decided 2026-10-22 (GH-189) and landed with GH-190: scrum-master is an application of this repository, applications/scrum-master, imported with its history. There is no parent to track from outside — the format library, the runtime, and this corpus are siblings at one tag, mage audit runs the sibling agent built from agent-core, and a document problem the format cannot express is solved by changing the format here. Program repositories remain satellites by design: each pins this repository's release in its docs/external-pins.yaml and vendors that release's profiles in .scrum-master, which install.sh's agent runs (GH-34); inside this repository the sibling is the release and nothing downloads.

<!-- article: D14 -->
### D14 - A skill is a profile

A skill's workflow is a declarative-agents profile under plugin/agents/<name>/: the machine's states are the skill's steps, its tools are the words the skill runs with --json, which the agent serves in its own process (GH-27), a word's refusal is a terminal state carrying the word's reason, and the model acts only in the machine's work state, extending D10 from instruction to enforcement. The profile is the source of truth: mage audit validates every shipped profile with the pinned runtime's --validate-config, and the skill install renders such a skill's SKILL.md from its machine, so an agent that does not run the runtime follows the same steps the runtime executes. D10's 50-line limit binds authored skill scripts; a rendered SKILL.md is generated data, bounded by its machine. implement was the pilot (GH-135); the remaining five converted on its evidence (GH-198). Every word a model profile runs also has a model-free profile of its own, which the model skill names, so a coding agent without a model does the model's part and runs each word through the agent (GH-48). Every profile ships and is vendored, but install writes a roster, not a skill per profile: the workflows a person starts and four group skills naming the single-word profiles a person runs now and then, so the agent's context carries a few skills a request can find rather than every step (srd037 R1.5, GH-2749). The agent serves the words to any profile that names their binary: that is intended, and a program may vendor profiles of its own beside the shipped ones. A profile routes a guide into its model's context with a word as the state before the model's first step: scrum-master backlog guide --with-request reads the run's request from command state ($from(seed), which the agent publishes for a command-line request as for a served one) and prints the guide then the request, the model's first message, so the guide is current with the release and the wiring is visible in the machine; specify and tasks route plan-vs-architecture this way (GH-59). Pasting a guide into a system prompt was rejected because the copy goes stale, and a profile-schema field because it would change agent-core's profile contract for one consumer.

<!-- article: D15 -->
### D15 - The architecture names a subsystem before its plan

ARCHITECTURE declares the intended state and release tags hold the actual state, so the plan is the difference between them, and each hierarchy document names the components it moves in its architecture context item (GH-55, docs/guides/plan-vs-architecture.md). A parent repository's ARCHITECTURE therefore names a subsystem before any hierarchy document exists in that subsystem's repository: a plan cannot transition what the architecture has not declared. scrum-master doctor enforces the order where it can read the system repository beside a subsystem one (srd045 R6.4, GH-56). A component declared ahead of any plan on purpose writes its status field beginning Deferred, with the reason after it, and the gate treats it as intended, drawing no warning however its text changes, and never as built (srd045 R6.3).

<!-- field: schema | form: "tree" -->
## Schema

- vision:
  - location: docs/VISION.md
  - required_fields:
    1. id
    1. title
    1. executive_summary
    1. problem
    1. what_this_does
    1. why_we_build_this
    1. success_criteria
    1. not
  - purpose: Defines what the process is, why it exists, and what it excludes.
- architecture:
  - location: docs/ARCHITECTURE.md
  - required_fields:
    1. id
    1. title
    1. overview
    1. interfaces
    1. components
    1. design_decisions
    1. technology_choices
    1. project_structure
    1. implementation_status
    1. related_documents
  - purpose: Defines planes, components, flows, decisions, and repository layout.
- specifications:
  - location: docs/SPECIFICATIONS.md
  - required_fields:
    1. id
    1. title
    1. overview
    1. roadmap_summary
    1. foundation_document_index
    1. srd_index
    1. config_format_index
    1. semantic_model_index
    1. use_case_index
    1. test_suite_index
    1. coverage_gaps
  - purpose: Indexes the corpus and records known coverage gaps.
- sprint_end_summary:
  - location: docs/sprint-ends/RELEASE.md in each repository whose sprint-end issue closes the period
  - sections: the period's summary in prose, then a Stories done section listing each story the owner reports done as project#story, or None.
  - purpose: Closes a period. The sprint-end skill writes it in the sprint-end issue's worktree, the sprint-end stories word writes its Stories done section, and the merge refuses a summary not in this format (srd038 R2.4). Its issue is the sprint-end issue, plane::data with kind::release (srd038 R1.1).
- roadmap:
  - location: docs/road-map.md
  - required_fields:
    1. id
    1. title
    1. releases
  - program_release_fields:
    - deliverables: the initiative items the release proves, inNNN-short-name#Rn.m (objNNN-short-name#Rn.m for an objective document), listed before release
    - release.publication: tag (the release tag Cites lines pin) and milestone, as tree items
  - release_statuses: specified (planned), implemented (built and proven by its test suite), released (tagged and handed off); the corpus gate refuses any other (srd045 R8.2)
  - purpose: Assigns SRDs and use cases to releases and status values. In a program's system repository a release also names the initiative items it proves and its tag and milestone, in program_release_fields (srd034 R3.1).
- srd:
  - location: docs/specs/software-requirements/srdNNN-short-name.md
  - required_fields:
    1. id
    1. title
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - numbering:
    - goals: G1, G2, G3
    - requirement_groups: R1, R2, R3
    - requirement_items: R1.1, R1.2, R2.1
    - acceptance_criteria: AC1, AC2, AC3
  - acceptance_criteria_fields:
    1. id
    1. criterion
    1. traces
    1. tests
    1. applicability
  - acceptance_criteria_note: tests and an environment are required in SRDs of consuming programs (D8); the criterion's applicability names its environment. This corpus specifies the process itself and has no environments yet, so its own SRDs omit them until the test subsystem's environment SRDs exist.
  - purpose: Defines one skill's requirements with checkable criteria.
- subsystem_srd:
  - location: docs/specs/software-requirements/srdNNN-short-name.md in the system repository
  - required_fields:
    1. id
    1. title
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
    1. implementation_targets
  - context_items:
    1. owner
    1. scrum_repository (optional)
  - implementation_targets_columns:
    1. name
    1. repository
    1. path
  - implementation_targets_empty: an SRD whose repositories are not registered yet writes the field as None under the field marker's form "empty list", the format's empty table; an empty table with only its header is not canonical, and doctor rewrites it to that form
  - numbering: same as srd
  - purpose: Declares one subsystem, its owner (a Product Owner, the owner item of context), and its components, each a row of implementation_targets by repository URL and path. Component label values (component::<name>) come from these rows. The optional scrum_repository context item names the team's planning repository, apart from its code, where its features, stories, Sprint Goals, sprint-end summaries, status reports, and team rollup live, and whose issues carry its first component's label; without it the team plans in its first component's repository, beside its code (GH-2738). A team with a scrum repository and no code repository yet lists its components with the scrum repository as their repository, until a code repository takes each row (GH-2745). The system SRD the init profile writes has the same two fields, declaring the system repository a component of the program owned by the technical program manager.
- environment_srd:
  - location: docs/specs/software-requirements/srdNNN-env-name.md in the test-subsystem repository
  - required_fields:
    1. id
    1. title
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
    1. context
    1. test_suite_traces
  - context_items:
    1. owner
    1. trigger
  - trigger_values:
    1. each_subsystem_release
    1. nightly_latest
    1. both
    1. human_approval
  - numbering: same as srd
  - purpose: "Declares one environment of the chain (local, dev, staging, production PoC, customer R\u0026D): who owns it and what triggers promotion into it (context items), and which test suites run there (test_suite_traces). Promotion is CI/CD in the test subsystem; handoff into a customer R\u0026D environment is a human approval."
- hierarchy_links:
  - parent: at most one, naming the level above by kind; theme, then initiative, then epic, then feature, then story; a story may name an epic directly, and only a feature must have a parent; a parent in another repository is a URL pinned to a tag, whose anchor is optional because it names the whole document
  - blocked_by: the documents that must finish before this one can; waiting derives from it (srd007)
  - depends_on: the documents whose output this one needs, which may proceed at the same time; reported, never blocking
  - relates_to: documents that share context; reported, never blocking
  - duplicates: the document this one repeats; the duplicate is retired
  - targets: a same-repository document id, or a URL pinned to a release tag (D9) for a document in another repository
  - architecture: the ARCHITECTURE components the document creates, modifies, or integrates, optional on every level (GH-55); a component of this repository is its bare id, which must be the id of a component heading in docs/ARCHITECTURE.md; a component of another repository is a URL pinned to a release tag (D9), …/-/blob/<tag>/docs/ARCHITECTURE.md#<component>; the grain is the component, decided 2026-10-24, and moves to finer elements only when a program shows components too coarse
  - purpose: The vertical and horizontal links of the planning hierarchy, written as context items of each hierarchy document; an issue carries the same links as body lines (srd007). Progress rolls up through parent, never through anchors.
- theme:
  - location: docs/specs/themes/thNNN-short-name.md in the system repository; the folder's README.md, which the init and onboard profiles write, explains the level and is not itself a hierarchy document
  - format: an SRD in the pinned release's Markdown format (D2), as every hierarchy document is; scrum-master's format tools read it with the release's SRD declaration
  - required_fields:
    1. id
    1. title
    1. status
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - status_values:
    1. to_be_agreed
    1. agreed
    1. retired
  - context_items:
    1. owner
    1. horizon
    1. relates_to
    1. architecture
  - numbering: same as srd; each requirement group is one strategic goal
  - acceptance_criteria_fields: same as srd
  - purpose: A strategic goal area for months to years, owned by the technical program manager. Initiatives name it as their parent, and reports group the initiatives and epics beneath it.
- initiative:
  - location: docs/specs/initiatives/inNNN-short-name.md in the system repository; the folder's README.md, which the init and onboard profiles write, explains the level and is not itself a hierarchy document; an objective document at docs/specs/objectives/objNNN-short-name.md is read as an initiative without change
  - format: an SRD in the pinned release's Markdown format (D2)
  - required_fields:
    1. id
    1. title
    1. status
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - status_values:
    1. to_be_agreed
    1. agreed
    1. retired
  - context_items:
    1. owner
    1. horizon
    1. parent
    1. blocked_by
    1. depends_on
    1. relates_to
    1. duplicates
    1. architecture
  - requirement_group_fields:
    1. title
    1. applicability
    1. items
  - requirement_group_applicability_items:
    1. due
    1. delivered_by
  - numbering: same as srd; each requirement group is one outcome
  - acceptance_criteria_fields: same as srd
  - acceptance_criteria_note: tests and applicability are required (D8); applicability names the environment. Until the program's environment SRDs exist, it names a chain stage D8 lists and the audit cannot resolve it (srd034 R1.5). A test written "name (R1.1, R1.2)" names the items it proves, so a criterion two teams' items share puts only each team's tests on its Sprint Goal; a test without items proves every item the criterion traces (GH-20).
  - purpose: A multi-team outcome over several quarters, the program's Product Goal, owned by the technical program manager. Its parent is a theme; epics name it as theirs. plan places its outcomes in a release and tasks cites them, pinned, on each team's Sprint Goal (srd034).
- epic:
  - location: docs/specs/epics/epNNN-short-name.md, in the system repository when it spans teams or in a subsystem repository when one team owns it; the folder's README.md, which the init and onboard profiles write, explains the level and is not itself a hierarchy document
  - format: an SRD in the pinned release's Markdown format (D2)
  - required_fields:
    1. id
    1. title
    1. status
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - status_values:
    1. to_be_agreed
    1. agreed
    1. retired
  - context_items:
    1. owner
    1. horizon
    1. parent
    1. blocked_by
    1. depends_on
    1. relates_to
    1. duplicates
    1. architecture
  - numbering: same as srd; each requirement group is one capability
  - acceptance_criteria_fields: same as srd
  - purpose: A large body of work spanning several sprints, typically one to three months, owned by a Product Owner. Its parent is an initiative; features and stories name it as theirs, and its progress is the share of its descendants' story points done (srd045). Not a unit of work (D4) and not a tracker object.
- feature:
  - location: docs/specs/features/ftNNN-short-name.md in the owning team's repository; the folder's README.md, which the init and onboard profiles write, explains the level and is not itself a hierarchy document
  - format: an SRD in the pinned release's Markdown format (D2)
  - required_fields:
    1. id
    1. title
    1. status
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - status_values:
    1. to_be_agreed
    1. agreed
    1. retired
  - context_items:
    1. owner
    1. parent
    1. blocked_by
    1. depends_on
    1. relates_to
    1. duplicates
    1. architecture
  - numbering: same as srd; each requirement group is one part of the capability
  - acceptance_criteria_fields: same as srd
  - purpose: An optional layer, a distinct capability within an epic spanning two to four sprints, used when an epic is too large to break straight into stories. Its parent, an epic, is required; stories name it as theirs.
- story:
  - location: docs/specs/stories/stNNN-short-name.md in the owning team's repository; the folder's README.md, which the init and onboard profiles write, explains the level and is not itself a hierarchy document
  - format: an SRD in the pinned release's Markdown format (D2)
  - required_fields:
    1. id
    1. title
    1. status
    1. context
    1. problem
    1. goals
    1. requirements
    1. non_goals
    1. acceptance_criteria
  - status_values:
    1. to_be_agreed
    1. agreed
    1. retired
  - context_items:
    1. owner
    1. parent
    1. as_a
    1. i_want
    1. so_that
    1. points
    1. rank
    1. cites
    1. blocked_by
    1. depends_on
    1. relates_to
    1. duplicates
    1. architecture
  - context_note: as_a, i_want, and so_that are the user story's statement, written from the user's view. points is a relative estimate of the story's work, never of a person (D5). rank orders the team's Product Backlog, lowest first, and the Product Owner sets it (srd045 R5.1). cites lists the SRD requirement items the story's criteria prove, each a URL pinned to a release tag (D9); a story that cites nothing leaves the item out, since an item with nothing under it does not parse.
  - numbering: same as srd; one requirement group holds the story's detail, and its acceptance criteria are the story's own
  - acceptance_criteria_fields: same as srd
  - acceptance_criteria_note: tests and applicability are required (D8). An evidence event proves one criterion; it is posted on the team's Sprint Goal and names the criterion by pinned URL (srd006).
  - purpose: The unit of value in the Product Backlog, sized to fit one sprint and owned by the team's Product Owner. Its parent is a feature or an epic. Sprint Planning selects stories onto the team's Sprint Goal, and tasks turns each into the issues that execute it, each with an Implements line (srd045). It is not a unit of work (D4); the issue that executes it is.
- objective:
  - status: retired
  - replaced_by: initiative
  - purpose: The former name of the initiative. An objective document at docs/specs/objectives/ is read as an initiative without change, and the objective issue is the Sprint Goal (srd034).
- use_case:
  - location: docs/specs/use-cases/relNN.N-ucNNN-short-name.md
  - required_fields:
    1. id
    1. title
    1. summary
    1. actor
    1. trigger
    1. flow
    1. touchpoints
    1. success_criteria
    1. out_of_scope
  - numbering:
    - flow_steps: F1, F2, F3
    - touchpoints: T1, T2, T3
    - success_criteria: S1, S2, S3
    - assumptions: A1, A2, A3
    - edge_cases: E1, E2, E3
  - optional_fields:
    1. status
    1. retired_in
    1. replaced_by
    1. needs_clarification
    1. assumptions
    1. edge_cases
    1. priority
  - priority: P1 must ship for the use case's release, P2 should, P3 could. An absent priority means unranked. No other value is valid. A use case schedules this corpus's own releases; in a program's Product Backlog the order of its stories is the priority (story, srd045).
  - purpose: Describes one end-to-end path through the process.
- test_suite:
  - location: docs/specs/test-suites/test-relNN.N-short-name.md
  - required_fields:
    1. id
    1. title
    1. release
    1. traces
    1. preconditions
    1. test_cases
  - optional_fields:
    1. status
    1. retired_in
    1. replaced_by
    1. needs_clarification
  - purpose: Maps release use cases and SRD criteria to named tests.
- semantic_model:
  - location: docs/specs/semantic-models/name.md
  - required_fields:
    1. id
    1. title
    1. purpose
  - purpose: Defines vocabulary, ownership, or cross-cutting concepts.
- config_format:
  - location: docs/specs/config-formats/name.md
  - required_fields:
    1. id
    1. title
    1. purpose
  - purpose: Defines a machine-readable format the scripts read or write.
- constitution:
  - location: docs/constitutions/name.md
  - required_fields:
    1. id
    1. title
    1. purpose
  - purpose: Defines rules for agents and contributors working in this repository; this constitution holds the schema of every document type.
- config_document:
  - location: control/labels.md and control/policy.md in a program repository, reports.md in the reports repository, person.md in the person's configuration directory, release-gates.md in this repository
  - required_fields:
    1. id
    1. title
  - purpose: A configuration file scrum-master reads, in the form D11 states; each config format names its settings.
- corpus_charter:
  - location: docs/corpus-charter.yaml
  - required_fields:
    1. id
    1. title
    1. checks
  - purpose: Names the specification-corpus checks the parent's corpus audit gates on in this repository (docs/guides/corpus-audit.md).
- external_pins:
  - location: docs/external-pins.yaml
  - required_fields:
    1. pins
  - purpose: Maps each parent repository to the release this corpus tracks; every derived_from entry is validated against a release of its parent.
- external_checkouts:
  - location: docs/external-checkouts.yaml
  - required_fields:
    1. checkouts
  - purpose: Points each parent repository at the downloaded release's source for one audit run; written and removed by mage audit and ignored by git, or written by a person to override the location.
- engineering_guideline:
  - location: docs/engineering/engNN-short-name.md
  - required_fields:
    1. id
    1. title
    1. introduction
    1. sections
    1. references
  - purpose:
    - "Defines the engineering of the repository's system or subsystem, in the detail ARCHITECTURE's index has no room for": architecture descriptions of how its parts are structured, run, and deployed, and the practices its parts share. ARCHITECTURE names each engineering document in its related documents.
- guide:
  - location: docs/guides/name.md
  - format: markdown
  - required_fields: []
  - purpose: Prose a person reads start to finish.

<!-- field: review_checklist -->
## Review checklist

- Style: Short, exact sentences. Active voice. No filler. Exact paths, commands, labels, and field names over broad claims.
- Voice: "We" for process behavior and obligations. Present tense for current behavior. Planned status only where implementation or tests are missing.
- Diction: No hype, vague intensifiers, stock transitions, or claims that do not name a behavior, file, command, label, or contract.
- Formatting, prose: Paragraphs for explanation. Lists for fields, steps, and checklists. Short table cells.
- Formatting, fields: Lowercase snake_case field names. IDs stable once referenced. mage fmt writes the canonical form.
- Formatting, diagrams: Mermaid in fenced code blocks. No generated image files in docs/.
