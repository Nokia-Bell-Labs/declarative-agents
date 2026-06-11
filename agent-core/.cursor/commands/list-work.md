# Command: List Work

List the current Beads work queue across all subdirectories.

## Multi-directory layout

This workspace contains multiple Go module subdirectories, each with its own Beads queue:

| Directory | Beads prefix |
|-----------|--------------|
| `agent-core/` | `agent-core-` |
| `generator/` | `generator-` |

## How to list work

1. For **each** subdirectory with a `.beads/` directory, run:
   ```bash
   cd <subdir> && bd list
   ```
2. If the user specified a directory (e.g., `/list-work agent-core`), show only that queue.
3. If the user specified an issue ID, use `bd show <issue-id>` in the appropriate subdirectory.

## Response format

Group by subdirectory:

```
## agent-core/
  (no open issues)

## generator/
  ○ generator-06a4c400 [P2] Update generator specs ...
  ○ generator-26c46812 [P2] Review specs ... (recurring)
  ◐ generator-28bc7263 [P2] Run benchmark suite (recurring)
```

- Ready issues first within each group.
- Include ID, priority, title, and status markers (recurring, blocked, etc.).
- Keep it short.

## Hard rules

- Do not edit `.beads/` files by hand.
- Do not create, update, close, or commit issues.
- Do not run `git push`.
