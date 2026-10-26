---
id: planes
title: The three planes
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Names the three layers the process separates, what each owns, how fast it changes, and the only things that cross between them. srd001 makes these enforceable as a label; this model is the vocabulary the other documents use when they say "management plane" or "data plane".

<!-- field: vocabulary -->
## Vocabulary

| plane | definition | timescale | owner | writes | never_writes | labels |
| --- | --- | --- | --- | --- | --- | --- |
| management_plane | The layer that sets intent: which releases exist, which initiatives, epics, and stories each serves, and the Sprint Goal each team receives. | once per sprint, three weeks by default | the technical program manager at system level, each Product Owner at subsystem level | road-map releases, themes, initiatives, epics, features, stories, system and subsystem SRDs, Sprint Goal issues, the sprint milestone, the sprint issue | any data-plane issue, label, or assignment | plane::management |
| control_plane | The layer that holds the rules an issue moves by: the label schema, the state machine, routing, admission, scheduling, escalation, the event vocabulary, and the gates. | rarely, by reviewed merge | the program lead, reviewed by leads | the policy files and the label schema | issue state; rules are read by scripts, not applied by hand | plane::control |
| data_plane | The layer that does the work: issues popped by the people who pull them, built in worktrees, merged, released, and handed downstream. | continuous; hours to days | whoever took the issue | every other issue, comment, merge, and tag | policy files, SRDs, or the road-map, except through a plane::control or plane::management issue | plane::data |

<!-- field: data_flow -->
## Data flow

| crossing | description |
| --- | --- |
| intent_down | The management plane hands the data plane a milestone, a sprint issue, and each team's Sprint Goal with the stories it lists. Nothing else. |
| rules_down | The control plane hands the data plane its policy files, read by scripts at run time. |
| state_up | The data plane hands the other planes tracker state: issues, labels, comments, merges, tags. The reporting scripts read it; nobody sends status any other way. |

<!-- field: invariants -->
## Invariants

- headless_data_plane: The data plane keeps running on the last pushed sprint and the last committed policy when the management plane does nothing.
- separate_timescales: Intent changes per sprint, rules rarely, work continuously. Keeping them apart stops planning from thrashing the work.
