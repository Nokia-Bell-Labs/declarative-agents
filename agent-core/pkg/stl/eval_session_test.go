// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestParseSuiteValidatesHarnessDataContract(t *testing.T) {
	base := suiteFixture(t)
	suite, err := ParseSuite([]byte(`
name: smoke
harnesses:
  - name: agent
    binary: agent
    flags:
      machine: agents/generator/machine.yaml
      tools: agents/generator/tools.yaml
models: [qwen3]
samples_dir: samples
`), base)

	require.NoError(t, err)
	require.Equal(t, "smoke", suite.Name)
	require.Len(t, suite.Harnesses, 1)
	require.Equal(t, "agent", suite.Harnesses[0].Name)
	require.Equal(t, "agent", suite.Harnesses[0].Binary)
	require.Len(t, suite.Models, 1)
	require.Len(t, suite.Samples, 1)
}

func TestParseSuiteRejectsMissingHarnesses(t *testing.T) {
	base := suiteFixture(t)
	_, err := ParseSuite([]byte(`
name: smoke
models: [qwen3]
samples_dir: samples
`), base)

	require.Error(t, err)
	require.Contains(t, err.Error(), "missing profiles or harnesses")
}

func TestParseSuiteRejectsMissingHarnessBinary(t *testing.T) {
	base := suiteFixture(t)
	_, err := ParseSuite([]byte(`
name: smoke
harnesses:
  - name: agent
models: [qwen3]
samples_dir: samples
`), base)

	require.Error(t, err)
	require.Contains(t, err.Error(), "missing binary")
}

func TestParseSuiteRejectsMissingModels(t *testing.T) {
	base := suiteFixture(t)
	_, err := ParseSuite([]byte(`
name: smoke
harnesses:
  - name: agent
    binary: agent
samples_dir: samples
`), base)

	require.Error(t, err)
	require.Contains(t, err.Error(), "missing models")
}

func TestEvaluatorSessionSetupToolSequence(t *testing.T) {
	base := suiteFixture(t)
	suitePath := filepath.Join(base, "suite.yaml")
	require.NoError(t, os.WriteFile(suitePath, []byte(`
name: smoke
harnesses:
  - name: agent
    binary: agent
models: [qwen3]
grid:
  effort: [low, high]
samples_dir: samples
timeout: 2m
repetitions: 2
ollama_url: http://suite.example
`), 0o644))

	var stderr bytes.Buffer
	outputDir := filepath.Join(base, "eval-results")
	es := &EvalSessionState{
		SuitePath: suitePath,
		OutputDir: outputDir,
		Stderr:    &stderr,
	}

	requireSignal(t, (&parseSuiteConfigCmd{es: es}).Execute(), SigSuiteConfigParsed)
	require.Equal(t, "smoke", es.Suite.Name)
	require.Empty(t, es.Suite.Samples)
	require.Equal(t, filepath.Join(base, "samples"), es.Suite.SamplesDir)

	requireSignal(t, (&discoverSuiteSamplesCmd{es: es}).Execute(), SigSuiteSamplesDiscovered)
	require.Len(t, es.Suite.Samples, 1)

	requireSignal(t, (&expandEvalGridCmd{es: es}).Execute(), SigEvalGridExpanded)
	require.Len(t, es.gridPoints, 2)

	requireSignal(t, (&initEvalSessionCmd{es: es}).Execute(), SigEvalSessionInitialized)
	require.DirExists(t, es.SessionDir)
	require.Equal(t, 2, es.reps)
	require.Equal(t, 2*time.Minute, es.timeout)
	require.Equal(t, "http://suite.example", es.ollamaURL)

	res := (&reportSuiteSummaryCmd{es: es}).Execute()
	requireSignal(t, res, SigSuiteLoaded)
	require.Contains(t, res.Output, "4 points")
	require.Contains(t, stderr.String(), "4 points")
}

func TestNextPointUndoRestoresEvaluatorSessionCursor(t *testing.T) {
	base := suiteFixture(t)
	suitePath := filepath.Join(base, "suite.yaml")
	require.NoError(t, os.WriteFile(suitePath, []byte(`
name: smoke
harnesses:
  - name: agent
    binary: agent
models: [qwen3]
samples_dir: samples
`), 0o644))

	es := &EvalSessionState{SuitePath: suitePath, OutputDir: filepath.Join(base, "out"), Stderr: &bytes.Buffer{}}
	requireSignal(t, (&parseSuiteConfigCmd{es: es}).Execute(), SigSuiteConfigParsed)
	requireSignal(t, (&discoverSuiteSamplesCmd{es: es}).Execute(), SigSuiteSamplesDiscovered)
	requireSignal(t, (&expandEvalGridCmd{es: es}).Execute(), SigEvalGridExpanded)
	requireSignal(t, (&initEvalSessionCmd{es: es}).Execute(), SigEvalSessionInitialized)

	cmd := &nextPointCmd{es: es}
	requireSignal(t, cmd.Execute(), SigPointReady)
	require.NotNil(t, es.PC)
	require.True(t, es.started)

	undo := cmd.Undo()
	requireSignal(t, undo, core.ToolDone)
	require.Nil(t, es.PC)
	require.False(t, es.started)

	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Contains(t, string(memento.Payload), `"domain_state"`)
}

