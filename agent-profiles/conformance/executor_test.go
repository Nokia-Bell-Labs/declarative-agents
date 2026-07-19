// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"path/filepath"
	"testing"
)

// generatorModel is the model the generator default LLM declaration configures
// (agents/executor/llm/default.yaml), shared by profile.yaml and
// profile-qwen35b.yaml. generatorQwen27bModel is the model the 27B variant
// configures (agents/executor/llm/qwen27b.yaml). The behavioral runs gate on
// the relevant model being served by Ollama.
const (
	generatorModel        = "qwen3.6:35b-mlx"
	generatorQwen27bModel = "qwen3.6:27b-mlx"
)

// TestExecutorConformance runs the shipped executor profile — the wrapper an
// operator ships, not a synthesized reconstruction — through its model
// boundary. The generator's first machine action is invoke_llm, which pings
// Ollama at tool registration and calls the model during the run, so the run is
// Ollama-gated: without a reachable model the profile cannot even register its
// tools.
//
// The full shipped coding session is slow and nondeterministic, so the run is
// bounded to a single iteration by patching only the machine's max_iterations
// field on a copied shipped profile (runBoundedShippedGenerator): exactly one
// invoke_llm fires and the machine reaches BudgetExceeded deterministically.
// Only the bounding field is patched — the shipped machine, tools, LLM
// declaration, and tool_config_dirs are exercised as shipped.
//
// Traces srd002-executor: R1.1 (invoke_llm as the machine's model-boundary
// action), R2.2 (the LLM tool family), and R3.2 (a clean terminal outcome).
func TestExecutorConformance(t *testing.T) {
	runBoundedShippedGenerator(t, filepath.Join("agents", "executor", "profile.yaml"), generatorModel)
}

// TestExecutorQwen35bConformance runs the shipped generator-qwen35b variant.
// It shares the default 35B model declaration, so it exercises the same model
// boundary as profile.yaml through the variant wrapper an operator ships.
func TestExecutorQwen35bConformance(t *testing.T) {
	runBoundedShippedGenerator(t, filepath.Join("agents", "executor", "profile-qwen35b.yaml"), generatorModel)
}

// TestExecutorQwen27bConformance runs the shipped generator-qwen27b variant,
// which points at the 27B model declaration. It is gated on that model being
// served and skips cleanly otherwise.
func TestExecutorQwen27bConformance(t *testing.T) {
	runBoundedShippedGenerator(t, filepath.Join("agents", "executor", "profile-qwen27b.yaml"), generatorQwen27bModel)
}

// runBoundedShippedGenerator copies the shipped executor profile at relProfile,
// bounds its machine to a single iteration by patching only max_iterations (no
// wrapper rebuild), runs it, and asserts the model boundary fired: a chat
// <model> gen_ai span under a single root, a clean BudgetExceeded terminal, and
// no error-status spans. It is Ollama-gated on model.
func runBoundedShippedGenerator(t *testing.T, relProfile, model string) {
	t.Helper()
	RequireCoreRoot(t)
	RequireOllama(t, model)

	// Bound the machine to one iteration: Idle -> Composing (invoke_llm), then
	// the iteration budget is exhausted before a second cycle, so the run
	// reaches BudgetExceeded after exactly one model call. Only max_iterations
	// is patched; the shipped machine, tools, LLM declaration, and
	// tool_config_dirs (/opt/agent-core, remapped by --core-root) run as shipped.
	profilePath := CopyShippedProfile(t, relProfile, map[string]string{
		"max_iterations: 100": "max_iterations: 1",
	})

	result := Run(t, RunConfig{Profile: profilePath, Directory: t.TempDir()})

	// srd002 R3.2: a clean terminal outcome with a single root and no error spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd002 R1.1/R2.2: the model boundary is exercised — a gen_ai chat span for
	// the configured model appears (the genai span vocabulary names model calls
	// "chat <model>").
	if got := result.Spans.NamePrefixed("chat "); len(got) == 0 {
		t.Fatalf("no gen_ai chat span for the model boundary; span names: %v\noutput:\n%s", result.Spans.Names(), result.Output)
	}

	// srd002 R3.2: the bounded run reaches the BudgetExceeded terminal state.
	result.RequireTerminalState(t, "BudgetExceeded")
}

// TestExecutorShippedProfileWiring asserts, model-free and ungated, that the
// three wrappers an operator ships for the generator family are wired as the
// behavioral runs assume: each profile references the shared machine, the tools
// manifest, and an invoke_llm LLM declaration, and the shared machine seeds the
// model boundary from Idle and drains its iteration budget to BudgetExceeded.
// It needs no model, so it holds in the fast default and where Ollama is absent.
//
// Traces srd002-executor R1.1 (invoke_llm as the seed model-boundary action)
// and R3.2 (BudgetExhausted reaches the BudgetExceeded terminal).
func TestExecutorShippedProfileWiring(t *testing.T) {
	type generatorProfile struct {
		rel     string
		llmDecl string
	}
	for _, gp := range []generatorProfile{
		{filepath.Join("agents", "executor", "profile.yaml"), "llm/default.yaml"},
		{filepath.Join("agents", "executor", "profile-qwen35b.yaml"), "llm/default.yaml"},
		{filepath.Join("agents", "executor", "profile-qwen27b.yaml"), "llm/qwen27b.yaml"},
	} {
		var profile struct {
			Machine          string   `yaml:"machine"`
			Tools            []string `yaml:"tools"`
			ToolDeclarations []string `yaml:"tool_declarations"`
		}
		unmarshalShipped(t, gp.rel, &profile)
		if profile.Machine != "machine.yaml" {
			t.Errorf("%s machine = %q, want machine.yaml", gp.rel, profile.Machine)
		}
		if !contains(profile.Tools, "tools.yaml") {
			t.Errorf("%s tools = %v, want to include tools.yaml", gp.rel, profile.Tools)
		}
		if !contains(profile.ToolDeclarations, gp.llmDecl) {
			t.Errorf("%s tool_declarations = %v, want to include %s", gp.rel, profile.ToolDeclarations, gp.llmDecl)
		}
	}

	var machine struct {
		InitialState string              `yaml:"initial_state"`
		Transitions  []machineTransition `yaml:"transitions"`
	}
	unmarshalShipped(t, filepath.Join("agents", "executor", "machine.yaml"), &machine)

	if machine.InitialState != "Idle" {
		t.Errorf("shipped generator machine initial_state = %q, want Idle", machine.InitialState)
	}
	// The model boundary seeds from Idle, and an exhausted budget reaches the
	// terminal the bounded behavioral runs assert.
	requireTransition(t, machine.Transitions, "Idle", "Seed", "Composing", "invoke_llm")
	requireTransition(t, machine.Transitions, "Composing", "BudgetExhausted", "BudgetExceeded", "")
}
