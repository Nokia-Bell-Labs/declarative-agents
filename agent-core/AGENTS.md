<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Agent Instructions

## Documentation authority

YAML under `docs/` and `docs/constitutions/` in this repository is the source of truth for agent-core behavior contracts. When workspace-wide editor rules or other guidance disagrees with that corpus, follow the repository docs.

Place a new document at the path for its type. Types and locations are declared in `docs/constitutions/design.yaml`. Required fields live there; do not copy them here.

| Type | Location |
|------|----------|
| vision | `docs/VISION.yaml` |
| architecture | `docs/ARCHITECTURE.yaml` |
| specifications | `docs/SPECIFICATIONS.yaml` |
| roadmap | `docs/road-map.yaml` |
| pattern_language | `docs/pattern-language.yaml` |
| engineering_guideline | `docs/engineering/engNN-short-name.yaml` |
| srd | `docs/specs/software-requirements/srdNNN-short-name.yaml` |
| use_case | `docs/specs/use-cases/relNN.N-ucNNN-short-name.yaml` |
| test_suite | `docs/specs/test-suites/test-relNN.N-short-name.yaml` |
| audit_register | `docs/specs/audits/name.yaml` |
| audit_report | `docs/specs/audits/name.md` |
| semantic_model | `docs/specs/semantic-models/name.yaml` |
| config_format | `docs/specs/config-formats/name.yaml` |
| constitution | `docs/constitutions/name.yaml` |
| migration | `docs/migrations/name.yaml` |
| guide | `docs/guides/name.md` |

Do not create a document that matches no declared type. Register the type in `docs/constitutions/design.yaml` first, or do not add the file. `mage audit` enforces this.

## Package layout

`cmd` packages hold entry points and adapter wiring only, per `docs/constitutions/go-style.yaml`. Runtime logic belongs in `internal` packages.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

### Must-follow rules
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
