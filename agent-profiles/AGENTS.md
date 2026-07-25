# Agent Instructions

## Repository Purpose

`agent-profiles` is a versioned library of reusable declarative agents, not a
general home for every YAML agent. A family belongs under `agents/` only when it
is independently useful or has multiple consumers. Application-internal
composition belongs under `examples/<application>/`.

Maintain one canonical home per agent. Applications reference library programs
and may supply wrappers/configuration, but must not fork reusable machines or
declarations. Library members require a profile family, SRD, conformance
coverage, portable closed references, and sufficient parameterization for
consumers to configure them without edits. Runtime implementation remains in
`agent-core`.

The public surface is versioned by `agent-profiles/v0.*` tags. Treat path,
machine/tool/signal/terminal contracts, request shapes, configuration names, and
closure membership as compatibility-sensitive. Record breaking migrations and
update consumers in the coordinated release.

Mock/assembler/rig and `testdata/conformance` runtime scaffolding are pending
relocation to `agent-core`; do not promote them as application library members
or broaden their contract while they remain here.

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
