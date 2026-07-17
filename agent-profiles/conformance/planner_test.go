// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"path/filepath"
	"testing"
)

// plannerConformanceMachine bounds the planner pipeline to its graph-loading
// boundary: load_graph builds the requirement graph, extract_all traverses it,
// and the run reaches a terminal without the heavy plan/execute/vet/build/test
// tail (which needs an LLM and a child generator). This mirrors the bounded
// ephemeral machine the generator conformance test uses to isolate one boundary.
const plannerConformanceMachine = `name: planner-conformance
initial_state: Idle
states:
- name: Idle
  meaning: Initial state before planning begins.
- name: Loading
  meaning: Loading the specification corpus and building the requirement graph.
- name: Extracting
  meaning: Selecting the ready task group from the requirement graph.
- name: Completed
  meaning: Terminal. The graph was loaded and traversed.
- name: Stalled
  meaning: Terminal. Remaining tasks are blocked.
- name: Failed
  meaning: Terminal. An unrecoverable error occurred.
terminal_states:
- Completed
- Stalled
- Failed
signals:
- name: Seed
  trigger: Loop initialization.
- name: GraphLoaded
  trigger: The requirement graph was loaded into pipeline state.
- name: TaskExtracted
  trigger: A task was extracted from the requirement graph.
- name: AllDone
  trigger: All tasks in the graph are done.
- name: Blocked
  trigger: Remaining tasks are blocked.
- name: CommandError
  trigger: Infrastructure-level error.
transitions:
- state: Idle
  signal: Seed
  next: Loading
  action: load_graph
- state: Loading
  signal: GraphLoaded
  next: Extracting
  action: extract_all
- state: Loading
  signal: CommandError
  next: Failed
- state: Extracting
  signal: TaskExtracted
  next: Completed
- state: Extracting
  signal: AllDone
  next: Completed
- state: Extracting
  signal: Blocked
  next: Stalled
- state: Extracting
  signal: CommandError
  next: Failed
`

// TestPlannerConformance exercises the planner pipeline's graph-loading boundary.
// The planner's first machine action is load_graph, which reads the
// specification corpus from the run directory and builds the requirement graph
// into pipeline state; extract_all then traverses that graph. Before load_graph
// existed, the planner machine jumped straight to extraction against a nil graph
// and panicked (agent-core graph-loading gap). This test proves the boundary is
// wired: load_graph and extract_all appear as tool spans, a
// pipeline.graph_loaded event records the built graph, and the run reaches a
// clean Completed terminal with no error-status spans.
//
// It is model-free and child-free: the corpus is agent-core's own valid spec
// fixture, and the bounded machine terminates after extraction rather than
// invoking the LLM plan and generator execution tail.
//
// Traces srd004-planner: AC1 (load_graph/extract_all as the pipeline's
// graph-boundary actions), AC2 (the planning tool family), and AC3 (a clean
// terminal outcome).
func TestPlannerConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	tmp := t.TempDir()

	machinePath := writeEphemeral(t, tmp, "machine.yaml", plannerConformanceMachine)
	toolsPath := writeEphemeral(t, tmp, "tools.yaml", "tools:\n  - load_graph\n  - extract_all\n")
	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: planner-conformance
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
`, machinePath, toolsPath, filepath.Join(coreRoot, "tools", "builtin", "planner")))

	// The valid spec corpus that ships with agent-core builds a real requirement
	// graph with ready nodes, so load_graph and extract_all have something to
	// traverse without any external inputs.
	corpus := filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid")

	result := Run(t, RunConfig{Profile: profilePath, Directory: corpus})

	// srd004 AC3: a clean terminal outcome with a single root and no error spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd004 AC1/AC2: the graph-boundary actions are visible as tool spans.
	result.RequireToolSpans(t, "load_graph", "extract_all")

	// srd004 AC1: load_graph seeded the requirement graph into pipeline state.
	if _, _, ok := result.Spans.FindEvent("pipeline.graph_loaded"); !ok {
		t.Fatalf("no pipeline.graph_loaded event; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}

	// srd004 AC3: the bounded run reaches the Completed terminal state.
	result.RequireTerminalState(t, "Completed")
}
