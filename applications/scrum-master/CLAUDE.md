<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->
# scrum-master

## Workflow

All work goes through GitHub issues and pull requests: file an issue, work it on a branch (`gh-<number>-<slug>`), and finish with the pull request that closes it. Never commit to `main` directly. Beads is the product's tracker backend (srd042, internal/beads), not this repository's tracker.

## Local conventions

- `.claude` is a gitignored symlink to shared local tooling; keep it out of tracking (`.git/info/exclude`).
- `.regression/` holds the regression rig's runs and stays untracked.
