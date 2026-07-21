// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const evaluatorGeneratorSuite = "testdata/integration/rel07-evaluator-generator/suite.yaml"

type evaluatorGeneratorMeta struct {
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Sample      string `json:"sample"`
	ExitCode    int    `json:"exit_code"`
	TestsPassed bool   `json:"tests_passed"`
	TimedOut    bool   `json:"timed_out"`
	TestOutput  string `json:"test_output"`
}

// EvaluatorGenerator proves evaluator can run generator as a benchmark subject.
func (Integration) EvaluatorGenerator() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	outputDir, err := os.MkdirTemp("", "agent-profiles-evaluator-generator-*")
	if err != nil {
		return fmt.Errorf("create evaluator output dir: %w", err)
	}
	defer os.RemoveAll(outputDir)
	fakeBinDir, err := os.MkdirTemp("", "agent-profiles-child-agent-*")
	if err != nil {
		return fmt.Errorf("create child agent bin dir: %w", err)
	}
	defer os.RemoveAll(fakeBinDir)
	if err := writeGeneratorChildAgent(fakeBinDir); err != nil {
		return err
	}
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	cmd := exec.Command(binary,
		// The evaluator role shipped as agents/critic after the rel10 rename
		// (GH-498); the removed agents/evaluator path failed this live target.
		"--profile", filepath.Join(profilesRoot, "agents", "critic", "profile.yaml"),
		"--request", filepath.Join(profilesRoot, evaluatorGeneratorSuite),
		"--output", outputDir,
		"--core-root", coreRoot,
		"--child-agent-binary", filepath.Join(fakeBinDir, "agent"),
	)
	cmd.Dir = profilesRoot
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	fmt.Printf("running evaluator-generator tracer with output at %s\n", outputDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("evaluator-generator run failed: %w\n%s", err, output.String())
	}
	if err := assertEvaluatorGeneratorOutput(outputDir); err != nil {
		return fmt.Errorf("%w\n%s", err, output.String())
	}
	fmt.Println("integration:evaluatorGenerator PASS - evaluator invoked generator profile and recorded point/session evidence")
	return nil
}

func writeGeneratorChildAgent(dir string) error {
	script := `#!/bin/sh
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
  printf '{"Name":"child generator shim","Attributes":[{"Key":"agent.version","Value":{"Type":"STRING","Value":"profile-owned-shim"}},{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}},{"Key":"gen_ai.usage.output_tokens","Value":{"Type":"INT64","Value":1}}]}\n' > "$trace"
fi
echo "generator profile boundary exercised"
`
	return writeExecutable(filepath.Join(dir, "agent"), script, "child agent shim")
}

func assertEvaluatorGeneratorOutput(outputDir string) error {
	points, err := evaluatorPointDirs(outputDir)
	if err != nil {
		return err
	}
	if len(points) != 1 {
		return fmt.Errorf("expected one evaluator point, got %d in %s", len(points), outputDir)
	}
	pointDir := points[0]
	meta, err := readEvaluatorMeta(filepath.Join(pointDir, "meta.json"))
	if err != nil {
		return err
	}
	if meta.Harness != "generator" {
		return fmt.Errorf("meta harness = %q, want generator", meta.Harness)
	}
	if meta.Sample != "greet" {
		return fmt.Errorf("meta sample = %q, want greet", meta.Sample)
	}
	if meta.ExitCode != 0 || meta.TimedOut {
		return fmt.Errorf("child run exit_code=%d timed_out=%t", meta.ExitCode, meta.TimedOut)
	}
	if !meta.TestsPassed {
		return fmt.Errorf("oracle did not pass:\n%s", meta.TestOutput)
	}
	experiment, err := os.ReadFile(filepath.Join(pointDir, "experiment.yaml"))
	if err != nil {
		return fmt.Errorf("read experiment.yaml: %w", err)
	}
	if !strings.Contains(string(experiment), "agents/executor/profile.yaml") {
		return fmt.Errorf("experiment.yaml does not record generator profile:\n%s", experiment)
	}
	workspace, err := os.ReadFile(filepath.Join(pointDir, "greet.go"))
	if err != nil {
		return fmt.Errorf("read generated workspace file: %w", err)
	}
	if !strings.Contains(string(workspace), `return "Hello, " + name + "!"`) {
		return fmt.Errorf("workspace does not contain generator-produced edit:\n%s", workspace)
	}
	trace, err := os.ReadFile(filepath.Join(pointDir, "trace.ndjson"))
	if err != nil {
		return fmt.Errorf("read trace.ndjson: %w", err)
	}
	if !strings.Contains(string(trace), "gen_ai.usage.input_tokens") {
		return fmt.Errorf("trace.ndjson missing child token evidence:\n%s", trace)
	}
	return nil
}

func evaluatorPointDirs(outputDir string) ([]string, error) {
	var points []string
	if err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Base(path) == "meta.json" {
			points = append(points, filepath.Dir(path))
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk evaluator output: %w", err)
	}
	return points, nil
}

func readEvaluatorMeta(path string) (evaluatorGeneratorMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return evaluatorGeneratorMeta{}, fmt.Errorf("read meta.json: %w", err)
	}
	var meta evaluatorGeneratorMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return evaluatorGeneratorMeta{}, fmt.Errorf("parse meta.json: %w", err)
	}
	return meta, nil
}
