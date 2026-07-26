# Agent Instructions

## Repository Purpose

`applications/catalog` is this repository's versioned catalog of reusable
declarative tool and agent blocks, not a universal library or a general home for
every YAML agent. A family belongs under `agents/` only when it is independently
useful or has multiple consumers in this repository. Application-internal
composition belongs under `applications/<application>/`.

Maintain one canonical home per agent. Applications reference catalog programs
and may supply wrappers/configuration, but must not fork reusable machines or
declarations. Catalog members require a profile family, SRD, conformance
coverage, portable closed references, and sufficient parameterization for
consumers to configure them without edits. Runtime implementation remains in
`agent-core`.

Treat `tools.yaml` as a name-only selection list, not a declaration block.
Reusable repository-specific ToolDef declarations are first-class catalog tool
blocks under their owning `agents/<family>/` closure. Profile-local overrides
remain in that closure. Generic Go implementations stay under
`agent-core/internal/tools`, and core shared declarations stay under
`agent-core/tools`. The complete contract is in `tools/README.md`.

The canonical compatibility surface is versioned by
`applications/catalog/v0.*`; matching `agent-profiles/v0.*` tags remain legacy
v0 compatibility identifiers. Treat path,
machine/tool/signal/terminal contracts, request shapes, configuration names, and
closure membership as compatibility-sensitive. Record breaking migrations and
update consumers in the coordinated release.

Assembler and mock are supported test-time catalog members under `agents/`; the
existing conformance vocabulary continues to classify them as supported
test-time library members. Preserve their SRD, conformance, portability,
release, and v0 compatibility contracts; do not repurpose them as application
roles or fork them. Rig-subject and `testdata/conformance`
REST/control/lifecycle fixtures remain internal test data in their current
paths and are not scheduled for relocation.

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

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
