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
| `pkg/cli` | Common CLI flag definitions for agent binaries |
| `pkg/tracing` | Tracer port interface (5 methods) decoupling engine from OTel internals |
| `pkg/telemetry` | Concrete OTel implementation: providers, exporters, trace adapter, replay |
| `pkg/spec` | Specification graph loader and cross-artifact validator |

## Quick Start

```go
package main

import (
    "context"
    "time"

    "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
    "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

func main() {
    reg := core.NewRegistry()
    reg.Register(core.ToolSpec{Name: "my_tool"}, myBuilder{})

    table := core.TransitionTable{
        {State: "init", Signal: core.Seed}:     {Next: "acting", Action: buildAction},
        {State: "acting", Signal: core.ToolDone}: {Next: "done", Action: nil},
    }

    result, err := core.Loop(core.LoopParams{
        InitialState:   "init",
        Registry:       reg,
        Table:          table,
        IsTerminal:     func(s core.State) bool { return s == "done" },
        Trace:          tracing.NoopTracer{},
        Budget:         core.Budget{MaxIterations: 50, MaxDuration: 5 * time.Minute},
        CommandTimeout:  30 * time.Second,
    }, context.Background())

    _ = result
    _ = err
}
```

## Installation

```bash
go get gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core
```

## License

Copyright (c) 2026 Nokia. All rights reserved.
