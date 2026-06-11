// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"bytes"
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
	require.Contains(t, err.Error(), "missing harnesses")
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
