// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"path/filepath"
	"testing"
)

// generatorModel is the model the generator LLM declaration configures
// (agents/generator/llm/default.yaml). The test gates on this model being
// served by Ollama.
const generatorModel = "qwen3.6:35b-mlx"

// TestGeneratorConformance exercises the generator profile's model boundary. The
// generator's first machine action is invoke_llm, which pings Ollama at tool
// registration and calls the model during the run, so the whole run is
// Ollama-gated — there is no no-model path (without a reachable model the
// profile fails to register its tools).
//
// The run is bounded to a single iteration through an ephemeral machine so
// exactly one invoke_llm call fires and the machine reaches BudgetExceeded
// deterministically, rather than a full (slow, nondeterministic) coding session.
// That is enough to prove the model boundary is wired: a chat <model> gen_ai
// span with token usage appears under a single root, and the run reaches a
// clean terminal with no error-status spans.
//
// Traces srd002-generator: R1.1 (invoke_llm as the machine's model-boundary
// action), R2.2 (the LLM tool family), and R3.2 (a clean terminal outcome).
func TestGeneratorConformance(t *testing.T) {
	RequireCoreRoot(t)
	RequireOllama(t, generatorModel)

	genDir := ProfilePath(filepath.Join("agents", "generator"))
	tmp := t.TempDir()

	// Bound the machine to one iteration: Idle -> Composing (invoke_llm), then
	// the iteration budget is exhausted before a second cycle, so the run reaches
	// BudgetExceeded after exactly one model call.
	machine := rewriteFile(t, filepath.Join(genDir, "machine.yaml"), map[string]string{
		"max_iterations: 100": "max_iterations: 1",
	})
	machinePath := writeEphemeral(t, tmp, "machine.yaml", machine)

	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: generator-conformance
machine: %q
tools:
  - %q
tool_config_dirs:
  - /opt/agent-core/tools/builtin/filesystem
  - /opt/agent-core/tools/builtin/llm
  - /opt/agent-core/tools/exec/go
tool_declarations:
  - %q
`, machinePath,
		filepath.Join(genDir, "tools.yaml"),
		filepath.Join(genDir, "llm", "default.yaml")))

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
