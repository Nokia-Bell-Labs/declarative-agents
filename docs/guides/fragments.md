<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Fragments and templates

Fragments are the smallest reuse unit: a parameterized YAML unit that a machine or declaration file instantiates with arguments. The mechanism lives in `agent-core/internal/fragments` (srd052) and substitutes `$param(name)` references at load time, so the instantiated closure remains plain YAML that `--dump-config` shows in full.

## Machine templates

A `machine.yaml` may carry `unit:` and `instantiate:` and nothing else. `applications/catalog/agents/runtime-state-reader/machine.yaml` is the shortest example — 14 lines instantiating `monitor-service-machine-template.yaml` with the profile's word names as arguments. Shipped templates live in `agent-core/tools/machines/`; applications keep local ones in `agents/units/`. Machines may instantiate but may not import: the `imports:` field is rejected with an explanatory error, which keeps every machine's full state space visible at its instantiation site.

## Declaration units

Tool declaration files (`catalog.ToolDefsFile`) compose with `unit:`, `imports:`, `params:`, `instantiate:`, and `override: true` for replacing a tool from an imported unit. `applications/chatbot-mesh/agents/chatbot/declarations.yaml` is one import and one instantiation: the monitor pair arrives by importing a unit, because it is the same in every agent, and the four lifecycle words by instantiating a fragment, because their names vary. The file carries no tool definition of its own.

The table lists the shared units that exist today and the ones the capability-profiles epic adds.

| Unit | Location | Status |
|---|---|---|
| monitor service machine template | `agent-core/tools/machines/monitor-service-machine-template.yaml` | shipped |
| serve machine template | `agent-core/tools/machines/serve-machine-template.yaml` | shipped |
| lifecycle approval machine template | `agent-core/tools/machines/lifecycle-approval-machine-template.yaml` | shipped |
| monitor REST pair (launch/stop, imported) | `agent-core/tools/units/monitor-rest-declarations.yaml` | shipped (GH-2180) |
| monitor control fragment (launch/await/stop trio) | `applications/catalog/agents/units/monitor-control-fragment.yaml` | shipped |
| serve-lifecycle declarations fragment | `agent-core/tools/units/serve-lifecycle-declarations-fragment.yaml` | shipped (GH-2166) |
| monitor server fragment | `agent-core/tools/rest/units/monitor-server-fragment.yaml` | shipped (GH-2167) |

## REST definitions

`rest.yaml` composes exactly as a declarations file does: `unit:`, `imports:` and `instantiate:` are implemented in the REST definition loader and enforce the srd052 rules. The canonical monitor server ships at `agent-core/tools/rest/units/monitor-server-fragment.yaml`, and an agent instantiates it with its address, limits profile and queue name.

Two rules shape where that instantiation goes. An instantiation none of whose produced definitions a closure selects is an unused import (srd052 R3.1), so a server cannot be instantiated in a `rest.yaml` that a request profile also loads — the request machine launches no monitor server. The instantiation therefore lives in a `monitor-rest.yaml` that only the agent's own profile lists. And a mapping name is never substituted (R2.3), so the fragment produces a server under a fixed name; an agent whose server takes another name instantiates it under `as`, which prefixes every produced name including the endpoints.

## When to make a fragment

We cut a fragment when the same block appears in a third place, or in a second repository. The reuse statistics (`mage stats:reuse`, [declaration-statistics.md](declaration-statistics.md)) surface the candidates: duplication and ceremony scores identify blocks that repeat, and the files-per-change table identifies wiring that a fragment would collapse to one instantiate line. A fragment takes parameters for what varies and nothing else; a fragment with many parameters is usually two fragments.

Where a unit lives, what earns it a shared location, and which reuse form fits a given repetition are conventions, not mechanics: [eng03-declaration-standard-library](../engineering/eng03-declaration-standard-library.yaml) states them.
