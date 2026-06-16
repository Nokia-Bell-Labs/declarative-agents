# Command: Make Work

Propose the next batch of work and file it as Beads issues in the appropriate fileable subdirectory queue. This workspace may contain Go module subdirectories with and without Beads trackers.

## Workspace discovery

When the user says `/make-work`, discover active project directories from the workspace root. Do not use a fixed directory list.

Scan only direct child directories of the workspace root. A directory is active when it has:

1. `go.mod`
2. `docs/`
3. source or configuration roots such as `cmd/`, `internal/`, `pkg/`, `tools/`, `agents/`, or `magefiles/`

A directory is fileable when it is active and also has `.beads/`.

When the user says `/make-work on <dir>`, scope to that directory if it exists. Explicit scope may include a deprecated directory, but the command must say it is deprecated before reviewing it.

Skip deprecated or archived directories by default. This includes directories named `DEPRICATED`, `DEPRECATED`, `deprecated`, `archive`, or paths below those directories. Do not infer active queues from samples, fixtures, experiments, generated worktrees, or nested `go.mod` files.

Report every discovered directory before proposing work:

- `fileable`: active and has `.beads/`; propose Beads issues after review.
- `non-fileable`: active but lacks `.beads/`; report possible work but do not try to file Beads issues there.
- `skipped`: deprecated, missing, fixture, generated, or missing required markers.

Current dry-run expectation for this workspace:

- `agent-core/`: fileable because it has `go.mod`, `docs/`, source roots, and `.beads/`.
- `go-unix-utils/`: non-fileable because it has `go.mod`, `docs/`, and source roots, but no `.beads/`.
- `DEPRICATED/generator/`: skipped by default because it is under a deprecated directory.

## Project context (read these first)

For each fileable or explicitly reviewed non-fileable subdirectory, read where present:

1. `<subdir>/docs/constitutions/issue-format.yaml` -- YAML schema for issue descriptions.
2. `<subdir>/docs/constitutions/design.yaml` -- documentation format authority.
3. `<subdir>/docs/constitutions/execution.yaml` -- code format authority.
4. `<subdir>/docs/constitutions/go-style.yaml` and `<subdir>/docs/constitutions/testing.yaml` -- additional code rules.
5. `<subdir>/docs/VISION.yaml`, `<subdir>/docs/ARCHITECTURE.yaml`, `<subdir>/docs/SPECIFICATIONS.yaml`, `<subdir>/docs/road-map.yaml`.
6. `<subdir>/docs/specs/software-requirements/`, `<subdir>/docs/specs/use-cases/`, `<subdir>/docs/specs/test-suites/`.
7. The source tree (`cmd/`, `internal/`, `pkg/`, etc.).

## Check current state

For each fileable subdirectory in scope:

```bash
cd <subdir>
bd list                          # all issues
bd ready                         # claimable issues
mage audit                       # consistency check
mage stats                       # LOC and doc counts
go build ./... 2>&1 | head -50   # build status
go test ./... 2>&1 | tail -30    # test status
```

For non-fileable directories, do not run `bd list`, `bd ready`, `bd create`, or any `.beads/` mutation command. Run only non-Beads checks that fit the project.

Run `mage -l` before mage checks. If `audit` is absent but `analyze` is present, run `mage analyze` instead of `mage audit`. If `stats` is absent but `stats:loc` is present, run `mage stats:loc` instead of `mage stats`. State every substitution in the summary.

## Summarize

For each subdirectory:

1. Discovery status: `fileable`, `non-fileable`, or `skipped`, with marker evidence.
2. Project purpose from `VISION.yaml`.
3. High-level architecture (from ARCHITECTURE.yaml).
4. Spec coverage: SRDs, use cases, test suites; gaps from `mage audit` or substituted `mage analyze`.
5. Code coverage: packages, LOC, build/test status.
6. Mage target substitutions used, such as `audit -> analyze` or `stats -> stats:loc`.

## Propose work

Build a dependency-ordered list of new issues **grouped by subdirectory**. Honor the design constitution article D1 (specification-driven development).

Propose Beads issues only for fileable directories. For non-fileable directories, use a "Non-fileable findings" section and state that no issues will be filed until a Beads tracker exists or the user explicitly asks to initialize one.

When proposing issues, clearly state which subdirectory each issue belongs to:

```
## agent-core/
1. [P1] ... (agent-core)
2. [P2] ... (agent-core)

## go-unix-utils/ (non-fileable)
- Finding: <one sentence>
- Filing status: no `.beads/`; do not create issues here.
```

Cross-directory dependencies are allowed and should be noted in the affected issue descriptions.

### Issue description format

Follow the same YAML description format as before (deliverable_type, required_reading, files, requirements, acceptance_criteria).

## After the user agrees

Create each issue with `bd create` only after user approval and only in the correct fileable subdirectory:

```bash
cd <subdir>
bd create "Title" --description "$(cat <<'EOF'
...
EOF
)" --labels <code|documentation>
```

Use `--deps` to encode ordering within the same Beads queue. For cross-directory dependencies, note them in the issue description since `--deps` only works within one queue. Do not run `bd create` in non-fileable directories.

Stage and commit **in each subdirectory that has new issues**:

```bash
cd <subdir>
git add .beads/
git commit -m "Plan: <short description>"
```

**Do not `git push`.** The user pushes manually or uses `/make-release`.
