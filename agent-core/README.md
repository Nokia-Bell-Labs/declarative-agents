# agent-core

A Go framework for building tool-augmented agentic loops.

## What It Provides

agent-core gives you the machinery every agentic system needs: a state machine,
command dispatch with tracing and panic recovery, a tool registry, budget
enforcement, LLM integration, prompt assembly, and a standard tool library.
Domain agents import agent-core and supply their own states, signals, tools, and
transition tables.

## Packages

| Package | Description |
|---------|-------------|
| `pkg/core` | State machine, command dispatch, tool registry, agentic loop, YAML machine config |
| `pkg/llm` | LLM client interface, conversation management, message types, model profiles |
| `pkg/llm/ollama` | Ollama adapter satisfying `llm.Client` |
| `pkg/prompt` | Prompt loading from YAML, system template rendering, manifest serialization |
| `pkg/stl` | Standard tool library: file tools, build tools, LLM commands, subprocess, process groups |
| `pkg/spec` | Specification graph loader and cross-artifact validator |

Private implementation packages are grouped under `internal/`. See
`package-layout.md` for the migration map and ownership rules. Current internal
domains include `internal/observability` for tracing and telemetry, and
`internal/support` for process, workspace, and CLI helper code.

## Lifecycle Operations

Checkpointing, suspend/resume, approval gates, history, and rollback are
opt-in lifecycle features. See `lifecycle-rollback.md` for the operator guide,
including `--state-store-dir`, `--resume-checkpoint`, `agent history`,
`agent rollback`, the three-layer state model, and safety rules for irreversible
tools and workspace restore.

## Quick Start

```bash
mage build
bin/agent --profile agents/generator/profile.yaml --directory "$PWD"
```

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
