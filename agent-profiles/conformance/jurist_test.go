// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestJuristConformance runs the jurist family through the agent CLI and
// asserts its deterministic, LLM-free load/validate/report pipeline from the
// trace. Jurist is the pilot family that proves the harness end to end because
// it needs no model, no child agent, and no server.
//
// Traces srd005-jurist R1 (deterministic tool pipeline, no LLM), R2 (selects
// load_corpus, validate_specs, format_report), and R3 (terminal Passed/Failed
// with a formatted report). Mirrors the profile wiring of
// magefiles/validation.go validateJuristCharterDemo.
func TestJuristConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)

	tmp := t.TempDir()
	profilePath := writeJuristProfile(t, coreRoot, tmp)
	fixtureDir := ProfilePath(filepath.Join("testdata", "integration", "jurist-charter-demo"))

	result := Run(t, RunConfig{
		Profile:   profilePath,
		Directory: fixtureDir,
	})

	// srd005-jurist R1: the deterministic pipeline runs to a clean CLI exit
	// with a single root span and no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd005-jurist R2: the jurist tool pipeline is visible as tool spans.
	result.RequireToolSpans(t, "load_corpus", "validate_specs", "format_report")

	// srd005-jurist R3: the machine reaches a terminal outcome. The demo
	// charter carries seeded violations, so this run reaches Failed; a clean
	// corpus would reach Passed. Both are valid jurist terminal states.
	finalState := result.RequireTerminalState(t, "Passed", "Failed")
	t.Logf("jurist terminal state: %s", finalState)
}

// writeJuristProfile writes an ephemeral jurist profile and load_corpus tool
// declaration bound to the demo charter suite, returning the profile path.
func writeJuristProfile(t *testing.T, coreRoot, tmp string) string {
	t.Helper()
	juristDir := ProfilePath(filepath.Join("agents", "jurist"))
	suitePath := filepath.Join(juristDir, "suites", "demo-charter.yaml")
	toolDeclPath := filepath.Join(tmp, "load-corpus-demo.yaml")
	profilePath := filepath.Join(tmp, "profile.yaml")

	profile := fmt.Sprintf(`name: jurist-conformance
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
tool_declarations:
  - %q
`, filepath.Join(juristDir, "machine.yaml"),
		filepath.Join(juristDir, "tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "spec-validation"),
		toolDeclPath)
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		t.Fatalf("write jurist profile: %v", err)
	}

	toolDecl := fmt.Sprintf(`includes:
  - %q
tools:
  - name: load_corpus
    type: builtin
    init: load_corpus
    visibility: internal
    config:
      suite_paths:
        - %q
    emits:
      - ToolDone
      - CommandError
`, filepath.Join(coreRoot, "tools", "builtin", "load-corpus.yaml"), suitePath)
	if err := os.WriteFile(toolDeclPath, []byte(toolDecl), 0o644); err != nil {
		t.Fatalf("write jurist tool declaration: %v", err)
	}
	return profilePath
}
