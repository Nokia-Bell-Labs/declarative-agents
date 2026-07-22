// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// plannerModel is the model the planner LLM declaration configures
// (agents/planner/llm/default.yaml). The behavioral shipped-profile run gates on
// this model being served by Ollama.
const plannerModel = "qwen3.6:35b-mlx"

// machineTransition is one transition row of a shipped machine.yaml, enough to
// assert the wiring the conformance tests care about.
type machineTransition struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
}

// TestPlannerShippedProfileWiring asserts, model-free and ungated, that the
// wrapper an operator ships — agents/planner/profile.yaml with its machine.yaml
// — wires the requirement-graph boundary: the profile references its own
// machine and tools, the machine seeds from Idle with load_graph, and the loaded
// graph hands off to extraction. This is the load + machine-wiring proof for the
// shipped wrapper; unlike the behavioral run below it needs no model, so it runs
// in the fast default and holds even where Ollama is absent.
//
// Traces srd004-planner AC1 (load_graph as the pipeline's graph-boundary action).
func TestPlannerShippedProfileWiring(t *testing.T) {
	var profile struct {
		Machine string   `yaml:"machine"`
		Tools   []string `yaml:"tools"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "profile.yaml"), &profile)

	if profile.Machine != "machine.yaml" {
		t.Errorf("shipped planner profile machine = %q, want machine.yaml", profile.Machine)
	}
	if !contains(profile.Tools, "tools.yaml") {
		t.Errorf("shipped planner profile tools = %v, want to include tools.yaml", profile.Tools)
	}

	var machine struct {
		InitialState string              `yaml:"initial_state"`
		Transitions  []machineTransition `yaml:"transitions"`
	}
	unmarshalShipped(t, filepath.Join("agents", "planner", "machine.yaml"), &machine)

	if machine.InitialState != "Idle" {
		t.Errorf("shipped planner machine initial_state = %q, want Idle", machine.InitialState)
	}
	// The graph-loading boundary: Idle seeds load_graph, and the loaded graph
	// hands off to task extraction.
	requireTransition(t, machine.Transitions, "Idle", "Seed", "Loading", "load_graph")
	requireTransition(t, machine.Transitions, "Loading", "GraphLoaded", "Extracting", "extract_task")
}

// TestPlannerConformance runs the shipped planner profile against agent-core's
// valid spec fixture and asserts the requirement-graph boundary from the trace:
// load_graph reads the corpus and builds the requirement graph into pipeline
// state (the pipeline.graph_loaded event), the boundary that the #211 nil-graph
// gap was about. This runs the wrapper an operator ships — not a synthesized,
// bounded machine — so it exercises the real planner tool declarations.
//
// It is Ollama-gated: the shipped planner machine declares invoke_llm, which
// pings Ollama at tool registration, so with no reachable model the profile
// cannot start (see ollama.go). The full pipeline tail beyond the graph boundary
// (assemble_prompt -> invoke_llm -> parse_plan -> create_issue via beads ->
// execute_task via a generator child -> vet/build/test) needs a beads project,
// a child agent, and the Go toolchain, which the conformance harness
// deliberately does not provide; the shipped planner is therefore behaviorally
// exercised to its requirement-graph boundary here and no further, so no clean
// terminal is asserted. The remaining boundary wiring is proven ungated by
// TestPlannerShippedProfileWiring.
//
// Traces srd004-planner: AC1 (load_graph as the graph-boundary action) and AC2
// (the requirement graph is built into pipeline state).
func TestPlannerConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	RequireOllama(t, plannerModel)

	corpus := filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid")

	result := Run(t, RunConfig{
		Profile:   filepath.Join("agents", "planner", "profile.yaml"),
		Directory: corpus,
	})

	// srd004 AC1: the shipped wrapper runs under a single root and selects
	// load_graph as its first, graph-boundary action.
	result.RootRequired(t)
	result.RequireToolSpans(t, "load_graph", "extract_task")

	// srd004 AC2: load_graph seeded the requirement graph into pipeline state.
	if _, _, ok := result.Spans.FindEvent("pipeline.graph_loaded"); !ok {
		t.Fatalf("no pipeline.graph_loaded event; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}
}

// TestPlannerShippedProfileTerminalExecution runs the shipped pass-through
// planner variant through its real execute_task factory and command. A controlled
// child executable isolates the process boundary while the shipped profile,
// declarations, graph loader, extractor, validators, and transition table remain
// in control of command selection and terminal state mapping.
func TestPlannerShippedProfileTerminalExecution(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3.6:35b-mlx"}]}`))
	}))
	defer ollama.Close()

	profile := CopyShippedProfile(t, filepath.Join("agents", "planner", "profile.yaml"), map[string]string{
		"machine: machine.yaml":  "machine: machine-passthrough.yaml",
		"http://localhost:11434": ollama.URL,
	})
	workspace := t.TempDir()
	requireCopyFS(t, workspace, filepath.Join(coreRoot, "pkg", "spec", "testdata", "valid"))
	writeEphemeral(t, workspace, "go.mod", "module plannerproof\n\ngo 1.26\n")
	writeEphemeral(t, workspace, "plannerproof_test.go", "package plannerproof\n\nimport \"testing\"\n\nfunc TestProof(t *testing.T) {}\n")

	childArgs := filepath.Join(t.TempDir(), "child-args.txt")
	child := filepath.Join(t.TempDir(), "agent")
	writeEphemeral(t, filepath.Dir(child), filepath.Base(child), fmt.Sprintf(
		"#!/bin/sh\nset -eu\nprintf '%%s\\n' \"$*\" > %q\n", childArgs,
	))
	if err := os.Chmod(child, 0o755); err != nil {
		t.Fatalf("chmod controlled child: %v", err)
	}

	result := Run(t, RunConfig{
		Profile: profile, Directory: workspace,
		Args: []string{"--child-agent-binary", child},
	})

	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireToolSpans(t, "load_graph", "extract_all", "execute_task", "vet", "build", "test", "check_result")
	result.RequireTerminalState(t, "Completed")
	args := readFile(t, childArgs)
	if !strings.Contains(args, "--profile agents/executor/profile.yaml") {
		t.Fatalf("execute_task child args do not select shipped executor profile:\n%s", args)
	}
}

func requireCopyFS(t *testing.T, destination, source string) {
	t.Helper()
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		t.Fatalf("copy planner proof corpus: %v", err)
	}
}

// unmarshalShipped reads a shipped YAML file (path relative to the agent-profiles
// root) and unmarshals it into out.
func unmarshalShipped(t *testing.T, rel string, out any) {
	t.Helper()
	data, err := os.ReadFile(ProfilePath(rel))
	if err != nil {
		t.Fatalf("read shipped %s: %v", rel, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal shipped %s: %v", rel, err)
	}
}

// requireTransition fails unless transitions contains an entry matching the
// given state, signal, next state, and action.
func requireTransition(t *testing.T, transitions []machineTransition, state, signal, next, action string) {
	t.Helper()
	for _, tr := range transitions {
		if tr.State == state && tr.Signal == signal {
			if tr.Next != next {
				t.Errorf("transition %s/%s next = %q, want %q", state, signal, tr.Next, next)
			}
			if tr.Action != action {
				t.Errorf("transition %s/%s action = %q, want %q", state, signal, tr.Action, action)
			}
			return
		}
	}
	t.Errorf("no transition for state %q signal %q found", state, signal)
}

// contains reports whether s is in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
