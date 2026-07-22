// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

const rigProfile = "testdata/rig/profile.yaml"

// rigVerdictPattern matches the assembler's per-scenario verdict signals in
// trace output order. Discovery sorts scenarios by subject directory then
// name, so the agent-profiles reference subject's three scenarios come first
// (its root path sorts before "agents"), then this example's rag-server query.
var rigExpectedVerdicts = []string{
	"ScenarioFailed", // rig-subject/broken: the deliberately wrong expectation must fail
	"ScenarioPassed", // rig-subject/dep-failure: the subject degraded correctly
	"ScenarioPassed", // rig-subject/happy-path
	"ScenarioPassed", // rag-server/query: the mesh agent against a twin Chroma
}

// Rig runs the assembler test rig over this example's agents and the
// agent-profiles reference subject in one pass — the cross-root proof: one
// rig, two repository areas, discovered by convention. The rag-server is
// exercised end to end against a digital-twin Chroma pinned to the port the
// server's network limits allow; no live Chroma, Ollama, Docker, or
// Kubernetes is involved. The aggregate is failed by design, because the
// reference subject ships a deliberately broken scenario that must fail; this
// target asserts the exact verdict pattern instead. Skips (does not fail)
// when the agent-core checkout is unavailable.
func (Integration) Rig() error {
	exampleRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault("AGENT_CORE_ROOT", siblingPath(exampleRoot, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP integration:rig: agent-core checkout not found at %s\n", coreRoot)
		return nil
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}

	// The assembler's children resolve "agent" from PATH; stage the built
	// binary under that name.
	binDir, err := os.MkdirTemp("", "rig-bin")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(binDir) }()
	staged := filepath.Join(binDir, "agent")
	data, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	if err := os.WriteFile(staged, data, 0o755); err != nil {
		return err
	}

	trace := filepath.Join(binDir, "rig.otel.json")
	cmd := exec.Command(binary,
		"--profile", rigProfile,
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	)
	cmd.Dir = exampleRoot
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	fmt.Println("running the assembler rig over agents/ and the agent-profiles reference subject...")
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rig run: %w\n%s", err, out.String())
	}

	verdicts := rigVerdicts(trace)
	if len(verdicts) != len(rigExpectedVerdicts) {
		return fmt.Errorf("rig verdicts = %v, want %d scenarios\noutput:\n%s", verdicts, len(rigExpectedVerdicts), out.String())
	}
	for i, want := range rigExpectedVerdicts {
		if verdicts[i] != want {
			return fmt.Errorf("rig verdict[%d] = %s, want %s (order: rig-subject broken, dep-failure, happy-path; rag-server query)\nall: %v",
				i, verdicts[i], want, verdicts)
		}
	}
	fmt.Printf("integration:rig passed in %s: %d scenarios across two roots, verdicts %v\n",
		time.Since(start).Round(time.Millisecond), len(verdicts), verdicts)
	return nil
}

var rigVerdictSignal = regexp.MustCompile(`"command\.signal"[^}]*?"(Scenario(?:Passed|Failed))"`)

// rigVerdicts reads the per-scenario verdict signals from the trace file, in
// execution order, from the verdict-collecting words' spans.
func rigVerdicts(tracePath string) []string {
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return nil
	}
	var verdicts []string
	for _, line := range bytes.Split(data, []byte("\n")) {
		if !bytes.Contains(line, []byte("collect_scenario_verdict")) && !bytes.Contains(line, []byte("fail_scenario")) {
			continue
		}
		if !bytes.Contains(line, []byte("execute_tool")) {
			continue
		}
		match := rigVerdictSignal.FindSubmatch(line)
		if match != nil {
			verdicts = append(verdicts, string(match[1]))
		}
	}
	return verdicts
}