func TestParseSuiteWithProfiles(t *testing.T) {
	base := suiteFixture(t)

	profileDir := filepath.Join(base, "profiles")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))

	machineDir := filepath.Join(base, "machines")
	require.NoError(t, os.MkdirAll(machineDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(machineDir, "machine.yaml"), []byte("states: [Init]\n"), 0o644))

	toolsDir := filepath.Join(base, "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "tools.yaml"), []byte("tools: [read]\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "fast.yaml"), []byte(fmt.Sprintf(`
name: fast-model
machine: %s/machine.yaml
tools:
  - %s/tools.yaml
`, machineDir, toolsDir)), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "slow.yaml"), []byte(fmt.Sprintf(`
name: slow-model
machine: %s/machine.yaml
tools:
  - %s/tools.yaml
`, machineDir, toolsDir)), 0o644))

	suite, err := ParseSuite([]byte(fmt.Sprintf(`
name: profile-test
profiles:
  - %s/fast.yaml
  - %s/slow.yaml
samples_dir: samples
`, profileDir, profileDir)), base)

	require.NoError(t, err)
	require.Equal(t, "profile-test", suite.Name)
	require.Len(t, suite.Profiles, 2)
	require.Equal(t, "fast-model", suite.Profiles[0].Name)
	require.Equal(t, "slow-model", suite.Profiles[1].Name)
	require.Empty(t, suite.Harnesses)
	require.Empty(t, suite.Models)
	require.Len(t, suite.Samples, 1)
}

func TestParseSuiteRejectsProfilesWithHarnesses(t *testing.T) {
	base := suiteFixture(t)
	_, err := ParseSuite([]byte(`
name: conflict
profiles: [a.yaml]
harnesses:
  - name: agent
    binary: agent
samples_dir: samples
`), base)

	require.Error(t, err)
	require.Contains(t, err.Error(), "mutually exclusive")
}

func TestNextPointIteratesProfiles(t *testing.T) {
	base := suiteFixture(t)

	profileDir := filepath.Join(base, "profiles")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))

	machineDir := filepath.Join(base, "machines")
	require.NoError(t, os.MkdirAll(machineDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(machineDir, "machine.yaml"), []byte("states: [Init]\n"), 0o644))

	toolsDir := filepath.Join(base, "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "tools.yaml"), []byte("tools: [read]\n"), 0o644))

	for _, name := range []string{"alpha", "beta"} {
		require.NoError(t, os.WriteFile(filepath.Join(profileDir, name+".yaml"), []byte(fmt.Sprintf(`
name: %s
machine: %s/machine.yaml
tools:
  - %s/tools.yaml
`, name, machineDir, toolsDir)), 0o644))
	}

	suitePath := filepath.Join(base, "suite.yaml")
	require.NoError(t, os.WriteFile(suitePath, []byte(fmt.Sprintf(`
name: iter-test
profiles:
  - %s/alpha.yaml
  - %s/beta.yaml
samples_dir: samples
`, profileDir, profileDir)), 0o644))

	es := &EvalSessionState{SuitePath: suitePath, OutputDir: filepath.Join(base, "out"), Stderr: &bytes.Buffer{}}
	requireSignal(t, (&parseSuiteConfigCmd{es: es}).Execute(), SigSuiteConfigParsed)
	requireSignal(t, (&discoverSuiteSamplesCmd{es: es}).Execute(), SigSuiteSamplesDiscovered)
	requireSignal(t, (&expandEvalGridCmd{es: es}).Execute(), SigEvalGridExpanded)
	requireSignal(t, (&initEvalSessionCmd{es: es}).Execute(), SigEvalSessionInitialized)

	var points []string
	for {
		pc, ok := es.NextPoint()
		if !ok {
			break
		}
		points = append(points, pc.Harness.Name+"_"+pc.Sample.Name)
		require.NotEmpty(t, pc.ProfilePath)
	}

	require.Equal(t, []string{"alpha_hello", "beta_hello"}, points)
}

func TestDiscoverSuiteSamplesReportsCommandError(t *testing.T) {
	es := &EvalSessionState{Suite: SuiteConfig{Name: "broken", SamplesDir: filepath.Join(t.TempDir(), "missing")}}
	res := (&discoverSuiteSamplesCmd{es: es}).Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "discover samples")
}

func suiteFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	sample := filepath.Join(base, "samples", "hello")
	require.NoError(t, os.MkdirAll(filepath.Join(sample, "workspace"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sample, "prompt.yaml"), []byte("prompt: hello\n"), 0o644))
	return base
}
