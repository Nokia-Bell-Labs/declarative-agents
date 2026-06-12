# Command: Do Work

Pick one ready issue from the appropriate Beads queue and complete it end-to-end. This workspace contains multiple Go module subdirectories, each with its own Beads issue tracker and cobbler-scaffold setup.

## Multi-directory layout

This workspace is a parent directory containing multiple project subdirectories. Each subdirectory is an independent Git repository with its own Beads queue:

| Directory | Description | Beads prefix |
|-----------|-------------|--------------|
| `agent-core/` | Generic agentic loop engine (library) | `agent-core-` |
| `generator/` | Go coding agent specialization | `generator-` |

When the user says `/do-work`, scan **all** subdirectories for ready issues. When the user says `/do-work on <dir>` or `/do-work <issue-id>`, scope to that directory or the directory owning that issue prefix.

## 1. Select the target directory and issue

1. If the user specified a directory name (e.g., `/do-work on agent-core`), `cd` into that subdirectory and run `bd list` there.
2. If the user specified an issue ID (e.g., `/do-work abc123`), determine the directory from the issue prefix (e.g., `agent-core-abc` → `agent-core/`, `generator-abc` → `generator/`).
3. If neither was specified, run `bd list` in **each** subdirectory to find the highest-priority ready issue across all queues. Pick the one with the highest priority (P1 > P2 > P3).
4. **All subsequent `bd` commands and file operations must run inside the selected subdirectory.** Use `cd <subdir>` before any `bd` or `mage` command.

## 2. Classify the task

The issue's label (`code` or `documentation`) determines the procedure.

| If the issue… | …treat it as |
|---|---|
| Has label `documentation`, names an output path under `docs/`, names a doc type from `docs/constitutions/design.yaml`, or lists `required_fields` for a doc type | **Documentation** — follow §3-D, then §4-D |
| Has label `code`, names files under `cmd/`, `internal/`, `pkg/`, `workers/`, or other source directories, or references an SRD with specific R-items to implement | **Code** — follow §3-C, then §4-C |

If both signals are present, treat it as **code**.

**Recurring issue exception.** Do not select an issue with the `recurring` label unless the user explicitly named that issue ID or explicitly asked to run recurring work.

## 3. Read project context

Read the constitutions and specs **from the selected subdirectory**:

1. `<subdir>/docs/constitutions/design.yaml` — documentation format authority.
2. `<subdir>/docs/constitutions/execution.yaml` — code format authority.
3. `<subdir>/docs/constitutions/go-style.yaml` and `<subdir>/docs/constitutions/testing.yaml` — additional code rules.
4. `<subdir>/docs/VISION.yaml`, `<subdir>/docs/ARCHITECTURE.yaml`, `<subdir>/docs/SPECIFICATIONS.yaml`, `<subdir>/docs/road-map.yaml` — whichever exist.

Claim the issue:

```bash
cd <subdir> && bd update <issue-id> --claim
```

### 3-D. Documentation

1. From the issue and the design constitution, identify output path, doc type, required fields, naming convention, numbering rules.
2. Read every upstream artifact the issue references.
3. Re-read `documentation_standards` in design constitution.

### 3-C. Code

1. Read the SRD referenced by the issue. Note R-items, non_goals, acceptance criteria.
2. Read relevant sections of ARCHITECTURE.yaml.
3. Read the test suite YAML for the use case.
4. Read existing source in the packages you will touch.
5. Read the issue in full.

## 4. Do the work

### 4-D. Write the doc

Follow the writing style rules from the generator's do-work (short sentences, lists over prose, no filler, active voice, no em-dashes). Write to the exact output path inside the subdirectory. Validate with:

```bash
cd <subdir> && mage audit
```

### 4-C. Implement the code

1. Touch only the files listed in the issue.
2. Respect hard limits: functions ≤ 40 LOC, files ≤ 500 LOC, DRY threshold of two.
3. Validate in order:
   ```bash
   cd <subdir> && go build ./... && go vet ./... && mage lint && go test ./...
   ```
4. If you touched docs, also run `mage audit`.

## 5. De-AI quality gate (documentation issues only)

```bash
bash .cursor/skills/de-ai/scripts/detect-lexical.sh <output-file>
python3 .cursor/skills/de-ai/scripts/detect-structural.py <output-file>
```

Fix every hit. Re-run until clean (max 3 iterations).

## 6. Close and commit

1. Add stats comment and close the issue **in the subdirectory**:

   ```bash
   cd <subdir>
   bd comment <issue-id> "stats: ..."
   bd close <issue-id> --reason "..."
   ```

2. If the issue has the `recurring` label, re-create it before committing.

3. Stage and commit **in the subdirectory**:

   ```bash
   cd <subdir>
   git add -A
   git commit -m "$(cat <<'EOF'
   <commit message>
   EOF
   )"
   ```

4. **Do not `git push`.** The user pushes manually or uses `/make-release`.

## Hard rules

- All `bd` commands run inside the target subdirectory (`cd <subdir>` first).
- Never edit `.beads/` files by hand.
- Always commit `.beads/` changes alongside the work.
- Do not run `bd sync` — it does not exist in bd 1.x.
- Do not call `git push` automatically.
- For code: do not modify files outside the issue's file list (article E3).
- For code: respect function-size (40 LOC), file-size (500 LOC), DRY-threshold (two) limits.
- If `mage audit` fails on artifacts you did not touch, open a new issue with `bd create` and finish your current one.
