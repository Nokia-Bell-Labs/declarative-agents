// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// stubGeneratorAgent is a fake child `agent` binary. The evaluator point machine
// launches the generator profile as a subprocess from the configured
// --child-agent-binary; this shim stands in for it so the session runs
// deterministically with no live model. It is intentionally confined to
// conformance; application integration must use the real built agent binary.
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
  *agents/executor/profile.yaml) ;;
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
		"materialize_eval_points",
		"run_point",
		"report_session",
	)

	// srd003 R3.2: the session machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}

// TestCriticChangedWorkspaceVerdicts proves the canonical changed-workspace
// mode makes its own gate decision. The test supplies only candidates; the
// profile runs the declared oracle, selects the matching machine branch, and
// writes the explicit verdict artifact consumed by applications.
func TestCriticChangedWorkspaceVerdicts(t *testing.T) {
	RequireCoreRoot(t)
	tests := []struct {
		name       string
		hello      string
		exit       int
		terminal   string
		verdict    string
		accepted   bool
		oracleStat string
	}{
		{
			name: "conforming candidate accepted", hello: `"Hello, " + name + "!"`,
			exit: 0, terminal: "Succeeded", verdict: "accepted", accepted: true, oracleStat: "passed",
		},
		{
			name: "nonconforming candidate rejected", hello: `""`,
			exit: 2, terminal: "Rejected", verdict: "rejected", accepted: false, oracleStat: "failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			writeEphemeral(t, workspace, "go.mod", "module criticcandidate\n\ngo 1.26\n")
			writeEphemeral(t, workspace, "greet.go",
				"package greet\n\nfunc Hello(name string) string { return "+tc.hello+" }\n")
			writeEphemeral(t, workspace, "greet_test.go", `package greet

import "testing"

func TestHello(t *testing.T) {
	if got := Hello("Go"); got != "Hello, Go!" {
		t.Fatalf("Hello = %q", got)
	}
}
`)

			result := Run(t, RunConfig{
				Profile:   filepath.Join("agents", "critic", "profile-workspace.yaml"),
				Directory: workspace,
			})
			result.RequireExit(t, tc.exit)
			result.RootRequired(t)
			result.RequireNoErrorSpans(t)
			result.RequireToolSpans(t, "evaluate_candidate", "emit_"+tc.verdict+"_verdict")
			result.RequireTerminalState(t, tc.terminal)

			data, err := os.ReadFile(filepath.Join(workspace, "critic-verdict.json"))
			if err != nil {
				t.Fatalf("read canonical critic verdict: %v", err)
			}
			var verdict struct {
				SchemaVersion string `json:"schema_version"`
				Mode          string `json:"mode"`
				Verdict       string `json:"verdict"`
				Accepted      bool   `json:"accepted"`
				Oracle        struct {
					Command string `json:"command"`
					Status  string `json:"status"`
				} `json:"oracle"`
			}
			if err := json.Unmarshal(data, &verdict); err != nil {
				t.Fatalf("parse canonical critic verdict: %v\n%s", err, data)
			}
			if verdict.SchemaVersion != "1" || verdict.Mode != "changed-workspace" ||
				verdict.Verdict != tc.verdict || verdict.Accepted != tc.accepted ||
				verdict.Oracle.Command != "go test ./..." || verdict.Oracle.Status != tc.oracleStat {
				t.Fatalf("verdict = %#v, want %s accepted=%t oracle=%s", verdict, tc.verdict, tc.accepted, tc.oracleStat)
			}
		})
	}
}
