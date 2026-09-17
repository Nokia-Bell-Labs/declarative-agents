<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Fragments and templates

Fragments are the smallest reuse unit: a parameterized YAML unit that a machine or declaration file instantiates with arguments. The mechanism lives in `agent-core/internal/fragments` (srd052) and substitutes `$param(name)` references at load time, so the instantiated closure remains plain YAML that `--dump-config` shows in full.

## Machine templates

A `machine.yaml` may carry `unit:` and `instantiate:` and nothing else. `applications/catalog/agents/runtime-state-reader/machine.yaml` is the shortest example — 14 lines instantiating `monitor-service-machine-template.yaml` with the profile's word names as arguments. Shipped templates live in `agent-core/tools/machines/`; applications keep local ones in `agents/units/`. Machines may instantiate but may not import: the `imports:` field is rejected with an explanatory error, which keeps every machine's full state space visible at its instantiation site.

## Declaration units

Tool declaration files (`catalog.ToolDefsFile`) compose with `unit:`, `imports:`, `params:`, `instantiate:`, and `override: true` for replacing a tool from an imported unit. `applications/chatbot-mesh/agents/chatbot/declarations.yaml` instantiates the mesh monitor fragment in two lines where other agents inline the same two tool definitions at about 120 lines.

The table lists the shared units that exist today and the ones the capability-profiles epic adds.

| Unit | Location | Status |
|---|---|---|
| monitor service machine template | `agent-core/tools/machines/monitor-service-machine-template.yaml` | shipped |
| serve machine template | `agent-core/tools/machines/serve-machine-template.yaml` | shipped |
| lifecycle approval machine template | `agent-core/tools/machines/lifecycle-approval-machine-template.yaml` | shipped |
| mesh monitor fragment (launch/stop pair) | `applications/chatbot-mesh/agents/units/mesh-monitor-fragment.yaml` | shipped |
| monitor control fragment (launch/await/stop trio) | `applications/catalog/agents/units/monitor-control-fragment.yaml` | shipped |
| serve-lifecycle declarations fragment | `agent-core/tools/units/serve-lifecycle-declarations-fragment.yaml` | shipped (GH-2166) |
| monitor/control REST servers fragment | `agent-core/tools/units/` | planned (GH-2167); the loader support it needs already ships |

## REST definitions

`rest.yaml` composes exactly as a declarations file does: `unit:`, `imports:`, and `instantiate:` are implemented in the REST definition loader and enforce the srd052 rules — a fragment is instantiated and never imported plainly, and nesting stops at one level. What is missing is not the mechanism but its use: no `rest.yaml` in the ecosystem instantiates anything, while 26 of them carry the same control server, monitor routes, and limits blocks by hand. GH-2167 adds the canonical fragment and converts the monorepo carriers.

## When to make a fragment

We cut a fragment when the same block appears in a third place, or in a second repository. The reuse statistics (`mage stats:reuse`, [declaration-statistics.md](declaration-statistics.md)) surface the candidates: duplication and ceremony scores identify blocks that repeat, and the files-per-change table identifies wiring that a fragment would collapse to one instantiate line. A fragment takes parameters for what varies and nothing else; a fragment with many parameters is usually two fragments.
