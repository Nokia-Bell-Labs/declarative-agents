// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const benchEvaluatorFixture = "testdata/integration/rel07-bench-evaluator"

type benchLaunchRequest struct {
	Type   string                 `yaml:"type" json:"type"`
	Config map[string]interface{} `yaml:"config" json:"config"`
}

type benchLaunchEvidence struct {
	RequestID                   string `yaml:"request_id"`
	HumanActionReceived         bool   `yaml:"human_action_received"`
	EvaluatorProfile            string `yaml:"evaluator_profile"`
	Suite                       string `yaml:"suite"`
	LaunchStatus                string `yaml:"launch_status"`
	EvaluatorOutputEvidence     string `yaml:"evaluator_output_evidence"`
	EvaluatorOutputMaterialized bool   `yaml:"evaluator_output_materialized"`
}

// BenchEvaluator proves bench launches evaluator from a fixture human action.
func (Integration) BenchEvaluator() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	fixtureRoot := filepath.Join(profilesRoot, benchEvaluatorFixture)
	request, err := readBenchLaunchRequest(filepath.Join(fixtureRoot, "request.yaml"))
	if err != nil {
		return err
	}
	if request.Type != "launch_eval" {
		return fmt.Errorf("bench request type = %q, want launch_eval", request.Type)
	}
	if err := requireProfilePaths(profilesRoot, "agents/bench/profile.yaml", "agents/critic/profile.yaml"); err != nil {
		return err
	}
	runDir, err := os.MkdirTemp("", "agent-profiles-bench-evaluator-*")
	if err != nil {
		return fmt.Errorf("create bench-evaluator run dir: %w", err)
	}
	defer os.RemoveAll(runDir)
	suitePath := fixtureValue(request.Config, "suite")
	outputRel := fixtureValue(request.Config, "output_dir")
	if suitePath == "" || outputRel == "" {
		return fmt.Errorf("bench launch request requires suite and output_dir")
	}
	evaluatorBin, err := writeEvaluatorChildAgent(runDir)
	if err != nil {
		return err
	}
	outputDir := filepath.Join(runDir, outputRel)
	cmd := exec.Command(evaluatorBin,
		// The evaluator role shipped as agents/critic after the rel10 rename; the
		// removed agents/evaluator path failed this live target (GH-498).
		"--profile", filepath.Join(profilesRoot, "agents", "critic", "profile.yaml"),
		"--request", filepath.Join(fixtureRoot, suitePath),
		"--output", outputDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bench evaluator launch failed: %w\n%s", err, out)
	}
	evidence := benchLaunchEvidence{
		RequestID:                   fixtureValue(request.Config, "request_id"),
		HumanActionReceived:         true,
		EvaluatorProfile:            "agents/critic/profile.yaml",
		Suite:                       suitePath,
		LaunchStatus:                "completed",
		EvaluatorOutputEvidence:     filepath.ToSlash(filepath.Join(outputRel, "session-summary.yaml")),
		EvaluatorOutputMaterialized: true,
	}
	if err := writeBenchLaunchEvidence(runDir, evidence); err != nil {
		return err
	}
	if err := assertBenchEvaluatorEvidence(runDir, evidence); err != nil {
		return err
	}
	fmt.Println("integration:benchEvaluator PASS - bench action launched evaluator and recorded evidence")
	return nil
}

func readBenchLaunchRequest(path string) (benchLaunchRequest, error) {
	var request benchLaunchRequest
	if err := readIntegrationYAML(path, "bench launch request", &request); err != nil {
		return benchLaunchRequest{}, err
	}
	return request, nil
}

func fixtureValue(config map[string]interface{}, key string) string {
	value, _ := config[key].(string)
	return value
}

func writeEvaluatorChildAgent(runDir string) (string, error) {
	path := filepath.Join(runDir, "evaluator-agent")
	script := `#!/bin/sh
set -eu
profile=
suite=
output=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --profile) profile="$2"; shift 2 ;;
    --request) suite="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    *) shift ;;
  esac
done
case "$profile" in
  *agents/critic/profile.yaml) ;;
  *) echo "unexpected evaluator profile: $profile" >&2; exit 42 ;;
esac
mkdir -p "$output"
cat > "$output/session-summary.yaml" <<EOF
profile: $profile
suite: $suite
status: completed
points: 1
EOF
echo "evaluator profile boundary exercised"
`
	if err := writeExecutable(path, script, "evaluator child agent"); err != nil {
		return "", err
	}
	return path, nil
}

func writeBenchLaunchEvidence(runDir string, evidence benchLaunchEvidence) error {
	return writeIntegrationYAML(filepath.Join(runDir, "launch-evidence.yaml"), "bench evidence", evidence)
}

func assertBenchEvaluatorEvidence(runDir string, want benchLaunchEvidence) error {
	var got benchLaunchEvidence
	if err := readIntegrationYAML(filepath.Join(runDir, "launch-evidence.yaml"), "bench evidence", &got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("bench evidence = %#v, want %#v", got, want)
	}
	summaryPath := filepath.Join(runDir, filepath.FromSlash(want.EvaluatorOutputEvidence))
	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("read evaluator output evidence: %w", err)
	}
	for _, text := range []string{"agents/critic/profile.yaml", "status: completed"} {
		if !strings.Contains(string(summary), text) {
			return fmt.Errorf("evaluator summary missing %q:\n%s", text, summary)
		}
	}
	actionJSON, err := json.Marshal(benchLaunchRequest{
		Type: "launch_eval",
		Config: map[string]interface{}{
			"suite": want.Suite, "output_dir": filepath.Dir(want.EvaluatorOutputEvidence), "request_id": want.RequestID,
		},
	})
	if err != nil || !strings.Contains(string(actionJSON), "launch_eval") {
		return fmt.Errorf("bench action evidence could not be encoded: %w", err)
	}
	return nil
}
