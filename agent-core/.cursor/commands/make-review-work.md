# Command: Make Review Work

Analyze code against the active design context, project requirements, and style rules, then propose or file Beads issues for the deviations found. Use this command for planning chats where the goal is to discover the next batch of work from review evidence instead of an existing issue.

## Workspace discovery

When the user says `/make-review-work`, discover active project directories from the workspace root instead of using a hard-coded directory list.

Scan only direct child directories of the workspace root. A directory is active when it has:

1. `go.mod`
2. `docs/`
3. source or configuration roots such as `cmd/`, `internal/`, `pkg/`, `tools/`, `agents/`, or `magefiles/`

A directory is fileable when it is active and also has `.beads/`.

When the user says `/make-review-work on <dir>`, scope to that directory if it exists. Explicit scope may include a deprecated directory, but the command must say it is deprecated before reviewing it.

Skip deprecated or archived directories by default. This includes directories named `DEPRICATED`, `DEPRECATED`, `deprecated`, `archive`, or paths below those directories. Do not infer active queues from samples, fixtures, experiments, generated worktrees, or nested `go.mod` files.

Report every discovered directory before review:

- `fileable`: active and has `.beads/`; review and allow Beads issues after user approval.
- `non-fileable`: active but lacks `.beads/`; review only if useful, report findings, and do not try to file issues there.
- `skipped`: deprecated, missing, fixture, generated, or missing required markers.

Current dry-run expectation for this workspace:

- `agent-core/`: fileable because it has `go.mod`, `docs/`, source roots, and `.beads/`.
- `go-unix-utils/`: non-fileable because it has `go.mod`, `docs/`, and source roots, but no `.beads/`.
- `DEPRICATED/generator/`: skipped by default because it is under a deprecated directory.

## Review goals

Look for work that should become issues:

1. Code that deviates from a recent chat design, documented design, architecture, semantic models, config formats, SRDs, use cases, or test suites.
2. Code that only partially implements requirements that appear supported by configuration or docs.
3. Duplicated implementation, fixtures, helpers, validation logic, schemas, redaction, metrics, parsing, or launch/test harnesses.
4. Code that violates execution, Go style, testing, or issue-format constitutions.
5. Missing or weak tests for public behavior, cross-module contracts, or requirement-level acceptance criteria.

Prioritize correctness and misleading public contracts over cleanup. Do not file style-only issues ahead of requirements or behavior gaps unless the style problem blocks safe implementation.

## Design context mode

Before reading files, decide which design context applies:

1. **Recent chat design mode**
   - Use this when the current chat recently produced or agreed on a design, plan, architecture decision, API contract, spec draft, or implementation approach.
   - Treat that design as the primary intended contract, even if it has not yet been written into `docs/`.
   - Reconstruct the design from the current conversation. If the design details are unclear, inspect available agent transcripts only for the relevant recent design discussion instead of reading them linearly.
   - Review the code for fit against that design, then check if the design needs documentation, SRD, use-case, test-suite, or config-format updates.
   - Proposed issues should separate "write down the design" from "make code conform to the design" when the design is not yet captured in repository docs.

2. **Repository design mode**
   - Use this when there is no recent chat design, the user asks for a general review, or the chat design is too vague to be a contract.
   - Treat the checked-in docs and specs as the primary intended contract.
   - Review code against VISION, ARCHITECTURE, SPECIFICATIONS, road-map, SRDs, use cases, test suites, semantic models, config formats, and constitutions.

If both modes apply, prefer the recent chat design for the specific area under discussion and use repository docs for surrounding constraints, terminology, quality gates, and issue format.

## Project context

For each subdirectory in scope, read where present. In recent chat design mode, read the files most relevant to the discussed design first, then broaden only as needed:

