// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSuiteValidatesHarnessDataContract(t *testing.T) {
	base := suiteFixture(t)
	suite, err := ParseSuite([]byte(`
name: smoke
harnesses:
  - name: agent
    binary: agent
    flags:
      machine: configs/generator/machine.yaml
      tools: configs/generator/tools.yaml
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

func suiteFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	sample := filepath.Join(base, "samples", "hello")
	require.NoError(t, os.MkdirAll(filepath.Join(sample, "workspace"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sample, "prompt.yaml"), []byte("prompt: hello\n"), 0o644))
	return base
}
