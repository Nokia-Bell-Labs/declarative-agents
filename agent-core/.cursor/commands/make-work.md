# Command: Make Work

Propose the next batch of work and file it as Beads issues in the appropriate subdirectory queue. This workspace contains multiple Go module subdirectories, each with its own Beads tracker.

## Multi-directory layout

| Directory | Description | Beads prefix |
|-----------|-------------|--------------|
| `agent-core/` | Generic agentic loop engine (library) | `agent-core-` |
| `generator/` | Go coding agent specialization | `generator-` |

When the user says `/make-work`, assess **all** subdirectories and propose work across the entire workspace. When the user says `/make-work on <dir>`, scope to that directory only.

## Project context (read these first)

For **each** subdirectory in scope, read (where they exist):

1. `<subdir>/docs/constitutions/issue-format.yaml` — YAML schema for issue descriptions.
2. `<subdir>/docs/constitutions/design.yaml` — documentation format authority.
3. `<subdir>/docs/constitutions/execution.yaml` — code format authority.
4. `<subdir>/docs/constitutions/go-style.yaml` and `<subdir>/docs/constitutions/testing.yaml` — additional code rules.
5. `<subdir>/docs/VISION.yaml`, `<subdir>/docs/ARCHITECTURE.yaml`, `<subdir>/docs/SPECIFICATIONS.yaml`, `<subdir>/docs/road-map.yaml`.
6. `<subdir>/docs/specs/software-requirements/`, `<subdir>/docs/specs/use-cases/`, `<subdir>/docs/specs/test-suites/`.
7. The source tree (`cmd/`, `internal/`, `pkg/`, etc.).

## Check current state

For **each** subdirectory in scope:

```bash
cd <subdir>
bd list                          # all issues
bd ready                         # claimable issues
mage analyze                     # consistency check
mage stats:loc                   # LOC and doc counts
go build ./... 2>&1 | head -50   # build status
go test ./... 2>&1 | tail -30    # test status
```

## Summarize

For each subdirectory:

1. What this project is for (from VISION.yaml).
2. High-level architecture (from ARCHITECTURE.yaml).
3. Spec coverage — SRDs, use cases, test suites; gaps from `mage analyze`.
4. Code coverage — packages, LOC, build/test status.

## Propose work

Build a dependency-ordered list of new issues **grouped by subdirectory**. Honor the design constitution article D1 (specification-driven development).

When proposing issues, clearly state which subdirectory each issue belongs to:

```
## agent-core/
1. [P1] ... (agent-core)
2. [P2] ... (agent-core)

## generator/
3. [P1] ... (generator) — depends on agent-core issue #1
4. [P2] ... (generator)
```

Cross-directory dependencies are allowed and should be noted (e.g., "generator cannot replace X until agent-core exports Y").

### Issue description format

Follow the same YAML description format as before (deliverable_type, required_reading, files, requirements, acceptance_criteria).

## After the user agrees

Create each issue with `bd create` **in the correct subdirectory**:

```bash
cd <subdir>
bd create "Title" --description "$(cat <<'EOF'
...
EOF
)" --labels <code|documentation>
```

Use `--deps` to encode ordering within the same Beads queue. For cross-directory dependencies, note them in the issue description since `--deps` only works within one queue.

Stage and commit **in each subdirectory that has new issues**:

```bash
cd <subdir>
git add .beads/
git commit -m "Plan: <short description>"
```

**Do not `git push`.** The user pushes manually or uses `/make-release`.
