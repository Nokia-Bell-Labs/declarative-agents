// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestLoadCorpusCarriesZeroCharters(t *testing.T) {
	root := filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid")
	vs := &SpecState{Directory: root}

	res := (&LoadCorpusBuilder{VS: vs}).Build(core.Result{}).Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.NotNil(t, vs.Corpus)
	require.Equal(t, root, vs.TargetDirectory)
	require.Empty(t, vs.Charters)
}

func TestLoadCorpusLoadsConfiguredCharters(t *testing.T) {
	root := filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid")
	path := writeValidationCharter(t, t.TempDir(), "suite.yaml")
	vs := &SpecState{Directory: root, SuitePaths: []string{path}}

	res := (&LoadCorpusBuilder{VS: vs}).Build(core.Result{}).Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, "1 charters")
	require.Len(t, vs.Charters, 1)
	require.Equal(t, "validation-suite", vs.Charters[0].ID)
	require.Equal(t, root, vs.TargetDirectory)
}

func TestRegisterSpecFactoriesConfiguresCharterSuitePaths(t *testing.T) {
	br := toolregistry.NewBuiltinRegistry()
	RegisterSpecFactories(br, "/target")
	factory, ok := br.Resolve("load_corpus")
	require.True(t, ok)

	builder, err := factory(catalog.ToolDef{
		Name: "load_corpus",
		Config: map[string]interface{}{
			"suite_paths": []interface{}{"a.yaml"},
			"charters":    []interface{}{"b.yaml"},
		},
	}, map[string]string{
		"directory":      "/work",
		"charter_suites": "c.yaml, d.yaml",
	})

	require.NoError(t, err)
	loadBuilder := builder.(*LoadCorpusBuilder)
	require.Equal(t, "/work", loadBuilder.VS.Directory)
	require.Equal(t, "/work", loadBuilder.VS.TargetDirectory)
	require.Equal(t, []string{"a.yaml", "b.yaml", "c.yaml", "d.yaml"}, loadBuilder.VS.SuitePaths)
}

func TestValidateSpecsRunsLoadedCharters(t *testing.T) {
	root := filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid")
	vs := &SpecState{
		Directory:       root,
		TargetDirectory: root,
		SuitePaths:      []string{writeValidationCharter(t, t.TempDir(), "suite.yaml")},
	}

	loadRes := (&LoadCorpusBuilder{VS: vs}).Build(core.Result{}).Execute()
	require.Equal(t, core.ToolDone, loadRes.Signal)

	res := (&ValidateSpecsBuilder{VS: vs}).Build(loadRes).Execute()

	require.NotEqual(t, core.CommandError, res.Signal)
	require.NotEmpty(t, vs.Findings)
	require.Equal(t, "validation-suite", vs.Findings[0].SuiteID)
	require.Equal(t, "spec_corpus", vs.Findings[0].Kind)
}

func writeValidationCharter(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(`
id: validation-suite
checks:
  - id: builtins
    kind: spec_corpus
`), 0o644))
	return path
}