1. `<subdir>/docs/constitutions/issue-format.yaml`
2. `<subdir>/docs/constitutions/design.yaml`
3. `<subdir>/docs/constitutions/execution.yaml`
4. `<subdir>/docs/constitutions/go-style.yaml`
5. `<subdir>/docs/constitutions/testing.yaml`
6. `<subdir>/docs/VISION.yaml`
7. `<subdir>/docs/ARCHITECTURE.yaml`
8. `<subdir>/docs/SPECIFICATIONS.yaml`
9. `<subdir>/docs/road-map.yaml`
10. `<subdir>/docs/specs/software-requirements/`
11. `<subdir>/docs/specs/use-cases/`
12. `<subdir>/docs/specs/test-suites/`
13. Relevant source trees such as `cmd/`, `internal/`, `pkg/`, `tools/`, `workers/`, `configs/`, and `agents/`.

## Check current state

For each subdirectory in scope:

```bash
cd <subdir>
bd list
bd ready
mage -l
mage audit
mage stats
go build ./...
go test ./...
```

Run `bd` commands only for fileable directories. For non-fileable directories, do not run `bd list`, `bd ready`, `bd create`, or any `.beads/` mutation command.

If the project uses different mage target names, use the names shown by `mage -l` and state the substitution. If `audit` is absent but `analyze` is present, run `mage analyze` instead of `mage audit`. If `stats` is absent but `stats:loc` is present, run `mage stats:loc` instead of `mage stats`. Do not use shell pipelines that hide failing commands when the full command result matters.

## Review method

Use parallel readonly exploration where it helps. For broad codebase review, prefer `Subagent` with `subagent_type="explore"` instead of ad hoc search. Give each explorer a narrow brief and ask for file references, requirement references, severity, and suggested issue boundaries.

Run at least these review tracks for each scoped subdirectory:

0. **Active design fit review**
   - If recent chat design mode applies, compare the implementation against the design agreed in the chat.
   - Identify mismatched state ownership, API contracts, config fields, workflow sequencing, persistence, observability, tests, and rollout assumptions.
   - Identify which parts of the chat design need to be promoted into specs before code work.

1. **Design conformance review**
   - Compare implementation behavior against SRDs, use cases, test suites, config formats, semantic models, VISION, ARCHITECTURE, SPECIFICATIONS, and road-map.
   - Identify advertised-but-inert config fields, incomplete runtime paths, unclear ownership boundaries, and missing traceability.

2. **Requirements and test review**
   - Check if requirement acceptance criteria have matching tests or integration coverage.
   - Identify behavior that can pass unit tests while still failing the documented use case.

3. **Duplication review**
   - Look for duplicated code after two occurrences, especially validation, schema conversion, metrics, redaction, OpenAPI generation, filesystem operations, launch helpers, and test fixtures.
   - Prefer issues that consolidate one concern at a time.

4. **Constitution review**
   - Check execution, Go style, and testing rules.
   - Include file/function size limits, interface size, forbidden generic filenames, declarative-vs-hardcoded policy placement, test parallelism, fixture placement, and package boundaries.

## Summarize findings

For each subdirectory, report:

1. Discovery status: `fileable`, `non-fileable`, or `skipped`, with marker evidence.
2. Scope reviewed and quality gates run.
3. Current build, test, audit or analyze, and stats status.
4. Mage target substitutions used, such as `audit -> analyze` or `stats -> stats:loc`.
5. Design context used:
   - `recent chat design`: name the design or decision from the chat.
   - `repository design`: name the governing docs/specs.
   - `mixed`: name both and explain the boundary.
6. Findings grouped by severity:
   - P1: public contract, data loss, correctness, security, or behavior that docs/configs imply but runtime does not support.
   - P2: important maintainability, duplicated logic, missing test coverage for stable behavior, or constitution drift that increases change risk.
   - P3: cleanup, fixture hygiene, minor style drift, or optional follow-up.
7. Existing Beads issues that already cover a finding. Do not propose duplicates.
8. Open questions or assumptions.

## Propose work

