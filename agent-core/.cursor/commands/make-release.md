# Command: Make Release

Tag and push all subdirectories that have unpushed commits. Each subdirectory is an independent Git repository with its own tags.

## Procedure

### 1. Discover subdirectories

Find all immediate subdirectories that contain a `.git/` directory:

```bash
for dir in */; do
  [ -d "$dir/.git" ] && echo "$dir"
done
```

### 2. For each repository

For each Git repository subdirectory, in order:

#### a. Check for unpushed commits

```bash
cd <subdir>
git status
git log --oneline origin/main..HEAD 2>/dev/null
```

If there are no unpushed commits, skip this subdirectory.

#### b. Push commits

```bash
cd <subdir>
git push origin main
```

If the push fails (e.g., remote is unreachable), report the error and continue to the next subdirectory. Do not abort the entire operation.

#### c. Create a release tag

Run `mage tag` if the subdirectory has a `magefiles/` directory with the Tag target:

```bash
cd <subdir>
mage tag
```

This creates a documentation release tag in the format `v0.YYYYMMDD.N` (from cobbler-scaffold's Releaser). The revision number auto-increments for multiple tags on the same day.

#### d. Push the tag

```bash
cd <subdir>
git push origin --tags
```

### 3. Summary

After processing all subdirectories, print a summary:

```
Tag and push summary:
  agent-core/  — pushed 3 commits, tagged v0.20260601.0
  generator/   — pushed 1 commit, tagged v0.20260601.0
```

If any subdirectory was skipped (no unpushed commits) or failed (push error), include that in the summary.

## Options

- `/make-release` — process all subdirectories
- `/make-release <dir>` — process only the named subdirectory
- `/make-release --no-tag` — push without tagging
- `/make-release --dry-run` — show what would happen without executing

## Hard rules

- Only push to `main` (or the branch shown by `git branch --show-current`).
- Never force-push.
- If `mage tag` fails (e.g., not on main branch), report the error and continue. The push still happens.
- Report each subdirectory's result individually so partial failures are visible.
