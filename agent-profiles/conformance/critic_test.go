// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

// stubGeneratorAgent is a fake child `agent` binary. The evaluator point machine
// launches the generator profile as a subprocess from the configured
// --child-agent-binary; this shim stands in for it so the session runs
// deterministically with no live model. It mirrors
// magefiles/integration_evaluator.go writeGeneratorChildAgent: it writes the
// expected workspace edit and a minimal child trace, then exits cleanly.
const stubGeneratorAgent = `#!/bin/sh
set -eu
profile=
workspace=
trace=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --profile) profile="$2"; shift 2 ;;
    --directory) workspace="$2"; shift 2 ;;
    --otel-log-file) trace="$2"; shift 2 ;;
    *) shift ;;
  esac
done
case "$profile" in
  *agents/generator/profile.yaml) ;;
  *) echo "unexpected child profile: $profile" >&2; exit 42 ;;
esac
cat > "$workspace/greet.go" <<'GOEOF'
package greet

// Hello returns a greeting for the given name.
func Hello(name string) string {
	return "Hello, " + name + "!"
}
GOEOF
if [ -n "$trace" ]; then
  mkdir -p "$(dirname "$trace")"
  printf '{"Name":"child generator shim","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}}]}\n' > "$trace"
fi
echo "generator profile boundary exercised"
`

// TestCriticConformance runs the critic profile over the proven
// rel07-evaluator-generator suite fixture with a stubbed generator child agent
// passed via --child-agent-binary, and asserts the deterministic session
// pipeline reaches the Done terminal state with no live model.
//
// It mirrors magefiles/integration_evaluator.go but asserts on the OTel trace
// instead of on-disk artifacts.
//
// Traces srd003-critic: R1.1 (deterministic parse -> expand -> nested point
// -> report session pipeline), R2.2 (evaluator session and child-execution tool
// families), and R3.2 (Done terminal outcome).
func TestCriticConformance(t *testing.T) {
	RequireCoreRoot(t)

	// The point machine launches the child generator agent from the configured
	// --child-agent-binary; the shim stands in for it so the session runs without
	// a live model.
	binDir := t.TempDir()
	stubAgent := filepath.Join(binDir, "agent")
	writeEphemeral(t, binDir, "agent", stubGeneratorAgent)
	if err := os.Chmod(stubAgent, 0o755); err != nil {
		t.Fatalf("chmod stub agent: %v", err)
	}

	result := Run(t, RunConfig{
		Profile: filepath.Join("agents", "critic", "profile.yaml"),
		Request: ProfilePath(filepath.Join("testdata", "integration", "rel07-evaluator-generator", "suite.yaml")),
		Output:  t.TempDir(),
		Args:    []string{"--child-agent-binary", stubAgent},
	})

	// srd003 R3.2: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd003 R1.1/R2.2: the deterministic session pipeline vocabulary is visible.
	result.RequireToolSpans(t,
		"parse_suite_config",
		"discover_suite_samples",
		"expand_eval_grid",
		"init_eval_session",
		"report_suite_summary",
		"next_point",
		"run_point",
		"report_session",
	)

	// srd003 R3.2: the session machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}
