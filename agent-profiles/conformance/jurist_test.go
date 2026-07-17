// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"path/filepath"
	"testing"
)

// TestJuristConformance runs the shipped jurist profile through the agent CLI
// and asserts its deterministic, LLM-free load/validate/report pipeline from the
// trace. Jurist is the pilot family that proves the harness end to end because
// it needs no model, no child agent, and no server.
//
// It runs the wrapper an operator ships — agents/jurist/profile.yaml — directly,
// not a synthesized reconstruction. The shipped profile's /opt/agent-core
// tool_config_dirs and its load_corpus tool declaration (bound to the builtin
// charter suite) are resolved onto the checkout by --core-root
// (spec.SetAgentCoreInstallRoot); load_corpus reads the specification docs from
// the run directory, so --directory points at the demo-charter fixture whose
// docs/ tree the profile validates. No field needs patching.
//
// Traces srd005-jurist R1 (deterministic tool pipeline, no LLM), R2 (selects
// load_corpus, validate_specs, format_report), and R3 (terminal Passed/Failed
// with a formatted report). Mirrors the profile wiring of
// magefiles/validation.go validateJuristCharterDemo.
func TestJuristConformance(t *testing.T) {
	RequireCoreRoot(t)

	fixtureDir := ProfilePath(filepath.Join("testdata", "integration", "jurist-charter-demo"))

	result := Run(t, RunConfig{
		Profile:   filepath.Join("agents", "jurist", "profile.yaml"),
		Directory: fixtureDir,
	})

	// srd005-jurist R1: the deterministic pipeline runs to a clean CLI exit
	// with a single root span and no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd005-jurist R2: the jurist tool pipeline is visible as tool spans.
	result.RequireToolSpans(t, "load_corpus", "validate_specs", "format_report")

	// srd005-jurist R3: the machine reaches a terminal outcome. Whether the
	// corpus validates clean (Passed) or carries violations (Failed), both are
	// valid jurist terminal states.
	finalState := result.RequireTerminalState(t, "Passed", "Failed")
	t.Logf("jurist terminal state: %s", finalState)
}
