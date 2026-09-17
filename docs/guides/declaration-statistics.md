<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Declaration statistics

`mage stats:reuse` measures declaration duplication, ceremony, unit sharing, and maintainability for every stats participant and folds them into `reuse_total`. The maintainability block reports file length (median, 90th percentile, maximum, files over 300 and 500 lines, the ten longest files), files per agent, and import fan-out and fan-in (GH-2111). Medians are lower medians of the observed values, and the repository total is computed over every module's samples rather than from module medians.

The JSON files in `docs/stats` record the numbers at a named commit so a factoring change can state what it moved.

| File | Records |
|---|---|
| `reuse-baseline.json` | duplication and ceremony |
| `reuse-units-baseline.json` | unit sharing and the maintainability block |

## Files a recurring change touches

Counting files per change from git history mostly measures migrations, so we list the recurring changes and the files each touches today instead. We update a row when a mechanism changes its count. The counts come from the survey in GH-2111.

| Change | Files today |
|---|---|
| Add a provider to the chatbot | declarations, machine, REST, two test copies, chart values |
| Add a knowledge source | topology declarations, REST client, chart |
| Add a tool to an agent | declarations, tool selection, machine transition |

The target for the first row, once provider wiring is a fragment, is one new fragment file plus one instantiate line.