Build a dependency-ordered list of issues grouped by subdirectory. Honor specification-driven development:

1. If a recent chat design is not documented, propose a documentation/spec issue before code issues unless the user explicitly wants exploratory code first.
2. If the docs/config contract is wrong or ambiguous, propose a documentation alignment issue before code issues.
3. If runtime behavior is missing for a documented or recently agreed requirement, propose a code issue tied to the exact requirement or chat design decision.
4. If tests are missing for required behavior, include test coverage in the same code issue unless the test harness itself needs separate design work.
5. If duplicate code increases implementation risk, propose a refactor issue after the behavior issues.

Only propose Beads issues for fileable directories. For non-fileable directories, report review findings under a separate "Non-fileable findings" section and state that no issues will be filed until a Beads tracker exists or the user explicitly asks to initialize one.

Use this proposal format:

```markdown
## agent-core/
1. [P1] Docs: reconcile <contract area> (agent-core)
   - Why: <one sentence>
   - Depends on: none
2. [P1] Code: implement <behavior> (agent-core)
   - Why: <one sentence>
   - Depends on: issue 1

## go-unix-utils/ (non-fileable)
- Finding: <one sentence>
- Filing status: no `.beads/`; do not create issues here.
```

Cross-directory dependencies are allowed, but Beads dependencies only work inside one queue. Note cross-directory dependencies in the child issue description.

## Issue description format

Follow the local `issue-format.yaml` schema:

```yaml
deliverable_type: code
required_reading:
  - docs/constitutions/design.yaml
  - docs/constitutions/execution.yaml
  - docs/constitutions/go-style.yaml
  - docs/constitutions/testing.yaml
  - docs/specs/software-requirements/<relevant-srd>.yaml
  - internal/<relevant-package>/
files:
  - path: internal/<relevant-package>/
    action: modify
requirements:
  - id: R1
    text: <specific requirement or recent design decision>
acceptance_criteria:
  - id: AC1
    text: <specific verification>
```

Every issue must include:

1. `deliverable_type`
2. `required_reading`
3. `files`
4. `requirements`
5. `acceptance_criteria`

For epics, use the same YAML shape and describe how child issues close the review findings.

## After the user agrees

Create issues only after the user explicitly agrees. Create issues with `bd create` inside the correct fileable subdirectory:

```bash
cd <subdir>
bd create "Epic: <short theme>" --type=epic --priority=P1 --labels code,documentation --description "$(cat <<'EOF'
...
EOF
)"
bd create "Docs: <short title>" --type=task --priority=P1 --labels documentation --parent <epic-id> --description "$(cat <<'EOF'
...
EOF
)"
bd create "Code: <short title>" --type=task --priority=P1 --labels code --parent <epic-id> --deps <same-queue-dependency-id> --description "$(cat <<'EOF'
...
EOF
)"
```

Use `--deps` only for dependencies inside the same Beads queue. Put cross-directory dependency notes in the YAML description. Do not run `bd create` in non-fileable directories.

Stage and commit in each subdirectory that has new issues:

```bash
cd <subdir>
git add .beads/
git commit -m "$(cat <<'EOF'
Plan: file <theme> review work

EOF
)"
```

Do not `git push`. The user pushes manually or uses `/make-release`.

## Hard rules

1. Do not modify source or docs while running this command unless the user explicitly asks to implement fixes.
2. Do not edit `.beads/` files by hand.
3. Do not create Beads issues until the user approves the proposed work.
4. Do not run Beads commands in active directories that lack `.beads/`.
5. Do not create duplicate issues for findings already covered by open Beads work.
6. Do not use `TodoWrite`; this repository uses Beads for task tracking.
7. Keep issue boundaries reviewable: one contract area, behavior path, or refactor concern per issue.
8. Prefer fewer high-signal issues over a long list of tiny cleanup tasks.
9. Do not assume a chat design exists. Detect it from the conversation; otherwise use repository design mode.
